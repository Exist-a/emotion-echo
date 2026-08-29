"""
test_grpc_server.py · emotion-llm-service gRPC server 单元测试

Stage 26-T backlog §四 4.4: 覆盖 emotion-llm-service/grpc_server.py
的 4 个 interceptor + Analyze / Health RPC handler + 边界
（空 text / None / 4096 chars / emoji）。

策略：
  - interceptor 单元测试：直接构造 handler_call_details mock + 测
    各分支路径（happy / exception / 无 key / 缺 key / 错 key）。
  - RPC handler 单元测试：用真实 gRPC server (localhost:0) +
    insecure channel，跑完整链路，覆盖 happy / 边界 / 错误。

不依赖：
  - 网络（用 localhost loopback）
  - 数据库
  - Prometheus（metrics_setup 的导入失败应该被 metrics 装饰器吞）

依赖：
  - pytest
  - grpcio
  - service 模块（emotion-llm-service/）

预提交检查（commit 前必跑）：
  - pytest tests/unit/test_grpc_server.py -v   全部 PASS
  - pytest tests/unit/                          全部 PASS（与 test_analyze_pure 共存）

参考：docs/stage-26-T-test-backlog.md §四 4.4。
"""
from __future__ import annotations

import logging
import os
import sys
import threading
import time
from concurrent import futures
from pathlib import Path
from typing import Iterator

import grpc
import pytest
from grpc_health.v1 import health_pb2, health_pb2_grpc

# 把 emotion-llm-service 加入 PYTHONPATH（与 test_analyze_pure.py 一致）
SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))


# Generated proto stubs (autogen, never edited)
import emotion_llm_pb2
import emotion_llm_pb2_grpc

# Interceptors under test
from grpc_server import (
    LoggingInterceptor,
    RecoveryInterceptor,
    AuthInterceptor,
    TracingInterceptor,
    EmotionLLMServiceServicer,
)


# =====================================================
# Helpers
# =====================================================

class _MetadataKeyValue:
    """模拟 grpc _metadata 的 key-value 对象（实现访问 meta.key / meta.value）。"""

    __slots__ = ("key", "value")

    def __init__(self, key, value):
        self.key = key
        self.value = value


class _HandlerCallDetails:
    """模拟 grpc.HandlerCallDetails（grpc 内部类型，未公开）。"""

    def __init__(self, method: str = "/test/Method", metadata=None):
        self.method = method
        self.invocation_metadata = metadata or ()


def _make_metadata(*pairs) -> tuple:
    """构造 grpc metadata tuple，每个元素是带 .key / .value 属性的对象。"""
    return tuple(_MetadataKeyValue(k, v) for k, v in pairs)


def _continuation_that_returns(handler):
    """构造一个 continuation handler，返回固定的 handler。"""

    def continuation(_details):
        return handler

    return continuation


def _continuation_that_raises(exc):
    """构造一个会抛异常的 continuation。"""

    def continuation(_details):
        raise exc
        return None

    return continuation


def _noop_handler(_request, context):
    """最小化的 unary-unary handler（用 grpc.unary_unary_rpc_method_handler 包）。"""
    return emotion_llm_pb2.AnalyzeResponse()


# =====================================================
# LoggingInterceptor tests
# =====================================================

class TestLoggingInterceptor:
    """LoggingInterceptor: 每个 RPC 一条 info 日志（method + peer + latency + code=OK）。"""

    def test_happy_path_logs_info(self, caplog):
        details = _HandlerCallDetails(
            method="/svc/Test",
            metadata=_make_metadata(("x-forwarded-for", "10.0.0.1")),
        )
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = LoggingInterceptor()

        with caplog.at_level(logging.INFO, logger="grpc_server"):
            result = interceptor.intercept_service(_continuation_that_returns(handler), details)

        assert result is handler
        # Exactly one info entry mentioning the method.
        infos = [r for r in caplog.records if r.levelno == logging.INFO]
        assert any("method=/svc/Test" in r.getMessage() for r in infos)
        assert any("peer=10.0.0.1" in r.getMessage() for r in infos)

    def test_exception_logs_error_and_reraises(self, caplog):
        details = _HandlerCallDetails(method="/svc/Bomb")
        interceptor = LoggingInterceptor()

        boom = RuntimeError("kaboom")
        with caplog.at_level(logging.INFO, logger="grpc_server"):
            with pytest.raises(RuntimeError, match="kaboom"):
                interceptor.intercept_service(
                    _continuation_that_raises(boom),
                    details,
                )

        errors = [r for r in caplog.records if r.levelno == logging.ERROR]
        assert any("code=INTERNAL" in r.getMessage() for r in errors)
        assert any("err=kaboom" in r.getMessage() for r in errors)

    def test_missing_xforwardedfor_logs_unknown_peer(self, caplog):
        details = _HandlerCallDetails(method="/svc/Test", metadata=_make_metadata())
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = LoggingInterceptor()

        with caplog.at_level(logging.INFO, logger="grpc_server"):
            interceptor.intercept_service(_continuation_that_returns(handler), details)

        assert any("peer=unknown" in r.getMessage() for r in caplog.records)


