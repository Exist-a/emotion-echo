---
status: landed
stage: 65
date: 2026-09-11
target: todo-pile §C7 Stage 36 dashboard chartData.length===0 根因诊断
related:
  - docs/plans/todo-pile-2026-09-04.md §C7
  - docs/stages/stage-36-followup-closure.md
  - docs/plans/kafka-reliability-gaps.md §1.1 DevEventPublisher
  - ADR-19 adr-2026-09-dev-publisher-user-behavior-events.md
---

# Stage 65 — dev 模式 dashboard chartData 空根因诊断（2026-09-11）

> **状态**：🟢 **纯诊断报告 · 根因锁定 · 不改代码**
> **触发**：todo-pile §C7 — Stage 36-FU 报告 16/16 smoke 绿，但 dev 模式4 个 dashboard `chartData.length === 0`。
> **结论**：

---

## 一、现状实测（2026-09-11 docker 端到端）

### 1.1 视图清单（db 实际状态）

`docker exec emotion-echo-postgres psql -c "SELECT schemaname, viewname FROM pg_views WHERE schemaname LIKE 'emotion_echo%'"`

| 视图 | 所在 schema | 行数（demo 用户）|
|---|---|---|
| `msg_summary_v` | `emotion_echo_chat` | **0** |
| `daily_emotion_v` | `emotion_echo_ai` | 4（KAFKA_ENABLED=true 时期遗留）|
| `daily_emotion_by_modality_v` | `emotion_echo_ai` | 2（同上）|
| `assessment_v` | `emotion_echo_assessment` | 0 |

### 1.2 底层表行数

```
emotion_analysis      |  4
user_behavior_events | 12
conversations         |  0    ← demo 用户没真实聊天
messages              |  0    ← demo 用户没真实聊天
outbox_events_pending | 15    ← KAFKA_ENABLED=false 永远不消费
outbox_events_sent    | 12    = user_behavior_events 行数
```

### 1.3 BFF 端到端实测

```bash
TOK=$(curl -s -X POST http://localhost:8894/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"echo","password":"echo123"}' | jq -r .data.accessToken)
curl -s -H "X-User-Id: 1" \
  "http://localhost:8894/api/v1/reports/daily?user_id=1&date=2026-09-11" | jq
```

**实际响应**：

```json
{
  "code": 0,
  "data": {
    "date": "2026-09-11",
    "summary": "2026-09-11，你有 0 段对话，0 条消息。整体心境 平稳。今天继续和 Echo 聊聊吧。",
    "emotionDistribution": [],
    "conversationCount": 0,
    "messageCount": 0
  }
}
```

**chartData.length === 0 实测复现**。

---

## 二、根因分析

### 2.1 daily report 的数据流

