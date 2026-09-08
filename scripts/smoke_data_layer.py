"""smoke_data_layer.py — Stage 37-A 数据契约 smoke（§2.4 AGENTS.md）

前置：docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d
     所有 svc healthy，postgres / kafka / bff 可用。

实现要点：
- 零 Python 依赖（仅 stdlib + docker CLI + psql）
- PG 操作走 docker exec（PG 端口未暴露到 host，dev compose 默认）
- BFF HTTP 走 urllib
- 每次跑会触发 1 条 message + 1 条 conversation，看完整链路数据落地

约定：
- 退出码 0 = 全 OK
- 退出码 1 = 至少一项 FAIL（print 表里带详细证据）
- SKIP = 跳过（需 integration test 覆盖，单 smoke 不可验）

脚本触发场景见 AGENTS.md §2.4，每个 PR 改动 chat-svc / analytics-svc / BFF / schema 都应跑。
"""

from __future__ import annotations

import json
import subprocess
import sys
import time
import urllib.error
import urllib.request
from typing import Any

# ====== 配置（dev compose 默认）======
BFF = "http://localhost:8894"
PG_CONTAINER = "emotion-echo-postgres"
PG_DB = "emotion_echo"
PG_USER = "postgres"  # superuser，仅 dev；prod 走 analytics_reader
ANALYTICS_READER_USER = "analytics_reader"
LOGIN_BODY = {"username": "echo", "password": "echo123"}

results: list[tuple[str, bool, str]] = []


def check(name: str, ok: bool, detail: str = "") -> None:
    sym = "OK  " if ok else "FAIL"
    results.append((name, ok, detail))
    print(f"[{sym}] {name}: {detail}")


def skip(name: str, detail: str) -> None:
    print(f"[SKIP] {name}: {detail}")
    results.append((name, True, f"SKIP: {detail}"))  # SKIP 不阻塞


def docker_psql(sql: str, user: str = PG_USER) -> tuple[int, str, str]:
    """在 PG 容器内跑 psql。返回 (rc, stdout, stderr)。"""
    proc = subprocess.run(
        ["docker", "exec", PG_CONTAINER, "psql", "-U", user, "-d", PG_DB,
         "-A", "-t", "-c", sql],
        capture_output=True, text=True, timeout=15,
    )
    return proc.returncode, proc.stdout.strip(), proc.stderr.strip()


def http_get(path: str, headers: dict[str, str] | None = None) -> dict[str, Any]:
    req = urllib.request.Request(BFF + path)
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    with urllib.request.urlopen(req, timeout=8) as r:
        return json.loads(r.read())


def http_post(path: str, body: dict, headers: dict[str, str] | None = None) -> dict[str, Any]:
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(BFF + path, data=data, method="POST")
    req.add_header("Content-Type", "application/json")
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    with urllib.request.urlopen(req, timeout=15) as r:
        return json.loads(r.read())


def unwrap(d: dict) -> dict:
    """BFF 用 {code, data, message} 封包；取 .data。"""
    if isinstance(d, dict) and "data" in d and "code" in d:
        return d.get("data") or {}
    return d


# ====== 0. 前置：BFF /health + 触发业务事件 ======
print("=" * 70)
print("Stage 37-A 数据契约 smoke (AGENTS.md §2.4)")
print("=" * 70)

try:
    health = http_get("/health")
    ds = health.get("downstream", {})
    ok_cnt = sum(1 for v in ds.values() if v.get("status") == "ok")
    print(f"\n[pre] BFF /health: status={health.get('status')} downstream_ok={ok_cnt}/{len(ds)}")
    if health.get("status") not in ("ok", "degraded"):
        print("[FATAL] BFF 状态非 ok/degraded，停止 smoke")
        sys.exit(2)
except Exception as e:
    print(f"[FATAL] BFF /health 不可达: {e}")
    sys.exit(2)

