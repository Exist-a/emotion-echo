"""Stage 82 RED · 意图分类 + 按意图风格指令（llm-chat-real-pipeline PR-3a）

docs/plans/intent-classification-6-types.md（2026-09-12 注记后的重写版范围）：
  - 6 类：emotional_support / study_help / tech_help / career_help / lifestyle / other
  - 规则式关键词打分（与情绪分析同款风格，确定性可测，无 LLM 依赖）
  - 每类有回复风格指令；ChatCompletion with_intent=True 时注入 system prompt 并首帧回带 intent
"""
import pytest

from intent import (
    INTENTS,
    classify_intent,
    inject_style,
    style_instruction,
)


class TestClassifyIntent:
    @pytest.mark.parametrize("text,expected", [
        # emotional_support：心情/压力/需要安慰
        ("我今天心情很低落，感觉撑不下去了", "emotional_support"),
        ("最近压力好大，晚上睡不着", "emotional_support"),
        ("我很难过，想找人聊聊", "emotional_support"),
        # study_help：作业/学习方法/考试
        ("这道数学题不会做，能教我一下吗", "study_help"),
        ("马上要期末考试了，有没有高效的复习方法", "study_help"),
        # tech_help：代码/工具/报错
        ("我的 Python 代码报错了，怎么调试", "tech_help"),
        ("帮我看看这段代码为什么跑不起来", "tech_help"),
        # career_help：职业/工作/面试
        ("面试被拒了，我该怎么改进简历", "career_help"),
        ("工作压力大，想聊聊职业规划", "career_help"),
        # lifestyle：日常/兴趣/娱乐
        ("周末有什么好看的轻松电影推荐吗", "lifestyle"),
        ("最近想培养一个新爱好，有什么建议", "lifestyle"),
    ])
    def test_classification(self, text, expected):
        intent, confidence = classify_intent(text)
        assert intent == expected
        assert 0.0 <= confidence <= 1.0

    @pytest.mark.parametrize("text", [
        "你好",
        "今天天气怎么样",
        "",
        "   ",
    ])
    def test_fallback_to_other(self, text):
        intent, confidence = classify_intent(text)
        assert intent == "other"

    def test_all_six_intents_defined(self):
        assert INTENTS == {
            "emotional_support", "study_help", "tech_help",
            "career_help", "lifestyle", "other",
        }

    def test_every_intent_has_style_instruction(self):
        for intent in INTENTS:
            instruction = style_instruction(intent)
            assert instruction.strip(), f"{intent} 必须有风格指令"


class TestInjectStyle:
    def test_appends_to_existing_system_message(self):
        msgs = [
            {"role": "system", "content": "你是情感陪伴助手。"},
            {"role": "user", "content": "我的代码报错了"},
        ]
        out = inject_style(msgs, "tech_help")
        assert len(out) == 2, "不新增消息，只扩写 system"
        assert out[0]["role"] == "system"
        assert "你是情感陪伴助手。" in out[0]["content"], "原 system 内容保留"
        assert style_instruction("tech_help") in out[0]["content"]
        assert out[1] == msgs[1], "user 消息不动"

    def test_prepends_system_when_missing(self):
        msgs = [{"role": "user", "content": "hi"}]
        out = inject_style(msgs, "emotional_support")
        assert out[0]["role"] == "system"
        assert style_instruction("emotional_support") in out[0]["content"]
        assert out[1] == msgs[0]

    def test_does_not_mutate_input(self):
        msgs = [{"role": "user", "content": "hi"}]
        inject_style(msgs, "lifestyle")
        assert msgs == [{"role": "user", "content": "hi"}]
