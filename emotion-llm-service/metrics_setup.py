"""
emotion-llm-service · Prometheus metrics（Stage 20-P0-2）

暴露 /metrics 端点供 Prometheus 抓取。当前指标：
  - llm_http_requests_total{method, path, status}      HTTP 请求计数
  - llm_http_request_duration_seconds{method, path}    HTTP 耗时直方图
  - llm_analyze_total{emotion, status}                  /analyze 调用统计
  - llm_grpc_requests_total{method, status}            gRPC 调用统计
"""
import os
import time

from fastapi import Request, Response
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Histogram, generate_latest

# ============ 业务指标 ============

HTTP_REQUESTS_TOTAL = Counter(
    "llm_http_requests_total",
    "Total number of HTTP requests processed, labeled by method, path, status.",
    ["method", "path", "status"],
)

HTTP_REQUEST_DURATION = Histogram(
    "llm_http_request_duration_seconds",
    "Histogram of HTTP request latency in seconds.",
    ["method", "path"],
    buckets=(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
)

ANALYZE_TOTAL = Counter(
    "llm_analyze_total",
    "Total number of /analyze invocations, labeled by emotion result and status.",
    ["emotion", "status"],
)

GRPC_REQUESTS_TOTAL = Counter(
    "llm_grpc_requests_total",
    "Total number of gRPC requests processed, labeled by method and status.",
    ["method", "status"],
)


def metrics_endpoint() -> Response:
    """/metrics HTTP handler（无 auth，无 metrics 自收集）"""
    return Response(content=generate_latest(), media_type=CONTENT_TYPE_LATEST)


class MetricsMiddleware:
    """ASGI 中间件：记录 HTTPRequestsTotal + HTTPRequestDuration。
    用类而不是装饰器，方便从 FastAPI app.middleware('http') 注入。
    """

    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        start = time.perf_counter()
        path = scope.get("path", "")
        method = scope.get("method", "")

        # /metrics 自身不计数（避免自循环）
        if path == "/metrics":
            await self.app(scope, receive, send)
            return

        # 包装 send 以捕获 status code
        status_holder = {"code": 500}

        async def wrapped_send(message):
            if message["type"] == "http.response.start":
                status_holder["code"] = message.get("status", 500)
            await send(message)

        await self.app(scope, receive, wrapped_send)

        HTTP_REQUESTS_TOTAL.labels(method, path, str(status_holder["code"])).inc()
        HTTP_REQUEST_DURATION.labels(method, path).observe(time.perf_counter() - start)