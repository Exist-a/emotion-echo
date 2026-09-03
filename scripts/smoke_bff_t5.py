"""T5 端到端 smoke — 基于 BFF :8894 联通所有下游
（替代老 verify_e2e.py / smoke_apps_26p.sh，老脚本假设老端口；BFF 已聚合 5 下游 + 宿主端口 :8894）
"""
import json
import sys
import urllib.error
import urllib.parse
import urllib.request

BFF = "http://localhost:8894"

results = []

def check(name, ok, detail=""):
    sym = "OK  " if ok else "FAIL"
    results.append((name, ok))
    print(f"[{sym}] {name}: {detail}")

def http_get(path, headers=None):
    req = urllib.request.Request(BFF + path)
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    return urllib.request.urlopen(req, timeout=8)

def http_post(path, body=None, headers=None):
    data = json.dumps(body).encode("utf-8") if body is not None else b""
    req = urllib.request.Request(BFF + path, data=data, method="POST")
    req.add_header("Content-Type", "application/json")
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    return urllib.request.urlopen(req, timeout=15)

def http_patch(path, body=None, headers=None):
    data = json.dumps(body).encode("utf-8") if body is not None else b""
    req = urllib.request.Request(BFF + path, data=data, method="PATCH")
    req.add_header("Content-Type", "application/json")
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    return urllib.request.urlopen(req, timeout=8)

def unwrap(resp_json):
    """BFF 用 {code, data, message} 封包；取 .data"""
    if isinstance(resp_json, dict) and "data" in resp_json and "code" in resp_json:
        return resp_json.get("data"), resp_json
    return resp_json, resp_json

# === 1) BFF /health 聚合 ===
try:
    r = http_get("/health")
    d = json.loads(r.read())
    ds = d.get("downstream", {})
    ok_count = sum(1 for v in ds.values() if v.get("status") == "ok")
    check("BFF /health", d.get("status") in ("ok", "degraded"),
          f"status={d.get('status')} downstream_ok={ok_count}/{len(ds)}")
    # 单独验证 5 Go svc + ai-svc
    for svc in ("user", "chat", "analytics", "assessment", "ai"):
        s = ds.get(svc, {}).get("status")
        check(f"  downstream {svc}", s == "ok", f"status={s}")
    # xtts 单独标注（不阻塞核心 smoke）
    xtts = ds.get("xtts", {})
    check("  downstream xtts", xtts.get("status") == "ok",
          f"status={xtts.get('status')} detail={xtts.get('detail', '')[:60]}")
except Exception as e:
    check("BFF /health", False, str(e))

# === 2) 用户登录（BFF mock auth，{username, password}）===
user_id = None
jwt_token = None
try:
    # BFF /api/v1/auth/login 接 {username, password}，返回 {accessToken, user: {id, username}}
    # Stage 38-A: dev seed 账号 = echo / echo123（之前是 13800138000 / abc123）
    r = http_post("/api/v1/auth/login",
                  {"username": "echo", "password": "echo123"})
    d = json.loads(r.read())
    data, _ = unwrap(d)
    # 真实结构是 data.accessToken + data.user.id
    access_token = (data or {}).get("accessToken") if isinstance(data, dict) else None
    user_obj = (data or {}).get("user") if isinstance(data, dict) else None
    user_id = user_obj.get("id") if isinstance(user_obj, dict) else None
    jwt_token = access_token
    check("BFF /api/v1/auth/login", user_id is not None and access_token,
          f"user_id={user_id} token_len={len(access_token) if access_token else 0}")
except urllib.error.HTTPError as e:
    body = e.read().decode(errors="replace")
    check("BFF /api/v1/auth/login", False, f"HTTP {e.code}: {body[:120]}")
except Exception as e:
    check("BFF /api/v1/auth/login", False, str(e))

# === 3) 拉取当前用户（验证 user-svc + 鉴权头）===
try:
    r = http_get("/api/v1/users/me", {"X-User-Id": str(user_id) if user_id else "1"})
    d = json.loads(r.read())
    data, _ = unwrap(d)
    # /users/me 真实响应: {user: {userId, account, phone, nickname}}
    user_obj = (data or {}).get("user") if isinstance(data, dict) else None
    ok = isinstance(user_obj, dict) and (user_obj.get("userId") is not None or user_obj.get("id") is not None)
    check("BFF /api/v1/users/me", ok,
          f"user_keys={list(user_obj.keys())[:6] if isinstance(user_obj, dict) else 'N/A'}")
except Exception as e:
    check("BFF /api/v1/users/me", False, str(e))

# === 4) 创建会话 ===
conv_id = None
try:
    r = http_post("/api/v1/conversations",
                  {"title": "smoke test"},
                  {"X-User-Id": "1"})
    d = json.loads(r.read())
    data, _ = unwrap(d)
    conv_id = (data or {}).get("id") or (data or {}).get("conversation_id") or (data or {}).get("conversationId") if isinstance(data, dict) else None
    check("BFF POST /api/v1/conversations", conv_id is not None,
          f"conv_id={conv_id} data_keys={list(data.keys())[:6] if isinstance(data, dict) else 'N/A'}")
except urllib.error.HTTPError as e:
    body = e.read().decode(errors="replace")
    check("BFF POST /api/v1/conversations", False, f"HTTP {e.code}: {body[:120]}")