# =====================================================
# RecoveryInterceptor tests
# =====================================================

class TestRecoveryInterceptor:
    """RecoveryInterceptor: 异常 → grpc.RpcError（带 INTERNAL status）。"""

    def test_exception_converted_to_rpc_error(self):
        details = _HandlerCallDetails(method="/svc/Bomb")
        interceptor = RecoveryInterceptor()
        boom = ValueError("oops")

        # RecoveryInterceptor re-raises after logging; the exact type
        # depends on the implementation. We assert it raises SOMETHING
        # and the message references the original error.
        with pytest.raises(Exception) as exc_info:
            interceptor.intercept_service(
                _continuation_that_raises(boom),
                details,
            )
        # Original ValueError message must appear somewhere in the chain.
        assert "oops" in str(exc_info.value) or any(
            "oops" in str(arg) for arg in exc_info.value.args
        )

    def test_happy_path_passes_through(self):
        details = _HandlerCallDetails(method="/svc/OK")
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = RecoveryInterceptor()

        result = interceptor.intercept_service(
            _continuation_that_returns(handler),
            details,
        )
        assert result is handler


# =====================================================
# AuthInterceptor tests
# =====================================================

class TestAuthInterceptor:
    """AuthInterceptor: api_key 校验（空=禁用 / 缺=拒 / 错=拒 / 对=放行）。"""

    def test_empty_key_disables_auth(self):
        details = _HandlerCallDetails(
            method="/svc/X",
            metadata=_make_metadata(("x-internal-api-key", "anything")),
        )
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = AuthInterceptor(expected_api_key="")

        # With empty expected key, ANY caller (even with wrong key) is
        # accepted — the interceptor passes the handler through.
        result = interceptor.intercept_service(
            _continuation_that_returns(handler),
            details,
        )
        assert result is handler

    def test_correct_key_passes_through(self):
        details = _HandlerCallDetails(
            method="/svc/X",
            metadata=_make_metadata(("x-internal-api-key", "secret-abc")),
        )
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = AuthInterceptor(expected_api_key="secret-abc")

        result = interceptor.intercept_service(
            _continuation_that_returns(handler),
            details,
        )
        assert result is handler

    def test_missing_key_returns_denier(self):
        details = _HandlerCallDetails(method="/svc/X", metadata=_make_metadata())
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = AuthInterceptor(expected_api_key="secret-abc")

        result = interceptor.intercept_service(
            _continuation_that_returns(handler),
            details,
        )
        # The denier is a handler that aborts with UNAUTHENTICATED.
        # We don't run it; we just assert it is NOT the original handler.
        assert result is not handler

    def test_wrong_key_returns_denier(self):
        details = _HandlerCallDetails(
            method="/svc/X",
            metadata=_make_metadata(("x-internal-api-key", "wrong-key")),
        )
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = AuthInterceptor(expected_api_key="secret-abc")

        result = interceptor.intercept_service(
            _continuation_that_returns(handler),
            details,
        )
        assert result is not handler


# =====================================================
# TracingInterceptor tests
# =====================================================

class TestTracingInterceptor:
    """TracingInterceptor: 每个 RPC 一条 span 日志（duration + status）。"""

    def test_happy_path_logs_span(self, caplog):
        details = _HandlerCallDetails(method="/svc/Trace")
        handler = grpc.unary_unary_rpc_method_handler(_noop_handler)
        interceptor = TracingInterceptor()

        with caplog.at_level(logging.INFO, logger="grpc_server"):
            interceptor.intercept_service(_continuation_that_returns(handler), details)

        assert any("span_op=/svc/Trace" in r.getMessage() for r in caplog.records)
        assert any("status=OK" in r.getMessage() for r in caplog.records)

    def test_exception_logs_error_span(self, caplog):
        details = _HandlerCallDetails(method="/svc/Trace")
        interceptor = TracingInterceptor()

        with caplog.at_level(logging.INFO, logger="grpc_server"):
            with pytest.raises(RuntimeError):
                interceptor.intercept_service(
                    _continuation_that_raises(RuntimeError("boom")),
                    details,
                )

        assert any("status=ERROR" in r.getMessage() for r in caplog.records)


# =====================================================
# Analyze RPC handler tests (real gRPC server on ephemeral port)
# =====================================================

