#!/usr/bin/env python3
"""eval_memory_recall.py — CL-08a · T-19 记忆价值证明（评测集 + 开/关 A/B）

目的：给出**数字**，而不是"感觉记得更好了"。四类判定：

  1. 跨会话召回   —— 在「我的记忆」写过的事实，新会话里问起能复述
  2. 事实更新     —— 同一键的新值生效（旧值不再被引用）
  3. 不相关不注入 —— 问与记忆无关的问题，回复里**不出现**已存事实
  4. 开/关 A/B    —— 删掉用户级摘要（注入为空）后，同一问题不再复述该事实

前置（与 smoke_data_layer.py 同）：
  - dev 栈已起，经 **APISIX 网关** `http://localhost:19080`（`EVAL_BASE` 可覆盖）
  - 网关登录可用（脚本自建独立评测账号 `mem_eval`，不动 echo 的数据）
  - 本脚本会清理**评测账号自己**的记忆行与 Redis 摘要（不会碰其他用户）

用法：
  python scripts/eval_memory_recall.py

退出码：0 = 全部通过；1 = 有 FAIL；2 = 前置不可用（登录/建号失败）
"""

from __future__ import annotations

import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.request

BASE = os.environ.get("EVAL_BASE", "http://localhost:19080")
EVAL_USER = os.environ.get("EVAL_USER", "mem_eval")
EVAL_PASS = os.environ.get("EVAL_PASS", "mem_eval_123")

PG_CONTAINER = "emotion-echo-postgres"
REDIS_CONTAINER = "emotion-echo-redis"
PG_DB = "emotion_echo"

TOKEN = ""
USER_ID = 0

results: list[tuple[str, str, str]] = []  # (name, verdict, detail)


def http(method: str, path: str, body=None, token: str | None = None, timeout: int = 20):
    url = BASE + path
    data = None
    headers = {"Content-Type": "application/json; charset=utf-8"}
    if body is not None:
        data = json.dumps(body, ensure_ascii=False).encode("utf-8")
    if token:
        headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        raw = e.read().decode("utf-8", "replace")
        raise RuntimeError(f"HTTP {e.code} {path}: {raw[:300]}")
    return json.loads(raw) if raw else {}


def unwrap(resp):
    if resp.get("code") not in (0, None):
        raise RuntimeError(f"业务错误: {resp}")
    return resp.get("data")


def psql(sql: str) -> str:
    out = subprocess.run(
        ["docker", "exec", PG_CONTAINER, "psql", "-U", "postgres", "-d", PG_DB, "-t", "-A", "-c", sql],
        capture_output=True, text=True, timeout=30,
    )
    if out.returncode != 0:
        raise RuntimeError(f"psql failed: {out.stderr[:300]}")
    return out.stdout.strip()


def redis_del(key: str) -> None:
    subprocess.run(["docker", "exec", REDIS_CONTAINER, "redis-cli", "DEL", key],
                   capture_output=True, text=True, timeout=15)


def login_or_register() -> None:
    global TOKEN, USER_ID
    # 1) 先尝试登录（重跑场景）
    try:
        data = unwrap(http("POST", "/api/v1/auth/login", {"username": EVAL_USER, "password": EVAL_PASS}))
        TOKEN = data["accessToken"]
        USER_ID = int(data["user"]["id"])
        print(f"[pre] 登录评测账号 OK user_id={USER_ID}")
        return
    except Exception:
        pass

    # 2) 注册（dev 回显验证码）
    #    注意：/auth/verification-code 有 60s 同用户名重发间隔（防枚举）⇒ 拿不到 devCode 时等下一轮。
    code = ""
    for _ in range(10):
        vc = unwrap(http("POST", "/api/v1/auth/verification-code", {"username": EVAL_USER})) or {}
        code = vc.get("devCode", "")
        if code:
            break
        time.sleep(10)
    if not code:
        raise RuntimeError("拿不到 devCode（60s 重发间隔未过，或 BFF_DEV_RETURN_CODE 未开启）")
    try:
        unwrap(http("POST", "/api/v1/auth/register", {
            "username": EVAL_USER, "password": EVAL_PASS, "verificationCode": code,
        }))
    except Exception as e:
        if "409" not in str(e):
            raise
    data = unwrap(http("POST", "/api/v1/auth/login", {"username": EVAL_USER, "password": EVAL_PASS}))
    TOKEN = data["accessToken"]
    USER_ID = int(data["user"]["id"])
    print(f"[pre] 注册并登录评测账号 OK user_id={USER_ID}")


