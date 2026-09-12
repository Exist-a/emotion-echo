"""Stage 87 RED · LLM 意图重分类（规则式兜底，key 可用时消歧）

docs/plans/intent-classification-6-types.md「LLM 式分类增强」（Stage 82 §三 open 项）：
  - 规则式先分类；置信度不足（other / confidence < 阈值）且 LLM_API_KEY 存在时，
    用上游 LLM 非流式调用重分类
  - LLM 失败 / 输出非法 / 无 key / env 关闭 → 原样返回规则结果（离线路径零变化，
    §契约 6 哲学：mock/离线必须可跑）
  - 规则式高置信结果不触发 LLM（省延迟 + 省 token）
"""
import pytest

import intent_llm
from intent import classify_intent
from intent_llm import _extract_label, classify_intent_adaptive


CONFIDENT_TEXT = "我的 Python 代码报错了，怎么调试"  # 多关键词命中，规则式高置信
AMBIGUOUS_TEXT = "帮我出个主意"  # 零关键词命中 → other / 0.0


class FakeMessage:
    def __init__(self, content):
        self.content = content


class FakeChoice:
    def __init__(self, content):
        self.message = FakeMessage(content)


class FakeResponse:
    def __init__(self, content):
        self.choices = [FakeChoice(content)]


class FakeCompletions:
    def __init__(self, reply=None, exc=None):
        self._reply = reply
        self._exc = exc
        self.calls = []

    def create(self, **kwargs):
        self.calls.append(kwargs)
        if self._exc is not None:
            raise self._exc
        return FakeResponse(self._reply)


class FakeClient:
    def __init__(self, reply=None, exc=None):
        class _Chat:
            def __init__(self, completions):
                self.completions = completions
        self.chat = _Chat(FakeCompletions(reply, exc))


def _with_llm_config(monkeypatch):
    """让 resolve_backend_config 返回可用配置（等价 LLM_API_KEY 已设置）"""
    monkeypatch.setattr(
        intent_llm,
        "resolve_backend_config",
        lambda: {"api_key": "test-key", "base_url": "http://fake", "model": "fake-model"},
    )


def _no_llm_key(monkeypatch):
    monkeypatch.delenv("LLM_API_KEY", raising=False)


