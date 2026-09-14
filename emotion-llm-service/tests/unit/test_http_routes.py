"""
test_http_routes.py · emotion-llm-service FastAPI HTTP 路由测试

Stage 26-T backlog §四 4.4: 覆盖 emotion-llm-service/main.py 的
GET /health、GET /metrics、POST /analyze 三个 HTTP 路由 + 边界
（空 text / None / 4096 chars / emoji / 非法 JSON）。

策略：
  - 用 fastapi.testclient.TestClient（同步）驱动真实 FastAPI app。
  - 不启外部服务：TestClient 是 ASGI in-process 测试驱动。
  - 对 /analyze 边界使用 parametrize 表驱动覆盖。

Per AGENTS.md §三.3：HTTP 测试是 in-process，不算真实副作用。
"""
from __future__ import annotations

import sys
from pathlib import Path

import pytest

# 把 emotion-llm-service 加入 PYTHONPATH
SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

from fastapi.testclient import TestClient

import main
from main import app


# =====================================================
# Fixtures
# =====================================================

@pytest.fixture
def client(monkeypatch):
    """FastAPI TestClient (with context-managed lifespan).

    Stage 31 PR-10 给 app 挂了 lifespan，NACOS_ENABLED 默认 true，进入
    lifespan 就会 wait_for_nacos("emotion-echo-nacos:8848", max_wait=60)。
    该主机名只在 compose 网络里解析得到，跑单测时每个用例白等 60s。
    lifespan 在调用时才读模块级 NACOS_ENABLED，故这里关掉它，
    让本文件回到 docstring 承诺的"不启外部服务"。
    """
    monkeypatch.setattr(main, "NACOS_ENABLED", False)
    with TestClient(app) as c:
        yield c


# =====================================================
# /health tests
# =====================================================

class TestHttpApiKey:
    """P0-R2-3: HTTP /analyze Internal-API-Key 鉴权契约。

    prod 模式（INTERNAL_API_KEY 配置且非空）必须强制鉴权：
      - 无 header → 401
      - 错 key → 401
      - 对 key → 200
    dev 模式（INTERNAL_API_KEY 未配置）必须保持开放，便于本地调试。
    """

    @pytest.fixture
    def prod_client(self, monkeypatch):
        """prod 模式：INTERNAL_API_KEY=secret32chars 模拟强制鉴权。"""
        monkeypatch.setattr(main, "NACOS_ENABLED", False)
        # module 启动时已读 os.environ；INTERNAL_API_KEY 走 _HTTP_API_KEY module 变量
        monkeypatch.setattr(main, "_HTTP_API_KEY", "secret32chars_min_prod_key")
        with TestClient(app) as c:
            yield c

    def test_analyze_no_header_returns_401(self, prod_client):
        r = prod_client.post("/analyze", json={"text": "hi"})
        assert r.status_code == 401, (
            f"missing Internal-API-Key must be 401, got {r.status_code} body={r.text}"
        )

    def test_analyze_wrong_key_returns_401(self, prod_client):
        r = prod_client.post(
            "/analyze",
            json={"text": "hi"},
            headers={"Internal-API-Key": "wrong-key"},
        )
        assert r.status_code == 401, (
            f"wrong Internal-API-Key must be 401, got {r.status_code} body={r.text}"
        )

    def test_analyze_correct_key_returns_200(self, prod_client):
        r = prod_client.post(
            "/analyze",
            json={"text": "我今天很开心"},
            headers={"Internal-API-Key": "secret32chars_min_prod_key"},
        )
        assert r.status_code == 200, (
            f"correct Internal-API-Key must be 200, got {r.status_code} body={r.text}"
        )
        body = r.json()
        assert body["primaryEmotion"] in {
            "happy", "sad", "angry", "anxious", "calm", "neutral"
        }

    def test_dev_mode_no_key_allows_request(self, client):
        """dev 模式（_HTTP_API_KEY=""）所有请求不鉴权，沿用历史 happy path。"""
        r = client.post("/analyze", json={"text": "hi"})
        assert r.status_code == 200


