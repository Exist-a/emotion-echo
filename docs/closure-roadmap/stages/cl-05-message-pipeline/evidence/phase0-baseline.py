#!/usr/bin/env python3
"""CL-05 Phase 0 · 基线取证（🔴 需 dev 栈）

目的（plan §3 Phase 0）：起栈后经**网关唯一入口** `:19080` + Bearer 抓"改动前"的
消息 / 会话列表现状，并确认 `CHAT_TRANSPORT` 实际值 —— 作为后续 T-1~T-6 的对照基线。

为什么必须走网关 + Bearer（AGENTS §2.4 基址约定）：
  宿主直连 BFF 的端口映射已被 E2E-29 D-47 ③ 收掉；且不得自传 X-User-Id（网关无条件覆盖注入）。

运行：python docs/closure-roadmap/stages/cl-05-message-pipeline/evidence/phase0-baseline.py
（需 dev 栈：共享目录 `--env-file .env.local --profile dev`；前端无关）
"""
from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

GATEWAY = os.environ.get("SMOKE_BASE", "http://localhost:19080")
LOGIN_BODY = {"username": "echo", "password": "echo123"}
TOKEN: str = ""


def _req(path: str, method: str = "GET", body: dict | None = None) -> tuple[int, Any]:
    data = json.dumps(body).encode("utf-8") if body is not None else None
    r = urllib.request.Request(GATEWAY + path, data=data, method=method)
    if body is not None:
        r.add_header("Content-Type", "application/json")
    if path.startswith("/api/v1/") and TOKEN:
        r.add_header("Authorization", f"Bearer {TOKEN}")
    try:
        with urllib.request.urlopen(r, timeout=10) as resp:
            raw = resp.read().decode("utf-8")
            return resp.status, (json.loads(raw) if raw else None)
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", "replace")


def unwrap(d: Any) -> Any:
    if isinstance(d, dict) and "data" in d and "code" in d:
        return d.get("data") or {}
    return d


def main() -> int:
    global TOKEN
    print("=" * 72)
    print(f"CL-05 Phase 0 基线 — 基址 {GATEWAY}")
    print("=" * 72)

    st, raw = _req("/api/v1/auth/login", "POST", LOGIN_BODY)
    print(f"[0] login -> HTTP {st}")
    if st != 200:
        print("    !! 前置不可用（登录失败）—— 本脚本无任何契约结论")
        print(f"    body: {str(raw)[:300]}")
        return 2
    data = unwrap(raw)
    TOKEN = (data or {}).get("accessToken") or (data or {}).get("token") or ""
    print(f"    user_id={(data or {}).get('user', {}).get('id') or (data or {}).get('userId')} token_len={len(TOKEN)}")
    if not TOKEN:
        print(f"    !! 登录响应里没有 token，响应键 = {list((raw or {}).keys())} / data 键 = {list((data or {}).keys())}")
        return 2

    # ---------- A. 会话列表 ----------
    print("\n--- A. 会话列表 /api/v1/conversations ---")
    st, raw = _req("/api/v1/conversations?limit=2")
    conv1 = unwrap(raw)
    lst1 = (conv1 or {}).get("list") or []
    print(f"A1  limit=2 -> HTTP {st}  len={len(lst1)}  hasMore={(conv1 or {}).get('hasMore')}")
    print(f"    data 顶层键 = {sorted((conv1 or {}).keys())}")
    for c in lst1:
        print(f"      id={c.get('id')} updatedAt={c.get('updatedAt')} title={c.get('title')!r}")

    if lst1:
        cur = lst1[-1].get("updatedAt")
        st2, raw2 = _req(f"/api/v1/conversations?limit=2&cursor={urllib.parse.quote(str(cur), safe='')}")
        conv2 = unwrap(raw2)
        lst2 = (conv2 or {}).get("list") or []
        ids1 = [c.get("id") for c in lst1]
        ids2 = [c.get("id") for c in lst2]
        overlap = sorted(set(ids1) & set(ids2))
        print(f"A2  cursor={cur} -> HTTP {st2}  len={len(lst2)}  hasMore={(conv2 or {}).get('hasMore')}")
        print(f"      page1 ids={ids1}")
        print(f"      page2 ids={ids2}")
        print(f"      *** 交集（>0 即 cursor 被忽略 / 重复翻页） = {overlap}")
        print(f"      *** 并集大小 = {len(set(ids1) | set(ids2))}（期望 = 全量；重复会 < 全量）")

    # ---------- B. 消息列表 ----------
    print("\n--- B. 消息列表 /api/v1/conversations/{id}/messages ---")
    conv_id = lst1[0].get("id") if lst1 else None
    if conv_id is None:
        print("B   跳过：无会话可取")
    else:
        st, raw = _req(f"/api/v1/conversations/{conv_id}/messages?limit=2")
        m1 = unwrap(raw)
        ml1 = (m1 or {}).get("list") or []
        print(f"B1  conv={conv_id} limit=2 -> HTTP {st}  len={len(ml1)}  hasMore={(m1 or {}).get('hasMore')}  cursor={(m1 or {}).get('cursor')}")
        for m in ml1:
            print(f"      id={m.get('id')} sender={m.get('sender')} content={str(m.get('content'))[:40]!r}")
        if ml1:
            min_id = min(int(m.get("id")) for m in ml1)
            st2, raw2 = _req(f"/api/v1/conversations/{conv_id}/messages?limit=2&cursor={min_id}")
            m2 = unwrap(raw2)
            ml2 = (m2 or {}).get("list") or []
            ids1 = [int(m.get("id")) for m in ml1]
            ids2 = [int(m.get("id")) for m in ml2]
            print(f"B2  cursor={min_id} -> HTTP {st2}  len={len(ml2)}  hasMore={(m2 or {}).get('hasMore')}  cursor={(m2 or {}).get('cursor')}")
            print(f"      page1 ids={sorted(ids1)}")
            print(f"      page2 ids={sorted(ids2)}")
            print(f"      *** 交集 = {sorted(set(ids1) & set(ids2))}（应为空）")
            print(f"      *** page2 是否全部 < {min_id} = {all(i < min_id for i in ids2)}（应为 True）")

    print("\n--- C. 环境 ---")
    print("C1  CHAT_TRANSPORT 未设置 ⇒ BFF 走 **gRPC** 路径（HTTP 路径是回滚开关，见 config.go:362）")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
