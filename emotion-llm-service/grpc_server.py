"""
emotion-llm-service · gRPC Server with interceptors (Stage 11)

Protocol: proto/emotion_llm.proto
Port: 50051

Interceptors:
  - LoggingInterceptor: logs every RPC call (method, peer, latency, code)
  - RecoveryInterceptor: catches exceptions and returns INTERNAL status

Analogous to Go's grpcinterceptor.ServerLoggingInterceptor /
ServerRecoveryInterceptor (emotion-echo-shared/pkg/grpcinterceptor).

Start:
  python grpc_server.py
"""
import logging
import os
import signal
import sys
import time
import traceback
from concurrent import futures

import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

# Import must work from emotion-llm-service cwd
import emotion_llm_pb2
import emotion_llm_pb2_grpc

from main import analyze as http_analyze

from logging_setup import setup_logging
from metrics_setup import GRPC_REQUESTS_TOTAL

# Stage 20-2: 结构化日志（默认 JSON，LOG_FORMAT=text 切换）
setup_logging()
logger = logging.getLogger(__name__)


# =====================================================
# Server interceptors
# =====================================================

class LoggingInterceptor(grpc.ServerInterceptor):
    """Logs every unary RPC call (method, peer, latency, status code)."""

    def intercept_service(self, continuation, handler_call_details):
        method = handler_call_details.method
        peer = handler_call_details.invocation_metadata  # metadata is iterable

        # Extract peer addr from metadata
        peer_addr = "unknown"
        for meta in peer or []:
            if meta.key == "x-forwarded-for":
                peer_addr = meta.value
                break

        start = time.time()
        try:
            response = continuation(handler_call_details)
            # Wrap handler so we can measure latency after the call returns
            # but since intercept_service is sync, we measure around continuation
            elapsed_ms = int((time.time() - start) * 1000)
            logger.info(f"[grpc-server] method={method} peer={peer_addr} latency={elapsed_ms}ms code=OK")
            return response
        except Exception as e:
            elapsed_ms = int((time.time() - start) * 1000)
            logger.error(
                f"[grpc-server] method={method} peer={peer_addr} "
                f"latency={elapsed_ms}ms code=INTERNAL err={e}"
            )
            raise


class RecoveryInterceptor(grpc.ServerInterceptor):
    """Converts Python exceptions in handlers to grpc INTERNAL status.

    Without this, an exception in a handler propagates up and may cause
    the entire gRPC server to crash on some Python versions.
    """

    def intercept_service(self, continuation, handler_call_details):
        try:
            return continuation(handler_call_details)
        except Exception as e:
            logger.error(
                f"[grpc-server] interceptor caught exception in {handler_call_details.method}: {e}\n"
                f"{traceback.format_exc()}"
            )
            # Abort the RPC with INTERNAL status
            raise grpc.RpcError(
                f"internal error: {e}"
            ) from e


class AuthInterceptor(grpc.ServerInterceptor):
    """Validates internal API key from incoming metadata.

    - If expected_api_key is empty: auth disabled (dev mode)
    - Otherwise: rejects UNAUTHENTICATED if missing or wrong key

    Analogous to Go's grpcinterceptor.NewServerAuthInterceptor().
    """

    def __init__(self, expected_api_key: str = ""):
        self.expected_api_key = expected_api_key

    def intercept_service(self, continuation, handler_call_details):
        if not self.expected_api_key:
            # auth disabled
            return continuation(handler_call_details)

        # Extract metadata
        metadata = handler_call_details.invocation_metadata
        api_key = None
        if metadata:
            for meta in metadata:
                if meta.key == "x-internal-api-key":
                    api_key = meta.value
                    break

        if api_key is None:
            logger.warning(
                f"[grpc-server] AUTH REJECTED: missing api key for {handler_call_details.method}"
            )
            return _abort_handler(grpc.StatusCode.UNAUTHENTICATED, "missing api key")

        if api_key != self.expected_api_key:
            logger.warning(
                f"[grpc-server] AUTH REJECTED: invalid api key for {handler_call_details.method}"
            )
            return _abort_handler(grpc.StatusCode.UNAUTHENTICATED, "invalid api key")

        return continuation(handler_call_details)


