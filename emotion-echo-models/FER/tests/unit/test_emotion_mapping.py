"""
test_emotion_mapping.py · FER 服务情绪映射表单测（Stage 26-T 重构）

FER 的核心非模型逻辑是 fer raw emotion -> unified emotion (5 类) 的映射。

重构要点（per AGENTS.md §四 禁止 snapshot-copy）：
  - 旧版在测试文件里复制 EMOTION_MAPPING / UNIFIED 快照，与 server.py
    易 drift。本测试改为从 server.py import — 任何 drift 立即在
    server.py 单测时也被本测试捕获（共享同一字面量）。
  - 仍然保留表驱动覆盖所有 (raw → mapped) 映射。
  - UNIFIED 集合（项目 5 类）由本测试独立维护，因为它不是
    server.py 的导出 — 但规模必须等于 5。
"""
from __future__ import annotations

import pytest

# Import from server.py directly — no snapshot (AGENTS.md §四).
# server.py 同时被 FastAPI 加载；本测试只引用纯数据常量，不触发
# 模型加载路径。
from server import EMOTION_MAPPING, EMOTIONS


# 与 server.py 保持同步：项目采用 5 类统一情感
UNIFIED = {"angry", "anxious", "happy", "sad", "neutral"}


@pytest.mark.parametrize("raw", EMOTIONS)
def test_mapping_covers_all_raw(raw):
    """每个 fer raw emotion 必须有映射条目"""
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
    """FER 必须识别 fer 库的全部 7 类（文档约定）。"""
    assert set(EMOTIONS) == {"angry", "disgust", "fear", "happy", "sad", "surprise", "neutral"}


def test_no_dup_mapping_keys():
    """EMOTIONS 不能有重复 key（防止 EMOTION_MAPPING.keys() 去重后导致映射丢失）。"""
    assert len(EMOTIONS) == len(set(EMOTIONS))


def test_mapping_is_total_function():
    """每个 raw emotion 都有非空字符串映射目标（不能映射到 None 或 ''）。"""
    for raw in EMOTIONS:
        target = EMOTION_MAPPING[raw]
        assert target, f"raw {raw!r} maps to empty value"
        assert isinstance(target, str)