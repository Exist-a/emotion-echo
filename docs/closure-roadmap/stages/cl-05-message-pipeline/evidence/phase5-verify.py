#!/usr/bin/env python3
"""CL-05 Phase 5 · 运行验证（🔴 需 dev 栈）— T-4 会话列表游标翻页

对照物 = `phase0-baseline.py`（改动前）。本脚本断言**修复后**的行为：
  A1 首页不带游标 → 给出非空 nextCursor（服务端给，不是前端推导）
  A2 第二页带 nextCursor → **与第一页无交集**、且全部"更旧"（== 修复前恒相等的那两个 id 现在必须变）
  A3 逐页翻到底 → 并集 = 全量、无重复（不重不漏）
  A4 非法游标 → 400/错误（**不得**静默退回首页）
  B  消息列表 cursor 仍生效（回归：不能被本阶段改坏）

基址：网关 :19080 + Bearer（AGENTS §2.4；不得自传 X-User-Id）。
"""
from __future__ import annotations

import json
import os
import subprocess
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

GATEWAY = os.environ.get("SMOKE_BASE", "http://localhost:19080")
LOGIN_BODY = {"username": "echo", "password": "echo123"}
TOKEN = ""

FAILS: list[str] = []
CHECKS = 0


def check(ok: bool, label: str) -> None:
    global CHECKS
    CHECKS += 1
    print(f"  [{'PASS' if ok else 'FAIL'}] {label}")
    if not ok:
        FAILS.append(label)



def docker_out(args: list[str]) -> str:
    """跑一句 docker 命令取输出（用于让本证据**自证**环境，而不是靠人贴标题）。"""
    import subprocess
    r = subprocess.run(["docker", *args], capture_output=True, text=True)
    if r.returncode != 0:
        return f"<err: {r.stderr.strip()[:120]}>"
    return r.stdout.strip()


def path_fingerprint() -> list[str]:
    """本证据测的是哪条传输路径 —— 必须由**证据自己**给出。

    为什么：核对方（2026-10-16）指出此前 gRPC 与 HTTP 两份输出**除手写标题外逐字节相同**
    ⇒ "两条路径各 17/17" 这条结论**不可核**。修法 = 把 BFF 容器实际的
    `CHAT_TRANSPORT`（以及两个相关镜像）打进输出，让每份证据自带身份。
    """
    transport = docker_out(["exec", "emotion-echo-web-bff", "sh", "-c",
                            "printenv CHAT_TRANSPORT || echo UNSET"]) or "UNSET"
    path = "http(降级路径)" if transport.strip().lower() == "http" else "grpc(默认)"
    return [
        f"BFF CHAT_TRANSPORT = {transport!r}  ⇒  实际走 {path}",
        "BFF image          = " + docker_out(["inspect", "--format", "{{.Config.Image}}", "emotion-echo-web-bff"]),
        "BFF image ID       = " + docker_out(["inspect", "--format", "{{.Image}}", "emotion-echo-web-bff"])[:19],
        "chat-svc image     = " + docker_out(["inspect", "--format", "{{.Config.Image}}", "emotion-echo-chat-svc"]),
        "chat-svc image ID  = " + docker_out(["inspect", "--format", "{{.Image}}", "emotion-echo-chat-svc"])[:19],
    ]


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
        raw = e.read().decode("utf-8", "replace")
        try:
            return e.code, json.loads(raw)
        except Exception:
            return e.code, raw


def unwrap(d: Any) -> Any:
    if isinstance(d, dict) and "data" in d and "code" in d:
        return d.get("data") or {}
    return d


PG_CONTAINER = os.environ.get("PG_CONTAINER", "emotion-echo-postgres")


def psql(sql: str) -> list[str]:
    """在 PG 容器内跑 psql（-tAc 单列裸输出）。失败打印 stderr 并返回空表。

    为什么用 DB 对账：plan §2 的判据是"与 DB ORDER BY id DESC 比对" ——
    只比 API 自己两次调用的结果无法排除"两次都错成一样"。
    """
    r = subprocess.run(
        ["docker", "exec", PG_CONTAINER, "psql", "-U", "postgres", "-d", "emotion_echo", "-tAc", sql],
        capture_output=True, text=True,
    )
    if r.returncode != 0:
        print(f"  [psql ERR] {r.stderr.strip()[:200]}")
        return []
    return [ln.strip() for ln in r.stdout.splitlines() if ln.strip()]


