"""
test_logging_schema_parity.py · Python 与 Go 日志字段一致性（E2E-F-148）

问题：Go 侧 shared/pkg/logging 输出 time / level / msg / svc / trace_id，
而 llm-service 的 JsonFormatter 输出 ts / level / logger / msg。
**level 与 msg 同名、time 与 logger 字段名不同** ⇒ 同一条 LogQL 无法同时按
时间字段与来源过滤，跨服务查询要写两套。

本文件钉住"Python 侧必须同时给出 Go 同名字段"，让一条 LogQL 通吃。
"""
from __future__ import annotations

import json
import logging
import sys
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from logging_setup import JsonFormatter, setup_logging  # noqa: E402


def _make_record(msg: str = "hello", level: int = logging.INFO,
                 extra: dict | None = None) -> logging.LogRecord:
    rec = logging.LogRecord(name="grpc_server", level=level, pathname=__file__,
                            lineno=1, msg=msg, args=(), exc_info=None)
    for k, v in (extra or {}).items():
        setattr(rec, k, v)
    return rec


class TestGoSchemaParity:
    def test_has_go_parity_time_field(self):
        """Go 用 time（不是 ts）—— 时间字段名必须一致，否则一条 LogQL 写不了两套。"""
        obj = json.loads(JsonFormatter().format(_make_record()))
        assert "time" in obj, "缺少与 Go 对齐的 time 字段"
        assert obj["time"].endswith("Z")

    def test_has_go_parity_svc_field(self):
        """Go 每行都带 svc；Python 必须能给出同样的字段。"""
        obj = json.loads(JsonFormatter().format(_make_record(extra={"svc": "llm-service"})))
        assert obj["svc"] == "llm-service"

    def test_svc_defaults_when_not_passed(self):
        """没显式传 svc 时按服务名兜底，而不是缺字段。"""
        obj = json.loads(JsonFormatter().format(_make_record()))
        assert obj.get("svc"), "svc 缺省值必须存在（Go 侧每行都有 svc）"

    def test_keeps_legacy_fields_for_backcompat(self):
        """旧字段 ts/logger 保留：仓内可能有解析脚本/看板在用。"""
        obj = json.loads(JsonFormatter().format(_make_record()))
        assert "ts" in obj and obj["ts"].endswith("Z")
        assert obj["logger"] == "grpc_server"

    def test_trace_id_passthrough(self):
        """trace_id 作为 extra 传入时必须原样出现在 JSON 顶层。"""
        obj = json.loads(JsonFormatter().format(
            _make_record(extra={"trace_id": "abc123"})))
        assert obj["trace_id"] == "abc123"