def _abort_handler(status_code, details):
    """Returns a handler that immediately aborts the RPC with given status.

    Used by AuthInterceptor to reject without continuing to the actual handler.
    """

    def deny(request, context):
        context.abort(status_code, details)

    return grpc.unary_unary_rpc_method_handler(deny)


class TracingInterceptor(grpc.ServerInterceptor):
    """Records per-RPC span metadata (start time, end time, status).

    In a production Python deployment, this would integrate with OpenTelemetry
    or a similar framework. Here we use simple stdout logging to keep deps minimal,
    while preserving the same "every RPC gets a span" semantics as the Go side.

    Analogous to Go's grpcinterceptor.NewServerTracingInterceptor().
    """

    def intercept_service(self, continuation, handler_call_details):
        method = handler_call_details.method
        start = time.time()
        try:
            response = continuation(handler_call_details)
            elapsed_ms = int((time.time() - start) * 1000)
            logger.info(
                f"[trace] span_op={method} duration={elapsed_ms}ms status=OK "
                f"tracer=python-stdout"
            )
            return response
        except Exception as e:
            elapsed_ms = int((time.time() - start) * 1000)
            logger.error(
                f"[trace] span_op={method} duration={elapsed_ms}ms status=ERROR "
                f"error={e} tracer=python-stdout"
            )
            raise


# =====================================================
# Service implementation
# =====================================================

class EmotionLLMServiceServicer(emotion_llm_pb2_grpc.EmotionLLMServiceServicer):
    """Implements emotion_llm.proto's gRPC service."""

    def __init__(self):
        self._fail_remaining = int(os.environ.get("FAIL_FIRST_N", "0"))
        self._call_count = 0

    def Analyze(self, request, context):
        try:
            self._call_count += 1

            # Stage 15 测试钩子：模拟 transient 错误
            # 让前 N 次调用返回 Unavailable，验证 client retry 是否触发
            # 用 set_code 而非 abort：避免被 RecoveryInterceptor 包装成 INTERNAL
            if self._fail_remaining > 0:
                self._fail_remaining -= 1
                logger.warning(
                    f"[transient-mock] simulating Unavailable (remaining={self._fail_remaining})"
                )
                context.set_code(grpc.StatusCode.UNAVAILABLE)
                context.set_details(
                    f"simulated transient failure (remaining={self._fail_remaining})"
                )
                # Stage 20-P0-2: 业务指标（mock failure 也算 err）
                GRPC_REQUESTS_TOTAL.labels(method="Analyze", status="err").inc()
                return emotion_llm_pb2.AnalyzeResponse()

            text = request.text
            logger.info(
                f"Analyze request: message_id={request.message_id} text_len={len(text)}"
            )

            # Reuse HTTP version's analyze() to keep two protocols identical
            http_resp = http_analyze(text)

            resp = emotion_llm_pb2.AnalyzeResponse(
                message_id=request.message_id,
                primary_emotion=http_resp.primaryEmotion,
                sentiment_score=http_resp.sentimentScore,
                confidence=http_resp.confidence,
                model=http_resp.model,
                raw_response="",
            )
            logger.info(
                f"Analyze done: emotion={resp.primary_emotion} "
                f"score={resp.sentiment_score} model={resp.model}"
            )
            # Stage 20-P0-2: 业务指标
            GRPC_REQUESTS_TOTAL.labels(method="Analyze", status="ok").inc()
            return resp
        except Exception as e:
            logger.error(f"Analyze error: {e}", exc_info=True)
            GRPC_REQUESTS_TOTAL.labels(method="Analyze", status="err").inc()
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))
            return emotion_llm_pb2.AnalyzeResponse()

    def AnalyzeBatch(self, request, context):
        """Server-streaming 批量分析（Stage 16）。

        接收多条消息，对每条独立调用 analyze()，逐条 yield 给 client。
        """
        try:
            items = list(request.items)
            logger.info(
                f"AnalyzeBatch request: items={len(items)} user_id={request.user_id}"
            )
            emitted = 0
            for item in items:
                text = item.text
                http_resp = http_analyze(text)
                resp = emotion_llm_pb2.AnalyzeResponse(
                    message_id=item.message_id,
                    primary_emotion=http_resp.primaryEmotion,
                    sentiment_score=http_resp.sentimentScore,
                    confidence=http_resp.confidence,
                    model=http_resp.model,
                    raw_response="",
                )
                logger.info(
                    f"AnalyzeBatch emit: msg_id={resp.message_id} emotion={resp.primary_emotion}"
                )
                emitted += 1
                yield resp
            # Stage 20-P0-2: 业务指标（按 emit 数计）
            GRPC_REQUESTS_TOTAL.labels(method="AnalyzeBatch", status="ok").inc(emitted)
        except Exception as e:
            logger.error(f"AnalyzeBatch error: {e}", exc_info=True)
            GRPC_REQUESTS_TOTAL.labels(method="AnalyzeBatch", status="err").inc()
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(str(e))