# 触发业务事件：login → conv → message
try:
    login_resp = unwrap(http_post("/api/v1/auth/login", LOGIN_BODY))
    user_id = login_resp.get("user", {}).get("id")
    access_token = login_resp.get("accessToken")
    if not user_id:
        print(f"[FATAL] login 失败: {login_resp}")
        sys.exit(2)
    print(f"[pre] login user_id={user_id}")

    conv_resp = unwrap(http_post("/api/v1/conversations",
                                  {"title": "smoke data layer"},
                                  {"X-User-Id": str(user_id)}))
    conv_id = conv_resp.get("id")
    if not conv_id:
        print(f"[FATAL] conv create 失败: {conv_resp}")
        sys.exit(2)
    print(f"[pre] conv_id={conv_id}")

    msg_resp = unwrap(http_post(f"/api/v1/conversations/{conv_id}/messages",
                                 {"role": "user", "content": "smoke 触发业务事件",
                                  "contentType": "text"},
                                 {"X-User-Id": str(user_id)}))
    msg_id = msg_resp.get("id")
    if not msg_id:
        print(f"[FATAL] msg send 失败: {msg_resp}")
        sys.exit(2)
    print(f"[pre] msg_id={msg_id}")

    # 触发 conversation.closed（DELETE）让 §2 能验 conversation_closed enum
    try:
        req = urllib.request.Request(
            BFF + f"/api/v1/conversations/{conv_id}",
            method="DELETE",
        )
        req.add_header("X-User-Id", str(user_id))
        with urllib.request.urlopen(req, timeout=8) as r:
            print(f"[pre] DELETE conv: HTTP {r.status}")
    except Exception as e:
        print(f"[pre] DELETE conv skipped: {e}")

    # 等 5s 让 outbox relay + 异步消费者有机会跑
    print("[pre] sleep 5s 等待 outbox relay + consumer...")
    time.sleep(5)
except Exception as e:
    print(f"[FATAL] 业务事件触发失败: {e}")
    sys.exit(2)


# ====== §契约 1：user_behavior_events 行数 ======
print("\n--- §契约 1: user_behavior_events 行数 ---")
rc, out, err = docker_psql("SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events")
if rc != 0:
    check("§1 行数查询", False, f"psql 失败: {err[:120]}")
else:
    total = int(out or "0")
    # 触发 1 个 conversation + 1 个 message，但 consumer group coordinator 不可用时永远 0
    check("§1 行数 ≥ 1（dev 模式应有数据）", total >= 1,
          f"actual={total} (期望 ≥ 1，0 = analytics-svc consumer 未消费)")

# 附加诊断：outbox 已 sent 但 events 表 0 → 抓"coordinator 不可用"bug
rc_ob, out_ob, err_ob = docker_psql("SELECT COUNT(*) FROM emotion_echo_chat.outbox_events WHERE status='sent'")
outbox_sent = int(out_ob or "0") if rc_ob == 0 else 0
if total >= 1:
    skip("§1 诊断 outbox→events 比", f"outbox_sent={outbox_sent} events={total} (正常)")
else:
    check("§1 诊断 outbox sent 但 events 空（coordinator 不可用或 consumer 未起）", False,
          f"outbox_sent={outbox_sent} events={total} — chat-svc 数据已发出但 analytics-svc 没消费")


# ====== §契约 2：event_type enum 细分 ======
print("\n--- §契约 2: event_type enum 细分 ---")
rc, out, err = docker_psql("SELECT event_type, COUNT(*) FROM emotion_echo_analytics.user_behavior_events GROUP BY 1 ORDER BY 1")
if rc != 0:
    check("§2 enum 分布查询", False, f"psql 失败: {err[:120]}")
else:
    lines = [l for l in out.splitlines() if l.strip()]
    types = {l.split("|")[0] for l in lines}
    # 期望 ≥ 2 种：message + conversation_created（或 conversation_closed）
    # 仅 1 种 = 全是 'conversation' (A3 bug) 或全是 'message'
    distinct = len(types)
    has_message = "message" in types
    has_conv_split = types >= {"conversation_created", "conversation_closed"}
    has_any_conv = any(t.startswith("conversation") for t in types)
    ok = distinct >= 2 and has_message and has_conv_split
    check("§2 event_type enum 细分", ok,
          f"distinct_types={distinct} types={sorted(types)} (期望 ≥ 2 种，含 message + conversation_created/closed)")


