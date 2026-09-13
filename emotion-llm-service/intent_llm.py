"""Stage 87 · LLM 意图重分类（规则式兜底，key 可用时消歧）

docs/plans/intent-classification-6-types.md「LLM 式分类增强」（Stage 82 §三 open 项）：
  - 规则式（intent.classify_intent）先分类；高置信结果直接返回，不调 LLM
  - 模糊结果（other / confidence < RULE_CONFIDENT_THRESHOLD）且 LLM_API_KEY 存在时，
    用上游 LLM 非流式调用重分类（temperature=0、小 max_tokens、短超时）
  - LLM 失败 / 输出非法 / 无 key / LLM_INTENT_RECLASSIFY=0 → 原样返回规则结果：
    离线路径与 Stage 82 行为完全一致（§契约 6：mock/离线必须可跑）
"""
import logging
import os

from chat_completion import resolve_backend_config
from intent import INTENTS, classify_intent

logger = logging.getLogger(__name__)

# 规则式置信度阈值：confidence = hits/(hits+3)，0.5 即 ≥3 次关键词命中。
# 低于阈值（含 other/并列）视为模糊，交给 LLM 消歧。
RULE_CONFIDENT_THRESHOLD = 0.5

# LLM 重分类成功的固定置信度（LLM 单选题输出，不额外让它自估置信度）
LLM_INTENT_CONFIDENCE = 0.9

# 分类调用配额上限（只输出类名，小配额即可）
LLM_CLASSIFY_MAX_TOKENS = 20

# 单次分类调用超时秒数（ClassifyIntent 在 BFF 发消息链路上是同步调用，不能拖）
LLM_CLASSIFY_TIMEOUT_SECONDS = 5

_CLASSIFY_SYSTEM_PROMPT = (
    "你是消息意图分类器。把用户消息分到以下六类之一：\n"
    "- emotional_support：情感疏导（倾诉心事、心情低落、需要安慰陪伴）\n"
    "- study_help：学习问题（作业、考试、复习、学习方法）\n"
    "- tech_help：技术问题（代码、报错、开发、运维）\n"
    "- career_help：职业问题（面试、简历、求职、职场）\n"
    "- lifestyle：生活问题（娱乐、美食、运动、日常建议）\n"
    "- other：其他\n"
    "只输出一个类名，不要输出任何解释。"
)


def reclassify_enabled() -> bool:
    """env 开关：LLM_INTENT_RECLASSIFY（默认开；"0"/"false"/"no" 显式关闭）"""
    return os.environ.get("LLM_INTENT_RECLASSIFY", "").strip().lower() not in (
        "0", "false", "no",
    )


def _extract_label(content: str) -> str | None:
    """从 LLM 回复里提取 6 类标签；找不到合法标签返回 None"""
    if not content:
        return None
    lowered = content.strip().lower()
    # 长标签优先匹配，避免未来出现前缀重叠的标签误配
    for label in sorted(INTENTS, key=len, reverse=True):
        if label in lowered:
            return label
    return None


def _default_openai_client(config: dict):
    """分类专用 client：禁用 SDK 重试——ClassifyIntent 在 BFF 发消息同步链路上，
    SDK 默认重试 2 次会把最坏耗时放大 3 倍（timeout 5s × 3 ≈ 15s+）"""
    from openai import OpenAI
    return OpenAI(
        api_key=config["api_key"],
        base_url=config["base_url"],
        max_retries=0,
    )


def llm_classify_label(text: str, config: dict, client=None) -> str | None:
    """调上游 LLM 做单选分类，返回合法标签；任何失败返回 None（不外抛）"""
    try:
        if client is None:
            client = _default_openai_client(config)
        resp = client.chat.completions.create(
            model=config["model"],
            messages=[
                {"role": "system", "content": _CLASSIFY_SYSTEM_PROMPT},
                {"role": "user", "content": text},
            ],
            temperature=0,
            max_tokens=LLM_CLASSIFY_MAX_TOKENS,
            timeout=LLM_CLASSIFY_TIMEOUT_SECONDS,
        )
        content = resp.choices[0].message.content or ""
    except Exception as e:
        logger.warning(f"[intent-llm] upstream classify failed, keep rule result: {e}")
        return None
    return _extract_label(content)


def classify_intent_adaptive(text: str, client=None) -> tuple[str, float]:
    """自适应意图分类：规则式兜底 + LLM 消歧。返回 (intent, confidence)。

    决策顺序：
      1. 规则式高置信（非 other 且 ≥ 阈值）→ 直接返回，零 LLM 开销
      2. env 关闭 / 无 key → 返回规则结果（离线路径零变化）
      3. LLM 重分类得合法非 other 标签 → 返回 (label, LLM_INTENT_CONFIDENCE)
      4. 其余（LLM 失败 / 输出 other / 非法）→ 返回规则结果
    """
    intent, confidence = classify_intent(text)
    if confidence >= RULE_CONFIDENT_THRESHOLD and intent != "other":
        return intent, confidence
    if not reclassify_enabled():
        return intent, confidence
    config = resolve_backend_config()
    if config is None:
        return intent, confidence
    label = llm_classify_label(text, config, client=client)
    if label is not None and label != "other":
        logger.info(f"[intent-llm] reclassified {intent!r}({confidence}) -> {label!r}")
        return label, LLM_INTENT_CONFIDENCE
    return intent, confidence
