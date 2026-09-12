"""Stage 87 RED · servicer 接线：ClassifyIntent / ChatCompletion with_intent 走 LLM 自适应分类

验收点（docs/plans/intent-classification-6-types.md「LLM 式分类增强」）：
  - LLM_API_KEY 存在 + 模糊消息 → RPC 返回 LLM 重分类标签
  - 无 LLM_API_KEY → 行为与 Stage 82 纯规则式完全一致（离线回归锁，§契约 6）
  - with_intent 流式首帧同样消费自适应分类结果
"""
from __future__ import annotations

import sys
from concurrent import futures
from pathlib import Path

import grpc
import pytest

SERVICE_DIR = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(SERVICE_DIR))

import emotion_llm_pb2
import emotion_llm_pb2_grpc
from grpc_server import (
    AuthInterceptor,
    EmotionLLMServiceServicer,
    LoggingInterceptor,
    RecoveryInterceptor,
    TracingInterceptor,
)

import intent_llm

AMBIGUOUS_TEXT = "帮我出个主意"  # 规则式零命中 → other / 0.0
LLM_LABEL = "emotional_support"


class _FakeMessage:
    def __init__(self, content):
        self.content = content


class _FakeChoice:
    def __init__(self, content):
        self.message = _FakeMessage(content)


class _FakeResponse:
    def __init__(self, content):
        self.choices = [_FakeChoice(content)]


class _FakeCompletions:
    def __init__(self, reply):
        self._reply = reply

    def create(self, **kwargs):
        return _FakeResponse(self._reply)


class _FakeClient:
    def __init__(self, reply):
        class _Chat:
            def __init__(self, completions):
                self.completions = completions
        self.chat = _Chat(_FakeCompletions(reply))


@pytest.fixture
def grpc_addr() -> str:
    """启动真实 gRPC server（dev 模式无 auth），返回 addr"""
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=2),
        interceptors=(
            LoggingInterceptor(),
            RecoveryInterceptor(),
            TracingInterceptor(),
            AuthInterceptor(expected_api_key=""),
        ),
    )
    emotion_llm_pb2_grpc.add_EmotionLLMServiceServicer_to_server(
        EmotionLLMServiceServicer(), server
    )
    port = server.add_insecure_port("127.0.0.1:0")
    server.start()
    try:
        yield f"127.0.0.1:{port}"
    finally:
        server.stop(grace=1)


@pytest.fixture
def llm_upstream(monkeypatch):
    """注入假 LLM 上游：LLM_API_KEY 存在 + client 工厂返回固定标签的 fake"""
    monkeypatch.setenv("LLM_API_KEY", "test-key-0123456789abcdef")
    monkeypatch.delenv("LLM_INTENT_RECLASSIFY", raising=False)
    monkeypatch.setattr(
        intent_llm, "_default_openai_client", lambda config: _FakeClient(LLM_LABEL)
    )


class TestClassifyIntentAdaptiveWiring:
    def test_ambiguous_text_reclassified_by_llm(self, grpc_addr, llm_upstream):
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.ClassifyIntent(
                emotion_llm_pb2.ClassifyIntentRequest(text=AMBIGUOUS_TEXT)
            )
        assert resp.intent == LLM_LABEL
        assert resp.confidence == pytest.approx(intent_llm.LLM_INTENT_CONFIDENCE)

    def test_no_api_key_keeps_rule_result(self, grpc_addr, monkeypatch):
        """无 key → 与 Stage 82 纯规则行为完全一致（离线回归锁）"""
        monkeypatch.delenv("LLM_API_KEY", raising=False)
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            resp = stub.ClassifyIntent(
                emotion_llm_pb2.ClassifyIntentRequest(text=AMBIGUOUS_TEXT)
            )
        assert resp.intent == "other"
        assert resp.confidence == 0.0

    def test_chat_completion_with_intent_uses_llm_label(self, grpc_addr, llm_upstream):
        """with_intent 流式：首帧 intent = LLM 重分类结果，仅首帧携带"""
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            chunks = list(stub.ChatCompletion(
                emotion_llm_pb2.ChatCompletionRequest(
                    messages=[
                        emotion_llm_pb2.ChatMessage(role="user", content=AMBIGUOUS_TEXT),
                    ],
                    with_intent=True,
                )
            ))
        assert chunks, "必须至少返回一帧"
        assert chunks[0].intent == LLM_LABEL
        assert all(c.intent == "" for c in chunks[1:])
        assert chunks[-1].done
