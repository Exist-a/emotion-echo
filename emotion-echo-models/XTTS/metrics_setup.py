"""Prometheus metrics for FastAPI services."""
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
    "xtts_http_requests_total",
    "Total HTTP requests processed, labeled by method, path, status.",
    ["method", "path", "status"],
)

HTTP_REQUEST_DURATION = Histogram(
    "xtts_http_request_duration_seconds",
    "Histogram of HTTP request latency in seconds.",
    ["method", "path"],
    buckets=(0.1, 0.5, 1, 2.5, 5, 10, 30, 60, 120),
)

TTS_SYNTHESIS_TOTAL = Counter(
    "xtts_synthesis_total",
    "Total /tts invocations, labeled by language and status.",
    ["language", "status"],
)

TTS_STREAM_TOTAL = Counter(
    "xtts_stream_total",
    "Total /tts_stream invocations, labeled by language and status.",
    ["language", "status"],
)

TTS_PHONEMES_TOTAL = Counter(
    "xtts_phonemes_total",
    "Total /tts_with_phonemes invocations, labeled by language and status.",
    ["language", "status"],
)

TTS_INFERENCE_DURATION = Histogram(
    "xtts_synthesis_seconds",
    "Histogram of TTS model inference latency.",
    ["endpoint"],
    buckets=(0.5, 1, 2, 5, 10, 30, 60, 120),
)


class MetricsMiddleware(BaseHTTPMiddleware):
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