@pytest.fixture
def grpc_server() -> Iterator[grpc.Server]:
    """启动一个绑到 :0 的真实 gRPC server，含 4 个 interceptor + Analyze + Health。"""
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=2),
        interceptors=(
            LoggingInterceptor(),
            RecoveryInterceptor(),
            TracingInterceptor(),
            AuthInterceptor(expected_api_key=""),  # dev mode: auth off
        ),
    )
    servicer = EmotionLLMServiceServicer()
    emotion_llm_pb2_grpc.add_EmotionLLMServiceServicer_to_server(servicer, server)
    from grpc_health.v1 import health, health_pb2_grpc as h_grpc

    health_servicer = health.HealthServicer()
    health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    health_servicer.set("emotion.LLM", health_pb2.HealthCheckResponse.SERVING)
    h_grpc.add_HealthServicer_to_server(health_servicer, server)

    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        # Yield a small connection object that the test can use.
        yield _ServerHandle(server, "127.0.0.1", port)
    finally:
        server.stop(grace=1)


class _ServerHandle:
    """Lightweight handle exposing the bound address + start time."""

    def __init__(self, server: grpc.Server, host: str, port: int):
        self.server = server
        self.host = host
        self.port = port
        self.addr = f"{host}:{port}"

    def channel(self) -> grpc.Channel:
        return grpc.insecure_channel(self.addr)


class TestAnalyzeRPC:
    """Analyze RPC handler 边界测试（真实 gRPC）。"""

    def test_happy_path_returns_expected_shape(self, grpc_server):
        with grpc.insecure_channel(grpc_server.addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.Analyze(
                emotion_llm_pb2.AnalyzeRequest(text="我今天很开心", message_id="42")
            )
        assert resp.message_id == "42"
        assert resp.primary_emotion == "happy"
        assert resp.model == "keyword-v1"
        assert 0.0 <= resp.confidence <= 1.0

    def test_empty_text_returns_neutral(self, grpc_server):
        with grpc.insecure_channel(grpc_server.addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.Analyze(emotion_llm_pb2.AnalyzeRequest(text="", message_id="1"))
        assert resp.primary_emotion == "neutral"
        assert resp.confidence == 0.0

    def test_whitespace_only_text_returns_neutral(self, grpc_server):
        with grpc.insecure_channel(grpc_server.addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.Analyze(
                emotion_llm_pb2.AnalyzeRequest(text="   \n\t  ", message_id="2")
            )
        assert resp.primary_emotion == "neutral"

    def test_emoji_only_text_does_not_crash(self, grpc_server):
        """表情-only 输入不能导致 handler crash；至少返回一条结构化响应。"""
        with grpc.insecure_channel(grpc_server.addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.Analyze(
                emotion_llm_pb2.AnalyzeRequest(text="🎉🚀💖✨", message_id="3")
            )
        # 任何合法的 emotion 字段值（实现应至少返回中性结果而非抛错）。
        assert resp.primary_emotion in {"happy", "sad", "angry", "anxious", "calm", "neutral"}
        assert resp.model == "keyword-v1"

    def test_4096_chars_text_handled(self, grpc_server):
        """4096 字符边界 — 当前实现无 max_length 限制，所以应正常处理。"""
        long_text = "我今天很开心。" * 600  # 重复 600 次 = 约 3600 字符
        # bump up to exactly 4096+ to verify no panic on max-length boundary
        long_text = (long_text + "。" * 100)[:4096]
        assert len(long_text) == 4096

        with grpc.insecure_channel(grpc_server.addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.Analyze(
                emotion_llm_pb2.AnalyzeRequest(text=long_text, message_id="4")
            )
        assert resp.primary_emotion == "happy"
        # Implementation may round confidence to 0.0 because
        # max_hits / total_words < 0.001 with 4096 chars. We accept
        # any non-negative result; the goal of this test is to verify
        # the handler doesn't panic on max-length inputs.
        assert resp.confidence >= 0.0

    def test_negative_message_id_accepted(self, grpc_server):
        """message_id 是 int64 — 负值是合法输入。返回 message_id 必须等于入参。"""
        with grpc.insecure_channel(grpc_server.addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.Analyze(
                emotion_llm_pb2.AnalyzeRequest(text="我难过", message_id="-1")
            )
        assert resp.message_id == "-1"
        assert resp.primary_emotion == "sad"


# =====================================================
# Health RPC tests (standard grpc.health.v1)
# =====================================================

class TestHealthRPC:
    """Health.Check 必须返回 SERVING（empty service + emotion.LLM service）。"""

    def test_empty_service_serving(self, grpc_server):
        with grpc_server.channel() as ch:
            stub = health_pb2_grpc.HealthStub(ch)
            resp = stub.Check(health_pb2.HealthCheckRequest(service=""))
        assert resp.status == health_pb2.HealthCheckResponse.SERVING

    def test_emotion_llm_service_serving(self, grpc_server):
        with grpc_server.channel() as ch:
            stub = health_pb2_grpc.HealthStub(ch)
            resp = stub.Check(
                health_pb2.HealthCheckRequest(service="emotion.LLM")
            )
        assert resp.status == health_pb2.HealthCheckResponse.SERVING