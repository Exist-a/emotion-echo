"""Prometheus metrics for FastAPI services (Stage 20-P0-2 pattern)."""
import time
from typing import Awaitable, Callable

from prometheus_client import (
    CONTENT_TYPE_LATEST,
    Counter,
    Histogram,
    generate_latest,
)
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import Response

HTTP_REQUESTS_TOTAL = Counter(
    "fer_http_requests_total",
    "Total HTTP requests processed, labeled by method, path, status.",
    ["method", "path", "status"],
)

HTTP_REQUEST_DURATION = Histogram(
    "fer_http_request_duration_seconds",
    "Histogram of HTTP request latency in seconds.",
    ["method", "path"],
    buckets=(0.005, 0.025, 0.1, 0.25, 1, 2.5, 5, 10, 30),
)

ANALYZE_TOTAL = Counter(
    "fer_analyze_total",
    "Total /analyze invocations, labeled by emotion result and status.",
    ["emotion", "status"],
)

MODEL_INFERENCE_DURATION = Histogram(
    "fer_model_inference_seconds",
    "Histogram of model inference latency (face detection + classification).",
    buckets=(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10),
)


class MetricsMiddleware(BaseHTTPMiddleware):
    """Records every HTTP request in Prometheus metrics.

    Skips /metrics itself to avoid cardinality loops.
    """

    async def dispatch(
        self,
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        if request.url.path == "/metrics":
            return await call_next(request)

        start = time.perf_counter()
        status_code = 500
        try:
            response = await call_next(request)
            status_code = response.status_code
            return response
        finally:
            elapsed = time.perf_counter() - start
            HTTP_REQUEST_DURATION.labels(
                method=request.method, path=request.url.path
            ).observe(elapsed)
            HTTP_REQUESTS_TOTAL.labels(
                method=request.method,
                path=request.url.path,
                status=str(status_code),
            ).inc()


async def metrics_endpoint() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)