class TestHealthRoute:
    """GET /health returns {status, service, version}."""

    def test_returns_200(self, client):
        r = client.get("/health")
        assert r.status_code == 200

    def test_returns_expected_schema(self, client):
        body = client.get("/health").json()
        assert set(body.keys()) >= {"status", "service", "version"}
        assert body["status"] == "ok"
        assert body["service"] == "emotion-llm"
        assert body["version"] == "0.1.0"

    def test_method_not_allowed(self, client):
        """POST /health 不是定义路由 → 405。"""
        r = client.post("/health")
        assert r.status_code == 405


# =====================================================
# /metrics tests
# =====================================================

class TestMetricsRoute:
    """GET /metrics returns Prometheus exposition format."""

    def test_returns_200(self, client):
        r = client.get("/metrics")
        assert r.status_code == 200

    def test_returns_text_plain_content_type(self, client):
        r = client.get("/metrics")
        # Prometheus exposition format is text/plain (with version + charset).
        assert r.headers["content-type"].startswith("text/plain;")
        assert "version=" in r.headers["content-type"]

    def test_body_contains_prom_metric_names(self, client):
        body = client.get("/metrics").text
        # Must include the registered counters (names from metrics_setup.py).
        assert "llm_analyze_total" in body
        assert "llm_http_requests_total" in body
        assert "llm_grpc_requests_total" in body


# =====================================================
# /analyze tests
# =====================================================

class TestAnalyzeRoute:
    """POST /analyze: JSON {text} → AnalyzeResponse."""

    def test_happy_path_returns_expected_fields(self, client):
        r = client.post("/analyze", json={"text": "我今天很开心"})
        assert r.status_code == 200
        body = r.json()
        # AnalyzeResponse schema: primaryEmotion, sentimentScore, confidence, model
        for k in ("primaryEmotion", "sentimentScore", "confidence", "model"):
            assert k in body
        assert body["primaryEmotion"] in {
            "happy", "sad", "angry", "anxious", "calm", "neutral"
        }
        assert body["model"] == "keyword-v1"

    def test_empty_text_returns_neutral(self, client):
        body = client.post("/analyze", json={"text": ""}).json()
        assert body["primaryEmotion"] == "neutral"
        assert body["confidence"] == 0.0

    def test_whitespace_only_text_returns_neutral(self, client):
        body = client.post("/analyze", json={"text": "   \n\t  "}).json()
        assert body["primaryEmotion"] == "neutral"

    def test_missing_text_field_returns_422(self, client):
        """FastAPI 对 missing required field 返回 422 Unprocessable Entity。"""
        r = client.post("/analyze", json={})
        assert r.status_code == 422

    def test_non_string_text_returns_422(self, client):
        """FastAPI 自动校验类型：text 必须是 string。"""
        r = client.post("/analyze", json={"text": 12345})
        assert r.status_code == 422

    def test_malformed_json_returns_422(self, client):
        """畸形 JSON → 422。"""
        r = client.post(
            "/analyze",
            content=b"{not json",
            headers={"content-type": "application/json"},
        )
        assert r.status_code == 422

    def test_wrong_method_get_returns_405(self, client):
        """/analyze 是 POST-only。"""
        r = client.get("/analyze")
        assert r.status_code == 405

    @pytest.mark.parametrize("text,expected_emotion", [
        ("今天很开心", "happy"),
        ("我很难过", "sad"),
        ("太生气了", "angry"),
        ("很焦虑", "anxious"),
        ("很放松", "calm"),
    ])
    def test_emotion_classification_table_driven(self, client, text, expected_emotion):
        body = client.post("/analyze", json={"text": text}).json()
        assert body["primaryEmotion"] == expected_emotion, (
            f"text={text!r} got={body['primaryEmotion']}"
        )

    def test_emoji_text_does_not_crash(self, client):
        """纯 emoji 输入不导致 handler crash，至少返回合法 emotion 字段。"""
        body = client.post("/analyze", json={"text": "🎉🚀💖✨"}).json()
        assert body["primaryEmotion"] in {
            "happy", "sad", "angry", "anxious", "calm", "neutral"
        }

    def test_long_text_handled(self, client):
        """4096 字符输入：handler 不 panic。"""
        text = "我今天很开心。" * 600
        text = (text + "。" * 100)[:4096]
        assert len(text) == 4096
        body = client.post("/analyze", json={"text": text}).json()
        # primary emotion 至少是合法集合值
        assert body["primaryEmotion"] in {
            "happy", "sad", "angry", "anxious", "calm", "neutral"
        }