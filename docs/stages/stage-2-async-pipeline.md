# Emotion-Echo · Stage 2 异步管道完成报告

> ⚠️ **架构决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档保留为历史过程记录（2026-07-13 当时状态）。
> **当前已变更**：go-zero → Gin（不影响 Kafka 管道逻辑）。

> 2026-07-13：chat-svc ↔ Kafka ↔ ai-svc 异步管道首次完整跑通。

## 🏆 战果

| 项 | 内容 |
|----|------|
| 业务端点 | `POST /api/v1/conversations` + `POST/GET /api/v1/conversations/:id/messages` |
| 事件总线 | Kafka topic `chat-events`，事件 schema 借鉴 CloudEvents |
| 同步部分 | chat-svc 写 DB + 发 Kafka（一个请求） |
| 异步部分 | ai-svc Consumer Group 消费 → Analyzer → 写 emotion_analysis 表 |
| 端到端验证 | 浏览器→APISIX→chat-svc→Kafka→ai-svc→DB，**全链路通** |

## 🔴🟢 TDD 闭环

```
chat-svc/repository:        5 PASS（Create/Get/Append/List/Increment/Ping）
chat-svc/logic:             8 PASS
  ├─ TestCreateConversationLogic_WithTitle_PublishesEvent
  ├─ TestCreateConversationLogic_EmptyTitle
  ├─ TestCreateConversationLogic_NoUserID_Returns401
  ├─ TestSendMessageLogic_PersistsAndPublishes
  ├─ TestSendMessageLogic_ConversationNotFound_Returns404
  ├─ TestSendMessageLogic_EmptyContent_ReturnsValidationError
  ├─ TestHealthLogic_Health_OK
  └─ TestHealthLogic_Health_NoKafka_Degraded
ai-svc/analyzer:            4 PASS（happy/sad/neutral/empty）
ai-svc/logic:               5 PASS（happy path/sad text/non-user skip/wrong type/bad data）
ai-svc/repository:          3 PASS
                   ─────────────────
                   总计 25 PASS
```

## 🎯 完整异步管道架构

```
        浏览器
          │  POST /api/v1/conversations/1/messages
          ▼
      APISIX (route r-msg-send)
          │
          ▼
      chat-svc (8889)
        │ ① 落库: emotion_echo_chat.messages (id=1)
        │ ② 增计数: emotion_echo_chat.conversations.message_count++
        │ ③ 发 Kafka: chat-events topic, message.created event
          │
          ▼
        Kafka broker
          │
          ▼
      ai-svc Consumer Group "ai-svc"
        │ ① 收 message.created event
        │ ② JSON 反序列化 MessageCreatedData
        │ ③ analyzer.Analyze(content) → emotion="happy" score=0.60
        │ ④ 写 emotion_echo_ai.emotion_analysis (id=1)
          │
          ▼
      （下一步：GET /api/v1/emotion/conversation/:id 由 ai-svc 或前端直查）
```

## 📁 新增/修改文件

```
emotion-echo-chat-svc/
├── chat.api                                ← 加 path:"id" + 删 options
├── chat.go                                 ← 加 Kafka producer 接线
├── internal/
│   ├── events/
│   │   ├── events.go                       ← Event/Topic/MessageCreatedData
│   │   └── kafka_publisher.go              ← sarama SyncProducer 实现
│   ├── middleware/auth.go                  ← X-User-Id → ctx
│   ├── logic/
│   │   ├── createconversationlogic.go      ← 业务实现
│   │   ├── sendmessagelogic.go             ← 含 publish event
│   │   ├── listmessageslogic.go
│   │   └── healthlogic.go                  ← 含 KafkaOK
│   ├── repository/                         ← 5 PASS 测试
│   └── svc/servicecontext.go               ← 加 EventPublisher 字段

emotion-echo-ai-svc/
├── ai.go                                   ← 加 Kafka consumer 接线
├── etc/ai-api.yaml                         ← 加 Kafka 段
├── internal/
│   ├── analyzer/                           ← 关键词分析器 + 4 PASS 测试
│   │   ├── analyzer.go
│   │   └── analyzer_test.go
│   ├── consumer/consumer.go                ← sarama ConsumerGroup 封装
│   ├── events/events.go                    ← 同构的事件 schema
│   └── logic/consumehandler.go             ← 5 PASS 测试

deploy/apisix/
├── apisix-r-conv-create.json               ← POST /api/v1/conversations
├── apisix-r-msg-send.json                  ← POST /api/v1/conversations/*/messages
└── apisix-r-msg-list.json                  ← GET /api/v1/conversations/*/messages

deploy/db/02-create-tables-in-schemas.sql   ← 需要 ALTER 加 title 列（首次跑缺）
```

