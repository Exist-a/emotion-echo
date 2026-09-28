"""cloud_gRPC 实现：调 emotion-llm-service.ChatCompletion 流，聚合 delta_content。

约定（与 chat_completion.py / grpc_server.py 一致）：
- localhost:50051（默认；可 env GRPC_TARGET 覆盖）
- **mTLS**：llm-service v0.1.2 默认 `TLS_ENABLED=1` + `TLS_REQUIRE_CLIENT_AUTH=1`
  （`emotion-llm-service/grpc_server.py:461-466`）。本模块走
  `grpc.secure_channel` + ca/client-cert/client-key 三证书（OND-F-09 修复）。
  证书路径沿用与 web-bff / ai-svc 完全相同的 env 口径
  （`TLS_CA_CERT` / `TLS_CLIENT_CERT` / `TLS_CLIENT_KEY`，见
  `deploy/docker-compose.apps.yml:657-660`），缺省回落 `deploy/tls/`
  （`ca.crt` + `ai-client.crt` + `ai-client.key`）。
- INTERNAL_API_KEY 缺失 = dev 模式无鉴权；存在则须 metadata x-internal-api-key 一致
- 内部 `iter_chat_chunks` 自动 mock fallback（LLM_API_KEY 空时）；本模块只做 IO 编排

错误处理（pytest 用 stub fake_stub 注入，不依赖真实容器）：
- **TLS 配置错误 → 构造 model_fn 时直接抛 TlsConfigError**（fail-loud）。
  绝不允许"配错证书"退化成明文再 mock fallback——那正是 OND-F-09 的病根：
  两套完全不同的根因共用同一个 fallback_reason，让 0% pass 看起来"符合预期"。
- 连接失败 / 超时 → ReplyResult(text=mock fallback, fallback_reason=grpc_unreachable:<msg>, ...)
- 业务错（empty stream / upstream_error）→ ReplyResult(fallback_reason=逐字透传)
- 上游 200 但 0 帧 → 服务端已 fallback；本端只透传 fallback_reason
"""
from __future__ import annotations

import logging
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Mapping

from model_fn_factory import BaselineModelFn, ReplyResult

logger = logging.getLogger(__name__)

# deploy/tls/ 默认回落目录（.gitignore:126 *.crt —— dev 证书不入仓，
# fresh clone 上这里是空的，此时 baseline 自动走明文而非报错）
REPO_ROOT = Path(__file__).resolve().parents[3]
DEFAULT_TLS_DIR = REPO_ROOT / "deploy" / "tls"
DEFAULT_CA_CERT = DEFAULT_TLS_DIR / "ca.crt"
DEFAULT_CLIENT_CERT = DEFAULT_TLS_DIR / "ai-client.crt"
DEFAULT_CLIENT_KEY = DEFAULT_TLS_DIR / "ai-client.key"

_TRUTHY = ("1", "true", "yes")
_FALSY = ("0", "false", "no")

# TLS 路径解析的三条 env（与 compose / BFF 同名，避免出现第二套口径）
_TLS_VARS = (
    ("ca_cert", "TLS_CA_CERT", DEFAULT_CA_CERT),
    ("client_cert", "TLS_CLIENT_CERT", DEFAULT_CLIENT_CERT),
    ("client_key", "TLS_CLIENT_KEY", DEFAULT_CLIENT_KEY),
)


class TlsConfigError(RuntimeError):
    """TLS 配置自相矛盾或缺件。

    只在两种情况抛：显式 `TLS_ENABLED=1` 却给不出三证书；或自动模式下**部分**
    证书存在（配了一半）。后者必须炸而不是当成"没配"——静默退回明文会重演
    OND-F-09（baseline 静默 mock，真机基线数据从未产生）。
    """


@dataclass(frozen=True)
class TlsConfig:
    """baseline 侧 mTLS 配置快照。enabled=False 时三证书一律为 None。"""

    enabled: bool
    ca_cert: Path | None = None
    client_cert: Path | None = None
    client_key: Path | None = None


