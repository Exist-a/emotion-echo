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
LOGIN_BODY = {"username": "13800138000", "password": "abc123"}

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
print("\n--- §契约 4: /reports/daily 数据真有 ---")
try:
    reports = unwrap(http_get(f"/api/v1/reports/daily?user_id={user_id}",
                              {"X-User-Id": str(user_id)}))
    # ADR-17 修复后 data 形状：{summary, emotionDistribution: [{name, value}], ...}
    # 修复前是 {report: {summary, ...}}
    if "report" in reports and isinstance(reports["report"], dict):
        reports = reports["report"]
    summary = reports.get("summary", "") or ""
    emo_dist = reports.get("emotionDistribution", [])
    ok = bool(summary.strip()) and len(emo_dist) > 0
    check("§4 /reports/daily 数据真有",
          ok,
          f"summary={summary!r} emotionDistribution.len={len(emo_dist)} (期望 summary 非空 + len>0)")
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
print("\n--- §契约 6: dev 模式消费链路可达性 ---")
# 综合诊断：若 §1 FAIL，则必须给出 actionable 修复方向
# 优先级：logs 中的 coordinator 错误 > outbox_sent > events
if total >= 1:
    skip("§6 dev 模式消费链路", f"§1 已 PASS，无需进一步诊断")
else:
    log_proc = subprocess.run(
        ["docker", "logs", "--tail", "500", "emotion-echo-analytics-svc"],
        capture_output=True, text=True, timeout=10,
    )
    # Go logger 把日志打到 stderr；docker logs 默认合并但 subprocess 不会
    log_out = log_proc.stdout + log_proc.stderr
    coordinator_err_count = log_out.count("coordinator is not available")
    consumer_started = "[kafka-consumer]" in log_out

    if coordinator_err_count >= 1:
        check("§6 dev 模式 Kafka consumer group 可用", False,
              f"FAIL: analytics-svc 日志含 {coordinator_err_count} 次 'coordinator is not available'（last 500 lines）。真因：Kafka dev 集群 consumer group coordinator 不可用，analytics-svc 永远消费不到 chat-events topic。修复方向：(1) Kafka broker 端 `auto.create.topics.enable=true` + `offsets.topic.replication.factor=1`（dev 单节点 Kafka）；(2) 或 chat-svc 起 dev publisher 同步写 user_behavior_events（绕开 Kafka）。")
    elif not consumer_started:
        check("§6 dev 模式 Kafka consumer 已启动", False,
              "FAIL: analytics-svc 日志未发现 [kafka-consumer] 标记——Kafka consumer goroutine 没起来。修复方向：检查 analytics-svc main.go Kafka.Enabled 启动条件。")
    else:
        check("§6 dev 模式 Kafka consumer 链路", False,
              f"FAIL: consumer 启动了但 events=0，原因待排查。日志摘录：{log_out[-200:].strip()[:300]}")


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