# ====== §契约 3：analytics_reader 视图可读 ======
print("\n--- §契约 3: analytics_reader 视图可读 ---")
# 测试已建视图 msg_summary_v / daily_emotion_v / assessment_v + user_behavior_events
views_to_test = [
    ("emotion_echo_chat.msg_summary_v", "msg_summary_v"),
    ("emotion_echo_ai.daily_emotion_v", "daily_emotion_v"),
    ("emotion_echo_assessment.assessment_v", "assessment_v"),
    ("emotion_echo_analytics.user_behavior_events", "user_behavior_events"),
]
for sql_name, short_name in views_to_test:
    rc, out, err = docker_psql(f"SELECT 1 FROM {sql_name} LIMIT 1", user=ANALYTICS_READER_USER)
    if rc == 0:
        check(f"§3 analytics_reader 读 {short_name}", True, f"OK")
    else:
        # permission denied → A4 GRANT 缺失
        check(f"§3 analytics_reader 读 {short_name}", False,
              f"FAIL: {err[:120]}")


# ====== §契约 4：dashboard 数据真有 ======
#
# Stage-53 修复（2026-09-08）：
#   旧策略：固定查"今天"——若 dev 库今天没人发消息/触发 AI 分析，§4 必 fail。
#   新策略：先查最近 7 天内有数据的日期，再以该日期调 /reports/daily。
#   业务语义保留（dashboard 真有数据），但不被"今天是否有数据"卡死。
#   触发真实 LLM 在 smoke 里属于违反 AGENTS.md §三"测试可重现 + 不调外部"
#   （LLM 模型 / 凭据 / 网络抖动都让 smoke 不稳定）。
print("\n--- §契约 4: /reports/daily 数据真有 (最近 7 天窗) ---")
recent_date = None
rc, out, err = docker_psql(
    "SELECT MAX(created_at)::date FROM emotion_echo_ai.emotion_analysis WHERE created_at > NOW() - INTERVAL '7 days'"
)
if rc == 0:
    line = [l for l in out.splitlines() if l.strip() and "|" not in l or l.count("|") >= 1]
    for ln in line:
        parts = [p.strip() for p in ln.split("|") if p.strip()]
        if parts and len(parts[0]) == 10 and parts[0][4] == "-" and parts[0][7] == "-":
            recent_date = parts[0]
            break

if not recent_date:
    check("§4 最近 7 天内有 emotion_analysis 数据", False,
          "MAX(created_at) 空 → dev 库 7 天内无人触发 AI 分析，跳过 §4 dashboard 断言（待业务链路补 LLM trigger 后再跑）")
else:
    check("§4 最近 7 天内有 emotion_analysis 数据", True, f"最近数据日={recent_date}")
    try:
        reports = unwrap(http_get(
            f"/api/v1/reports/daily?user_id={user_id}&date={recent_date}",
            {"X-User-Id": str(user_id)}))
        # ADR-17 修复后 data 形状：{summary, emotionDistribution: [{name, value}], ...}
        if "report" in reports and isinstance(reports["report"], dict):
            reports = reports["report"]
        summary = reports.get("summary", "") or ""
        emo_dist = reports.get("emotionDistribution", [])
        ok = bool(summary.strip()) and len(emo_dist) > 0
        check("§4 /reports/daily 数据真有",
              ok,
              f"date={recent_date} summary={summary!r} emotionDistribution.len={len(emo_dist)} (期望 summary 非空 + len>0)")
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="replace")
        check("§4 /reports/daily", False, f"HTTP {e.code}: {body[:120]}")
    except Exception as e:
        check("§4 /reports/daily", False, str(e))


# ====== §契约 5：schema 与写入端一致性 ======
print("\n--- §契约 5: schema 一致性 (待 integration test) ---")
skip("§5 schema 一致性",
     "需 integration test 覆盖（PR-A1.1 / A2.1 / A3.1 的 RED 测试自动覆盖）")


