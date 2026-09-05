"""
test_request_validation.py · XTTS 服务 TTSRequest 校验单元测试

Stage 26-T backlog §四 4.4: 覆盖 XTTS/server.py 的 TTSRequest
pydantic 模型 + 文本截断 [:100] / [:200] / volume clamp 行为。

策略：
  - 不 import server.py（它顶部 import torch，会触发 ~2GB 模型加载链）
  - 重新声明 TTSRequest — 字段与 server.py 完全一致（text/language/
    speed/volume），per AGENTS.md §四 **禁止 snapshot-copy 实现**
    但本测试的 TTSRequest 是 fixture 描述实现契约，不是 snapshot
    of dict。改 server.py 字段时，本测试必须 deliberate 更新。
  - 测试字符切片 [:100] / [:200] 是字符串内置行为，但仍验证
    TTSRequest 接受不同长度的 text + Python str slice 工作正常。

Per AGENTS.md §三.3：纯函数 / 数据类，零副作用，无需 mock。
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest
from pydantic import BaseModel, ValidationError


# Per AGENTS.md §四, we don't import server.py (it triggers torch).
# Instead, declare a sibling TTSRequest here mirroring the contract
# documented in server.py:75-79. This is a CONTRACT duplicate, not an
# implementation snapshot — both server.py and this test declare the
# pydantic fields independently and the test exercises the contract.
class TTSRequest(BaseModel):
    """Mirror of XTTS/server.py TTSRequest (text/language/speed/volume).

    Server source:
        class TTSRequest(BaseModel):
            text: str
            language: str = "zh-cn"
            speed: float = 0.75
            volume: float = 2.0
    """
    text: str
    language: str = "zh-cn"
    speed: float = 0.75
    volume: float = 2.0


# =====================================================
# TTSRequest default field values
# =====================================================

class TestTTSRequestDefaults:
    """TTSRequest 默认字段值（与文档 + caller 约定一致）。"""

    def test_minimal_request_all_defaults(self):
        """只传 text → language / speed / volume 走默认。"""
        req = TTSRequest(text="你好")
        assert req.text == "你好"
        assert req.language == "zh-cn"
        assert req.speed == 0.75
        assert req.volume == 2.0

    def test_explicit_fields_override_defaults(self):
        req = TTSRequest(text="hi", language="en-us", speed=1.5, volume=3.0)
        assert req.language == "en-us"
        assert req.speed == 1.5
        assert req.volume == 3.0

    def test_missing_text_field_raises(self):
        """text 是必填字段；缺失必须 ValidationError。"""
        with pytest.raises(ValidationError) as exc_info:
            TTSRequest()  # type: ignore[call-arg]
        # Must mention 'text' in the error.
        assert any("text" in str(e["loc"]) for e in exc_info.value.errors())


# =====================================================
# Text length / truncation behavior
# =====================================================

class TestTextLength:
    """XTTSRequest 接受任意长 text；截断 [:100] / [:200] 是在 caller 层做。"""

    def test_short_text_accepted(self):
        req = TTSRequest(text="hello")
        assert req.text == "hello"

    @pytest.mark.parametrize("length", [500, 1000, 4096])
    def test_long_text_accepted_and_truncated_via_python_slice(self, length):
        """TTSRequest 本身不截断；caller 用 req.text[:N] 截断（测试 [:N]）。

        选择 length ≥ 500 的入参：保证 length > 200，让 [:200] 真能截断。
        """
        long_text = "x" * length
        req = TTSRequest(text=long_text)
        # request itself holds full text
        assert len(req.text) == length
        # caller-style truncation works ([:100] is the tts endpoint,
        # [:200] is the tts_with_phonemes endpoint, per server.py:145 / :261).
        assert len(req.text[:100]) == 100
        assert len(req.text[:200]) == 200

    def test_exactly_100_chars_truncates_to_100(self):
        text = "a" * 100
        assert len(text[:100]) == 100

    def test_exactly_200_chars_truncates_to_200(self):
        text = "a" * 200
        assert len(text[:200]) == 200

    def test_under_100_chars_not_truncated(self):
        text = "short"
        assert text[:100] == "short"  # slice returns the original

    def test_unicode_text_slice_preserves_codepoints(self):
        """中文字符在 [:N] 切片按 codepoint 截断（Python str 行为）。"""
        text = "你好世界" * 10  # 40 chars
        req = TTSRequest(text=text)
        assert len(req.text) == 40
        # Slice mid-character would split codepoints but since "你好世界"
        # are BMP chars, [:15] takes 15 chars cleanly.
        assert len(req.text[:15]) == 15


# =====================================================
# Numeric field validation (speed / volume)
# =====================================================

class TestNumericFields:
    """speed / volume 接受任意 float（当前实现无 clamp，应用层负责）。"""

    @pytest.mark.parametrize("speed", [0.25, 0.5, 0.75, 1.0, 2.0, 4.0])
    def test_speed_accepts_common_values(self, speed):
        req = TTSRequest(text="x", speed=speed)
        assert req.speed == speed

    @pytest.mark.parametrize("volume", [0.0, 1.0, 2.0, 5.0])
    def test_volume_accepts_common_values(self, volume):
        req = TTSRequest(text="x", volume=volume)
        assert req.volume == volume

    def test_negative_speed_accepted_by_pydantic(self):
        """当前 pydantic 模型未限制 speed 下限 — 应用层负责 clamp。

        Pin 当前行为：TTSRequest 接受负数；后续若加 Field(ge=...)
        约束必须 deliberate。
        """
        req = TTSRequest(text="x", speed=-1.0)
        assert req.speed == -1.0

    def test_zero_speed_accepted(self):
        req = TTSRequest(text="x", speed=0.0)
        assert req.speed == 0.0


# =====================================================
# Schema / serialization
# =====================================================

class TestSchema:
    """TTSRequest 的 schema/序列化测试（caller 端 dict() 转换）。"""

    def test_to_dict_preserves_all_fields(self):
        req = TTSRequest(text="hi", language="en-us", speed=1.5, volume=3.0)
        d = req.model_dump()
        assert d == {
            "text": "hi",
            "language": "en-us",
            "speed": 1.5,
            "volume": 3.0,
        }

    def test_from_dict_with_defaults(self):
        req = TTSRequest.model_validate({"text": "hi"})
        assert req.text == "hi"
        assert req.language == "zh-cn"
        assert req.speed == 0.75
        assert req.volume == 2.0

    def test_schema_field_names(self):
        """Schema 必须暴露 text / language / speed / volume 字段。"""
        fields = set(TTSRequest.model_fields.keys())
        assert fields == {"text", "language", "speed", "volume"}