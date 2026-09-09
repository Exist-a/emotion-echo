"""
test_metrics_setup.py · FER-tflite metrics_setup 单元测试

PR-TTS-VENDOR Phase 2: 验证 Prometheus 4 个 metric + MetricsMiddleware 行为。
"""
from __future__ import annotations

import sys
from pathlib import Path

from starlette.applications import Starlette
from starlette.middleware import Middleware
from starlette.requests import Request
from starlette.responses import PlainTextResponse
from starlette.testclient import TestClient

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


class TestMetricsMiddleware:
    """Middleware 在 /metrics 路径必须旁路，其它路径必须 increment counter。"""

    def _make_app(self):
        async def homepage(request: Request):
            return PlainTextResponse("ok")

        from starlette.routing import Route
        return Starlette(
            middleware=[Middleware(MetricsMiddleware)],
            routes=[Route("/", homepage, methods=["GET"])],
        )

    def test_non_metrics_path_increments_counter(self):
        client = TestClient(self._make_app())
        client.get("/")
        # Counter 已被调用，验证内部样本数 > 0
        for metric in HTTP_REQUESTS_TOTAL.collect():
            for sample in metric.samples:
                if sample.name.endswith("_total"):
                    assert sample.value >= 1.0
                    return
        pytest.fail("HTTP_REQUESTS_TOTAL did not record any sample")

    def test_metrics_endpoint_returns_prometheus_format(self):
        from prometheus_client import CONTENT_TYPE_LATEST
        import asyncio

        result = asyncio.run(metrics_endpoint())
        assert result.media_type == CONTENT_TYPE_LATEST
        assert b"fer_http_requests_total" in result.body