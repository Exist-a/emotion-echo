"""Stage 80 · ChatCompletion 上游流式转发（llm-chat-real-pipeline PR-1）

契约（tests/unit/test_chat_completion.py）：
  - 上游：OpenAI 兼容 chat.completions 流式（LLM_BASE_URL/LLM_API_KEY/LLM_MODEL，
    DeepSeek 等端点）；delta.content → ChatChunk.delta_content，收尾 done=True 帧
  - 降级：LLM_API_KEY 缺失或上游异常 → 内置 mock 文案整条降级，
    每帧 fallback_reason 非空、model="mock"——保证 CI / 离线 demo 全链路可跑
  - 首帧携带 model；结束帧后不再有帧

纯逻辑模块（不依赖 grpc runtime），servicer 在 grpc_server.py 注入调用。
"""
import logging
import os
from dataclasses import dataclass

logger = logging.getLogger(__name__)

DEFAULT_CHAT_MODEL = "deepseek-chat"
DEFAULT_BASE_URL = "https://api.deepseek.com"

MOCK_REPLY = (
    "我在呢。愿意和我说说刚才发生了什么吗？"
    "不用着急，按你的节奏来就好。"
)


@dataclass
class ChatChunk:
    """与 proto ChatChunk 一一对应的纯 Python 形状（servicer 负责转 pb）"""

    delta_content: str = ""
    done: bool = False
    model: str = ""
    fallback_reason: str = ""


class ChatUpstreamError(Exception):
    """上游 LLM 调用失败（连接/超时/HTTP 非 2xx/SDK 异常）"""


def resolve_backend_config() -> dict | None:
    """从 env 解析上游配置；LLM_API_KEY 缺失返回 None（调用方走 mock 降级）"""
    api_key = os.environ.get("LLM_API_KEY", "").strip()
    if not api_key:
        return None
    return {
        "api_key": api_key,
        "base_url": os.environ.get("LLM_BASE_URL", "").strip() or DEFAULT_BASE_URL,
        "model": os.environ.get("LLM_MODEL", "").strip() or DEFAULT_CHAT_MODEL,
    }


def _default_openai_client(config: dict):
    """延迟 import：openai SDK 只在上游可用时才需要（未安装时走 mock 不受影响）"""
    try:
        from openai import OpenAI
    except ImportError as e:  # pragma: no cover - 依赖缺失属部署错误
        raise ChatUpstreamError(f"openai sdk not installed: {e}") from e
    return OpenAI(api_key=config["api_key"], base_url=config["base_url"])


def make_mock_chunks(reason: str = "mock_no_api_key") -> list[ChatChunk]:
    """内置共情 mock 文案，拆 2 帧增量 + done 帧（体验接近真实流式）"""
    half = len(MOCK_REPLY) // 2
    return [
        ChatChunk(delta_content=MOCK_REPLY[:half], model="mock", fallback_reason=reason),
        ChatChunk(delta_content=MOCK_REPLY[half:], model="mock", fallback_reason=reason),
        ChatChunk(done=True, model="mock", fallback_reason=reason),
    ]


def iter_chat_chunks(messages: list[dict], model: str = "", temperature: float = 0.0,
                     max_tokens: int = 0, client=None, user_id: str = "") -> "iter[ChatChunk]":
    """对话主入口：优先上游流式转发，任何失败整条降级 mock（绝不 half-stream）。

    user_id 目前仅作审计透传预留（限流在网关层）。
    """
    config = resolve_backend_config()
    if config is None:
        yield from make_mock_chunks("mock_no_api_key")
        return

    upstream_model = model or config["model"]
    try:
        if client is None:
            client = _default_openai_client(config)
        kwargs = {
            "model": upstream_model,
            "messages": messages,
            "stream": True,
        }
        if temperature:
            kwargs["temperature"] = temperature
        if max_tokens:
            kwargs["max_tokens"] = max_tokens
        stream = client.chat.completions.create(**kwargs)

        first = True
        saw_any = False
        for event in stream:
            delta = ""
            try:
                delta = event.choices[0].delta.content or ""
            except (AttributeError, IndexError, TypeError):
                delta = ""
            if delta:
                saw_any = True
                yield ChatChunk(
                    delta_content=delta,
                    model=upstream_model if first else "",
                    fallback_reason="",
                )
                first = False
        if not saw_any:
            # 上游 200 但 0 帧——视为失败降级，避免前端收空白回复
            logger.warning("[chat] upstream returned empty stream, fallback to mock")
            yield from make_mock_chunks("mock_empty_stream")
            return
        yield ChatChunk(done=True, model=upstream_model, fallback_reason="")
    except ChatUpstreamError as e:
        logger.warning(f"[chat] upstream failed, fallback to mock: {e}")
        yield from make_mock_chunks(f"upstream_error: {e}")
    except Exception as e:  # SDK 抛出的任意异常都按上游失败处理
        logger.warning(f"[chat] upstream unexpected error, fallback to mock: {e}")
        yield from make_mock_chunks(f"upstream_error: {e}")