def conv_page(cursor: str, limit: int = 2) -> tuple[int, list, bool, str]:
    q = f"/api/v1/conversations?limit={limit}"
    if cursor:
        q += "&cursor=" + urllib.parse.quote(cursor, safe="")
    st, raw = _req(q)
    d = unwrap(raw)
    return st, (d or {}).get("list") or [], bool((d or {}).get("hasMore")), (d or {}).get("cursor") or ""


def main() -> int:
    global TOKEN
    print("=" * 72)
    print(f"CL-05 Phase 5 运行验证（T-4 会话列表游标） — {GATEWAY}")
    print("=" * 72)

    # ---- P 路径指纹（先打，让整份输出自带身份）----
    print("")
    print("--- P. 路径指纹（本证据自证：测的是哪条传输路径）---")
    for line in path_fingerprint():
        print("  " + line)

    st, raw = _req("/api/v1/auth/login", "POST", LOGIN_BODY)
    if st != 200:
        print(f"[pre] login HTTP {st} —— 前置不可用（无任何契约结论）：{str(raw)[:200]}")
        return 2
    TOKEN = (unwrap(raw) or {}).get("accessToken") or (unwrap(raw) or {}).get("token") or ""
    print(f"[pre] login OK token_len={len(TOKEN)}")
    if not TOKEN:
        return 2

    # ---- A1 首页给出 nextCursor ----
    print("\n--- A. 会话列表游标（T-4）---")
    st, page1, has1, cur1 = conv_page("", 2)
    ids1 = [c.get("id") for c in page1]
    print(f"A1 首页 -> HTTP {st} ids={ids1} hasMore={has1} nextCursor={cur1!r}")
    check(st == 200, "A1 首页 HTTP 200")
    check(len(page1) > 0, "A1 首页非空")
    if has1:
        check(cur1 != "", "A1 hasMore=true 时必须给出**非空** nextCursor（前端推进的唯一依据）")
    else:
        print("  [SKIP] A1 只有一页数据，无法验证 nextCursor")

    if not cur1:
        print("\n!! 无 nextCursor —— 后续 A2/A3 无法进行（数据不足一页）")
        return 1 if FAILS else 0

    # ---- A2 第二页不与第一页重叠 ----
    st, page2, has2, cur2 = conv_page(cur1, 2)
    ids2 = [c.get("id") for c in page2]
    overlap = sorted(set(ids1) & set(ids2))
    print(f"A2 第二页 -> HTTP {st} ids={ids2} hasMore={has2} nextCursor={cur2!r}")
    print(f"      交集 = {overlap}（改动前这里恒等于 {ids1} —— 见 phase0 基线）")
    check(st == 200, "A2 第二页 HTTP 200")
    check(not overlap, "A2 第二页与第一页**无交集**（改动前：cursor 被忽略 ⇒ 恒有交集）")
    check(len(page2) > 0, "A2 第二页非空")

    # ---- A3 翻到底：不重不漏 ----
    # 用 limit=50 走（2 条一页翻 506 条要 253 页，超出页数上限会让本断言假红 —— 上一版就是这么错的）
    seen = list(ids1) + list(ids2)
    cursor = cur2
    pages = 2
    while cursor and pages < 40:
        st, rows, more, cursor = conv_page(cursor, 50)
        if st != 200 or not rows:
            break
        seen.extend([c.get("id") for c in rows])
        pages += 1
        if not more:
            break
    _, allrows, _, _ = conv_page("", 1000)  # 全量（limit 上限内）
    full_ids = [c.get("id") for c in allrows]
    print(f"A3 翻页共 {pages} 页，累计 {len(seen)} 条；全量 {len(full_ids)} 条")
    check(len(seen) == len(set(seen)), "A3 跨页**无重复**")
    check(set(full_ids) == set(seen), "A3 翻页并集 == 全量（不漏）")

    # ---- A4 非法游标必须报错（且是 400 —— 客户端非法输入，不是服务端坏了）----
    st, raw = _req("/api/v1/conversations?limit=2&cursor=not-a-cursor")
    print(f"A4 非法游标 -> HTTP {st}")
    check(st == 400, "A4 非法游标必须 400（**不得**静默退回首页；也不该报 500 让归因跑偏）")

    # ---- B 消息列表回归 ----
    print("\n--- B. 消息列表 cursor 回归（本阶段不得改坏）---")
    if ids1:
        cid = ids1[0]
        st, raw = _req(f"/api/v1/conversations/{cid}/messages?limit=2")
        m1 = unwrap(raw)
        ml1 = (m1 or {}).get("list") or []
        check(st == 200, "B1 消息首页 HTTP 200")
        if ml1:
            min_id = min(int(m.get("id")) for m in ml1)
            st2, raw2 = _req(f"/api/v1/conversations/{cid}/messages?limit=2&cursor={min_id}")
            m2 = unwrap(raw2)
            ml2 = (m2 or {}).get("list") or []
            ids_a = [int(m.get("id")) for m in ml1]
            ids_b = [int(m.get("id")) for m in ml2]
            print(f"B2 cursor={min_id} -> HTTP {st2} page1={sorted(ids_a)} page2={sorted(ids_b)}")
            check(not (set(ids_a) & set(ids_b)), "B2 消息翻页无交集（回归）")
            check(all(i < min_id for i in ids_b), "B2 第二页全部更旧（回归）")

    # ---- C 消息分页与 DB 对账（T-1 / T-2 / T-3）----
    print("\n--- C. 消息分页与 DB 对账（T-1/T-2/T-3）---")
    row = psql("SELECT conversation_id, COUNT(*) c FROM emotion_echo_chat.messages "
               "GROUP BY 1 HAVING COUNT(*) >= 3 ORDER BY c DESC LIMIT 1")
    if not row:
        print("  [SKIP] 库里没有 >=3 条消息的会话")
    else:
        cid = row[0].split("|")[0].strip()
        all_db = [int(x) for x in psql(
            f"SELECT id FROM emotion_echo_chat.messages WHERE conversation_id={cid} ORDER BY id DESC")]
        st, raw = _req(f"/api/v1/conversations/{cid}/messages?limit=2")
        d = unwrap(raw)
        api1 = [int(m["id"]) for m in (d or {}).get("list") or []]
        newest_db = sorted(all_db[:2])  # DB 最新 2 条，升序写法
        print(f"C1 conv={cid} 全量 {len(all_db)}；DB 最新2(升序)={newest_db}；API limit=2 = {api1}")
        check(api1 == newest_db, "C1/T-1 首屏取**最新** N 条（不是最早 N 条）")
        check(api1 == sorted(api1), "C3/T-3 返回体为**时间升序**（最早在前）")

        seen: list[int] = list(api1)
        cursor = min(api1)
        monotonic, more = True, bool((d or {}).get("hasMore"))
        guard = 0
        while more and guard < 60:
            guard += 1
            st, raw = _req(f"/api/v1/conversations/{cid}/messages?limit=2&cursor={cursor}")
            d = unwrap(raw)
            got = [int(m["id"]) for m in (d or {}).get("list") or []]
            if not got:
                break
            if not all(i < cursor for i in got):
                monotonic = False
            seen.extend(got)
            cursor = min(got)
            more = bool((d or {}).get("hasMore"))
        print(f"C2 翻页 {guard} 次，累计 {len(seen)} 条 / DB 全量 {len(all_db)} 条")
        check(monotonic, "C2/T-2 每页都严格更旧（cursor 真生效）")
        check(len(seen) == len(set(seen)), "C2/T-2 跨页**无重复**")
        check(set(seen) == set(all_db), "C2/T-2 翻页并集 == DB 全量（**不漏**）")

    print("\n" + "=" * 72)
    print(f"结论：{CHECKS - len(FAILS)}/{CHECKS} PASS" + ("" if not FAILS else f" —— FAIL: {FAILS}"))
    print("=" * 72)
    return 1 if FAILS else 0


if __name__ == "__main__":
    raise SystemExit(main())