# =====================================================
# Server bootstrap
# =====================================================

def serve(port: int = 50051):
    api_key = os.environ.get("INTERNAL_API_KEY", "")
    required = os.environ.get("INTERNAL_API_KEY_REQUIRED", "").lower() in ("1", "true", "yes")

    # Stage 17：启动校验 INTERNAL_API_KEY
    if required and not api_key:
        logger.error(
            "INTERNAL_API_KEY_REQUIRED=1 but INTERNAL_API_KEY is empty. "
            "Refusing to start (dev mode would expose service unauthenticated)."
        )
        sys.exit(1)

    if api_key:
        # 安全检查：弱 key 警告
        if len(api_key) < 16:
            logger.warning(
                f"INTERNAL_API_KEY length={len(api_key)} < 16, recommend >= 32 chars"
            )
        weak = {"test", "dev", "changeme", "default", "secret", "password"}
        if api_key.lower() in weak or any(w in api_key.lower() for w in weak):
            logger.warning(
                f"INTERNAL_API_KEY contains weak pattern (test/dev/changeme/default/secret/password). "
                "Use strong random key in production."
            )

    # gRPC 标准 health/v1 服务（Stage 14）
    # 用于：服务发现 / 探活 / 启动自检
    # 客户端可通过 Health.Check / Health.Watch 查询整体或子服务状态
    health_servicer_impl = health.HealthServicer()
    # 默认：空 service（server liveness）置为 SERVING
    health_servicer_impl.set("", health_pb2.HealthCheckResponse.SERVING)
    # 业务服务：emotion.LLM 单独管理
    health_servicer_impl.set("emotion.LLM", health_pb2.HealthCheckResponse.SERVING)

    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=10),
        interceptors=(
            LoggingInterceptor(),
            RecoveryInterceptor(),
            TracingInterceptor(),
            AuthInterceptor(expected_api_key=api_key),
        ),
    )
    emotion_llm_pb2_grpc.add_EmotionLLMServiceServicer_to_server(
        EmotionLLMServiceServicer(), server
    )
    # 注册 health service（注意：必须用 add_HealthServicer_to_server）
    health_pb2_grpc.add_HealthServicer_to_server(health_servicer_impl, server)

    # Stage 18：mTLS（双向认证）
    # 通过 TLS_ENABLED=1 启用；证书从 TLS_CA_CERT / TLS_SERVER_CERT / TLS_SERVER_KEY 读取
    tls_enabled = os.environ.get("TLS_ENABLED", "").lower() in ("1", "true", "yes")
    if tls_enabled:
        # 用 emotion-echo 项目根目录解析相对路径（避免 cwd 依赖）
        project_root = os.environ.get("PROJECT_ROOT", os.getcwd())
        # 启发式：cwd 包含 emotion-llm-service 就用上级目录
        if os.path.basename(project_root) == "emotion-llm-service":
            project_root = os.path.dirname(project_root)

        ca_path = os.environ.get("TLS_CA_CERT")
        if not ca_path:
            ca_path = os.path.join(project_root, "deploy", "tls", "ca.crt")
        cert_path = os.environ.get("TLS_SERVER_CERT")
        if not cert_path:
            cert_path = os.path.join(project_root, "deploy", "tls", "llm-server.crt")
        key_path = os.environ.get("TLS_SERVER_KEY")
        if not key_path:
            key_path = os.path.join(project_root, "deploy", "tls", "llm-server.key")
        require_client_auth = os.environ.get("TLS_REQUIRE_CLIENT_AUTH", "1").lower() in ("1", "true", "yes")

        with open(ca_path, "rb") as f:
            ca_cert = f.read()
        with open(cert_path, "rb") as f:
            server_cert = f.read()
        with open(key_path, "rb") as f:
            server_key = f.read()

        creds = grpc.ssl_server_credentials(
            [(server_key, server_cert)],
            root_certificates=ca_cert,
            require_client_auth=require_client_auth,
        )
        server.add_secure_port(f"[::]:{port}", creds)
        logger.info(f"mTLS enabled: ca={ca_path} cert={cert_path} require_client_auth={require_client_auth}")
    else:
        server.add_insecure_port(f"[::]:{port}")
        logger.info("mTLS disabled (insecure port, dev mode)")

    server.start()
    logger.info(f"gRPC server started on port {port}")
    logger.info(
        f"interceptors: Logging + Recovery + Tracing + Auth(auth={'enabled' if api_key else 'disabled'}, required={required})"
    )
    logger.info(f"proto: emotion_llm.proto (Analyze RPC) + grpc.health.v1 (Check/Watch)")
    logger.info(f"health: empty=Serving, emotion.LLM=Serving (default)")

    # Stage 20-1: graceful shutdown
    # 收到 SIGTERM（K8s pod 终止 / docker stop）→ 把 health 置 NOT_SERVING → 等待 in-flight RPC → 关闭
    # 同时保留 KeyboardInterrupt（开发环境 Ctrl-C）兼容
    GRACE_SECONDS = int(os.environ.get("GRPC_GRACE_SECONDS", "5"))

    def _shutdown_handler(signum, frame):
        signame = "SIGTERM" if signum == signal.SIGTERM else f"signal {signum}"
        logger.info(
            f"received {signame}, starting graceful shutdown "
            f"(grace={GRACE_SECONDS}s, set health=NOT_SERVING)"
        )
        # 1) 先把 health 置为 NOT_SERVING，让 client / LB 提前停止发新请求
        try:
            health_servicer_impl.set("", health_pb2.HealthCheckResponse.NOT_SERVING)
            health_servicer_impl.set("emotion.LLM", health_pb2.HealthCheckResponse.NOT_SERVING)
        except Exception as e:
            logger.warning(f"health set NOT_SERVING failed: {e}")
        # 2) 等 in-flight RPC 完成（grace=GRACE_SECONDS 强制上限）
        server.stop(grace=GRACE_SECONDS)

    signal.signal(signal.SIGTERM, _shutdown_handler)
    # SIGINT 在 Python 默认抛 KeyboardInterrupt；显式注册保持一致行为
    signal.signal(signal.SIGINT, _shutdown_handler)

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        # 兜底（理论上 signal handler 已处理）
        _shutdown_handler(signal.SIGINT, None)
    logger.info("gRPC server stopped")


if __name__ == "__main__":
    port = int(os.environ.get("GRPC_PORT", "50051"))
    serve(port)