def _resolve_tls_config(env: Mapping[str, str] | None = None) -> TlsConfig:
    """按 `TLS_ENABLED` 语义解析出三证书路径。

    - `TLS_ENABLED=1/true/yes`：三证书必须齐且存在，否则 TlsConfigError
    - `TLS_ENABLED=0/false/no`：明文（显式关闭，允许）
    - 未设：自动 —— 三证书齐全 → mTLS；一个都没有 → 明文；**部分存在 → 抛**
    """
    env = os.environ if env is None else env
    raw = (env.get("TLS_ENABLED") or "").strip().lower()
    explicit_on = raw in _TRUTHY
    explicit_off = raw in _FALSY

    resolved = {
        name: Path(env[var]) if (env.get(var) or "").strip() else default
        for name, var, default in _TLS_VARS
    }
    existing = {name: p for name, p in resolved.items() if p.exists()}
    missing = sorted(
        f"{var}={p}"
        for name, var, default in _TLS_VARS
        if not (p := resolved[name]).exists()
    )

    if explicit_off:
        return TlsConfig(enabled=False)

    if explicit_on:
        if missing:
            raise TlsConfigError(
                f"TLS_ENABLED=1 但证书缺失: {', '.join(missing)}。"
                f" 生成证书: python scripts/generate_dev_tls.py；"
                f" 或显式设 TLS_ENABLED=0 关掉 mTLS。"
            )
        return TlsConfig(enabled=True, **existing)

    if not existing:
        return TlsConfig(enabled=False)
    if len(existing) < len(_TLS_VARS):
        raise TlsConfigError(
            f"TLS_ENABLED 未设，但只找到部分证书（缺: {', '.join(missing)}）。"
            f" 请补齐或显式设 TLS_ENABLED=0 关掉 mTLS。"
        )
    return TlsConfig(enabled=True, **existing)


def build_channel(target: str, tls_config: TlsConfig):
    """按 TlsConfig 造 gRPC channel（mTLS → secure_channel，否则明文）。

    超时不本函数内消费——留给调用方 `channel_ready_future(...).result(timeout=)`，
    与旧实现保持同一处超时判定，baseline 报告的 latency_ms 语义不变。
    """
    import grpc

    if not tls_config.enabled:
        return grpc.insecure_channel(target)
    return grpc.secure_channel(
        target,
        grpc.ssl_channel_credentials(
            root_certificates=tls_config.ca_cert.read_bytes(),
            private_key=tls_config.client_key.read_bytes(),
            certificate_chain=tls_config.client_cert.read_bytes(),
        ),
    )



def _import_pb():
    """延迟 import：emotion_llm_pb2 在 emotion-llm-service/ 才存在。

    把 emotion-llm-service/ 加进 sys.path 是工程妥协（pb2 与 server 配套）；
    仅在调用时触发，pytest 不走真实 gRPC 时不污染 sys.path。
    """
    from pathlib import Path

    svc_dir = Path(__file__).resolve().parents[3] / "emotion-llm-service"
    svc_dir_str = str(svc_dir)
    if svc_dir_str not in sys.path:
        sys.path.insert(0, svc_dir_str)
    pb2 = __import__("emotion_llm_pb2")
    pb2_grpc = __import__("emotion_llm_pb2_grpc")
    return pb2, pb2_grpc


def _build_messages(case: dict[str, Any], system_prompt: str):
    """把 case 转 ChatMessage；system 固定来自 system_prompt 模板（v0.2 §6.1）。"""
    pb2, _ = _import_pb()
    msgs = [pb2.ChatMessage(role="system", content=system_prompt)]
    msgs.append(pb2.ChatMessage(role="user", content=case["input"]))
    return msgs


# v0.2 §6.1 强约束 system prompt（与 emotion-echo-web-bff personality_directive 同源风格简化）
DEFAULT_SYSTEM_PROMPT = (
    "你是一名温暖、专业的心理疏导伙伴，名叫「小暖」。"
    "倾听优先：先共情用户的情绪，再给建议，不急于评判或说教。"
    "安全边界：如用户表达自伤、自杀想法，立即严肃对待，建议拨打心理援助热线 400-161-9995 或寻求专业医疗帮助。"
    "回复风格：100~300 字，简洁温暖，结尾提一个温和的开放式问题。"
)


