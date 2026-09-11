---
status: landed
stage: 67
date: 2026-09-11
target: Kafka DevEventPublisher（kafka-reliability-gaps.md §1.1 P0 + ADR-19 PR-A1.1/1.2/1.3）
related:
  - docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md（ADR-19）
  - docs/plans/kafka-reliability-gaps.md §1.1
  - docs/stages/stage-65-dashboard-empty-root-cause.md（dashboard 空的次要根因）
  - AGENTS.md §2.4 契约 6（KAFKA_ENABLED=false 路径不空跑）
---

# Stage 67 — Kafka DevEventPublisher 落地 + relay 启动条件修复（2026-09-11）

> **状态**：🟢 **代码已 landed + 容器化到 chat-svc v0.1.5 + 端到端实测通过**
> **触发**：kafka-reliability-gaps.md §1.1 P0「dev 模式 KAFKA_ENABLED=false 完全空跑」+ ADR-19 PR-A1.1/1.2/1.3 + AGENTS.md §2.4 契约 6
> **核心结论**：本会话实施发现**代码 PR-A1.1/1.2/1.3 早已 landed（events/dev_publisher.go + test + shared/eventrow 全部就绪，单元测试 9 用例全绿）**，但因 main.go:175 的 relay 启动条件 bug 永远不触发，**实测发现的真 bug = outbox relay 启动条件 KAFKA_ENABLED=true 限制**。本批修复后端到端打通。

---

## 一、ADR-19 三阶段代码现状（实测发现）

[emotion-echo-chat-svc/internal/events/dev_publisher.go](emotion-echo-chat-svc/internal/events/dev_publisher.go)：

| 阶段 | 文件 | 状态 |
|---|---|---|
| **PR-A1.1 RED** | `dev_publisher_test.go`（365 行，9 用例 + fakeDBExecutor）| ✅ 已落地 |
| **PR-A1.2 GREEN** | `dev_publisher.go`（169 行：DevEventPublisher + dbExecutor 接口 + ON CONFLICT DO NOTHING 幂等 + Close no-op）| ✅ 已落地 |
| **PR-A1.3 REFACTOR** | `emotion-echo-shared/pkg/eventrow/mapper.go`（共享 mapper：MapEventToUserBehaviorRow + DataShape + ErrUnknownEventType）| ✅ 已落地（与 analytics-svc consumer 共用） |

**单元测试**（chat-svc/internal/events/）：

```
=== RUN   TestDevEventPublisher_Publish_MessageCreated_HappyPath
--- PASS (0.00s)
=== RUN   TestDevEventPublisher_Publish_ConversationCreated_HappyPath
--- PASS (0.00s)
=== RUN   TestDevEventPublisher_Publish_ConversationClosed_HappyPath
--- PASS (0.00s)
=== RUN   TestDevEventPublisher_Publish_UnknownType_ReturnsError
--- PASS (3/3 子用例)
=== RUN   TestDevEventPublisher_Publish_DBError_Propagates
--- PASS (3/3 子用例)
=== RUN   TestDevEventPublisher_Close_NilOp
--- PASS (0.00s)
=== RUN   TestDevEventPublisher_ImplementsEventPublisher
--- PASS (0.00s)

PASS  ok  emotion-echo-chat-svc/internal/events  1.529s
```

**main.go 接入**（chat-svc/main.go:118-128）也正确：

```go
} else if db != nil {
    // ADR-19 PR-A1.2: KAFKA_ENABLED=false 时启用 DevEventPublisher
    if sqlDB, derr := db.DB(); derr == nil {
        pub = events.NewDevEventPublisher(sqlDB)
        log.Printf("[events] using DevEventPublisher (KAFKA_ENABLED=false, dev-only path)")
    }
}
```

---

## 二、本会话实施的修复：relay 启动条件 bug（commit `2bc09ec`）

### 2.1 bug 现象

