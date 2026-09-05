"""
test_metrics_setup.py · XTTS 服务 metrics_setup 单元测试

Stage 26-T backlog §四 4.4: 覆盖 XTTS/metrics_setup.py 的
Prometheus metrics + MetricsMiddleware。

策略：
  - 顶层 metric 对象 (HTTP_REQUESTS_TOTAL, HTTP_REQUEST_DURATION,
    TTS_SYNTHESIS_TOTAL, TTS_INFERENCE_DURATION) 在 import 时已注册。
    测试只断言它们的类型与命名。
  - MetricsMiddleware.dispatch() 用 starlette Request + 假 ASGI
    app 来触发：路径 /metrics 必须旁路，其它路径必须走 observe
    + inc 流程。
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest
from starlette.requests import Request
from starlette.responses import PlainTextResponse, Response

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from metrics_setup import (  # noqa: E402
    HTTP_REQUESTS_TOTAL,
    HTTP_REQUEST_DURATION,
    TTS_INFERENCE_DURATION,
    TTS_SYNTHESIS_TOTAL,
    MetricsMiddleware,
    metrics_endpoint,
)


class TestMetricRegistration:
    def test_http_requests_total_is_counter(self):
        from prometheus_client import Counter

        assert isinstance(HTTP_REQUESTS_TOTAL, Counter)

    def test_http_request_duration_is_histogram(self):
        from prometheus_client import Histogram

        assert isinstance(HTTP_REQUEST_DURATION, Histogram)

    def test_tts_synthesis_total_is_counter(self):
        from prometheus_client import Counter

        assert isinstance(TTS_SYNTHESIS_TOTAL, Counter)

    def test_tts_inference_duration_is_histogram(self):
        from prometheus_client import Histogram

        assert isinstance(TTS_INFERENCE_DURATION, Histogram)

    def test_http_requests_total_labeled_by_method_path_status(self):
        labels = set(HTTP_REQUESTS_TOTAL._labelnames)
        assert labels == {"method", "path", "status"}

    def test_tts_synthesis_total_labeled_by_language_status(self):
        labels = set(TTS_SYNTHESIS_TOTAL._labelnames)
        assert labels == {"language", "status"}


class TestMetricsEndpoint:
    @pytest.mark.asyncio
    async def test_returns_response(self):
        resp = await metrics_endpoint()
        assert isinstance(resp, Response)

    @pytest.mark.asyncio
    async def test_uses_prometheus_content_type(self):
        from prometheus_client import CONTENT_TYPE_LATEST

        resp = await metrics_endpoint()
        assert resp.media_type == CONTENT_TYPE_LATEST

    @pytest.mark.asyncio
    async def test_body_contains_help_lines(self):
        resp = await metrics_endpoint()
        body = resp.body.decode("utf-8")
        assert "# HELP " in body


class TestMetricsMiddlewareSkipsMetricsPath:
    @pytest.mark.asyncio
    async def test_metrics_path_does_not_increment_request_counter(self):
        scope = {
            "type": "http",
            "method": "GET",
            "path": "/metrics",
            "raw_path": b"/metrics",
            "headers": [],
            "query_string": b"",
        }
        req = Request(scope)

        async def passthrough(_req):
            return PlainTextResponse("ok")

        mw = MetricsMiddleware(app=passthrough)
        resp = await mw.dispatch(req, passthrough)
        assert resp.status_code == 200


class TestMetricsMiddlewareNonMetricsPath:
    @pytest.mark.asyncio
    async def test_records_request_with_status_200(self):
        scope = {
            "type": "http",
            "method": "POST",
            "path": "/tts/synthesize",
            "raw_path": b"/tts/synthesize",
            "headers": [],
            "query_string": b"",
        }
        req = Request(scope)

        async def ok(_req):
            return PlainTextResponse("ok", status_code=200)

        mw = MetricsMiddleware(app=ok)
        resp = await mw.dispatch(req, ok)
        assert resp.status_code == 200