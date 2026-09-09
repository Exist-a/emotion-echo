"""
test_logging_setup.py · FER-tflite logging_setup 单元测试

PR-TTS-VENDOR Phase 2: 验证 JSON logger 模块契约（与原 FER 共享 setup_logging()）。
"""
from __future__ import annotations

import logging
import sys
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from logging_setup import setup_logging


@pytest.fixture(autouse=True)
def restore_root_logger():
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
    def test_returns_logger_with_requested_name(self):
        log = setup_logging("fer.test")
        assert isinstance(log, logging.Logger)
        assert log.name == "fer.test"

    def test_replaces_root_handlers(self):
        """重复调用 setup_logging() 不会累积 handler。"""
        setup_logging("a")
        first_count = len(logging.getLogger().handlers)
        setup_logging("b")
        second_count = len(logging.getLogger().handlers)
        assert first_count == second_count == 1

    def test_sets_root_level_from_env(self, monkeypatch):
        monkeypatch.setenv("LOG_LEVEL", "DEBUG")
        setup_logging("c")
        assert logging.getLogger().level == logging.DEBUG

    def test_default_level_is_info(self, monkeypatch):
        monkeypatch.delenv("LOG_LEVEL", raising=False)
        setup_logging("d")
        assert logging.getLogger().level == logging.INFO