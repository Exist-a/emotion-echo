"""CL-04 · Phase 6 追加（T-17 终态）：**每个意图的风格指令都必须带"篇幅上限"**。

背景（实测驱动）：A 轨 Phase 5 接回 `with_intent` 后，`STYLE_INSTRUCTIONS`（"分点给出建议 1.2.3.、
关键建议加粗"）与 BFF 基础人设的"简短（2-3 句话）"在**同一份 system 里互相矛盾**；
用户先裁定去掉基础人设的硬约束，实测回复随即涨到 657/766/629 字（≈18 句），
再裁定"按目前状态适中"。

结论：篇幅约束必须**两侧一致**——
  · BFF 基础人设给"全文不超过 300 字 / 倾诉时更短"；
  · llm-service 的风格指令同样带硬上限（本文件钉死它），否则风格一侧会把回复顶回长文。
"""

import pytest

from intent import INTENTS, style_instruction

# 上限口径（与 BFF baseSystemPrompt 的措辞一致：全文不超过 300 字）
EXPECTED_CAP_MARKER = "300 字"


class TestStyleInstructionLengthCap:
    def test_every_intent_instruction_carries_length_cap(self):
        for intent in INTENTS:
            instruction = style_instruction(intent)
            assert instruction, f"意图 {intent} 的风格指令不得为空"
            assert EXPECTED_CAP_MARKER in instruction, (
                f"意图 {intent} 的风格指令缺少篇幅上限「{EXPECTED_CAP_MARKER}」——"
                "与 BFF 基础人设的长度指引必须一致，否则 prompt 内自相矛盾"
            )

    def test_unknown_intent_falls_back_with_cap(self):
        # 未知意图回落 other，同样必须带上限
        assert EXPECTED_CAP_MARKER in style_instruction("不存在的意图")

    def test_emotional_support_prefers_short(self):
        """情绪倾诉类必须显式"更短"，否则长文形态会盖过陪伴感。"""
        instruction = style_instruction("emotional_support")
        assert "倾诉" in instruction or "更短" in instruction or "简短" in instruction, (
            f"emotional_support 的风格指令应显式要求短：{instruction!r}"
        )

    def test_cap_is_appended_not_replacing_intent_specific_guidance(self):
        """上限是**追加**约束：各意图原有结构指引不得被顶掉。"""
        assert "分点" in style_instruction("career_help") or "1." in style_instruction("career_help")
        assert "代码块" in style_instruction("tech_help") or "序号" in style_instruction("tech_help")