def reset_eval_memory() -> None:
    psql(f"DELETE FROM emotion_echo_ai.memory_fact WHERE user_id={USER_ID};")
    psql(f"DELETE FROM emotion_echo_ai.memory_note WHERE user_id={USER_ID};")
    psql(f"DELETE FROM emotion_echo_ai.memory_extract_run WHERE user_id={USER_ID};")
    redis_del(f"memory:summary:user:{USER_ID}")
    print(f"[pre] 已清理评测账号（user_id={USER_ID}）的记忆行与摘要")


def create_conversation() -> int:
    last = None
    for _ in range(3):
        try:
            data = unwrap(http("POST", "/api/v1/conversations", {"title": "T-19 评测"}, token=TOKEN))
            return int(data["id"])
        except Exception as e:  # 首次调用可能撞 5s 截止（E2E-F-115）
            last = e
            time.sleep(3)
    raise RuntimeError(f"create conversation failed: {last}")


def ask(question: str) -> str:
    """新建会话 + 发一条消息，返回 AI 回复全文。"""
    conv = create_conversation()
    payload = {
        "conversationId": str(conv),
        "message": question,
        "messages": [{"role": "user", "content": question}],
        "stream": True,
    }
    url = BASE + "/api/v1/ai/stream"
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST", headers={
        "Content-Type": "application/json; charset=utf-8",
        "Authorization": "Bearer " + TOKEN,
    })
    chunks: list[str] = []
    with urllib.request.urlopen(req, timeout=120) as resp:
        for raw in resp:
            line = raw.decode("utf-8", "replace").strip()
            if not line.startswith("data:"):
                continue
            body = line[5:].strip()
            if body in ("", "[DONE]"):
                continue
            try:
                obj = json.loads(body)
            except json.JSONDecodeError:
                continue
            delta = ((obj.get("choices") or [{}])[0].get("delta") or {}).get("content")
            if delta:
                chunks.append(delta)
    return "".join(chunks)


def record(name: str, ok: bool, detail: str) -> None:
    results.append((name, "PASS" if ok else "FAIL", detail))
    print(f"[{'PASS' if ok else 'FAIL'}] {name}: {detail[:160]}")


SEEDS = [
    ("我叫阿哲，是一名后端工程师。", "阿哲"),
    ("我养了一只叫团子的橘猫。", "团子"),
    ("我现在住在杭州。", "杭州"),
    ("我每周跑三次步，每次五公里。", "跑"),
]

RECALL = [
    ("你还记得我叫什么吗？", "阿哲"),
    ("你还记得我的猫叫什么吗？", "团子"),
    ("你还记得我现在住在哪座城市吗？", "杭州"),
    ("你还记得我的运动习惯吗？", "跑"),
]

UPDATE_NOTE = "我上个月从杭州搬到成都了。"
UPDATE_EXPECT_NEW = "成都"
UPDATE_EXPECT_OLD = "杭州"


def graph_objects() -> set[str]:
    """当前图上的实体标签集合（图只含 active 事实）。"""
    try:
        data = unwrap(http("GET", "/api/v1/memory/graph", token=TOKEN))
    except Exception:
        return set()
    return {n.get("label", "") for n in ((data or {}).get("nodes") or []) if n.get("kind") != "user"}


