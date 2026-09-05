"""
test_health_route.py · SenseVoice 服务 /health 路由测试

Stage 26-T backlog §四 4.4: 覆盖 sensevoice-small/server.py 的
GET /health 路由 + JSON schema 契约。

策略：
  - 用 fastapi.testclient.TestClient（in-process，不加载 funasr 模型）。
  - 测试 /analyze 时必须 mock funasr 模型（避免 936MB 权重 + 60s
    冷启动）——本测试仅覆盖 /health，不触 /analyze。
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from fastapi.testclient import TestClient

import server


@pytest.fixture
def client():
    with TestClient(server.app) as c:
        yield c


class TestHealthRoute:
    """GET /health returns JSON {status, service, device, model_loaded}."""

    def test_returns_200(self, client):
        r = client.get("/health")
        assert r.status_code == 200

    def test_returns_json_content_type(self, client):
        r = client.get("/health")
        assert r.headers["content-type"].startswith("application/json")

    def test_schema_keys(self, client):
        """/health 必须包含 status / service / device / model_loaded 4 个字段。"""
        body = client.get("/health").json()
        assert set(body.keys()) == {"status", "service", "device", "model_loaded"}

    def test_service_is_sensevoice(self, client):
        body = client.get("/health").json()
        assert body["service"] == "sensevoice"

    def test_device_is_cpu_or_cuda(self, client):
        body = client.get("/health").json()
        assert body["device"] in {"cpu", "cuda", "cuda:0"}

    def test_status_is_ok_or_loading(self, client):
        """模型加载状态决定 status：未加载 = 'loading'，已加载 = 'ok'。"""
        body = client.get("/health").json()
        assert body["status"] in {"ok", "loading"}

    def test_model_loaded_is_bool(self, client):
        body = client.get("/health").json()
        assert isinstance(body["model_loaded"], bool)

    def test_status_matches_model_loaded(self, client):
        """status 与 model_loaded 字段必须一致：loaded→ok，未 loaded→loading。"""
        body = client.get("/health").json()
        if body["model_loaded"]:
            assert body["status"] == "ok"
        else:
            assert body["status"] == "loading"

    def test_method_not_allowed(self, client):
        """/health 是 GET-only。"""
        r = client.post("/health")
        assert r.status_code == 405