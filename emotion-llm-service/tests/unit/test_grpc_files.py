"""Stage 89 PR-4 RED：ChatCompletion servicer 附件上下文注入集成测试。

真实 gRPC server + 本地 HTTP 附件服务器（不碰真网）：request.files 的 url 指向
本地 server 上的 txt 文件，断言传给上游的 messages 中 system 段包含抽取出的文本；
ClassifyIntent 请求不受文件影响（文本仍为最后一条 user 消息）。
"""

import threading
from http.server import BaseHTTPRequestHandler, HTTPServer

import grpc
import pytest

import emotion_llm_pb2
import emotion_llm_pb2_grpc
import grpc_server
from grpc_server import EmotionLLMServiceServicer


@pytest.fixture
def http_server():
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            body = "E2E文件哨兵内容-蓝天白云".encode()
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
    server = grpc.server(
        futures_pool := __import__("concurrent.futures", fromlist=["ThreadPoolExecutor"]).ThreadPoolExecutor(max_workers=2),
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
def capture_upstream(monkeypatch):
    """打桩 iter_chat_chunks：捕获 messages，回放一个 done 帧"""
    captured = {}

    def fake_iter(messages, **kwargs):
        captured["messages"] = messages
        yield emotion_llm_pb2.ChatChunk(delta_content="好的", model="deepseek-chat", fallback_reason="")
        yield emotion_llm_pb2.ChatChunk(done=True, model="deepseek-chat", fallback_reason="")

    monkeypatch.setattr(grpc_server, "iter_chat_chunks", fake_iter)
    return captured


class TestChatCompletionFileContext:
    def test_files_injected_into_system_message(self, grpc_addr, http_server, capture_upstream, monkeypatch):
        host, port = http_server
        monkeypatch.setenv("FILE_FETCH_ALLOWLIST", host)
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            chunks = list(stub.ChatCompletion(
                emotion_llm_pb2.ChatCompletionRequest(
                    messages=[emotion_llm_pb2.ChatMessage(role="user", content="看看这个文件讲了什么")],
                    files=[emotion_llm_pb2.FileAttachment(
                        url=f"http://{host}/uploads/u1-abc.txt", name="notes.txt",
                    )],
                )
            ))
        assert chunks[-1].done
        msgs = capture_upstream["messages"]
        # Stage 90：file_context 注入到 user 消息尾部而非 system
        user_msgs = [m["content"] for m in msgs if m["role"] == "user"]
        assert len(user_msgs) == 1, "应有且仅有一条 user 消息承载附件上下文"
        user_text = user_msgs[0]
        assert "附件上下文" in user_text
        assert "notes.txt" in user_text
        assert "E2E文件哨兵内容-蓝天白云" in user_text, "抽取的文件文本必须进入 user 上下文"
        # system 不应被附件改动
        system_texts = [m["content"] for m in msgs if m["role"] == "system"]
        assert all("附件上下文" not in t for t in system_texts), "system 不应包含附件上下文"

    def test_no_files_no_context_block(self, grpc_addr, capture_upstream):
        with grpc.insecure_channel(grpc_addr) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            list(stub.ChatCompletion(
                emotion_llm_pb2.ChatCompletionRequest(
                    messages=[emotion_llm_pb2.ChatMessage(role="user", content="hi")],
                )
            ))
        msgs = capture_upstream["messages"]
        assert not any("附件上下文" in m["content"] for m in msgs)
