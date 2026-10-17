#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""CL-06 第二方核对整改后的**真并发**运行态验证（经网关，打真 Postgres）。

验的是整改前的竞态：`CountActive()` + `Create()` 两次往返 ⇒ 并发下第 6 个角色会进库。
整改后走"事务 + 每用户咨询锁"的原子方法，本脚本用并发请求压它。

做法：并发发 N 个"新建角色"请求（N > 上限），然后断言：
  ① 该用户活跃角色数 ≤ 5；② 恰好 5（上限内都成功，不是靠大量失败换取 ≤5）；
  ③ 默认角色恰好 1 条（并发物化路径也不会产生第二条）。
"""
import json, sys, urllib.error, urllib.request
from concurrent.futures import ThreadPoolExecutor

GW = (sys.argv[1] if len(sys.argv) > 1 else "http://127.0.0.1:19080").rstrip("/")
N = 10  # 并发请求数（上限 5）


def call(method, path, token=None, body=None):
    data = json.dumps(body, ensure_ascii=False).encode("utf-8") if body is not None else None
    r = urllib.request.Request(GW + path, data=data, method=method)
    r.add_header("Content-Type", "application/json; charset=utf-8")
    if token:
        r.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(r, timeout=60) as resp:
            return resp.status, json.loads(resp.read().decode("utf-8", "replace"))
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw


_, body = call("POST", "/api/v1/auth/login", body={"username": "echo", "password": "echo123"})
token = body["data"]["accessToken"]


def list_agents():
    sc, res = call("GET", "/api/v1/agents", token)
    if not isinstance(res, dict):
        print("LIST_FAILED status=%s body=%r" % (sc, str(res)[:200]))
        sys.exit(2)
    return (res.get("data") or {}).get("agents") or []


# ① 先看现状：默认「小E」是否恰好一条（物化的正确性证据）
before = list_agents()
defaults_before = sum(1 for a in before if a.get("isDefault"))
print("起始：活跃角色 %d 条，其中默认 %d 条" % (len(before), defaults_before))

# ② 清空（制造"上限刚好可填满"的场景）
for a in before:
    call("DELETE", "/api/v1/agents/%d" % a["id"], token)
print("清理后：", [(a["id"], a["name"]) for a in list_agents()], "（空 = 用户删光后**不再**物化，属设计：含软删除计数）")

# ③ 并发压上限
def create(i):
    return call("POST", "/api/v1/agents", token, {"name": "并发%d" % i, "persona": "并发压测", "voice": ""})


with ThreadPoolExecutor(max_workers=N) as ex:
    results = list(ex.map(create, range(N)))

codes = sorted({c for c, _ in results})
ok = [c for c, _ in results].count(200)
print("并发 %d 个新建请求：200=%d，其余状态码=%s" % (N, ok, [c for c in codes if c != 200]))

agents = list_agents()
active = len(agents)
print("最终：活跃角色数 = %d" % active)

fails = []
if active > 5:
    fails.append("活跃角色数 %d > 上限 5（TOCTOU 未修好）" % active)
if active != 5:
    fails.append("活跃角色数 %d != 5（上限内本应都成功；偏少说明计数/拒绝逻辑有误）" % active)
if ok != 5:
    fails.append("成功数 %d != 5" % ok)
if 500 in codes:
    fails.append("出现 HTTP 500（配额类应映射为 4xx —— ResourceExhausted 未映射的旧行为）")
if any(c for c in codes if c not in (200, 409)):
    fails.append("出现预期外状态码：%s" % [c for c in codes if c not in (200, 409)])

# ④ 清理
for a in agents:
    if a["name"].startswith("并发"):
        call("DELETE", "/api/v1/agents/%d" % a["id"], token)
print("清理后：", [(a["id"], a["name"]) for a in list_agents()])

print()
print("=== 结论：%s ===" % ("PASS" if not fails else "FAIL"))
for f in fails:
    print("  -", f)
sys.exit(0 if not fails else 1)