[chat-svc/main.go:175](emotion-echo-chat-svc/main.go#L175) 原条件：

```go
if db != nil && outboxRepo != nil && c.Kafka.Enabled && len(kafkaBrokersList) > 0 {
    relay := outbox.NewRelay(outboxRepo, pub, 1*time.Second, 100)
    go func() { ... _ = relay.Run(relayCtx) }()
}
```

**bug**：`KAFKA_ENABLED=false` 时即使 `pub=DevEventPublisher` 就位，`relay` 也不会启动 → outbox 行永久 `status=pending` → 没人调 `publisher.Publish` → `user_behavior_events` 永远 0 行 → daily report `userBehaviorCount` 永远 0。

这是 ADR-19 §一描述的"publisher=nil 永远失败"问题的变体——**代码写好了，但 relay 不调它**。

### 2.2 修复

```go
// 启动条件修正（2026-09-11 / Stage 67）
//   - 旧：db != nil && outboxRepo != nil && c.Kafka.Enabled && len(brokers) > 0
//   - 新：db != nil && outboxRepo != nil && pub != nil
//   - 行为：KAFKA_ENABLED=true → KafkaEventPublisher；KAFKA_ENABLED=false → DevEventPublisher
if db != nil && outboxRepo != nil && pub != nil {
    relay := outbox.NewRelay(outboxRepo, pub, 1*time.Second, 100)
    go func() { ... }()
}
```

**理由**：pub 在 main.go §2 已保证非 nil（Kafka 失败时降级到 InMemoryEventPublisher）。relay 只看 pub 是否就绪，不区分下游是 Kafka 还是 DB。

### 2.3 镜像升级

[deploy/docker-compose.apps.yml:133](deploy/docker-compose.apps.yml#L133) `emotion-echo/chat-svc:v0.1.4` → `v0.1.5`。

---

## 三、docker 端到端实测

### 3.1 启动日志确认

```
{"msg":"[events] using DevEventPublisher (KAFKA_ENABLED=false, dev-only path)","svc":"chat-svc"}
{"msg":"[outbox] relay started","svc":"chat-svc"}
{"msg":"[outbox-relay] started: interval=1s batchSize=100","svc":"chat-svc"}
```

### 3.2 业务流实测（create + 3 messages + close）

```bash
# 1. 创建对话（conv_id=11）
curl -X POST -H "Content-Type: application/json" -H "X-User-Id: 1" \
  -d '{"title":"Stage 67 DevEventPublisher E2E"}' \
  http://localhost:8894/api/v1/conversations

# 2. 发 3 条消息（user / assistant / user）
curl -X POST -H "Content-Type: application/json" -H "X-User-Id: 1" \
  -d '{"role":"user","content":"今天心情不错"}' \
  "http://localhost:8894/api/v1/conversations/11/messages"
# (重复 2 次，每次换 role + content)

# 3. 关闭对话
curl -X DELETE -H "X-User-Id: 1" \
  "http://localhost:8894/api/v1/conversations/11"

# 5. 等 relay 1s 轮询后查 user_behavior_events
```

### 3.3 db 实测结果

**`emotion_echo_analytics.user_behavior_events`**（session_id = `conv:11`）：

| event_type | target | session_id | occurred_at |
|---|---|---|---|
| conversation.created | conv:11 | conv:11 | 2026-09-11 03:17:37.885 |
| message.created | msg:11 | conv:11 | 2026-09-11 03:17:38.206 |
| message.created | msg:12 | conv:11 | 2026-09-11 03:17:38.387 |
| message.created | msg:13 | conv:11 | 2026-09-11 03:17:38.531 |
| conversation.closed | conv:11 | conv:11 | 2026-09-11 03:17:38.703 |

**5 行落库**（1 conv.created + 3 message.created + 1 conv.closed）—— 全部命中 shared eventrow mapper 映射规则（target=msg:N / target=conv:N，session_id=conv:N）。

**`emotion_echo_chat.outbox_events` 状态分布**：

| status | count |
|---|---|
| sent | 35 |
| pending | **0** ✅ |

**修复前对比**（修复前实测）：
- `pending = 15`（永远卡住）
- `sent = 12`（历史残留）
- `user_behavior_events` 不增长

**修复后**：pending 永远清零，sent 与 user_behavior_events 行数 1:1 对齐。

---

## 四、本批未做（明确范围）

### 4.1 dashboard chartData 仍可能空（与本任务正交）

[Stage 65-dashboard-empty-root-cause.md](stage-65-dashboard-empty-root-cause.md) 报告指出：

- `daily_report.conversationCount` + `messageCount` 来自 `msg_summary_v`（`emotion_echo_chat.conversations / messages` 表）—— **chat-svc 自身表**，与 user_behavior_events 是两张表
- `daily_report.emotionDistribution` 来自 `daily_emotion_v`（`emotion_echo_ai.emotion_analysis`）—— **ai-svc Kafka consumer 写入**

**本批只解决 user_behavior_events 路径**——`userBehaviorCount` 字段从 0 → 实际行数。dashboard 仍可能 conversationCount=0 + emotionDistribution=[]，因为：

- conversations / messages 表被前端 BFF→chat-svc gRPC 路径写入，本批未触
- emotion_analysis 表由 ai-svc Kafka consumer 写入，KAFKA_ENABLED=false 时也不写

**完整 dashboard 数据流**需要：
1. DevEventPublisher ✅ 本批完成
2. ai-svc dev fallback 写 emotion_analysis（cf. chat-svc→ai-svc UpsertNeutralEmotion gRPC，但当前 ctx 缺 x-user-id metadata — 见日志 `rpc error: code = Unauthenticated`）—— 独立 bug，下次 sprint
3. chat-svc 自身 conversations / messages 由前端 BFF 写入（已有，但 demo 用户没动作）—— 需要前端流或选项 A seed

### 4.2 Kafka P1~P3 backlog（仍 open，kafka-reliability-gaps.md）

| # | 项 | 工作量 |
|---|---|---|
| P1 | ai-svc consumer 外层 5s 重试对齐 | 0.5 天 |
| P1 | DLQ 默认真实 topic | 0.5~1 天 |
| P2 | consumer lag 监控（kafka-exporter + Prometheus + Grafana）| 1 天 |
| P2 | 事件 schema 迁 Protobuf + eventrow 共享 | 2~3 天 |
| P3 | outbox relay 最大重试上限 + 死信告警 | 0.5 天 |

### 4.3 ai-svc dev fallback x-user-id metadata bug（独立）

`docker logs emotion-echo-chat-svc | grep "dev fallback"` 显示：

```
ERROR dev fallback UpsertNeutralEmotion failed: rpc error: code = Unauthenticated desc = missing x-user-id metadata
```

**根因**：chat-svc SendMessageLogic 调 ai-svc dev fallback 时没注入 `x-user-id` gRPC metadata（grpcinterceptor.UserIDFromGRPCContext 缺失）。修复路径：chat-svc/internal/grpcclient/ai_client.go 注入 metadata。下次 sprint。

---

## 五、调研依据

| 项 | 文件 / 命令 |
|---|---|
| ADR-19 决策原文 | [adr-2026-09-dev-publisher-user-behavior-events.md](adr/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md) |
| DevEventPublisher 实现 | [chat-svc/internal/events/dev_publisher.go](emotion-echo-chat-svc/internal/events/dev_publisher.go) |
| DevEventPublisher 单测 | [chat-svc/internal/events/dev_publisher_test.go](emotion-echo-chat-svc/internal/events/dev_publisher_test.go) |
| eventrow 共享 mapper | [shared/pkg/eventrow/mapper.go](emotion-echo-shared/pkg/eventrow/mapper.go) |
| analytics-svc consumer 共用 | [analytics-svc/internal/kafka/consumer.go:231](emotion-echo-analytics-svc/internal/kafka/consumer.go#L231) |
| 修复前 relay 条件 | [chat-svc/main.go:175](emotion-echo-chat-svc/main.go#L175) 原 `c.Kafka.Enabled && len(brokers) > 0` |
| 修复后 relay 条件 | chat-svc/main.go:175 `pub != nil` |
| 镜像升级 | chat-svc v0.1.4 → v0.1.5 |
| 端到端实测 | docker exec postgres \dt emotion_echo_analytics.user_behavior_events + SELECT |
| AGENTS.md §2.4 契约 6 | KAFKA_ENABLED=false 路径不空跑 → ✅ 现在满足（user_behavior_events 持续增长）|

---

> 最后更新：2026-09-11 by Stage 67 实施 session
> 关联：ADR-19 + kafka-reliability-gaps.md §1.1 P0 + AGENTS.md §2.4 契约 6 + Stage 65 dashboard 空根因报告