"""
test_logging_setup.py · emotion-llm-service 日志配置单元测试

Stage 26-T backlog §四 4.4: 覆盖 logging_setup.py 的 JsonFormatter /
TextFormatter / setup_logging()。所有测试只动 root logger，结束后
恢复。Coverage:

  - JsonFormatter: basic record → valid JSON line with ts/level/logger/msg
  - JsonFormatter: extra= fields appear at top level (no snapshot)
  - JsonFormatter: unjsonable extra falls back to repr()
  - JsonFormatter: exc_info serialized as 'exc' key
  - TextFormatter: produces human-readable "[ts] [level] [name] msg"
  - setup_logging: LOG_FORMAT=json picks JsonFormatter
  - setup_logging: LOG_FORMAT=text picks TextFormatter
  - setup_logging: LOG_LEVEL=DEBUG propagates to root
  - setup_logging: replaces existing handlers (idempotent)
"""
from __future__ import annotations

import json
import logging
import os
import sys
from io import StringIO
from pathlib import Path

import pytest

# 把 emotion-llm-service 加入 PYTHONPATH
SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from logging_setup import (
    JsonFormatter,
    TextFormatter,
    setup_logging,
)


# =====================================================
# Helpers
# =====================================================

@pytest.fixture(autouse=True)
def restore_root_logger():
    """每个 test 后还原 root logger（避免 side-effect 泄漏）。"""
    root = logging.getLogger()
    original_handlers = list(root.handlers)
    original_level = root.level
    yield
    # Restore: remove anything we added, re-add original handlers.
    for h in list(root.handlers):
        root.removeHandler(h)
    for h in original_handlers:
        root.addHandler(h)
    root.setLevel(original_level)


def _make_record(name: str = "test", level: int = logging.INFO, msg: str = "hello",
                 exc_info=None, extra: dict | None = None) -> logging.LogRecord:
    """构造一个 LogRecord（绕过 logger 触发调用栈的 msecs 抖动）。"""
    rec = logging.LogRecord(
        name=name, level=level, pathname=__file__, lineno=1,
        msg=msg, args=(), exc_info=exc_info,
    )
    if extra:
        for k, v in extra.items():
            setattr(rec, k, v)
    return rec


# =====================================================
# JsonFormatter tests
# =====================================================

class TestJsonFormatter:
    """JsonFormatter: 一行 JSON，含 ts/level/logger/msg + extra + exc。"""

    def test_basic_record_produces_valid_json(self):
        rec = _make_record(name="svc.test", level=logging.INFO, msg="hello")
        out = JsonFormatter().format(rec)
        # Must be a single line of valid JSON.
        obj = json.loads(out)
        assert obj["logger"] == "svc.test"
        assert obj["level"] == "INFO"
        assert obj["msg"] == "hello"
        # ts is ISO8601 with milliseconds + Z suffix.
        assert obj["ts"].endswith("Z")
        assert "." in obj["ts"]

    def test_extra_fields_at_top_level(self):
        """per AGENTS.md §四禁止 snapshot-copy：直接传 extra 即可被序列化。"""
        rec = _make_record(
            name="svc", msg="done",
            extra={"message_id": 42, "emotion": "happy", "user_id": 7},
        )
        obj = json.loads(JsonFormatter().format(rec))
        assert obj["message_id"] == 42
        assert obj["emotion"] == "happy"
        assert obj["user_id"] == 7

    def test_unjsonable_extra_falls_back_to_repr(self):
        """自定义对象（无 __json__）走 repr() 路径，不抛。"""
        class Custom:
            def __repr__(self):
                return "<Custom obj>"

        rec = _make_record(msg="x", extra={"obj": Custom()})
        out = JsonFormatter().format(rec)
        obj = json.loads(out)
        assert "obj" in obj
        assert "<Custom obj>" in obj["obj"]

    def test_exc_info_serialized_as_exc(self):
        try:
            raise ValueError("test boom")
        except ValueError:
            rec = _make_record(msg="failed", exc_info=sys.exc_info())
        obj = json.loads(JsonFormatter().format(rec))
        assert "exc" in obj
        assert "ValueError" in obj["exc"]
        assert "test boom" in obj["exc"]

    def test_reserved_logrecord_fields_excluded(self):
        """reserved field (filename, lineno 等) 不应泄漏到 JSON 顶层。"""
        rec = _make_record(msg="x")
        obj = json.loads(JsonFormatter().format(rec))
        # 这些是 LogRecord 内置字段，不应作为 extra 进入顶层
        # （它们是 reserved，应被排除）。
        for reserved in ("args", "pathname", "filename", "module",
                         "funcName", "created", "msecs", "thread",
                         "threadName", "processName", "process"):
            assert reserved not in obj, f"reserved field {reserved!r} leaked to JSON"


# =====================================================
# TextFormatter tests
# =====================================================

class TestTextFormatter:
    """TextFormatter: 人类可读格式 '[ts] [level] [name] msg'。"""

    def test_basic_record_produces_human_readable(self):
        rec = _make_record(name="svc.test", level=logging.WARNING, msg="be careful")
        out = TextFormatter().format(rec)
        assert "WARNING" in out
        assert "svc.test" in out
        assert "be careful" in out
        # Must contain timestamp + brackets (per fmt string).
        assert "[" in out and "]" in out


# =====================================================
# setup_logging tests
# =====================================================

class TestSetupLogging:
    """setup_logging: 按 env 选 formatter + 级别，并替换现有 handler（idempotent）。"""

    def test_json_format_picks_jsonformatter(self, monkeypatch):
        monkeypatch.setenv("LOG_FORMAT", "json")
        monkeypatch.setenv("LOG_LEVEL", "INFO")
        setup_logging()
        root = logging.getLogger()
        assert len(root.handlers) == 1
        assert isinstance(root.handlers[0].formatter, JsonFormatter)
        assert root.level == logging.INFO

    def test_text_format_picks_textformatter(self, monkeypatch):
        monkeypatch.setenv("LOG_FORMAT", "text")
        setup_logging()
        root = logging.getLogger()
        assert len(root.handlers) == 1
        assert isinstance(root.handlers[0].formatter, TextFormatter)

    def test_log_level_propagates(self, monkeypatch):
        monkeypatch.setenv("LOG_FORMAT", "json")
        monkeypatch.setenv("LOG_LEVEL", "DEBUG")
        setup_logging()
        assert logging.getLogger().level == logging.DEBUG

    def test_explicit_level_arg_overrides_env(self, monkeypatch):
        """setup_logging(level='WARNING') 覆盖 LOG_LEVEL env。"""
        monkeypatch.setenv("LOG_LEVEL", "DEBUG")
        setup_logging(level="ERROR")
        assert logging.getLogger().level == logging.ERROR

    def test_idempotent_replaces_handlers(self, monkeypatch):
        """连续调用 setup_logging() 不会累积 handler（避免双倍日志）。"""
        monkeypatch.setenv("LOG_FORMAT", "json")
        setup_logging()
        first_count = len(logging.getLogger().handlers)
        setup_logging()
        second_count = len(logging.getLogger().handlers)
        assert second_count == 1, (
            f"setup_logging must be idempotent; got {first_count} then {second_count} handlers"
        )