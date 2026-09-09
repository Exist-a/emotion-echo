"""
test_emotion_mapping.py · FER-tflite 服务情绪映射表单测

PR-TTS-VENDOR Phase 2: tflite + Haar cascade 后端的纯数据契约。

策略：
  - 从 server.py import 共享常量，不复制（per AGENTS.md §四 禁止 snapshot-copy）。
  - 7 类 raw → 5 类 unified 的映射必须与 emotion-llm-service 对齐。
"""
from __future__ import annotations

import pytest

from server import EMOTION_MAPPING, EMOTIONS


# 项目 5 类统一情感（与 emotion-llm-service 对齐）
UNIFIED = {"angry", "anxious", "happy", "sad", "neutral"}


@pytest.mark.parametrize("raw", EMOTIONS)
def test_mapping_covers_all_raw(raw):
    """每个 raw emotion 必须有映射条目"""
    assert raw in EMOTION_MAPPING, (
        f"raw emotion {raw!r} is in EMOTIONS but missing from EMOTION_MAPPING"
    )


@pytest.mark.parametrize("raw,mapped", [
    ("angry", "angry"),
    ("happy", "happy"),
    ("sad", "sad"),
    ("fear", "anxious"),
    ("disgust", "neutral"),
    ("surprise", "neutral"),
    ("neutral", "neutral"),
])
def test_emotion_mapping_table_driven(raw, mapped):
    """逐条 raw→mapped 映射必须与 server.py 一致。"""
    assert EMOTION_MAPPING[raw] == mapped


@pytest.mark.parametrize("raw", EMOTIONS)
def test_mapped_value_in_unified_set(raw):
    """所有映射目标必须在统一 5 类集合内（防止映射到项目未定义的情绪）。"""
    target = EMOTION_MAPPING[raw]
    assert target in UNIFIED, (
        f"raw={raw!r} maps to {target!r} which is not in the unified 5-class set"
    )


def test_unified_set_size():
    """项目 5 类统一情感：必须严格 5 个（不能因为新情绪加进来而漏检）。"""
    assert len(UNIFIED) == 5


def test_emotions_list_is_complete():
    """FER-tflite 必须识别全部 7 类（文档约定）。"""
    assert set(EMOTIONS) == {"angry", "disgust", "fear", "happy", "sad", "surprise", "neutral"}


def test_no_dup_mapping_keys():
    """EMOTIONS 不能有重复 key。"""
    assert len(EMOTIONS) == len(set(EMOTIONS))


def test_mapping_is_total_function():
    """每个 raw emotion 都有非空字符串映射目标（不能映射到 None 或 ''）。"""
    for raw in EMOTIONS:
        target = EMOTION_MAPPING[raw]
        assert target, f"raw {raw!r} maps to empty value"
        assert isinstance(target, str)