[`emotion-echo-analytics-svc/internal/repository/report_repository.go:175-205`](emotion-echo-analytics-svc/internal/repository/report_repository.go#L175) `PostgresReportRepo.GetDailyReport` SQL：

```sql
SELECT
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_chat.msg_summary_v ...), 0) AS conversation_count,
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_chat.msg_summary_v ...), 0) AS message_count,
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_analytics.user_behavior_events ...), 0) AS user_behavior_count,
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_assessment.assessment_v ...), 0) AS assessment_count,
    COALESCE((SELECT AVG(sentiment_score)::float8 FROM emotion_echo_ai.daily_emotion_v ...), 0) AS avg_sentiment,
    COALESCE((SELECT AVG(confidence)::float8 FROM emotion_echo_ai.daily_emotion_v ...), 0) AS avg_confidence
```

**关键依赖**：

| 输出字段 | 数据源 | 当前行数 | dashboard 表现 |
|---|---|---|---|
| `conversationCount` | `msg_summary_v`（依赖 `emotion_echo_chat.conversations`）| 0 | "0 段对话" |
| `messageCount` | `msg_summary_v`（依赖 `emotion_echo_chat.messages`）| 0 | "0 条消息" |
| `emotionDistribution` | `daily_emotion_v` 聚合 | 4 行 | `emotionDistribution: []`（聚合后按 user_id=1 / date=2026-09-11 过滤为 0）|

### 2.2 根因锁定

**`msg_summary_v` 依赖的 `emotion_echo_chat.conversations / messages` 两表都 0 行**。

**这与 KAFKA_ENABLED 无关**——`/api/v1/reports/daily` 走的是 chat-svc 自身的 `conversations / messages` 表，与 `emotion_echo_analytics.user_behavior_events`（Kafka 写入）无关。

**真正根因**：dev demo 用户（`echo / echo123`，uid=1）从来没有真实对话流量。
- `conversations = 0` ↔ demo 用户没点过"开始新对话"
- `messages = 0` ↔ demo 用户没发过消息
- `emotion_analysis = 4`（历史KAFKA_ENABLED=true 时期测试残留）
- `user_behavior_events = 12`（同上）

### 2.3 Stage 36 报告的误判

Stage 36-FU 报告"smoke 16/16 全绿，但 dev 模式 4 dashboard chartData.length===0" 时归因于"Kafka off 导致 user_behavior_events 无数据"。

**实际**：
- daily report **不依赖** `user_behavior_events`（聚合 message/conversation 计数）
- weekly / monthly / annual report 同理（都基于 `msg_summary_v` 或 conversations 表）
- emotion_distribution 维度确实依赖 `daily_emotion_v`（来自 `emotion_analysis`，走 Kafka 路径）—— 但 KAFKA_ENABLED=true 时期已有 4 行残留，所以分布**非空**

**修正路径**：Stage 36 把 4 dashboard 都归到"Kafka off"是过度推断。**真正归因**是：

| dashboard 字段 | 依赖数据源 | 现状 | 是否与 KAFKA 相关 |
|---|---|---|---|
| conversationCount / messageCount | `msg_summary_v` | 0 | ❌（不依赖 Kafka）|
| emotionDistribution | `daily_emotion_v` → `emotion_analysis` | 4（按 user+date 过滤后 0）| ✅（依赖 Kafka consumer）|
| userBehaviorCount | `user_behavior_events` | 12（按 user+date 过滤后可能 0）| ✅（依赖 Kafka consumer）|

**混合原因**：count 类字段（无 Kafka 依赖）+ 分布类字段（有 Kafka 依赖）叠加。

---

## 三、修复路径（建议选项）

### 3.1 选项 A：dev 模式 seed 历史聊天数据（半天）

新建 `scripts/seed_demo_chat.sql`，启动时通过 db-migrate 容器或独立 job 执行：

```sql
-- demo 用户 echo (uid=1) 历史聊天记录
INSERT INTO emotion_echo_chat.conversations (user_id, title, message_count, last_message_at, status, created_at, updated_at)
VALUES
  (1, '工作压力倾诉', 3, NOW() - INTERVAL '3 hours', 1, NOW() - INTERVAL '4 hours', NOW() - INTERVAL '3 hours'),
  (1, '睡眠质量讨论', 2, NOW() - INTERVAL '2 days', 1, NOW() - INTERVAL '2 days 4 hours', NOW() - INTERVAL '2 days');

INSERT INTO emotion_echo_chat.messages (conversation_id, user_id, role, content, content_type, tokens_used, created_at)
VALUES
  (1, 1, 'user', '最近项目 deadline 太紧', 'text', 0, NOW() - INTERVAL '4 hours'),
  (1, 1, 'assistant', '深呼吸，先聊聊哪部分压力最大？', 'text', 0, NOW() - INTERVAL '3 hours 50 minutes'),
  ...
```

**优点**：dashboard 立刻有数据，E2E 可视化验收
**预估**：30 分钟（含 dev-compose 接入）

### 3.2 选项 B：dev 模式 demo 登录后自动 bot reply（半天）

修改 `emotion-echo-web-bff/internal/handler/auth_handler.go` `LoginHandler`：demo 用户（uid=1）登录后端到端执行：

1. `POST /api/v1/conversations` 新建一条"自动化演示对话"
2. `POST /api/v1/conversations/{id}/messages` 发 user message + ai-svc dev fallback 自动回复
3. ai-svc fallback 写 `emotion_analysis`（dev fallback 路径）

**优点**：从源头产生数据，"用户行为即测试"
**预估**：半天（含 e2e 验证）
**风险**：与 Stage 36-A3.2 dev fallback 的 ai-svc 改动耦合

### 3.3 选项 C：不修复（保留根因记录）

接受 dev 模式 dashboard 空是"无真实流量"的合理表现。E2E 测试可在 Stage 36 模式（KAFKA_ENABLED=true + dev seed）下验证。

**优点**：零工作量
**缺点**：新人 onboard 看到空 dashboard 困惑（cf. todo-pile §C7 描述）

### 3.4 建议

**采用选项 A + 选项 B 混合**：
- 选项 A：1 个 SQL 脚本 + db-migrate 接入，半小时
- 选项 B：登录后自动 bot 1 次对话，半天

合计 1 天工作量。**下次 sprint 排期**。

---

## 四、本报告未做（明确留给下次 sprint）

- ❌ 不修复 dashboard 空数据（按选项 A+B 后续排期）
- ❌ 不补 Kafka DevEventPublisher（[kafka-reliability-gaps.md §1.1](docs/plans/kafka-reliability-gaps.md) 独立 P0）
- ❌ 不改 daily report SQL（当前 SQL 正确，问题在数据源）
- ❌ 不写 E2E dashboard 验证脚本（待修复后）

---

## 五、调研依据

| 项 | 文件 / 命令 |
|---|---|
| 当前 4 dashboard 端点 | [dailyReport.vue:65](emotion-echo-web/app/pages/chat/dashboard/dailyReport.vue#L65) / `weeklyReport.vue:70` / `monthlyReport.vue` / `annualReport.vue:63` 都调 `get<Daily|Weekly|Monthly|Annual>Report` → BFF `/api/v1/reports/{daily|trend}` |
| BFF→analytics-svc gRPC | `web-bff/internal/downstream/analytics_grpc.go` ReportsDaily / ReportsTrend |
| analytics-svc logic | [logic/reports_daily_logic.go](emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go) `GetDailyReport` |
| analytics-svc SQL | [repository/report_repository.go:175-205](emotion-echo-analytics-svc/internal/repository/report_repository.go#L175) |
| msg_summary_v 定义 | `deploy/db/04-create-views.sql` + `emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql` |
| 当前 db 视图状态 | `docker exec emotion-echo-postgres psql -c "SELECT * FROM pg_views WHERE schemaname LIKE 'emotion_echo%'"` |
| 当前 db 表行数 | `docker exec emotion-echo-postgres psql -c "SELECT count(*) FROM ..."` |
| KAFKA_ENABLED 路径 | [kafka-reliability-gaps.md §1.1](docs/plans/kafka-reliability-gaps.md) + ADR-19 |

---

> 最后更新：2026-09-11 by Stage 65 诊断 session
> 关联：决策 18 #32 + todo-pile §C7 + kafka-reliability-gaps P0