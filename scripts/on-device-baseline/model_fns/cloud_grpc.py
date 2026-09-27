"""cloud_gRPC 实现：调 emotion-llm-service.ChatCompletion 流，聚合 delta_content。

约定（与 chat_completion.py / grpc_server.py 一致）：
- localhost:50051（默认；可 env GRPC_TARGET 覆盖）
- INTERNAL_API_KEY 缺失 = dev 模式无鉴权；存在则须 metadata x-internal-api-key 一致
- 内部 `iter_chat_chunks` 自动 mock fallback（LLM_API_KEY 空时）；本模块只做 IO 编排

错误处理（pytest 用 stub fake_stub 注入，不依赖真实容器）：
- 连接失败 / 超时 → ReplyResult(text=mock fallback, fallback_reason=connection_error:<msg>, ...)
- 业务错（empty stream / upstream_error）→ ReplyResult(fallback_reason=逐字透传）
- 上游 200 但 0 帧 → 服务端已 fallback；本端只透传 fallback_reason
"""
from __future__ import annotations

import logging
import os
import sys
import time
from typing import Any

from model_fn_factory import BaselineModelFn, ReplyResult

logger = logging.getLogger(__name__)


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
) -> BaselineModelFn:
    """构造一个 cloud_gRPC model_fn。

    fake_stub —— pytest 注入点：传入一个带 `ChatCompletion(request, context=None)` 的对象；
                 返回可迭代的 ChatChunk（proto 或 mock duck-type）。
                 None = 真实 grpc 连接（pytest 默认不走这条）。
    """
    resolved_target = target or os.environ.get("GRPC_TARGET", "localhost:50051")
    resolved_key = api_key if api_key is not None else os.environ.get("INTERNAL_API_KEY", "")

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
                channel = grpc.insecure_channel(resolved_target)
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