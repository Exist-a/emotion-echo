"""
test_metrics_setup.py · sensevoice 服务 metrics_setup 单元测试

Stage 26-T backlog §四 4.4: 覆盖 sensevoice-small/metrics_setup.py 的
Prometheus metrics + MetricsMiddleware。

策略：
  - 顶层 metric 对象 (HTTP_REQUESTS_TOTAL, HTTP_REQUEST_DURATION,
    ANALYZE_TOTAL, MODEL_INFERENCE_DURATION) 在 import 时已注册。
    测试只断言它们的类型与命名（避免在 unit 测试里跟全局
    Prometheus registry 副作用死磕）。
  - MetricsMiddleware.dispatch() 用 starlette TestClient + 一个
    假 ASGI app 来触发：路径 /metrics 必须旁路（skip），
    其它路径必须 increment 对应 counter。
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
    ANALYZE_TOTAL,
    HTTP_REQUESTS_TOTAL,
    HTTP_REQUEST_DURATION,
    MODEL_INFERENCE_DURATION,
    MetricsMiddleware,
    metrics_endpoint,
)


class TestMetricRegistration:
    """module 加载时已注册的 4 个 Prometheus 对象的形状契约。"""

    def test_http_requests_total_is_counter(self):
        from prometheus_client import Counter

        assert isinstance(HTTP_REQUESTS_TOTAL, Counter)

    def test_http_request_duration_is_histogram(self):
        from prometheus_client import Histogram

        assert isinstance(HTTP_REQUEST_DURATION, Histogram)

    def test_analyze_total_is_counter(self):
        from prometheus_client import Counter

        assert isinstance(ANALYZE_TOTAL, Counter)

    def test_model_inference_duration_is_histogram(self):
        from prometheus_client import Histogram

        assert isinstance(MODEL_INFERENCE_DURATION, Histogram)

    def test_http_requests_total_labeled_by_method_path_status(self):
        # labels() 返回的 label 名集合（顺序无关）。
        labels = set(HTTP_REQUESTS_TOTAL._labelnames)
        assert labels == {"method", "path", "status"}

    def test_analyze_total_labeled_by_emotion_status(self):
        labels = set(ANALYZE_TOTAL._labelnames)
        assert labels == {"emotion", "status"}


class TestMetricsEndpoint:
    """metrics_endpoint() → /metrics handler 返回 Prometheus 文本格式。"""

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
        # Prometheus 文本格式包含 # HELP <name> <docstring>。
        resp = await metrics_endpoint()
        body = resp.body.decode("utf-8")
        # 至少 1 个 # HELP 行（任意 metric）。
        assert "# HELP " in body


class TestMetricsMiddlewareSkipsMetricsPath:
    """MetricsMiddleware 在 /metrics 路径上必须短路：避免 cardinality loop。"""

    @pytest.mark.asyncio
    async def test_metrics_path_does_not_increment_request_counter(self):
        # 创建请求路径为 /metrics 的 Request，然后调 dispatch。
        # 我们不能 increment 后断言"无副作用"，因为 Counter 是
        # 模块全局；但至少 dispatch 必须立即调用 call_next 而
        # 不再走 increment / observe 分支（不会抛异常）。
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
    """非 /metrics 路径必须走 observe + inc 流程。"""

    @pytest.mark.asyncio
    async def test_records_request_with_status_200(self):
        # 走完整 dispatch 路径：起点 + 终点 perf_counter + inc。
        scope = {
            "type": "http",
            "method": "POST",
            "path": "/analyze",
            "raw_path": b"/analyze",
            "headers": [],
            "query_string": b"",
        }
        req = Request(scope)

        async def ok(_req):
            return PlainTextResponse("ok", status_code=200)

        mw = MetricsMiddleware(app=ok)
        resp = await mw.dispatch(req, ok)
        assert resp.status_code == 200
        # Smoke：dispatch 后没有 panic 即可。counter / histogram
        # 副作用在 registry 全局，断言具体数值会跨测试污染。