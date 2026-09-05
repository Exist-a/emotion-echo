"""
test_emotion_parser.py · SenseVoice 服务纯函数情绪解析单元测试

Stage 26-T backlog §四 4.4: 覆盖 sensevoice-small/server.py 的
extract_emotion_from_raw() / extract_text_only() 纯函数 + EMOTION_TOKEN_RE
正则 + EMOTION_TOKEN_MAPPING 字典。

策略：
  - 不加载 funasr 模型（避免 ~30s + ~936MB 权重）
  - 直接 import server.py 的纯函数（它们在模块顶层，无副作用）
  - 用 table-driven 测试覆盖所有合法 + 边界 token

Per AGENTS.md §三.3（mock 与 prod 一致）：被测函数本身就是纯函数，
没有副作用，无需 mock。
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

import pytest

# 把 sensevoice-small 加入 PYTHONPATH
SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

# 1. 直接 import 目标函数（无副作用）
from server import (
    extract_emotion_from_raw,
    extract_text_only,
    EMOTION_TOKEN_RE,
    EMOTION_TOKEN_MAPPING,
    EMOTIONS,
)


# =====================================================
# EMOTION_TOKEN_RE tests
# =====================================================

class TestEmotionTokenRegex:
    """EMOTION_TOKEN_RE = re.compile(r'<\\|([A-Z_]+)\\|>')"""

    def test_single_token_extracted(self):
        m = EMOTION_TOKEN_RE.search("<|HAPPY|>")
        assert m is not None
        assert m.group(1) == "HAPPY"

    def test_token_in_text_extracted(self):
        """Token 嵌入在更长的 raw_text 中（如 <|HAPPY|><|zh|>你好）也能匹配。"""
        m = EMOTION_TOKEN_RE.search("<|HAPPY|><|zh|>你好世界")
        assert m is not None
        assert m.group(1) == "HAPPY"

    def test_underscore_token(self):
        m = EMOTION_TOKEN_RE.search("<|EMO_UNKNOWN|>")
        assert m is not None
        assert m.group(1) == "EMO_UNKNOWN"

    def test_no_token_returns_none(self):
        assert EMOTION_TOKEN_RE.search("plain text without tokens") is None

    def test_malformed_token_not_matched(self):
        """<|lowercase|> 或 <|1number|> 应不匹配（仅 A-Z_）。"""
        assert EMOTION_TOKEN_RE.search("<|lowercase|>") is None
        assert EMOTION_TOKEN_RE.search("<|123|>") is None

    def test_unclosed_token_not_matched(self):
        assert EMOTION_TOKEN_RE.search("<|HAPPY") is None
        assert EMOTION_TOKEN_RE.search("HAPPY|>") is None


# =====================================================
# extract_emotion_from_raw tests
# =====================================================

class TestExtractEmotionFromRaw:
    """extract_emotion_from_raw(raw_text) → (emotion, confidence)"""

    @pytest.mark.parametrize("raw_text,expected,expected_conf", [
        # SenseVoice 输出格式：<|EMOTION|><|LANG|>...（情绪 token 在前）。
        # 实现为显式 emotion token 分配 conf=0.9。
        ("<|HAPPY|><|zh|>你好", "happy", 0.9),
        ("<|ANGRY|><|zh|>滚", "angry", 0.9),
        ("<|SAD|><|zh|>唉", "sad", 0.9),
        ("<|SURPRISE|><|zh|>哇", "surprise", 0.9),
        ("<|FEAR|><|zh|>怕", "fear", 0.9),
        ("<|DISGUST|><|zh|>恶心", "disgust", 0.9),
        ("<|NEUTRAL|><|zh|>哦", "neutral", 0.6),  # NEUTRAL 不在显式集合
    ])
    def test_known_token_to_emotion(self, raw_text, expected, expected_conf):
        emotion, conf = extract_emotion_from_raw(raw_text)
        assert emotion == expected
        # SenseVoice 不输出概率，按惯例给固定 confidence：
        # 显式 emotion (HAPPY/ANGRY/SAD/SURPRISE/FEAR/DISGUST) → 0.9
        # NEUTRAL / EMO_UNKNOWN / 无 token → 0.6
        assert conf == expected_conf

    def test_unknown_token_falls_back_to_neutral(self):
        """EMOTION_TOKEN_MAPPING 把 EMO_UNKNOWN 归 neutral。"""
        emotion, conf = extract_emotion_from_raw("<|EMO_UNKNOWN|><|zh|>x")
        assert emotion == "neutral"
        # EMO_UNKNOWN 是 fall-back token，confidence 比显式情绪低
        assert conf == 0.6

    def test_no_token_falls_back_to_neutral(self):
        emotion, conf = extract_emotion_from_raw("plain text without tokens")
        assert emotion == "neutral"
        # 无 token 时实现返回 0.5
        assert conf == 0.5

    def test_emotion_extraction_takes_first_token(self):
        """多个 emotion token 时取第一个（顺序：先 HAPPY 后 ANGRY → happy）。"""
        emotion, _ = extract_emotion_from_raw("<|HAPPY|><|ANGRY|>...")
        assert emotion == "happy"

    def test_confidence_always_in_zero_one_range(self):
        for raw in ["<|HAPPY|>x", "no tokens", "<|NEUTRAL|>y"]:
            _, conf = extract_emotion_from_raw(raw)
            assert 0.0 <= conf <= 1.0

    def test_explicit_emotion_higher_confidence_than_unknown(self):
        """HAPPY 等显式情绪 → 0.9 信心；EMO_UNKNOWN → 0.6（实现约定）。"""
        explicit, conf_explicit = extract_emotion_from_raw("<|HAPPY|><|zh|>x")
        unknown, conf_unknown = extract_emotion_from_raw("<|EMO_UNKNOWN|><|zh|>x")
        assert explicit == "happy"
        assert unknown == "neutral"
        assert conf_explicit > conf_unknown, (
            f"explicit emotion conf ({conf_explicit}) should exceed "
            f"unknown token conf ({conf_unknown})"
        )


# =====================================================
# extract_text_only tests
# =====================================================

class TestExtractTextOnly:
    """extract_text_only(raw_text) → 去除 <|...|> token 后的纯文本。"""

    def test_strips_all_tokens(self):
        """所有 <|XXX|> token 都被移除，剩纯文本。

        注：当前 regex 限定 [A-Z_]，故 <|zh|> 等小写 lang token
        不被剥——这是 pinned 行为。如果未来扩展到 [A-Za-z_]，
        本测试需要相应更新（应断言 "你好世界" 而非 "<|zh|>你好世界"）。
        """
        out = extract_text_only("<|HAPPY|>你好世界")
        assert out == "你好世界"

    def test_strips_multiple_tokens(self):
        out = extract_text_only("<|NEUTRAL|>中间文本<|END|>")
        # 所有大写 emotion token 都被剥除
        assert "中间文本" in out
        assert "<|NEUTRAL|>" not in out
        assert "<|END|>" not in out

    def test_text_without_tokens_unchanged(self):
        assert extract_text_only("plain text") == "plain text"

    def test_empty_string(self):
        assert extract_text_only("") == ""

    def test_only_tokens_returns_stripped_with_remainder(self):
        """纯 emotion token → 剥完后是空字符串（剩余全是 token）。"""
        # 注：当前实现的 EMOTION_TOKEN_RE 限定 A-Z_，所以 <|zh|> 不被
        # 剥除；只有 <|HAPPY|>/<|ANGRY|> 等全大写 token 才会被剥。
        out = extract_text_only("<|HAPPY|>")
        assert out == ""

    def test_strips_lowercase_tokens_not_matched(self):
        """<|lowercase|> 不被 regex 匹配（A-Z_），应保留在结果中。"""
        # 这是当前实现的 pinned 行为：regex 限定 [A-Z_]，故 <|lowercase|>
        # 不是合法 emotion token 格式，不被剥除。这一行为如果未来调整
        # （如改成 [A-Za-z_]），本测试需要相应更新。
        out = extract_text_only("<|lowercase|>保留<|HAPPY|>")
        assert "保留" in out
        assert "<|HAPPY|>" not in out  # 大写被剥
        assert "<|lowercase|>" in out  # 小写保留（regex 不匹配）


# =====================================================
# EMOTION_TOKEN_MAPPING tests
# =====================================================

class TestEmotionTokenMapping:
    """EMOTION_TOKEN_MAPPING 字典完整性 + 一致性。"""

    def test_mapping_covers_all_required_emotions(self):
        """happy/angry/sad/neutral 必须出现（项目核心 4 类）。"""
        for required in ("HAPPY", "ANGRY", "SAD", "NEUTRAL"):
            assert required in EMOTION_TOKEN_MAPPING

    def test_mapped_values_in_unified_set(self):
        """所有映射目标必须在 EMOTIONS（7 类 fer 标准）中。"""
        for token, target in EMOTION_TOKEN_MAPPING.items():
            assert target in EMOTIONS, (
                f"token {token!r} maps to {target!r} which is not in EMOTIONS"
            )

    def test_unknown_token_maps_to_neutral(self):
        """EMO_UNKNOWN 必须归 neutral（fail-safe）。"""
        assert EMOTION_TOKEN_MAPPING["EMO_UNKNOWN"] == "neutral"