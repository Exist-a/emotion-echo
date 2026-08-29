"""
test_metrics_setup.py · emotion-llm-service Prometheus metrics 单元测试

Stage 26-T backlog §四 4.4: 覆盖 metrics_setup.py 的 Counter /


Histogram 暴露 + metrics_endpoint / MetricsMiddleware 行为。Coverage:

  - metrics_endpoint() returns prometheus exposition format
  - HTTP_REQUESTS_TOTAL counter accepts labels
  - HTTP_REQUEST_DURATION histogram observes values
  - ANALYZE_TOTAL counter accepts (emotion, status) labels
  - GRPC_REQUESTS_TOTAL counter accepts (method, status) labels
  - MetricsMiddleware: /metrics path is excluded from self-counting
  - MetricsMiddleware: counts non-/metrics requests with status
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from metrics_setup import (
    HTTP_REQUESTS_TOTAL,
    HTTP_REQUEST_DURATION,
    ANALYZE_TOTAL,
    GRPC_REQUESTS_TOTAL,
    metrics_endpoint,
    MetricsMiddleware,
)


# =====================================================
# Counter / Histogram exposure tests
# =====================================================

class TestCounters:
    """Prometheus 指标必须能被 inc / observe 操作（不会因模块重复 import 失败）。"""

    def test_http_requests_total_increments(self):
        before = HTTP_REQUESTS_TOTAL.labels("GET", "/analyze", "200")._value.get()
        HTTP_REQUESTS_TOTAL.labels("GET", "/analyze", "200").inc()
        after = HTTP_REQUESTS_TOTAL.labels("GET", "/analyze", "200")._value.get()
        assert after == before + 1

    def test_http_request_duration_observes(self):
        # observe() 不抛即 OK；histogram 的 _sum 累加
        before_sum = HTTP_REQUEST_DURATION.labels("POST", "/analyze")._sum.get()
        HTTP_REQUEST_DURATION.labels("POST", "/analyze").observe(0.123)
        after_sum = HTTP_REQUEST_DURATION.labels("POST", "/analyze")._sum.get()
        # Use absolute tolerance because prometheus_client stores the
        # _sum as a float with limited precision.
        assert after_sum == pytest.approx(before_sum + 0.123, abs=1e-6)

    def test_analyze_total_increments(self):
        before = ANALYZE_TOTAL.labels("happy", "ok")._value.get()
        ANALYZE_TOTAL.labels("happy", "ok").inc()
        after = ANALYZE_TOTAL.labels("happy", "ok")._value.get()
        assert after == before + 1

    def test_grpc_requests_total_increments(self):
        before = GRPC_REQUESTS_TOTAL.labels("Analyze", "ok")._value.get()
        GRPC_REQUESTS_TOTAL.labels("Analyze", "ok").inc()
        after = GRPC_REQUESTS_TOTAL.labels("Analyze", "ok")._value.get()
        assert after == before + 1


# =====================================================
# metrics_endpoint tests
# =====================================================

class TestMetricsEndpoint:
    """metrics_endpoint() 返回 Prometheus 文本格式。"""

    def test_returns_prometheus_content_type(self):
        # prometheus_client 0.26+ uses version=1.0.0; older was 0.0.4.
        # We assert the prefix + charset + content-type token family,
        # not the exact version string, to be future-proof.
        HTTP_REQUESTS_TOTAL.labels("GET", "/probe", "200").inc(0)
        resp = metrics_endpoint()
        assert resp.media_type.startswith("text/plain;")
        assert "version=" in resp.media_type
        assert "charset=utf-8" in resp.media_type

    def test_returns_text_containing_metric_names(self):
        # Increment to ensure the counter is registered.
        HTTP_REQUESTS_TOTAL.labels("GET", "/probe2", "200").inc()
        body = metrics_endpoint().body.decode("utf-8")
        # Must contain the counter's help line.
        assert "llm_http_requests_total" in body
        # Must contain the histogram.
        assert "llm_http_request_duration_seconds" in body


# =====================================================
# MetricsMiddleware tests (ASGI)
# =====================================================

class TestMetricsMiddleware:
    """ASGI MetricsMiddleware：捕获 HTTP requests + 排除 /metrics 自身。"""

    @pytest.mark.asyncio
    async def test_non_metrics_path_increments_counter(self):
        # First, capture baseline
        before = HTTP_REQUESTS_TOTAL.labels("GET", "/probe-mw", "200")._value.get()

        # Minimal ASGI app: returns 200.
        async def app(scope, receive, send):
            await send({"type": "http.response.start", "status": 200, "headers": []})
            await send({"type": "http.response.body", "body": b"ok"})

        wrapped = MetricsMiddleware(app)

        scope = {
            "type": "http",
            "method": "GET",
            "path": "/probe-mw",
            "headers": [],
            "raw_path": b"/probe-mw",
        }

        sent: list = []

        async def receive():
            return {"type": "http.request", "body": b"", "more_body": False}

        async def send(msg):
            sent.append(msg)

        await wrapped(scope, receive, send)
        after = HTTP_REQUESTS_TOTAL.labels("GET", "/probe-mw", "200")._value.get()
        assert after == before + 1

    @pytest.mark.asyncio
    async def test_metrics_path_excluded(self):
        # /metrics must NOT increment the counter (avoid self-loop).
        before = HTTP_REQUESTS_TOTAL.labels("GET", "/metrics", "200")._value.get()

        async def app(scope, receive, send):
            await send({"type": "http.response.start", "status": 200, "headers": []})
            await send({"type": "http.response.body", "body": b""})

        wrapped = MetricsMiddleware(app)
        scope = {
            "type": "http",
            "method": "GET",
            "path": "/metrics",
            "headers": [],
            "raw_path": b"/metrics",
        }

        async def receive():
            return {"type": "http.request", "body": b"", "more_body": False}

        async def send(msg):
            pass

        await wrapped(scope, receive, send)
        after = HTTP_REQUESTS_TOTAL.labels("GET", "/metrics", "200")._value.get()
        assert after == before, "metrics path must be excluded from counter"