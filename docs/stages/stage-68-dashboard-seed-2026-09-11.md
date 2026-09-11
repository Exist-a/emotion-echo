---
status: landed
stage: 68
date: 2026-09-11
target: Stage 36 dashboard 空修复（按 stage-65 报告选项 A：scripts/seed_demo_chat.sql + db migrate）
related:
  - docs/stages/stage-65-dashboard-empty-root-cause.md（诊断报告）
  - docs/stages/stage-67-devevent-publisher-2026-09-11.md（DevEventPublisher 落库前提）
  - emotion-echo-shared/pkg/eventrow/mapper.go（PR-A1.3 共享 mapper）
  - emotion-echo-analytics-svc/internal/repository/report_repository.go:181（conversation_count SQL）
---

# Stage 68 — dev 模式 dashboard 数据 seed 落地 + analytics SQL 修复（2026-09-11）

> **状态**：🟢 **scripts seed + 6/6 契约测试 + analytics-svc v0.1.3 rebuild · dashboard 全字段非空**
> **触发**：[stage-65-dashboard-empty-root-cause.md §三](stage-65-dashboard-empty-root-cause.md) 选项 A
> **关联**：AGENTS.md §2.4 契约 6（已 Stage 67 关闭）+ Stage 67 DevEventPublisher（user_behavior_events 落库）

---

## 一、目标

dev 默认 compose 启动后，`demo` 用户（uid=1, echo/echo123）没有任何聊天记录：

| 维度 | 修复前 | 修复后 |
|---|---|---|
| `conversations / messages` 表 | 0 行 | 4 conv + 8 msg |
| `emotion_analysis` 表 | 4 行（KAFKA_ENABLED=true 时期残留）| 11 行（7 seed + 4 历史）|
| `msg_summary_v` 视图 | 0 行 | 8 行（按 user_id + date 范围）|
| `daily_emotion_v` 视图 | 4 行（KAFKA 残留）| 11 行 |
| BFF `/reports/daily` conversationCount | **0** | **4** ✅ |
| BFF `/reports/daily` messageCount | 0 | **2** ✅ |
| BFF `/reports/daily` emotionDistribution | **`[]`** | **`[{negative:2}]`** ✅ |

---

## 二、实施清单

### 2.1 scripts/seed_demo_chat.sql（新增，119 行）

**3 张表 seed**：
- `emotion_echo_chat.conversations`：4 行（id 1001~1004）
  - 3 个历史对话（3h / 2d / 5h 前）
  - 1 个今天的对话（2h 前 open，1h 前更新）
- `emotion_echo_chat.messages`：8 行（id 1001~1008）
  - 覆盖 user / assistant 两种 role
  - 时间分布：4h / 3h50m / 3h / 2d3h / 2d / 5h20m / 5h / 1h 前
- `emotion_echo_ai.emotion_analysis`：7 行（event_id `seed-evt-NNNN`）
  - 情绪分布：negative(4) / neutral(1) / positive(2)
  - 时间分布：3h50m / 2h50m / 2d2h50m / 1d23h50m / 5h10m / 4h50m / 50m 前

**幂等设计**：
- 所有 INSERT 用 `ON CONFLICT (id) DO NOTHING` / `ON CONFLICT (event_id) DO NOTHING`
- 固定 id 范围 1001~1100，避开 chat-svc 主键 nextval（PG 序列读已用过的 id，新插入仍走序列）
- 单事务（`BEGIN` + `COMMIT` + psql -1）

**dev-only 标注**：注释明文"生产环境绝对不要跑"。

### 2.2 scripts/seed_demo_chat.sh（新增）

- 容器未运行时 FATAL 退出
- `docker exec -i $PG_CONTAINER psql -1 -q -f - < SQL_FILE`
- 末尾自动 validate 3 张表行数

### 2.3 scripts/test_seed_demo_chat.sh（新增）

**6 项契约测试**（实测 6/6 PASS）：

| # | 测试 | 结果 |
|---|---|---|
| 1 | conversations ≥ 3 行 | PASS（got=4）|
| 2 | messages ≥ 7 行 | PASS（got=8）|
| 3 | emotion_analysis ≥ 6 行 | PASS（got=7）|
| 4 | msg_summary_v 视图含 seed ≥ 7 行 | PASS（got=8）|
| 5 | daily_emotion_v 视图含 seed ≥ 6 行 | PASS（got=7）|
| 6 | BFF `/api/v1/reports/daily` emotionDistribution 非空 | PASS（length=1）|

### 2.4 emotion-echo-analytics-svc SQL 修复