class TestClassifyIntentAdaptive:
    def test_confident_rule_result_skips_llm(self, monkeypatch):
        """规则式高置信（≥阈值且非 other）→ 不调 LLM，结果与纯规则一致"""
        fake = FakeClient(reply="emotional_support")
        intent, conf = classify_intent_adaptive(CONFIDENT_TEXT, client=fake)
        rule_intent, rule_conf = classify_intent(CONFIDENT_TEXT)
        assert (intent, conf) == (rule_intent, rule_conf)
        assert fake.chat.completions.calls == [], "高置信结果不应触发 LLM 调用"

    def test_ambiguous_reclassified_by_llm(self, monkeypatch):
        """规则式 other → LLM 重分类成功 → 返回 LLM 标签 + 固定置信度"""
        _with_llm_config(monkeypatch)
        fake = FakeClient(reply="emotional_support")
        intent, conf = classify_intent_adaptive(AMBIGUOUS_TEXT, client=fake)
        assert intent == "emotional_support"
        assert conf == intent_llm.LLM_INTENT_CONFIDENCE
        assert len(fake.chat.completions.calls) == 1

    def test_llm_call_contract(self, monkeypatch):
        """LLM 调用契约：temperature=0（确定性）、max_tokens 走小配额、prompt 含 6 类定义"""
        _with_llm_config(monkeypatch)
        fake = FakeClient(reply="tech_help")
        classify_intent_adaptive(AMBIGUOUS_TEXT, client=fake)
        kwargs = fake.chat.completions.calls[0]
        assert kwargs["temperature"] == 0
        assert kwargs["max_tokens"] <= 64, "分类调用只需输出类名，必须小配额"
        assert kwargs["model"] == "fake-model"
        system_msg = kwargs["messages"][0]
        assert system_msg["role"] == "system"
        for label in ("emotional_support", "study_help", "tech_help",
                      "career_help", "lifestyle", "other"):
            assert label in system_msg["content"], f"prompt 必须列出 {label}"
        user_msg = kwargs["messages"][1]
        assert user_msg["role"] == "user"
        assert AMBIGUOUS_TEXT in user_msg["content"]

    def test_llm_invalid_label_falls_back_to_rule(self, monkeypatch):
        """LLM 输出不在 6 类里 → 原样返回规则结果"""
        _with_llm_config(monkeypatch)
        fake = FakeClient(reply="我觉得应该是购物问题吧")
        intent, conf = classify_intent_adaptive(AMBIGUOUS_TEXT, client=fake)
        rule = classify_intent(AMBIGUOUS_TEXT)
        assert (intent, conf) == rule

    def test_llm_says_other_falls_back_to_rule(self, monkeypatch):
        """LLM 也判 other → 保留规则结果（避免把弱规则命中降级成 other）"""
        _with_llm_config(monkeypatch)
        weak_text = "周末去哪玩比较好"  # lifestyle 单命中 → 弱置信但非 other
        fake = FakeClient(reply="other")
        intent, conf = classify_intent_adaptive(weak_text, client=fake)
        assert (intent, conf) == classify_intent(weak_text)
        assert intent != "other"

    def test_llm_error_falls_back_to_rule(self, monkeypatch):
        """LLM 抛异常（网络/SDK）→ 不外抛，降级规则结果"""
        _with_llm_config(monkeypatch)
        fake = FakeClient(exc=RuntimeError("connection refused"))
        intent, conf = classify_intent_adaptive(AMBIGUOUS_TEXT, client=fake)
        assert (intent, conf) == classify_intent(AMBIGUOUS_TEXT)

    def test_no_api_key_skips_llm(self, monkeypatch):
        """无 LLM_API_KEY → 不调 LLM（离线路径与 Stage 82 行为完全一致）"""
        _no_llm_key(monkeypatch)
        fake = FakeClient(reply="emotional_support")
        intent, conf = classify_intent_adaptive(AMBIGUOUS_TEXT, client=fake)
        assert (intent, conf) == classify_intent(AMBIGUOUS_TEXT)
        assert fake.chat.completions.calls == []

    def test_env_disable_skips_llm(self, monkeypatch):
        """LLM_INTENT_RECLASSIFY=0 → 显式关闭，即使 key 可用也不调 LLM"""
        _with_llm_config(monkeypatch)
        monkeypatch.setenv("LLM_INTENT_RECLASSIFY", "0")
        fake = FakeClient(reply="emotional_support")
        intent, conf = classify_intent_adaptive(AMBIGUOUS_TEXT, client=fake)
        assert (intent, conf) == classify_intent(AMBIGUOUS_TEXT)
        assert fake.chat.completions.calls == []

    def test_low_confidence_non_other_also_reclassified(self, monkeypatch):
        """弱置信（<阈值）但非 other 也走 LLM 消歧——歧义消息不止零命中一种"""
        _with_llm_config(monkeypatch)
        weak_text = "周末去哪玩比较好"
        rule_intent, rule_conf = classify_intent(weak_text)
        assert rule_intent != "other" and rule_conf < intent_llm.RULE_CONFIDENT_THRESHOLD
        fake = FakeClient(reply="lifestyle")
        intent, conf = classify_intent_adaptive(weak_text, client=fake)
        assert intent == "lifestyle"
        assert conf == intent_llm.LLM_INTENT_CONFIDENCE


class TestExtractLabel:
    @pytest.mark.parametrize("content,expected", [
        ("tech_help", "tech_help"),
        ("tech_help。", "tech_help"),
        ("分类结果：career_help", "career_help"),
        ("I think this is lifestyle", "lifestyle"),
        ("emotional_support\n", "emotional_support"),
        ("随便说说", None),
        ("", None),
    ])
    def test_extraction(self, content, expected):
        assert _extract_label(content) == expected

    def test_no_extra_labels_invented(self):
        assert _extract_label("unknown_category") is None
