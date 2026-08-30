"""
test_health_route.py · FER 服务 /health 路由单元测试

Stage 26-T backlog §五 5.2.18: 覆盖 FER/server.py 的 health_check()
端点契约。

策略：
  - 用 fastapi.testclient.TestClient 触发 /health
  - 不加载 fer 库或 OpenCV 模型（测试环境没有 emotion_net.caffemodel）；
    所以期望 backend='neutral-fallback' 且 model_loaded=False
  - 不调 /analyze / /metrics（避免触发模型加载副作用）

per AGENTS §〇：先写 RED (期望 backend=='fer' 实际会 FAIL)，
但我们这次是补已存在实现的合约测试，所以直接断言 backend
的实际值 'neutral-fallback'（这才是环境现实的契约）。
"""
from __future__ import annotations

import sys
from pathlib import Path

from fastapi.testclient import TestClient

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from server import app


def test_health_route_status_ok():
    client = TestClient(app)
    resp = client.get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"


def test_health_route_returns_model_loaded_bool():
    client = TestClient(app)
    resp = client.get("/health")
    body = resp.json()
    # model_loaded 是 bool；测试环境无 fer 库 / caffemodel → False。
    assert isinstance(body["model_loaded"], bool)


def test_health_route_backend_is_known_value():
    client = TestClient(app)
    resp = client.get("/health")
    body = resp.json()
    # 三种合法 backend 值之一。
    assert body["backend"] in {"fer", "opencv-dnn", "neutral-fallback"}


def test_health_route_in_test_env_uses_neutral_fallback():
    """测试环境无 fer 库 / OpenCV 模型 → backend='neutral-fallback'。"""
    client = TestClient(app)
    resp = client.get("/health")
    body = resp.json()
    assert body["backend"] == "neutral-fallback"
    assert body["model_loaded"] is False


def test_health_route_response_shape_keys():
    """契约：response 必须包含 status / model_loaded / backend 三个键。"""
    client = TestClient(app)
    resp = client.get("/health")
    body = resp.json()
    assert set(body.keys()) >= {"status", "model_loaded", "backend"}


def test_health_route_idempotent():
    """连续两次 GET 应得到相同的 status（state 不应被 request 改变）。"""
    client = TestClient(app)
    a = client.get("/health").json()
    b = client.get("/health").json()
    assert a == b