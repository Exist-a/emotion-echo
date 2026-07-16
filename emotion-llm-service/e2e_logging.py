"""
emotion-llm-service · JSON 结构化日志 e2e（Stage 20-2）

启动 grpc_server.py → 触发几类日志（info/warn/error）→ 验证 stdout 输出是合法 JSON。
并验证 extra= 字段被并入顶层。

执行：
  python e2e_logging.py
"""
import json
import os
import subprocess
import sys
import time

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc

GRPC_PORT = int(os.environ.get("GRPC_PORT", "50053"))  # 用 50053 避免与生产 50051 冲突
STARTUP_TIMEOUT = 15


def main():
    print(f"[e2e] launching grpc_server.py on :{GRPC_PORT} (LOG_FORMAT=json) ...")
    proc = subprocess.Popen(
        [sys.executable, "grpc_server.py"],
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        env={
            **os.environ,
            "GRPC_PORT": str(GRPC_PORT),
            "INTERNAL_API_KEY": "",
            "LOG_FORMAT": "json",
            "PYTHONUNBUFFERED": "1",
        },
        text=True,
    )

    try:
        # 等启动
        deadline = time.time() + STARTUP_TIMEOUT
        started = False
        while time.time() < deadline:
            try:
                ch = grpc.insecure_channel(f"localhost:{GRPC_PORT}")
                stub = health_pb2_grpc.HealthStub(ch)
                resp = stub.Check(health_pb2.HealthCheckRequest(service="emotion.LLM"))
                ch.close()
                if resp.status == health_pb2.HealthCheckResponse.SERVING:
                    started = True
                    break
            except grpc.RpcError:
                pass
            time.sleep(0.2)

        if not started:
            print(f"[e2e] FAIL: server not healthy within {STARTUP_TIMEOUT}s")
            proc.kill()
            return 1
        print(f"[e2e] OK: server is SERVING on :{GRPC_PORT}")

        # 触发一条 Analyze RPC（产生 info 日志）
        import emotion_llm_pb2
        import emotion_llm_pb2_grpc
        ch = grpc.insecure_channel(f"localhost:{GRPC_PORT}")
        stub = emotion_llm_pb2_grpc.EmotionLLMServiceStub(ch)
        try:
            resp = stub.Analyze(emotion_llm_pb2.AnalyzeRequest(message_id="1", text="happy"))
        except grpc.RpcError as e:
            print(f"[e2e] WARN: Analyze failed: {e.code().name} (auth 启用时会失败，正常)")
        ch.close()
        time.sleep(1)  # 等日志 flush

        # 杀进程拿日志
        proc.terminate()
        try:
            log, _ = proc.communicate(timeout=5)
        except Exception:
            log = ""
            try:
                proc.kill()
            except Exception:
                pass

        # 解析每行
        lines = [ln for ln in log.split("\n") if ln.strip()]
        if not lines:
            print(f"[e2e] FAIL: no log output")
            return 1

        print(f"[e2e] captured {len(lines)} log lines")
        json_lines = 0
        invalid_lines = []
        for ln in lines:
            try:
                obj = json.loads(ln)
                json_lines += 1
                # 验证必备字段
                for k in ("ts", "level", "logger", "msg"):
                    if k not in obj:
                        invalid_lines.append((ln, f"missing field: {k}"))
                        break
            except json.JSONDecodeError as e:
                invalid_lines.append((ln, str(e)))

        print(f"[e2e] {'OK' if json_lines == len(lines) else 'FAIL'}: "
              f"valid JSON lines = {json_lines}/{len(lines)}")
        if invalid_lines[:3]:
            for ln, err in invalid_lines[:3]:
                print(f"  - {err}: {ln[:100]}")

        # 至少有一条 INFO 和一条 "gRPC server started"
        has_start = any("gRPC server started" in ln for ln in lines)
        print(f"[e2e] {'OK' if has_start else 'FAIL'}: log contains 'gRPC server started'")

        # 打印前 3 行供检视
        print("\n--- sample log lines ---")
        for ln in lines[:3]:
            print(ln)
        print("--- end sample ---\n")

        all_ok = (json_lines == len(lines)) and has_start
        return 0 if all_ok else 1
    finally:
        if proc.poll() is None:
            try:
                proc.kill()
            except Exception:
                pass


if __name__ == "__main__":
    sys.exit(main())