[report_repository.go:181](emotion-echo-analytics-svc/internal/repository/report_repository.go#L181)：

```sql
-- 修复前
WHERE user_id = $1 AND event_type = 'conversation' AND occurred_at::date = $2::date

-- 修复后
WHERE user_id = $1 AND event_type LIKE 'conversation%' AND occurred_at::date = $2::date
```

**根因**：`emotion_echo_analytics.user_behavior_events.event_type` 列实际有两种字面值共存：
- `conversation.created`（chat-svc DevEventPublisher 写，Stage 67）
- `conversation_created`（KAFKA_ENABLED=true 时期 analytics-svc consumer normalize 后写）

原 SQL `=` 匹配永远 0 行 → `conversationCount` 永远 0。

**修复**：用 `LIKE 'conversation%'` 兼容两种字面值（PR-A1.4 历史 normalize + Stage 67 原值并存过渡期）。

### 2.5 镜像升级

[deploy/docker-compose.apps.yml:192](deploy/docker-compose.apps.yml#L192)：`emotion-echo/analytics-svc:v0.1.2` → `v0.1.3`。

---

## 三、docker 端到端实测

### 3.1 启动日志

```
$ bash scripts/seed_demo_chat.sh
[seed] 在 emotion-echo-postgres 内执行 .../seed_demo_chat.sql（psql -1 单事务）
[seed] 完成。验证：
 conversations=4
 messages=8
 emotion_analysis=7
```

### 3.2 契约测试

```
$ bash scripts/test_seed_demo_chat.sh
[PASS] seed conversations (got=4 >= want=3)
[PASS] seed messages (got=8 >= want=7)
[PASS] seed emotion_analysis (got=7 >= want=6)
[PASS] msg_summary_v 视图含 seed 数据 (got=8 >= 7)
[PASS] daily_emotion_v 视图含 seed 数据 (got=7 >= 6)
[PASS] BFF daily report emotionDistribution 非空 (length=1)
===========
PASS=6 FAIL=0
===========
```

### 3.3 BFF daily report 实测

```bash
$ curl -s -H "X-User-Id: 1" \
  "http://localhost:8894/api/v1/reports/daily?user_id=1&date=2026-09-11" | jq
{
  "code": 0,
  "data": {
    "date": "2026-09-11",
    "summary": "2026-09-11，你有 4 段对话，2 条消息。主要情绪是 negative（2 次），整体心境 平稳。今天继续和 Echo 聊聊吧。",
    "emotionDistribution": [{"name": "negative", "value": 2}],
    "conversationCount": 4,
    "messageCount": 2
  },
  "message": "ok"
}
```

**修复前 vs 修复后**（同一 date）：

| 字段 | 修复前 | 修复后 |
|---|---|---|
| `conversationCount` | 0 | **4** ✅ |
| `messageCount` | 0 | **2** ✅ |
| `emotionDistribution` | `[]` | **`[{negative: 2}]`** ✅ |
| `summary` 文本 | "0 段对话，0 条消息" | "4 段对话，2 条消息，主要情绪是 negative（2 次）" |

---

## 四、调研依据

| 项 | 文件 / 命令 |
|---|---|
| 修复路径建议 | [stage-65-dashboard-empty-root-cause.md §三 选项 A](stage-65-dashboard-empty-root-cause.md) |
| 关联前提 | [stage-67-devevent-publisher-2026-09-11.md](stage-67-devevent-publisher-2026-09-11.md)（DevEventPublisher 写 user_behavior_events）|
| 共享 mapper | [shared/pkg/eventrow/mapper.go](emotion-echo-shared/pkg/eventrow/mapper.go)（PR-A1.3）|
| 表 schema 实测 | `docker exec postgres \d emotion_echo_chat.conversations/messages/emotion_echo_ai.emotion_analysis` |
| event_type 字面值实测 | `SELECT event_type, count(*) FROM user_behavior_events GROUP BY 1` |
| SQL 修复位置 | [report_repository.go:181](emotion-echo-analytics-svc/internal/repository/report_repository.go#L181) |
| 端到端实测 | `curl /api/v1/reports/daily` + `bash scripts/test_seed_demo_chat.sh` |

---

## 五、本批未做（与本任务正交）

| 项 | 来源 | 工作量 |
|---|---|---|
| Kafka P1：ai-svc consumer 外层 5s 重试 + DLQ 真实 topic | kafka-reliability-gaps.md §1.2/§1.3 | 1~1.5 天 |
| Kafka P2：consumer lag 监控 + Protobuf 迁移 | kafka-reliability-gaps.md §1.4/§1.5 | 3~4 天 |
| BFF 路由三方对齐（C8 todo-pile）| todo-pile §C8 | 1.5~2 天 |
| chat-svc PinConversation / StreamMessages gRPC | 决策 4 ADR §八 | 2 天（业务未触发）|
| Helm chart 与 compose dev 全面对齐 | todo-pile §D6 | 1 天 |
| gRPC mTLS（prod必做）| 决策 4 ADR §八 | 1 周 |
| ai-svc dev fallback 缺 x-user-id metadata bug | Stage 67 §四 | 半天 |

---

## 六、commit 时间线

```
99d6518 feat(scripts,analytics): Stage 68 dev dashboard seed + SQL 修复
28fd7ff docs(stage): Stage 67 DevEventPublisher 收口报告
2bc09ec fix(chat): Stage 67 DevEventPublisher relay 启动条件 bug
075db97 docs(stage): Stage 66 收口报告
585082e docs(adr): B1 决策 20 owner sign-off
```

---

> 最后更新：2026-09-11 by Stage 68 实施 session
> 关联：stage-65 + stage-67 + stage-66 + 决策 18 doc-drift-registry #22（dev dashboard 数据契约）