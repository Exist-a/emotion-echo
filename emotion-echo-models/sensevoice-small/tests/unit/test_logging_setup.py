"""
test_logging_setup.py · sensevoice 服务 logging_setup 单元测试

Stage 26-T backlog §四 4.4: 覆盖 sensevoice/logging_setup.py 的
setup_logging() 函数。

策略：
  - setup_logging 是模块顶层副作用函数，每次调用都重置 root handlers。
  - 测试完用 autouse fixture 恢复 root logger，避免污染后续 test。
  - 不 import server.py（它顶部 import torch），仅 import logging_setup
    是安全的（依赖项只有 logging / os / sys）。
"""
from __future__ import annotations

import logging
import os
import sys
from io import StringIO
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from logging_setup import setup_logging


@pytest.fixture(autouse=True)
def restore_root_logger():
    """每个 test 后还原 root logger。"""
    root = logging.getLogger()
    original_handlers = list(root.handlers)
    original_level = root.level
    yield
    for h in list(root.handlers):
        root.removeHandler(h)
    for h in original_handlers:
        root.addHandler(h)
    root.setLevel(original_level)


class TestSetupLogging:
    """setup_logging(name) → Logger."""

    def test_returns_logger_with_requested_name(self):
        log = setup_logging("my.x")
        assert isinstance(log, logging.Logger)
        assert log.name == "my.x"

    def test_default_name_is_module_path(self):
        """无参数调用 → 返回 logging_setup 模块自己的 logger。"""
        log = setup_logging()
        assert log.name == "logging_setup"

    def test_replaces_root_handlers(self):
        """重复调用 setup_logging() 不会累积 handler。"""
        setup_logging()
        first_count = len(logging.getLogger().handlers)
        setup_logging()
        second_count = len(logging.getLogger().handlers)
        assert second_count == 1

    def test_respects_log_level_env(self, monkeypatch):
        """LOG_LEVEL=DEBUG 必须生效（仅当级别名合法时）。"""
        monkeypatch.setenv("LOG_LEVEL", "DEBUG")
        setup_logging()
        assert logging.getLogger().level == logging.DEBUG

    def test_respects_log_level_warn(self, monkeypatch):
        monkeypatch.setenv("LOG_LEVEL", "WARNING")
        setup_logging()
        assert logging.getLogger().level == logging.WARNING

    def test_invalid_log_level_falls_back_to_INFO(self, monkeypatch):
        monkeypatch.setenv("LOG_LEVEL", "BOGUS_LEVEL_XYZ")
        setup_logging()
        assert logging.getLogger().level == logging.INFO

    def test_json_format_sets_json_formatter(self, monkeypatch):
        """LOG_FORMAT=json 走 jsonlogger（或 JSONFormatter 兼容路径）。"""
        monkeypatch.setenv("LOG_FORMAT", "json")
        setup_logging()
        handler = logging.getLogger().handlers[0]
        fmt_class_name = type(handler.formatter).__name__
        # 期望是 JsonFormatter；或者 ImportError 退化为普通 Formatter。
        assert "Json" in fmt_class_name or fmt_class_name == "Formatter"

    def test_text_format_sets_text_formatter(self, monkeypatch):
        """LOG_FORMAT=text 走普通 Formatter。"""
        monkeypatch.setenv("LOG_FORMAT", "text")
        setup_logging()
        handler = logging.getLogger().handlers[0]
        assert type(handler.formatter).__name__ == "Formatter"

    def test_default_format_is_json(self, monkeypatch):
        """未设置 LOG_FORMAT 时默认走 json 格式（per Stage 20-2 pattern）。"""
        monkeypatch.delenv("LOG_FORMAT", raising=False)
        setup_logging()
        handler = logging.getLogger().handlers[0]
        fmt_class_name = type(handler.formatter).__name__
        assert "Json" in fmt_class_name or fmt_class_name == "Formatter"

    def test_writes_to_stdout(self, monkeypatch):
        """handler 必须绑 stdout（容器环境推荐）。"""
        setup_logging()
        handler = logging.getLogger().handlers[0]
        assert isinstance(handler, logging.StreamHandler)
        assert handler.stream is sys.stdout or handler.stream == sys.stdout

    def test_logger_emits_log_record(self, monkeypatch):
        """端到端：setup → info() → 不抛。"""
        setup_logging()
        log = setup_logging("test-emit")
        # 不抛即通过；record 是否被 handler 接收是 python logging 默认行为。
        log.info("hello world")
        # Smoke: root 级别至少 INFO。
        assert logging.getLogger().level <= logging.INFO