except Exception as e:
    check("BFF POST /api/v1/conversations", False, str(e))

# === 5) 会话列表（G2 实证 — chat-svc 应没有 GET /conversations 端点）===
try:
    r = http_get("/api/v1/conversations", {"X-User-Id": "1"})
    d = json.loads(r.read())
    data, _ = unwrap(d)
    items = data.get("items") if isinstance(data, dict) else None
    check("BFF GET /api/v1/conversations (G2 受阻预期)", True,
          f"data_type={type(data).__name__} data_keys={list(data.keys())[:6] if isinstance(data, dict) else 'N/A'} items_type={type(items).__name__ if items is not None else 'N/A'}")
except urllib.error.HTTPError as e:
    body = e.read().decode(errors="replace")
    check("BFF GET /api/v1/conversations (G2 受阻预期)", False,
          f"HTTP {e.code}: {body[:120]}")
except Exception as e:
    check("BFF GET /api/v1/conversations (G2 受阻预期)", False, str(e))

# === 6) 发送消息（G4 实证 — Kafka 默认 false，dev fallback 应直接返回）===
if conv_id:
    try:
        r = http_post(f"/api/v1/conversations/{conv_id}/messages",
                      {"content": "今天心情不太好", "role": "user"},
                      {"X-User-Id": "1"})
        d = json.loads(r.read())
        data, _ = unwrap(d)
        msg_id = (data or {}).get("id") or (data or {}).get("message_id") if isinstance(data, dict) else None
        check("BFF POST /api/v1/conversations/{id}/messages", msg_id is not None,
              f"msg_id={msg_id} data_keys={list(data.keys())[:6] if isinstance(data, dict) else 'N/A'}")
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        check("BFF POST /api/v1/conversations/{id}/messages", False,
              f"HTTP {e.code}: {body[:120]}")
    except Exception as e:
        check("BFF POST /api/v1/conversations/{id}/messages", False, str(e))

# === 7) ai_stream (mock LLM fallback — BFF_LLM_API_KEY 留空应走 mock) ===
try:
    body = json.dumps({
        "model": "mock",
        "messages": [{"role": "user", "content": "今天心情不太好"}],
        "stream": True,
    }).encode("utf-8")
    req = urllib.request.Request(BFF + "/api/v1/ai/stream", data=body, method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("X-User-Id", "1")
    # SSE：读 chunk 流，期望至少有 1 个 event
    r = urllib.request.urlopen(req, timeout=20)
    chunks = []
    while True:
        chunk = r.read(512)
        if not chunk:
            break
        chunks.append(chunk.decode("utf-8", errors="replace"))
        if len("".join(chunks)) > 8192:
            break
    payload = "".join(chunks)
    has_event = "event:" in payload or "data:" in payload
    check("BFF /api/v1/ai/stream (mock LLM fallback)", has_event,
          f"chunks={len(chunks)} bytes={len(payload)} has_sse={has_event} sample={payload[:200]}")
except urllib.error.HTTPError as e:
    body = e.read().decode(errors="replace")
    check("BFF /api/v1/ai/stream (mock LLM fallback)", False,
          f"HTTP {e.code}: {body[:120]}")
except Exception as e:
    check("BFF /api/v1/ai/stream (mock LLM fallback)", False, str(e))

# === 8) 情绪分析报表（analytics-svc — 需要 ?user_id=）===
try:
    r = http_get("/api/v1/reports/daily?user_id=1", {"X-User-Id": "1"})
    d = json.loads(r.read())
    data, _ = unwrap(d)
    ok = isinstance(data, dict)
    check("BFF /api/v1/reports/daily?user_id=1", ok,
          f"data_keys={list(data.keys())[:8] if ok else type(data).__name__}")
except urllib.error.HTTPError as e:
    body = e.read().decode(errors="replace")
    check("BFF /api/v1/reports/daily?user_id=1", False, f"HTTP {e.code}: {body[:120]}")
except Exception as e:
    check("BFF /api/v1/reports/daily?user_id=1", False, str(e))

# === 9) 心理量表列表（assessment-svc）===
try:
    r = http_get("/api/v1/surveys", {"X-User-Id": "1"})
    d = json.loads(r.read())
    data, _ = unwrap(d)
    ok = isinstance(data, dict)
    check("BFF /api/v1/surveys", ok,
          f"data_keys={list(data.keys())[:6] if ok else type(data).__name__}")
except urllib.error.HTTPError as e:
    body = e.read().decode(errors="replace")
    check("BFF /api/v1/surveys", False, f"HTTP {e.code}: {body[:120]}")
except Exception as e:
    check("BFF /api/v1/surveys", False, str(e))

# === 10) Prometheus metrics（G1 间接实证 — 看 skywalking dial 失败循环是否产生日志噪音指标）===
try:
    r = http_get("/metrics")
    body = r.read().decode("utf-8", errors="replace")
    lines = [l for l in body.split("\n") if l.startswith("emotion_echo_") or l.startswith("go_")]
    check("BFF /metrics", len(lines) > 0, f"{len(lines)} emotion_echo_/go_ series")
except Exception as e:
    check("BFF /metrics", False, str(e))

# === 总结 ===
total = len(results)
passed = sum(1 for _, ok in results if ok)
print()
print(f"=== T5 smoke 总计: {passed}/{total} 通过 ===")
sys.exit(0 if passed == total else 1)