"""model_fn 契约 + 工厂。

设计动机：
- 既有 `scripts/on-device-golden/runner.py` 的 `ModelFn = Callable[[dict], str]`
  只能返回纯字符串（evaluate_case 只看 reply 文本）。
- 但 T2#3 baseline 需要携带 **fallback_reason / model / latency_ms** 进报告，
  才能区分"真 LLM 命中" vs "mock 兜底" vs "上游失败"。
- 因此 baseline 层自己定义 `ReplyResult`，再把 `.text` 喂给 evaluate_case。

契约：
  ReplyResult.text —— evaluate_case 入参
  ReplyResult.fallback_reason 非空 ⇒ 走了 mock / 上游失败
  ReplyResult.model —— 实际模型名（首帧携带；mock 时为 "mock"）
  ReplyResult.latency_ms —— 实测端到端延迟（端侧 T3 留 perf 基线对照）
"""
from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Callable


@dataclass
class ReplyResult:
    """model_fn 真实返回值；与 `runner.ModelFn` 返回 str 的边界由 baseline_run 桥接。"""

    text: str
    model: str = ""
    fallback_reason: str = ""
    latency_ms: int = 0
    extras: dict[str, Any] = field(default_factory=dict)


# baseline model_fn 签名：case -> ReplyResult
BaselineModelFn = Callable[[dict[str, Any]], ReplyResult]


def make_model_fn(impl: str, **kwargs: Any) -> BaselineModelFn:
    """工厂：按 impl 字符串选择实现。kwargs 透传给具体实现。

    支持 impl：
      - "cloud_grpc"   → EmotionLLMService.ChatCompletion（port 50051，mock fallback 自动）
      - "cloud_deepseek" → DeepSeek HTTP OpenAI 兼容（无 key 抛 ConfigurationError，**不静默降级**）
      - "record"       → 先读 <replay_dir>/<case.id>.json 命中即回放；未命中转发到 inner_impl 落盘

    未知 impl → ValueError。
    """
    impl = impl.strip().lower()
    if impl == "cloud_grpc":
        from model_fns.cloud_grpc import make as _make
        return _make(**kwargs)
    if impl == "cloud_deepseek":
        from model_fns.cloud_deepseek import make as _make
        return _make(**kwargs)
    if impl == "record":
        from model_fns.record import make as _make
        return _make(**kwargs)
    raise ValueError(
        f"unknown model_fn impl: {impl!r} (supported: cloud_grpc | cloud_deepseek | record)"
    )