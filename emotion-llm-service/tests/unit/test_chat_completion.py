"""Stage 80 RED · ChatCompletion 契约测试（llm-chat-real-pipeline PR-1）

docs/plans/llm-chat-real-pipeline.md §三 PR-1：
  - OpenAI 兼容流式转发（LLM_BASE_URL/LLM_API_KEY/LLM_MODEL，DeepSeek 等端点）
  - 无 key 或上游失败 → 内置 mock 文案降级（fallback_reason 非空），CI/离线可跑
  - 结束帧 done=True；model 首帧携带；mock 时 model="mock"

测试用假 client 注入，不打真实网络。
"""
import os
from unittest import mock as uti_mock

import pytest

import chat_completion
from chat_completion import (
    ChatUpstreamError,
    iter_chat_chunks,
    make_mock_chunks,
    resolve_backend_config,
)


class FakeOpenAIStream:
    """模拟 openai SDK 的 stream：迭代出带 choices[0].delta.content 的事件"""

    def __init__(self, deltas):
        self._deltas = deltas

    def __iter__(self):
        for d in self._deltas:
            ev = uti_mock.MagicMock()
            ev.choices = [uti_mock.MagicMock()]
            ev.choices[0].delta.content = d
            yield ev


class FakeCompletions:
    def __init__(self, deltas):
        self._deltas = deltas

    def create(self, **kwargs):
        self.kwargs = kwargs
        return FakeOpenAIStream(self._deltas)


class FakeClient:
    """模拟 openai.OpenAI(base_url=..., api_key=...) 客户端"""

    def __init__(self, deltas, error=None):
        self.chat = uti_mock.MagicMock()
        self._deltas = deltas
        self._error = error
        # 注意必须挂在 chat 上（实现访问 client.chat.completions）
        self.chat.completions = FakeCompletions(deltas)
        if error:
            self.chat.completions.create = uti_mock.MagicMock(side_effect=error)


class TestResolveBackendConfig:
    def test_no_api_key_returns_none(self, monkeypatch):
        monkeypatch.delenv("LLM_API_KEY", raising=False)
        assert resolve_backend_config() is None

    def test_key_env_resolved(self, monkeypatch):
        monkeypatch.setenv("LLM_API_KEY", "sk-test")
        monkeypatch.setenv("LLM_BASE_URL", "https://api.deepseek.com")
        monkeypatch.delenv("LLM_MODEL", raising=False)
        cfg = resolve_backend_config()
        assert cfg == {
            "api_key": "sk-test",
            "base_url": "https://api.deepseek.com",
            "model": chat_completion.DEFAULT_CHAT_MODEL,
        }


class TestMockFallback:
    def test_mock_chunks_shape(self):
        chunks = make_mock_chunks(["你好，我在。"])
        assert chunks[0].model == "mock"
        assert chunks[0].delta_content != ""
        assert chunks[-1].done is True
        assert all(c.fallback_reason for c in chunks)

    def test_no_api_key_yields_mock(self, monkeypatch):
        monkeypatch.delenv("LLM_API_KEY", raising=False)
        chunks = list(iter_chat_chunks([{"role": "user", "content": "hi"}]))
        assert chunks[-1].done is True
        assert all(c.fallback_reason for c in chunks), "无 key 必须整条降级 mock"
        assert chunks[0].model == "mock"


class TestUpstreamStreaming:
    def test_deltas_map_to_chunks_with_done_frame(self, monkeypatch):
        monkeypatch.setenv("LLM_API_KEY", "sk-test")
        fake = FakeClient(["你好", "，", "我在。"])
        chunks = list(
            iter_chat_chunks([{"role": "user", "content": "hi"}], client=fake)
        )
        deltas = [c.delta_content for c in chunks if c.delta_content]
        assert deltas == ["你好", "，", "我在。"]
        assert chunks[-1].done is True
        assert chunks[0].fallback_reason == "", "正常转发不应标记 fallback"
        # 首帧带 model
        assert chunks[0].model != ""

    def test_upstream_error_falls_back_to_mock(self, monkeypatch):
        monkeypatch.setenv("LLM_API_KEY", "sk-test")
        fake = FakeClient([], error=ChatUpstreamError("connect timeout"))
        chunks = list(
            iter_chat_chunks([{"role": "user", "content": "hi"}], client=fake)
        )
        assert chunks[-1].done is True
        assert all(c.fallback_reason for c in chunks), "上游失败必须整条降级 mock"
        assert "timeout" in chunks[0].fallback_reason or chunks[0].fallback_reason

    def test_messages_passed_to_upstream(self, monkeypatch):
        monkeypatch.setenv("LLM_API_KEY", "sk-test")
        msgs = [
            {"role": "system", "content": "你是情感陪伴助手"},
            {"role": "user", "content": "hi"},
        ]
        fake = FakeClient(["ok"])
        list(iter_chat_chunks(msgs, client=fake))
        assert fake.chat.completions.kwargs["messages"] == msgs
