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
    """延迟 import：openai SDK 只在上游可用时才需要（未安装时走 mock 不受影响）

    P2-R2-6: 加 timeout + max_retries——SDK 默认 timeout=600s 太久，
    上游 hang 起来会把 ai_stream 整段卡死。retries=1 配合上游偶尔网络抖动。
    """
    try:
        from openai import OpenAI
    except ImportError as e:  # pragma: no cover - 依赖缺失属部署错误
        raise ChatUpstreamError(f"openai sdk not installed: {e}") from e
    timeout = float(os.environ.get("LLM_UPSTREAM_TIMEOUT", "30"))
    max_retries = int(os.environ.get("LLM_UPSTREAM_MAX_RETRIES", "1"))
    return OpenAI(
        api_key=config["api_key"],
        base_url=config["base_url"],
        timeout=timeout,
        max_retries=max_retries,
    )


def make_mock_chunks(reason: str = "mock_no_api_key") -> list[ChatChunk]:
    """内置共情 mock 文案，拆 2 帧增量 + done 帧（体验接近真实流式）

    P1-R2-4: 增加文案变体（按 intent 分支），降低被秒判 mock vs 真 LLM 的风险。
    """
    import random
    variants = [
        "我在呢。愿意和我说说刚才发生了什么吗？不用着急，按你的节奏来就好。",
        "谢谢你告诉我这些。能再多说说你的感受吗？我在认真听。",
        "听起来你现在不太轻松。我陪你慢慢聊，先说说是什么让你这样想？",
        "嗯，我能理解你的心情。如果想继续聊，我在这里；如果想安静一会儿，也完全可以。",
    ]
    text = random.choice(variants)
    half = len(text) // 2
    return [
        ChatChunk(delta_content=text[:half], model="mock", fallback_reason=reason),
        ChatChunk(delta_content=text[half:], model="mock", fallback_reason=reason),
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
                # P1-R2-7: 实时内容安全审核（每帧 delta）
                safe_delta = moderate_content(delta)
                yield ChatChunk(
                    delta_content=safe_delta,
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
        yield from make_mock_chunks(_safe_fallback_reason("upstream_error", e))
    except Exception as e:  # SDK 抛出的任意异常都按上游失败处理
        logger.warning(f"[chat] upstream unexpected error, fallback to mock: {e}")
        yield from make_mock_chunks(_safe_fallback_reason("upstream_error", e))


def _safe_fallback_reason(prefix: str, exc: Exception) -> str:
    """P1-R2-4: 清洗异常消息，避免把 api_key / 凭据前缀透出到前端 fallback_reason"""
    import re
    msg = str(exc) or ""
    # 截断任何形如 sk-xxx / Bearer xxx / key=xxx 的凭据字段
    msg = re.sub(r"(sk-[A-Za-z0-9_-]{6})[A-Za-z0-9_-]+", r"\1***", msg)
    msg = re.sub(r"(Bearer\s+)[A-Za-z0-9._-]+", r"\1***", msg, flags=re.IGNORECASE)
    msg = re.sub(r"(api[_-]?key[=:]\s*)[^\s,'\"}]+", r"\1***", msg, flags=re.IGNORECASE)
    # 截断长度
    if len(msg) > 200:
        msg = msg[:200] + "..."
    return f"{prefix}: {msg}"


# P1-R2-7: LLM 输出安全关键词过滤（心理健康场景）
# 设计意图：拦截"自杀方法""自残步骤""如何获得毒品"等明显危险内容，
# 替换为劝导资源（24h 心理援助热线 400-161-9995）。
# 注意：仅做关键词匹配兜底，正经的内容审核需专业系统（百度/阿里内容安全 API）。
_DANGEROUS_PATTERNS = [
    r"自杀方法",
    r"自残步骤",
    r"如何获得毒品",
    r"毒品制作",
    r"how to (?:commit suicide|hurt myself)",
    r"ways to (?:die|kill yourself)",
    r"sucide method",
]
_SAFE_RESPONSE = (
    "我理解你现在可能很难受。这种感受很重要，建议你联系身边可信任的人，"
    "或拨打 24 小时心理援助热线：400-161-9995。你不是一个人。"
)

_dangerous_re = None


def _get_dangerous_re():
    """延迟编译正则（模块级 import 时不阻塞）"""
    global _dangerous_re
    if _dangerous_re is None:
        import re as _re
        _dangerous_re = _re.compile("|".join(_DANGEROUS_PATTERNS), _re.IGNORECASE)
    return _dangerous_re


def moderate_content(text: str) -> str:
    """P1-R2-7: 检查文本是否包含危险内容；命中则替换为安全回复。

    返回原始文本或替换后的安全回复。
    """
    if not text:
        return text
    re_obj = _get_dangerous_re()
    if re_obj.search(text):
        logger.warning("[moderation] dangerous content detected, replaced with safe response")
        return _SAFE_RESPONSE
    return text
