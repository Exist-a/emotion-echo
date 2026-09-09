"""
test_analyze_route.py · FER-tflite 服务 /analyze 路由单元测试

PR-TTS-VENDOR Phase 2: tflite + Haar cascade 后端的图像分析契约。

策略：
  - 用 fastapi.testclient.TestClient（in-process）。
  - 用合成的灰度图测试（不依赖真实数据集）。
  - 断言 response 满足 emotion-echo FER 契约：{emotion, confidence, scores, source}。
"""
from __future__ import annotations

import io
import sys
from pathlib import Path

import numpy as np
import cv2
import pytest
from fastapi.testclient import TestClient

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from server import app


def _make_jpeg_bytes(width: int = 200, height: int = 200) -> bytes:
    """合成一张灰度矩形图的 JPEG bytes（无真实人脸 → Haar 检测不到 → 走 no-face 路径）。"""
    img = np.full((height, width, 3), 50, dtype=np.uint8)
    cv2.rectangle(img, (60, 60), (140, 140), (220, 220, 220), -1)
    ok, buf = cv2.imencode(".jpg", img)
    assert ok
    return buf.tobytes()


def test_analyze_route_returns_200_on_valid_image():
    client = TestClient(app)
    files = {"file": ("test.jpg", _make_jpeg_bytes(), "image/jpeg")}
    resp = client.post("/analyze", files=files)
    assert resp.status_code == 200


def test_analyze_route_response_schema_keys():
    """契约：response 必须包含 emotion / confidence / scores / source 四键。"""
    client = TestClient(app)
    files = {"file": ("test.jpg", _make_jpeg_bytes(), "image/jpeg")}
    body = client.post("/analyze", files=files).json()
    assert set(body.keys()) >= {"emotion", "confidence", "scores", "source"}


def test_analyze_route_emotion_in_unified_set():
    """emotion 必须是项目 5 类统一情感之一。"""
    client = TestClient(app)
    files = {"file": ("test.jpg", _make_jpeg_bytes(), "image/jpeg")}
    body = client.post("/analyze", files=files).json()
    assert body["emotion"] in {"angry", "anxious", "happy", "sad", "neutral"}


def test_analyze_route_confidence_is_float_in_unit_interval():
    client = TestClient(app)
    files = {"file": ("test.jpg", _make_jpeg_bytes(), "image/jpeg")}
    body = client.post("/analyze", files=files).json()
    conf = body["confidence"]
    assert isinstance(conf, (int, float))
    assert 0.0 <= conf <= 1.0


def test_analyze_route_source_is_known_value():
    """source 必须是已知后端之一（tflite 后端或无脸/无模型 fallback）。"""
    client = TestClient(app)
    files = {"file": ("test.jpg", _make_jpeg_bytes(), "image/jpeg")}
    body = client.post("/analyze", files=files).json()
    assert body["source"] in {"tflite", "no-face", "no-model"}


def test_analyze_route_scores_is_dict():
    """scores 是 dict 形态（即使 no-face 时是空 dict）。"""
    client = TestClient(app)
    files = {"file": ("test.jpg", _make_jpeg_bytes(), "image/jpeg")}
    body = client.post("/analyze", files=files).json()
    assert isinstance(body["scores"], dict)


def test_analyze_route_rejects_empty_file():
    client = TestClient(app)
    files = {"file": ("empty.jpg", b"", "image/jpeg")}
    resp = client.post("/analyze", files=files)
    assert resp.status_code in (400, 422)  # 400 = our explicit, 422 = FastAPI default


def test_analyze_route_rejects_invalid_image():
    """非图片 bytes 必须返回 400（'invalid image'）。"""
    client = TestClient(app)
    files = {"file": ("fake.jpg", b"not an image at all", "image/jpeg")}
    resp = client.post("/analyze", files=files)
    assert resp.status_code == 400