"""Stage 91 e2e：PDF 短文本哨兵真实容器 DeepSeek 调 5 次，验证拒读消除

跑法：
  docker exec emotion-llm-service python /app/stage91_pdf_sentinel.py

前置：容器内 /app/etc/tls/ 需含 ai-client.crt/key + ca.crt（与 ai-svc 共享 client 证书）。
"""
import base64
import os
import socket
import subprocess
import sys
import tempfile
import threading
import time
from http.server import BaseHTTPRequestHandler, HTTPServer

import grpc

import emotion_llm_pb2
import emotion_llm_pb2_grpc

PDF_B64 = (
    "JVBERi0xLjQKMSAwIG9iago8PCAvVHlwZSAvQ2F0YWxvZyAvUGFnZXMgMiAwIFIgPj4KZW5kb2Jq"
    "CjIgMCBvYmo8PCAvVHlwZSAvUGFnZXMgL0tpZHMgWzMgMCBSXSAvQ291bnQgMSA+PgplbmRvYmoK"
    "MyAwIG9iajw8IC9UeXBlIC9QYWdlIC9QYXJlbnQgMiAwIFIgL01lZGlhQm94IFswIDAgNjEyIDc5"
    "Ml0gL0NvbnRlbnRzIDQgMCBSIC9SZXNvdXJjZXMgPDwgL0ZvbnQgPDwgL0YxIDUgMCBSID4+ID4+"
    "ID4+CjQgMCBvYmo8PCAvTGVuZ3RoIDU5ID4+CnN0cmVhbQpCVCAvRjEgMjQgVGYgNzIgNzIwIFRk"
    "IChQREYtU0VOVElORUwtcHlwZGYtT0stMjAyNikgVGogRVQKZW5kc3RyZWFtCmVuZG9iago1IDAg"
    "b2JqPDwgL1R5cGUgL0ZvbnQgL1N1YnR5cGUgL1R5cGUxIC9CYXNlRm9udCAvSGVsdmV0aWNhID4+"
    "CmVuZG9iagp0cmFpbGVyCjw8IC9TaXplIDYgL1Jvb3QgMSAwIFIgPj4Kc3RhcnR4cmVmCjAKJSVF"
    "T0YK"
)
PDF_BYTES = base64.b64decode(PDF_B64)
SENTINEL = "PDF-SENTINEL-pypdf-OK-2026"
USER_PROMPT = "请引用附件里那句话"
RUNS = int(os.environ.get("E2E_RUNS", "5"))

# mTLS：与 ai-svc 共享 ai-client.crt/key；服务器证书由 ca.crt 验证
TLS_CA = "/app/etc/tls/ca.crt"
TLS_CLIENT_CERT = "/app/etc/tls/ai-client.crt"
TLS_CLIENT_KEY = "/app/etc/tls/ai-client.key"


def build_credentials():
    with open(TLS_CA, "rb") as f:
        ca = f.read()
    with open(TLS_CLIENT_CERT, "rb") as f:
        cert = f.read()
    with open(TLS_CLIENT_KEY, "rb") as f:
        key = f.read()
    return grpc.ssl_channel_credentials(
        root_certificates=ca, private_key=key, certificate_chain=cert
    )


class PdfHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200)
        self.send_header("Content-Type", "application/pdf")
        self.send_header("Content-Length", str(len(PDF_BYTES)))
        self.end_headers()
        self.wfile.write(PDF_BYTES)

    def log_message(self, *a, **kw):
        pass  # 静默 HTTP server 日志


def start_http_server():
    return start_http_server_at(0)


def start_http_server_at(port: int):
    class ReusableHTTPServer(HTTPServer):
        allow_reuse_address = True
    srv = ReusableHTTPServer(("127.0.0.1", port), PdfHandler)
    actual_port = srv.server_address[1]
    t = threading.Thread(target=srv.serve_forever, daemon=True)
    t.start()
    return srv, actual_port


def _port_free(port: int) -> bool:
    """检查端口是否可用（SO_REUSEADDR + bind 测试）

    注意：bind 失败可能因为端口残留（CLOSE_WAIT/TIME_WAIT），不是真占用
    """
    s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        s.bind(("127.0.0.1", port))
        s.close()
        return True
    except OSError:
        return False