# ====== §契约 6：KAFKA_ENABLED=false 路径不空跑 ======
# ADR-19 PR-A1.2 落地后,chat-svc 在 KAFKA_ENABLED=false 时启用 DevEventPublisher
# 同步写 user_behavior_events(无需 Kafka)。§契约 6 改为真正跑这条路径,
# 不再依赖 §1 PASS 时 skip。
#
# 决策依据(env 优先级):
#   - KAFKA_ENABLED=false → 必须 user_behavior_events 有行(DevEventPublisher 路径)
#   - KAFKA_ENABLED=true 或未设置 → 必须 §1 + §2 PASS(Kafka 消费者路径)
print("\n--- §契约 6: KAFKA_ENABLED=false 路径不空跑 (ADR-19) ---")
import os  # noqa: E402  # 推迟到此处避免顶部 import 顺位歧义

# 读 chat-svc 容器环境变量(若容器在跑)
kafka_enabled_env = "true"  # 默认假设 Kafka 路径
try:
    env_proc = subprocess.run(
        ["docker", "inspect", "--format",
         "{{range .Config.Env}}{{println .}}{{end}}", "emotion-echo-chat-svc"],
        capture_output=True, text=True, timeout=10,
    )
    if env_proc.returncode == 0:
        for line in env_proc.stdout.splitlines():
            if line.startswith("KAFKA_ENABLED="):
                kafka_enabled_env = line.split("=", 1)[1].strip().lower()
except Exception:
    pass  # docker 不在/容器未跑 → 跳过

if kafka_enabled_env == "false":
    # KAFKA_ENABLED=false → 验证 DevEventPublisher 路径
    # 应有 user_behavior_events 行（chat-svc 同步写,绕开 Kafka）
    rc, out, err = docker_psql(
        "SELECT COUNT(*) FROM emotion_echo_analytics.user_behavior_events "
        "WHERE event_id LIKE 'smoke-%'"
    )
    row_count = 0
    if rc == 0:
        try:
            row_count = int(out.strip().splitlines()[0])
        except (ValueError, IndexError):
            row_count = 0
    if row_count >= 1:
        check("§6 KAFKA_ENABLED=false DevEventPublisher 写入",
              True, f"OK: smoke 触发的事件已写入 user_behavior_events ({row_count} 行)")
    else:
        check("§6 KAFKA_ENABLED=false DevEventPublisher 写入", False,
              "FAIL: KAFKA_ENABLED=false 但 user_behavior_events 0 行。DevEventPublisher 未生效。")
else:
    # KAFKA_ENABLED=true 或未设置 → 旧 §1 路径
    if total >= 1:
        skip("§6 dev 模式消费链路", f"§1 已 PASS,KAFKA_ENABLED={kafka_enabled_env} 走 Kafka 路径")
    else:
        log_proc = subprocess.run(
            ["docker", "logs", "--tail", "500", "emotion-echo-analytics-svc"],
            capture_output=True, text=True, timeout=10,
        )
        log_out = log_proc.stdout + log_proc.stderr
        coordinator_err_count = log_out.count("coordinator is not available")
        consumer_started = "[kafka-consumer]" in log_out

        if coordinator_err_count >= 1:
            check("§6 dev 模式 Kafka consumer group 可用", False,
                  f"FAIL: analytics-svc 日志含 {coordinator_err_count} 次 'coordinator is not available'")
        elif not consumer_started:
            check("§6 dev 模式 Kafka consumer 已启动", False,
                  "FAIL: analytics-svc 日志未发现 [kafka-consumer]")
        else:
            check("§6 dev 模式 Kafka consumer 链路", False,
                  f"FAIL: consumer 启动了但 events=0。日志摘录：{log_out[-200:].strip()[:300]}")


# ====== 汇总 ======
print("\n" + "=" * 70)
ok_cnt = sum(1 for _, ok, _ in results if ok)
fail_cnt = sum(1 for _, ok, _ in results if not ok)
total = len(results)
print(f"汇总: {ok_cnt}/{total} PASS, {fail_cnt} FAIL")
print("=" * 70)

if fail_cnt > 0:
    print("\n失败项汇总（按 A1-A4 修复顺序排列）：")
    fail_items = [(n, d) for n, ok, d in results if not ok]
    for name, detail in fail_items:
        print(f"  - {name}")
        print(f"      {detail}")
    print("\n参考：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)")
    sys.exit(1)

print("\n[OK] 全 PASS — Stage 37-A 数据契约全部满足")
sys.exit(0)
