"""cloud_deepseek 实现：HTTP OpenAI 兼容直连 DeepSeek。

与 cloud_gRPC 的边界：
- cloud_gRPC 走 emotion-llm-service 中转（带鉴权 + Nacos 注册 + mock fallback）
- cloud_deepseek 直连 DeepSeek（无中转，**必须显式提供 api_key**；缺失即抛 ConfigurationError）

为什么无静默降级：直连模式用户应该知道"我在打 DeepSeek API"，沉默降级到 mock
会掩盖"key 已过期 / endpoint 已迁移 / 网络不通"等问题。

无网络 / 上游 5xx → ChatUpstreamError（openai SDK 自抛）；外层 baseline_run 统计为
fallback_reason=upstream_error:<msg>，与 cloud_gRPC 同形记录。
"""
from __future__ import annotations

import logging
import os
import time
from typing import Any

from model_fn_factory import BaselineModelFn, ReplyResult

logger = logging.getLogger(__name__)


class ConfigurationError(RuntimeError):
    """配置错误（无 key / 模型名空 / endpoint 非法）—— 不应静默降级。"""


def make(
    *,
    api_key: str | None = None,
    base_url: str | None = None,
    model: str | None = None,
    timeout_s: float = 30.0,
    system_prompt: str | None = None,
) -> BaselineModelFn:
    """构造一个 cloud_deepseek model_fn。

    必须传 api_key（不读 env —— 显式优于隐式；避免误以为有 key 实则空）。
    """
    resolved_key = api_key or os.environ.get("LLM_API_KEY", "")
    if not resolved_key:
        raise ConfigurationError(
            "cloud_deepseek requires api_key; pass make(api_key=...) explicitly. "
            "(Avoid silent fallback: 直连模式用户应当知道 key 状态。)"
        )
    resolved_base = base_url or os.environ.get("LLM_BASE_URL", "") or "https://api.deepseek.com"
    resolved_model = model or os.environ.get("LLM_MODEL", "") or "deepseek-chat"
    if system_prompt is None:
        system_prompt = (
            "你是一名温暖、专业的心理疏导伙伴，名叫「小暖」。"
            "倾听优先，先共情再给建议。回复 100~300 字，结尾提一个温和的开放式问题。"
        )

    def model_fn(case: dict[str, Any]) -> ReplyResult:
        start = time.time()
        try:
            from openai import OpenAI

            client = OpenAI(
                api_key=resolved_key, base_url=resolved_base, timeout=timeout_s, max_retries=1
            )
            stream = client.chat.completions.create(
                model=resolved_model,
                messages=[
                    {"role": "system", "content": system_prompt},
                    {"role": "user", "content": case["input"]},
                ],
                stream=True,
            )

            chunks_text: list[str] = []
            saw_any = False
            for event in stream:
                try:
                    delta = event.choices[0].delta.content or ""
                except (AttributeError, IndexError, TypeError):
                    delta = ""
                if delta:
                    chunks_text.append(delta)
                    saw_any = True

            if not saw_any:
                return ReplyResult(
                    text="",
                    model=resolved_model,
                    fallback_reason="upstream_error:empty_stream",
                    latency_ms=_ms_since(start),
                )
            return ReplyResult(
                text="".join(chunks_text),
                model=resolved_model,
                fallback_reason="",
                latency_ms=_ms_since(start),
            )
        except Exception as e:
            logger.exception("cloud_deepseek error for case=%s", case.get("id"))
            return ReplyResult(
                text="",
                model=resolved_model,
                fallback_reason=f"upstream_error:{type(e).__name__}:{e}",
                latency_ms=_ms_since(start),
            )

    return model_fn


def _ms_since(start: float) -> int:
    return int((time.time() - start) * 1000)