def call_chat_completion(host: str, port: int):
    """调用一次 ChatCompletion，返 (success, response_text, error)"""
    creds = build_credentials()
    api_key = os.environ.get("INTERNAL_API_KEY", "")
    metadata = [("x-internal-api-key", api_key)]
    try:
        with grpc.secure_channel(f"{host}:{port}", creds) as ch:
            stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
            req = emotion_llm_pb2.ChatCompletionRequest(
                messages=[emotion_llm_pb2.ChatMessage(role="user", content=USER_PROMPT)],
                files=[
                    emotion_llm_pb2.FileAttachment(
                        url=f"http://127.0.0.1:{HTTP_PORT}/sentinel.pdf",
                        name="sentinel.pdf",
                    )
                ],
            )
            try:
                chunks = list(stub.ChatCompletion(req, metadata=metadata, timeout=60))
            except grpc.RpcError as e:
                return False, "", f"gRPC error: code={e.code()} details={e.details()}"
    except Exception as e:
        return False, "", f"channel error: {type(e).__name__}: {e}"
    full = "".join(c.delta_content for c in chunks if c.delta_content)
    fallback = "".join(c.fallback_reason for c in chunks if c.fallback_reason).strip()
    return True, full, fallback


HTTP_PORT = None


def main():
    global HTTP_PORT
    # 用 'localhost:<port>' 让 llm-service 默认白名单能命中（DEFAULT_ALLOWLIST
    # 含 'localhost:9000'，url_allowed 在 port 不匹配时返 False——所以这里我们
    # 必须把实际端口注入到 llm-service 进程的 env。最简单的做法：用环境变量传递
    # HTTP port 给 ChatCompletion——但 grpc_server 没暴露这个。我们走另一个路径：
    # 用 chat-svc 真实请求链路：上传文件到 MinIO → 用真实 MinIO URL → MinIO 在
    # llm-service 默认白名单（'emotion-echo-minio:9000'）。本次 e2e 跳过 MinIO，
    # 改为临时把 file_context 的白名单扩展：把 chat_svc 真实附件 URL 走 MinIO
    # 路径。e2e 改简单：让我们的 HTTP server 监听 9000 端口，碰巧命中
    # 'localhost:9000' 和 '127.0.0.1:9000' 两个白名单条目
    import socket
    # 尝试拿 9000（被 MinIO 占用则 fail back 到 任意 + 注入环境）
    if _port_free(9000):
        srv, HTTP_PORT = start_http_server_at(9000)
        host_part = "127.0.0.1"
    else:
        # 走默认白名单不行——改用：把所有调用改成通过 chat-svc 上传链路
        # 这条 e2e 暂时跳过（要求 MinIO + chat-svc 全链路），改报 BLOCKED
        print("[e2e] port 9000 in use (MinIO likely); e2e 改走 chat-svc 全链路或阻塞")
        sys.exit(2)
    print(f"[e2e] HTTP PDF server up on {host_part}:{HTTP_PORT}")
    # 注入白名单
    allowlist_value = f"{host_part}:{HTTP_PORT}"
    os.environ["FILE_FETCH_ALLOWLIST"] = allowlist_value
    print(f"[e2e] FILE_FETCH_ALLOWLIST={allowlist_value!r}")
    target = f"http://{host_part}:{HTTP_PORT}/sentinel.pdf"
    print(f"[e2e] target url: {target}")
    # sanity
    from file_context import url_allowed, resolve_fetch_config
    cfg = resolve_fetch_config()
    print(f"[e2e] url_allowed(target) = {url_allowed(target, cfg.allowlist)}")
    # grpc server 已在 :50051 监听（容器内 localhost）
    GRPC_PORT = 50051
    hits, misses, errors = 0, 0, 0
    print(f"[e2e] start {RUNS} runs vs DeepSeek, sentinel={SENTINEL!r}")
    for i in range(1, RUNS + 1):
        t0 = time.time()
        try:
            ok, text, fallback = call_chat_completion("127.0.0.1", GRPC_PORT)
        except Exception as e:
            import traceback
            print(f"  [{i}/{RUNS}] CRASH:")
            traceback.print_exc()
            errors += 1
            continue
        dt = time.time() - t0
        if not ok:
            errors += 1
            print(f"  [{i}/{RUNS}] ERROR ({dt:.1f}s): {text!r}")
            continue
        cited = SENTINEL in text
        # 关键判定：模型是否在响应里"引用/复述"哨兵
        if cited:
            hits += 1
            print(f"  [{i}/{RUNS}] HIT    ({dt:.1f}s, {len(text)} chars) text={text!r}")
        else:
            misses += 1
            # 截断输出便于排查
            preview = text[:200].replace("\n", " ")
            print(f"  [{i}/{RUNS}] MISS   ({dt:.1f}s, {len(text)} chars) preview={preview!r}")
        if fallback:
            print(f"         fallback_reason={fallback!r}")
    print()
    print(f"[e2e] DONE: hits={hits} misses={misses} errors={errors} total={RUNS}")
    rate = hits / max(hits + misses, 1)
    print(f"[e2e] reference_rate={rate:.0%}  (Stage 90 baseline ~50%, target >=80%)")
    srv.shutdown()
    sys.exit(0 if rate >= 0.8 and errors == 0 else 1)


if __name__ == "__main__":
    main()