## 📊 端到端验证日志

```
2026-07-14 11:59:02 [chat-svc] msg1 published to chat-events
2026-07-14 11:59:02 [ai-svc]   analyzed messageID=1 emotion=happy    score=0.60
2026-07-14 11:59:02 [chat-svc] msg2 published to chat-events
2026-07-14 11:59:02 [ai-svc]   analyzed messageID=2 emotion=anxious  score=-0.40

emotion_echo_ai.emotion_analysis:
  id | message_id | user_id | primary_emotion | score | model
 ----+------------+---------+-----------------+-------+-----------------
  1 |          1 |       1 | happy           |  0.60 | keyword-stub-v1
  2 |          2 |       1 | anxious         | -0.40 | keyword-stub-v1
```

## 🎓 这次的核心认知（白盒审计友好）

| 概念 | 体验 |
|------|------|
| **godoc 每个公开符号** | `NewMessageCreatedHandler`、`MessageCreatedData` 等都有完整注释 |
| **错误显式处理** | 不吞 err、不用 `_ = x`（除 `defer kp.Close()` 兜底） |
| **接口边界清晰** | `EventPublisher`、`ConversationRepo`、`Analyzer`、`Consumer` 都接口化 |
| **生产/测试双实现** | Kafka producer/consumer 与 InMemory 版本可互换 |
| **事件 schema 集中管理** | `events.go` 包是契约，chat-svc 和 ai-svc 共享语义 |
| **Topic 常量化** | `TopicChatEvents = "chat-events"` 不散落字符串 |

## ⚠️ 这一轮踩到的坑（白盒审计可追）

1. **首次 migration 缺 title 列** — 第一次用 `SET search_path` 模式建的表，第二次 `IF NOT EXISTS` 跳过，加 `ALTER TABLE` 修复
2. **goctl `options` 标签解析有 bug** — 用了转义引号格式 `options=[\"user\"]` 失败，改成 logic 层 validate
3. **goctl 重新生成覆盖 config.go** — 每次 `goctl api go` 后必须恢复人工加的字段
4. **uint64 vs int64 ID** — 最初用了 uint64，与 user_id 不匹配，统一 int64
5. **JSON 转义在 PowerShell 是地狱** — 用文件 + `--data-binary @file`

## 📊 当前进度

```
Phase 0 基础设施    ████████████████████ 100% ✅
Phase 1 go-zero     ████████████████████ 100% ✅
Phase 2 Kafka       ████████████████░░░░  85% ✅ (管道跑通, 缺 DLQ/重试)
Phase 3 韧性         ░░░░░░░░░░░░░░░░░░░░   0%
Phase 4 业务深化      ██░░░░░░░░░░░░░░░░░  10%
Phase 5 K8s          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🚀 下一步候选

- **A**：接 `Emotion-Echo-LLM` 替换 keyword analyzer（接真实 LLM）
- **B**：DLQ + 重试（sarama 出错入 dead letter topic）
- **C**：analytics-svc 也接 consumer（订阅 chat-events 做行为分析）
- **D**：chat-svc 加 close conversation 端点 + event
- **E**：schema migration 工具（atlas / sqlx-migrate）

走 A/B/C/D/E？