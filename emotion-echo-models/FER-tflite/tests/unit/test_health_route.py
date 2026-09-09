"""
test_health_route.py · FER-tflite 服务 /health 路由单元测试

PR-TTS-VENDOR Phase 2: tflite + Haar cascade 后端的健康检查契约。

backend 枚举：原 FER 是 {fer, opencv-dnn, neutral-fallback}，
tflite 路径新增 'tflite+haar'。当 .tflite 模型 + Haar cascade 都加载成功时，
backend == 'tflite+haar' 且 model_loaded=True。
"""
from __future__ import annotations

import sys
from pathlib import Path

from fastapi.testclient import TestClient

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from server import app


def test_health_route_status_ok():
    resp = TestClient(app).get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"


def test_health_route_returns_model_loaded_bool():
    body = TestClient(app).get("/health").json()
    assert isinstance(body["model_loaded"], bool)


def test_health_route_backend_is_known_value():
    """四种合法 backend 值之一。"""
    body = TestClient(app).get("/health").json()
    assert body["backend"] in {"tflite+haar", "neutral-fallback", "fer", "opencv-dnn"}


def test_health_route_in_test_env_uses_tflite_backend():
    """测试环境：emotion_model_quantized.tflite 已在仓里，Haar cascade 已预烘焙。
    因此 backend 应为 'tflite+haar' 且 model_loaded=True。"""
    body = TestClient(app).get("/health").json()
    assert body["backend"] == "tflite+haar"
    assert body["model_loaded"] is True


def test_health_route_response_shape_keys():
    """契约：response 必须包含 status / model_loaded / backend 三个键。"""
    body = TestClient(app).get("/health").json()
    assert set(body.keys()) >= {"status", "model_loaded", "backend"}


def test_health_route_idempotent():
    """连续两次 GET 应得到相同的 status（state 不应被 request 改变）。"""
    client = TestClient(app)
    a = client.get("/health").json()
    b = client.get("/health").json()
    assert a == b