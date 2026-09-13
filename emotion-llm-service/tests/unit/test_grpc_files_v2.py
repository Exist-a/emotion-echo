"""Stage 90 PR-1 RED：file_context 注入位置从 system 末尾改到 user 消息尾部。

Stage 89 residual：DeepSeek 对 system 末尾追加的 file_context 多次拒读（尤其 PDF）。
推测：注入到 user 消息尾部更接近"用户粘贴文本提问"的形态，模型行为更稳。
同时明确 prompt 文案让 LLM 不再返"我没看到附件"。

注入规则：
- 有 user 消息：最后一条 user.content += "\n\n" + file_context_text（不改 system）
- 无 user 消息（罕见，例如纯 system 摘要场景）：fallback 旧行为——system 末尾追加
"""

import threading
from http.server import BaseHTTPRequestHandler, HTTPServer
from concurrent.futures import ThreadPoolExecutor

import grpc
import pytest

import emotion_llm_pb2
import emotion_llm_pb2_grpc
from grpc_server import EmotionLLMServiceServicer
import grpc_server


@pytest.fixture
def http_server():
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = "PDF-SENTINEL-pypdf-OK-2026".encode()
            self.send_response(200)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *a):
            pass

    srv = HTTPServer(("127.0.0.1", 0), Handler)
    port = srv.server_address[1]
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    yield f"127.0.0.1:{port}", port
    srv.shutdown()


@pytest.fixture
def grpc_addr():
    server = grpc.server(ThreadPoolExecutor(max_workers=2))
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
def capture_upstream(monkeypatch):
    captured = {}

    def fake_iter(messages, **kwargs):
        captured["messages"] = messages
        yield emotion_llm_pb2.ChatChunk(delta_content="好", model="deepseek-chat", fallback_reason="")
        yield emotion_llm_pb2.ChatChunk(done=True, model="deepseek-chat", fallback_reason="")

    monkeypatch.setattr(grpc_server, "iter_chat_chunks", fake_iter)
    return captured


class TestChatCompletionFileContextInjectionSite:
    def test_file_context_injected_into_last_user_message(
        self, grpc_addr, http_server, capture_upstream, monkeypatch
    ):
        """file_context 必须追加到最后一条 user 消息尾部（Stage 90）"""
        host, port = http_server
        monkeypatch.setenv("FILE_FETCH_ALLOWLIST", host)
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            list(stub.ChatCompletion(
                emotion_llm_pb2.ChatCompletionRequest(
                    messages=[
                        emotion_llm_pb2.ChatMessage(role="system", content="你是共情陪伴者"),
                        emotion_llm_pb2.ChatMessage(role="user", content="请引用附件里那句话"),
                    ],
                    files=[emotion_llm_pb2.FileAttachment(
                        url=f"http://{host}/uploads/x.pdf", name="report.pdf",
                    )],
                )
            ))
        msgs = capture_upstream["messages"]
        # 必须是 system + user 两条，system 不变；file_context 进 user 尾部
        assert len(msgs) == 2
        assert msgs[0] == {"role": "system", "content": "你是共情陪伴者"}, \
            "system 消息不应被 file_context 改动"
        # user 消息必须含附件上下文（含哨兵）
        assert "PDF-SENTINEL-pypdf-OK-2026" in msgs[1]["content"]
        assert "report.pdf" in msgs[1]["content"]
        assert "请引用附件里那句话" in msgs[1]["content"], "原 user 文本必须保留"
        # 明确提示词：让 LLM 知道附件已抽到文本
        assert "以下内容来自用户上传的附件" in msgs[1]["content"]

    def test_no_user_message_fallback_to_system(
        self, grpc_addr, capture_upstream
    ):
        """无 user 消息（极少见）→ 仍按 system 末尾追加（fallback 行为不变）"""
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            # 文件 url 不会去白名单，build_file_context_text 会产'未能读取'注记
            list(stub.ChatCompletion(
                emotion_llm_pb2.ChatCompletionRequest(
                    messages=[
                        emotion_llm_pb2.ChatMessage(role="system", content="你是摘要器"),
                    ],
                    files=[emotion_llm_pb2.FileAttachment(
                        url="http://unlisted.example/x.pdf", name="x.pdf",
                    )],
                )
            ))
        msgs = capture_upstream["messages"]
        # 无 user 消息 → fallback 到 system 末尾追加
        assert len(msgs) == 1
        assert "附件上下文" in msgs[0]["content"]
        assert "你是摘要器" in msgs[0]["content"], "原 system 内容必须保留"