def make(
    *,
    target: str | None = None,
    api_key: str | None = None,
    timeout_s: float = 2.0,
    system_prompt: str = DEFAULT_SYSTEM_PROMPT,
    fake_stub: Any = None,
    tls_env: Mapping[str, str] | None = None,
) -> BaselineModelFn:
    """构造一个 cloud_gRPC model_fn。

    fake_stub —— pytest 注入点：传入一个带 `ChatCompletion(request, context=None)` 的对象；
                 返回可迭代的 ChatChunk（proto 或 mock duck-type）。
                 None = 真实 grpc 连接（pytest 默认不走这条）。
    tls_env —— TLS 环境变量快照；None = 读 os.environ。TlsConfigError 在**构造时**
              就抛（不拖到第一次调用），避免配置错被误读成"服务不可达"。
    """
    resolved_target = target or os.environ.get("GRPC_TARGET", "localhost:50051")
    resolved_key = api_key if api_key is not None else os.environ.get("INTERNAL_API_KEY", "")
    tls_config = _resolve_tls_config(env=tls_env)

    def model_fn(case: dict[str, Any]) -> ReplyResult:
        start = time.time()
        try:
            if fake_stub is not None:
                stub = fake_stub
            else:
                pb2, pb2_grpc = _import_pb()
                import grpc

                auth_meta = (
                    [("x-internal-api-key", resolved_key)] if resolved_key else []
                )
                channel = build_channel(resolved_target, tls_config)
                try:
                    grpc.channel_ready_future(channel).result(timeout=timeout_s)
                except Exception as e:  # grpc.FutureTimeoutError / grpc.RpcError
                    # gRPC 不可达 → 回退到 chat_completion 的 mock fallback 路径
                    # （等同 emotion-llm-service 跑着但 LLM_API_KEY 空时的输出）
                    return _mock_fallback_reply(case, system_prompt, f"grpc_unreachable:{type(e).__name__}", start)
                stub = pb2_grpc.EmotionLLMServiceStub(channel)

            pb2, _pb2_grpc = _import_pb()
            messages = _build_messages(case, system_prompt)
            req = pb2.ChatCompletionRequest(messages=messages, model="", temperature=0.0, max_tokens=512)

            chunks_text: list[str] = []
            model = ""
            fallback_reason = ""
            try:
                for chunk in stub.ChatCompletion(req):
                    delta = getattr(chunk, "delta_content", "") or ""
                    if delta:
                        chunks_text.append(delta)
                    if not model:
                        model = getattr(chunk, "model", "") or ""
                    fr = getattr(chunk, "fallback_reason", "") or ""
                    if fr:
                        fallback_reason = fr
            except Exception as e:
                return ReplyResult(
                    text="".join(chunks_text),
                    model=model,
                    fallback_reason=f"grpc_call_error:{type(e).__name__}:{e}",
                    latency_ms=_ms_since(start),
                )

            return ReplyResult(
                text="".join(chunks_text),
                model=model or "unknown",
                fallback_reason=fallback_reason,
                latency_ms=_ms_since(start),
            )
        except Exception as e:
            # 顶层兜底：任何未捕获异常 = 记录但不抛（baseline 报告需要继续跑后续 case）
            logger.exception("cloud_grpc model_fn unexpected error for case=%s", case.get("id"))
            return ReplyResult(
                text="",
                model="",
                fallback_reason=f"unexpected:{type(e).__name__}:{e}",
                latency_ms=_ms_since(start),
            )

    return model_fn


def _ms_since(start: float) -> int:
    return int((time.time() - start) * 1000)


def _mock_fallback_reply(case: dict[str, Any], system_prompt: str, prefix: str, start: float) -> ReplyResult:
    """gRPC 不可达时复用 chat_completion 的 mock 文案，标签 prefix 标真实根因。

    行为同 emotion-llm-service 在 LLM_API_KEY 空时的输出（`make_mock_chunks` 4 变体随机）。
    """
    try:
        from chat_completion import make_mock_chunks
        chunks = make_mock_chunks(reason="mock_no_api_key")
        text = "".join(c.delta_content for c in chunks if c.delta_content)
        return ReplyResult(
            text=text,
            model="mock",
            fallback_reason=f"{prefix}:mock_fallback",
            latency_ms=_ms_since(start),
        )
    except Exception as e:
        return ReplyResult(
            text="",
            model="",
            fallback_reason=f"{prefix}:mock_import_failed:{type(e).__name__}:{e}",
            latency_ms=_ms_since(start),
        )