def wait_for_objects(expected: list[str], absent: list[str] | None = None, timeout: int = 120) -> tuple[bool, set[str]]:
    """轮询图，直到 expected 全部出现（子串匹配）且 absent 全部消失（图只展示 active 事实）。"""
    absent = absent or []
    deadline = time.time() + timeout
    labels: set[str] = set()
    while time.time() < deadline:
        labels = graph_objects()
        text = "|".join(sorted(labels))
        if all(e in text for e in expected) and not any(a in text for a in absent):
            return True, labels
        time.sleep(3)
    return False, labels


def main() -> int:
    print(f"== CL-08a T-19 记忆评测（base={BASE}）==")
    try:
        login_or_register()
        reset_eval_memory()
    except Exception as e:
        print(f"[FATAL] 前置失败：{e}")
        return 2

    # 1) 写入自述
    for content, _ in SEEDS:
        try:
            unwrap(http("POST", "/api/v1/memory/notes", {"content": content}, token=TOKEN))
        except Exception as e:
            print(f"[FAIL] 写入自述失败：{e}")
            return 1
    ok, labels = wait_for_objects(["阿哲", "团子", "杭州", "跑"])
    record("事实落图", ok, f"实体={sorted(labels)}")

    # 2) 跨会话召回
    for question, expect in RECALL:
        reply = ask(question)
        record(f"召回：{question}", expect in reply, reply.replace("\n", " ")[:120])

    # 3) 不相关不注入（**观察项**：本阶段设计 = 全量摘要常带注入，
    #    按需相关性检索归 CL-08c ⇒ 这里记录现象，不作为通过门槛）
    unrelated = ask("今天天气怎么样？")
    leaked = [kw for kw in ("阿哲", "团子", "杭州") if kw in unrelated]
    results.append(("不相关不注入（观察）", "N/A", f"提及={leaked or '无'}"))
    print(f"[N/A ] 不相关不注入（观察，按需检索归 CL-08c）：提及={leaked or '无'}；"
          f"回复={unrelated.replace(chr(10),' ')[:110]}")

    # 4) 事实更新（等到图上成都出现、杭州消失 —— 图只展示 active）
    unwrap(http("POST", "/api/v1/memory/notes", {"content": UPDATE_NOTE}, token=TOKEN))
    ok, labels = wait_for_objects([UPDATE_EXPECT_NEW], absent=[UPDATE_EXPECT_OLD])
    record("事实更新：图上新值生效且旧值下线", ok, f"实体={sorted(labels)}")
    updated = ask("你现在知道我住在哪座城市吗？")
    record("事实更新：新值生效", UPDATE_EXPECT_NEW in updated, updated.replace("\n", " ")[:120])
    record("事实更新：旧值不再引用", UPDATE_EXPECT_OLD not in updated, updated.replace("\n", " ")[:120])

    # 5) 开/关 A/B（删摘要 = 注入为空）
    #    用**开放式**问题 + 稀有标记（团子）：避免"我的猫叫什么"这种题目本身诱导猜名。
    on_reply = ask("你还记得关于我的哪些事？尽量具体说说。")
    record("A/B：开记忆能说出具体事实", "团子" in on_reply, on_reply.replace("\n", " ")[:120])
    redis_del(f"memory:summary:user:{USER_ID}")
    off_reply = ask("你还记得关于我的哪些事？尽量具体说说。")
    record("A/B：关记忆后不复述", "团子" not in off_reply, off_reply.replace("\n", " ")[:120])

    passed = sum(1 for _, v, _ in results if v == "PASS")
    failed = sum(1 for _, v, _ in results if v == "FAIL")
    na = sum(1 for _, v, _ in results if v == "N/A")
    print(f"\n== 汇总：PASS={passed} FAIL={failed} N/A={na} ==")
    for name, verdict, _ in results:
        print(f"  {verdict:4}  {name}")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    try:
        sys.exit(main())
    except KeyboardInterrupt:
        sys.exit(130)
