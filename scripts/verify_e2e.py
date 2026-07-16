"""端到端验证（Stage 20 P0 增强版）"""
import json
import os
import urllib.request
import sys

import grpc
from grpc_health.v1 import health_pb2, health_pb2_grpc

results = []

def check(name, ok, detail=""):
    sym = "OK  " if ok else "FAIL"
    results.append((name, ok))
    print(f"[{sym}] {name}: {detail}")

# 1) llm HTTP /health
try:
    r = urllib.request.urlopen("http://localhost:8000/health", timeout=3)
    d = json.loads(r.read())
    check("llm HTTP :8000/health", d.get("status") == "ok", d)
except Exception as e:
    check("llm HTTP :8000/health", False, str(e))

# 2) llm HTTP /analyze
try:
    body = json.dumps({"text": "今天好开心"}).encode("utf-8")
    req = urllib.request.Request("http://localhost:8000/analyze", data=body,
                                 headers={"Content-Type": "application/json"})
    r = urllib.request.urlopen(req, timeout=3)
    d = json.loads(r.read())
    check("llm HTTP :8000/analyze", d.get("primaryEmotion") == "happy",
          f"emotion={d.get('primaryEmotion')} score={d.get('sentimentScore')}")
except Exception as e:
    check("llm HTTP :8000/analyze", False, str(e))

# 2b) llm HTTP /metrics (Stage 20-P0-2)
try:
    body = urllib.request.urlopen("http://localhost:8000/metrics", timeout=3).read().decode()
    llm_lines = [l for l in body.split("\n") if l.startswith("llm_") and not l.startswith("llm_http_request_duration")]
    check("llm HTTP :8000/metrics", len(llm_lines) > 0, f"{len(llm_lines)} llm_* series")
    for l in llm_lines[:5]:
        print(f"        {l}")
except Exception as e:
    check("llm HTTP :8000/metrics", False, str(e))

# 3) ai-svc HTTP /health
try:
    r = urllib.request.urlopen("http://localhost:8891/health", timeout=3)
    d = json.loads(r.read())
    check("ai-svc HTTP :8891/health", d.get("status") == "ok" and d.get("dbOk") is True,
          f"dbOk={d.get('dbOk')}")
except Exception as e:
    check("ai-svc HTTP :8891/health", False, str(e))

# 4) ai-svc gRPC Health
try:
    ch = grpc.insecure_channel("localhost:8892")
    r = health_pb2_grpc.HealthStub(ch).Check(
        health_pb2.HealthCheckRequest(service="emotion.AI"))
    ch.close()
    check("ai-svc gRPC :8892 emotion.AI",
          r.status == health_pb2.HealthCheckResponse.SERVING,
          f"status={r.status}")
except Exception as e:
    check("ai-svc gRPC :8892 emotion.AI", False, str(e))

# 5) ai-svc /metrics
try:
    body = urllib.request.urlopen("http://localhost:8891/metrics", timeout=3).read().decode()
    lines = [l for l in body.split("\n") if l.startswith("ai_svc_http_requests_total")]
    check("ai-svc /metrics", len(lines) > 0, f"{len(lines)} ai_svc_http_requests_total series")
    for l in lines[:3]:
        print(f"        {l}")
except Exception as e:
    check("ai-svc /metrics", False, str(e))

# 6) ai-svc gRPC health for emotion.LLM (容器内服务发现验证)
#     通过 ai-svc 容器内调用 llm gRPC 看是否连得上 → 但 host 端没证书，直接调 llm 失败
#     改：看 ai-svc 日志里有没有"using gRPC analyzer (target=emotion-llm-service:50051)"

# 7) llm gRPC health (mTLS, host 端没证书，跳过)
#     注：本地用 insecure 调不通（容器里开了 mTLS）

# Summary
total = len(results)
passed = sum(1 for _, ok in results if ok)
print(f"\n=== Summary: {passed}/{total} passed ===")
sys.exit(0 if passed == total else 1)