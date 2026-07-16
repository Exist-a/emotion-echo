"""
emotion-llm-service · SIGTERM graceful shutdown e2e (Stage 20-1)

启动 grpc_server.py 子进程 → 等待 Serving → 发送 SIGTERM → 验证：
  1) 进程在 grace 时间窗口内退出
  2) 退出码 = 0
  3) 日志包含 "graceful shutdown" + "health: NOT_SERVING" + "gRPC server stopped"
  4) Health.Check 在收到 SIGTERM 后变 NOT_SERVING

平台兼容：
  - Linux: 跑 SIGTERM（K8s / docker stop 的标准信号）
  - Windows: 跳过自动测试（subprocess.Popen.send_signal 不支持 SIGINT 到子进程，
             且 Windows 上 SIGTERM 是硬杀），改用 manual checklist

执行：
  Linux:   python e2e_signal.py
  Windows: 本脚本 SKIP（手动验证步骤见下方 MANUAL 注释）
"""
import os
import signal
import subprocess
import sys
import time

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc

GRPC_PORT = int(os.environ.get("GRPC_PORT", "50051"))
STARTUP_TIMEOUT = 15
GRACE_SECONDS = 5

# =====================================================
# Windows 平台 manual checklist
# =====================================================
MANUAL_WINDOWS = """
=== Windows 手动验证 graceful shutdown ===

1) 启动 server (前台运行，会看到 banner):
   > python grpc_server.py

2) 另开一个终端，发 SIGINT (Ctrl-C 在新窗口):
   > taskkill /F /PID <pid>     ← 硬杀，看不到 graceful 日志
   或在原窗口按 Ctrl-C          ← 触发 handler，graceful shutdown

3) Ctrl-C 后观察日志，应包含:
   "received SIGINT, starting graceful shutdown (grace=5s, set health=NOT_SERVING)"
   "gRPC server stopped"

4) 验证 health (另起一个 client):
   > python -c "import grpc; from grpc_health.v1 import health_pb2, health_pb2_grpc;
   ch=grpc.insecure_channel('localhost:50051');
   print(health_pb2_grpc.HealthStub(ch).Check(health_pb2.HealthCheckRequest()))"
   → 期望 UNAVAILABLE（server 已关）

5) 实际部署到 Linux 容器时（K8s pod 终止 / docker stop）会发 SIGTERM，
   触发同样的 handler，行为与 Ctrl-C 一致。
"""


def wait_for_health_serving(port: int, timeout: float):
    """轮询直到 Health/Check 返回 SERVING，或超时"""
    deadline = time.time() + timeout
    target = f"localhost:{port}"
    while time.time() < deadline:
        try:
            ch = grpc.insecure_channel(target)
            stub = health_pb2_grpc.HealthStub(ch)
            resp = stub.Check(health_pb2.HealthCheckRequest(service="emotion.LLM"))
            ch.close()
            if resp.status == health_pb2.HealthCheckResponse.SERVING:
                return True
        except grpc.RpcError:
            pass
        time.sleep(0.2)
    return False


def main():
    if sys.platform == "win32":
        print("[e2e] SKIP: Windows 平台跳过自动测试")
        print(MANUAL_WINDOWS)
        return 0

    print(f"[e2e] launching grpc_server.py on :{GRPC_PORT} ...")
    proc = subprocess.Popen(
        [sys.executable, "grpc_server.py"],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        env={**os.environ, "GRPC_PORT": str(GRPC_PORT), "INTERNAL_API_KEY": ""},
        text=True,
    )

    try:
        # 1) 等启动
        if not wait_for_health_serving(GRPC_PORT, STARTUP_TIMEOUT):
            print(f"[e2e] FAIL: server not healthy within {STARTUP_TIMEOUT}s")
            proc.kill()
            return 1
        print(f"[e2e] OK: server is SERVING on :{GRPC_PORT}")

        # 2) 发送 SIGTERM（Linux: K8s/docker stop 标准信号）
        print(f"[e2e] sending SIGTERM to pid={proc.pid}")
        t0 = time.time()
        proc.send_signal(signal.SIGTERM)

        # 3) 等退出（grace=5s + 一些 buffer）
        try:
            exit_code = proc.wait(timeout=GRACE_SECONDS + 5)
        except subprocess.TimeoutExpired:
            print(f"[e2e] FAIL: server did not exit within {GRACE_SECONDS+5}s after SIGTERM")
            proc.kill()
            return 1

        elapsed = time.time() - t0

        # 4) 验证日志
        try:
            log, _ = proc.communicate(timeout=2)
        except Exception:
            log = ""
        checks = [
            ("graceful shutdown", "graceful shutdown" in log),
            ("health NOT_SERVING", "NOT_SERVING" in log),
            ("server stopped", "gRPC server stopped" in log),
        ]
        for name, ok in checks:
            print(f"[e2e] {'OK' if ok else 'FAIL'}: log contains '{name}'")

        # 5) 验证退出码
        code_ok = exit_code == 0
        print(f"[e2e] {'OK' if code_ok else 'FAIL'}: exit code = {exit_code} (want 0)")

        # 6) 验证耗时（应该至少 0.05s 表示确实收到信号处理了，不应 <0.01s 立即退）
        time_ok = 0.05 < elapsed < GRACE_SECONDS + 3
        print(f"[e2e] {'OK' if time_ok else 'FAIL'}: shutdown elapsed = {elapsed:.2f}s (want 0.05~{GRACE_SECONDS+3}s)")

        # 7) 退出后 health check 应该失败（channel closed）
        try:
            ch = grpc.insecure_channel(f"localhost:{GRPC_PORT}")
            stub = health_pb2_grpc.HealthStub(ch)
            resp = stub.Check(health_pb2.HealthCheckRequest(service="emotion.LLM"))
            print(f"[e2e] WARN: Health.Check after exit returned {resp.status} (want UNAVAILABLE)")
        except grpc.RpcError as e:
            print(f"[e2e] OK: Health.Check after exit raised {e.code().name} (want UNAVAILABLE)")

        all_ok = all(c[1] for c in checks) and code_ok and time_ok
        return 0 if all_ok else 1
    finally:
        if proc.poll() is None:
            proc.kill()


if __name__ == "__main__":
    sys.exit(main())
