# ADR · 2026-09 · chat-svc 表依赖清单 + 字段变更契约

- **编号**：决策 22
- **日期**：2026-09-11
- **状态**：✅ **Accepted**
- **触发**：todo-pile §D5「chat-svc 表依赖清单 ADR（防 PR-GRPC 错装 stage 38 重演）」
- **关联**：
  - [stage-37-B-landing.md §三 P3](../../stages/stage-37-B-landing.md)
  - [stage-38-system-status.md §四隐患 1/2](../../stages/stage-38-system-status.md)
  - [stage-36-D-bug-2-fix](../../stages/stage-36-D-bugfix.md)（initdb 容错包错语句）

---

## 一、目的

chat-svc 拥有 `emotion_echo_chat` schema 的 3 张表（`conversations / messages / outbox_events`）。这些表：

1. **被跨 schema 视图依赖**（`emotion_echo_chat.msg_summary_v` 在 `02-create-tables-in-schemas.sql` + `04-create-views.sql` 中）
3. **被 Kafka 消费者依赖**（ai-svc / analytics-svc 从 `message.created` / `conversation.created` / `conversation.closed` 事件读 `messages.id` / `conversations.id`）
4. **被聚合分析端点依赖**（analytics-svc `/api/v1/reports/daily` 经 `msg_summary_v` 聚合 conversations/messages 计数）

字段变更 / 表重建如果走错顺序，会让前端 dashboard 静默空数据（cf. Stage 36-FU dashboard 4 个 chartData.length===0）。本 ADR 把"chat-svc 表依赖" 显式列出 + 制定变更 checklist。

---

## 二、chat-svc 表清单（3 张）

### 2.1 `emotion_echo_chat.conversations`

**DDL 来源**：`deploy/db/02-create-tables-in-schemas.sql:62-73`（集中式初始化），chat-svc 自身**不创建**（`outbox_events` 反而是 chat-svc 自管的）

| 字段 | 类型 | 用途 | 跨域引用方 |
|---|---|---|---|
| `id` | BIGSERIAL PK | 会话 ID | analytics-svc `msg_summary_v.conversation_id`、ai-svc Kafka 事件 `conversation.id` |
| `user_id` | BIGINT NOT NULL | 归属用户（**无外键约束**，user-svc 拥有 users 表）| 所有读端按 user_id 过滤（用户隔离契约）|
| `title` | VARCHAR(255) | 用户可改标题 | 前端展示 |
| `context` | JSONB | 会话上下文预留（**当前未使用**）| — |
| `message_count` | INT | 消息计数（chat-svc IncrementMessageCount 原子更新）| analytics-svc 聚合视图 |
| `last_message_at` | TIMESTAMPTZ | 最近消息时间（SendMessage 更新）| analytics-svc 排序依据 |
| `status` | SMALLINT (1=open / 2=closed) | 会话状态 | chat-svc DeleteConversation 流转 |
| `created_at` / `updated_at` | TIMESTAMPTZ | 自动维护 | analytics-svc 排序（updated_at desc）|
| `closed_at` | TIMESTAMPTZ | 关闭时间 | — |

**索引**：`idx_conversations_user_at(user_id, last_message_at DESC)`

### 2.2 `emotion_echo_chat.messages`

**DDL 来源**：`deploy/db/02-create-tables-in-schemas.sql:76-86`（集中式初始化），后续由 chat-svc migration 002 加 `client_msg_id`

| 字段 | 类型 | 用途 | 跨域引用方 |
|---|---|---|---|
| `id` | BIGSERIAL PK | 消息 ID | ai-svc Kafka consumer（`message.id` → `emotion_analysis.message_id`）、analytics-svc 消息计数 |
| `conversation_id` | BIGINT FK → conversations(id) ON DELETE CASCADE | 所属会话 | analytics-svc `msg_summary_v.conversation_id` |
| `user_id` | BIGINT NOT NULL | 用户（冗余存）| analytics-svc 用户隔离 |
| `role` | VARCHAR(16)（user / assistant / system）| 角色 | 前端展示 |
| `content` | TEXT NOT NULL | 消息内容 | 前端展示 |
| `content_type` | VARCHAR(16) DEFAULT 'text' | 类型（text / image / voice）| 前端展示（Stage 58 后端支持 image / voice）|
| `metadata` | JSONB DEFAULT '{}' | 元数据预留 | — |
| `tokens_used` | INT DEFAULT 0 | LLM token 计数（debug 用）| — |
| `created_at` | TIMESTAMPTZ DEFAULT NOW() | 创建时间 | analytics-svc 排序 |
| `client_msg_id` | UUID NULL（migration 002）| 客户端幂等键（partial UNIQUE INDEX）| — |

**索引**：`idx_messages_conv_time(conversation_id, created_at)`、`uq_messages_client_msg_id(client_msg_id) WHERE client_msg_id IS NOT NULL`

### 2.3 `emotion_echo_chat.outbox_events`

**DDL 来源**：`emotion-echo-chat-svc/migrations/001_create_outbox_events.sql`（chat-svc 自治，不在集中 initdb），由 `chat-svc/main.go:296-318 runOutboxMigration` 在启动期 apply

| 字段 | 类型 | 用途 | 跨域引用方 |
|---|---|---|---|
| `id` | BIGSERIAL PK | outbox 行 ID | — |
| `event_id` | VARCHAR(64) UNIQUE | 业务事件 ID（与 events.Event.ID 一致；也是 ai-svc emotion_analysis.event_id 幂等键）| ai-svc Kafka consumer 幂等去重 |
| `event_type` | VARCHAR(64)（conversation.created / message.created / conversation.closed）| 事件类型 | ai-svc / analytics-svc 分支判断 |
| `topic` | VARCHAR(64)（chat-events）| Kafka topic | relay Publish 路由 |
| `payload` | JSONB NOT NULL | 已序列化事件 JSON | Kafka consumer 反序列化 |
| `status` | VARCHAR(16)（pending / sent / failed）| relay 状态机 | chat-svc relay 内部 |
| `attempts` | INT DEFAULT 0 | Publish 失败次数 | chat-svc relay 内部 |
| `last_error` | TEXT | 最近失败原因 | debug |
| `created_at` / `sent_at` | TIMESTAMPTZ | relay 排序 / 成功时间 | chat-svc relay 内部 |

**索引**：`UNIQUE(event_id)`、`idx_outbox_pending(created_at) WHERE status='pending'`、`idx_outbox_attempts(attempts) WHERE attempts>0`

---

## 三、跨域依赖矩阵

### 3.1 chat-svc 表 → 其他 svc

| 表 | 下游 | 依赖方式 | 关键字段 |
|---|---|---|---|
| `conversations` | analytics-svc | 视图 `msg_summary_v` | id / user_id / created_at / updated_at |
| `conversations` | ai-svc | Kafka 事件 `conversation.created/closed` | id / user_id / title |
| `conversations` | 前端 dashboard | BFF `/api/v1/conversations`（BFF→chat-svc gRPC）| id / title / updated_at |
| `messages` | analytics-svc | 视图 `msg_summary_v` | id / conversation_id / user_id / created_at / content_type |
| `messages` | ai-svc | Kafka 事件 `message.created` | id / conversation_id / user_id / content |
| `messages` | 前端聊天页 | BFF `/api/v1/conversations/:id/messages`（gRPC）| id / role / content / content_type / created_at |
| `outbox_events` | ai-svc Kafka consumer | event_id 幂等去重 | event_id（与 emotion_analysis.event_id UNIQUE 对齐）|
| `outbox_events` | analytics-svc Kafka consumer | 同上 | event_id |

### 3.2 chat-svc 表 ← 其他 svc

| 表 | 上游（写端） | 写入路径 |
|---|---|---|
| `conversations` | chat-svc logic | Create / Delete（无跨 svc 写入）|
| `messages` | chat-svc logic | SendMessage（同事务 + outbox row）|
| `outbox_events` | chat-svc logic | 同事务随 messages.conversations 写（`CreateInTx`）|

**chat-svc 表零跨 svc 写入**——所有写入都是 chat-svc 自治。**但** `outbox_events.event_id` 必须与 ai-svc `emotion_analysis.event_id` 同 namespace（共享 UNIQUE 约束），因此**事件 ID 生成约定**是跨 svc 契约。

### 3.3 跨 schema 视图依赖

| 视图 | 依赖 chat-svc 表 | 部署位置 |
|---|---|---|
| `emotion_echo_chat.msg_summary_v` | conversations / messages | `deploy/db/04-create-views.sql` + 集中 initdb |

---

## 四、字段变更 checklist（强制 TDD）

任何 chat-svc 表字段变更（含加字段 / 改类型 / 删字段 / 加索引）必须按以下顺序执行：

```
□ 1. 写 migration 文件（emotion-echo-chat-svc/migrations/NNN_*.sql）
     - 用 IF NOT EXISTS / IF EXISTS 守卫保证幂等
     - 注释说明：变更原因 + 跨域影响 + 回滚方法
□ 2. 同步集中 initdb（deploy/db/02-create-tables-in-schemas.sql）
     - 单一 schema 来源（SSoT），db-migrate 容器保证迁移先于业务服务启动
     - 旧 initdb 路径仅作"新环境"fallback
□ 3. 更新 chat-svc model struct（emotion-echo-chat-svc/internal/model/*.go）
     - gorm tag 必须与 DDL 列名一致
     - 加 unit test 验证映射
□ 4. 更新 chat-svc types（emotion-echo-chat-svc/internal/types/types.go）
     - JSON tag 与 proto 字段对齐（grpc proto 文件）
□ 5. 更新 chat-svc proto（proto/chat.proto）
     - 字段编号严格管理（加字段不破坏旧 client）
     - bash proto/gen.sh 重生成 stub
□ 6. 跨域影响评估（按 §三矩阵遍历）
     - analytics-svc 视图是否要改（msg_summary_v 加列 / 加聚合）
     - ai-svc Kafka consumer 解析 payload 是否兼容
     - 前端 components 是否要改（field rename 会破显示）
□ 7. 集成测试
     - chat-svc `go test ./...`
     - analytics-svc / ai-svc `go test ./...`
     - BFF `go test ./...`
     - docker compose smoke（scripts/smoke_data_layer.py）
□ 8. 跨域 regression 守护
     - 加契约测试：检查视图列对齐 / Kafka 事件 JSON schema
     - 加 schema 漂移检测脚本（grep 关键列名）
□ 9. 文档同步
     - 本 ADR §二表清单更新
     - 决策 18 doc-drift-registry 登记（如有失真风险）
     - 跨 svc stage landing 文档（如涉及）
```

---

## 五、跨 svc 契约（必须维护）

### 5.1 event_id 生成与去重

- chat-svc 在 `events.Event.ID`（UUID v4）生成，传入 outbox 的 `event_id` 列
- ai-svc Kafka consumer 写入 `emotion_analysis.event_id`，加 UNIQUE 约束
- analytics-svc Kafka consumer 类似
- **约定**：event_id 必须全局唯一；chat-svc 不能复用（即使业务事件同 payload）

### 5.2 message.created 事件 payload

[emotion-echo-chat-svc/internal/events/events.go](emotion-echo-chat-svc/internal/events/events.go) 定义 payload schema：

```json
{
  "event_id": "uuid",
  "event_type": "message.created",
  "data": {
    "message_id": 123,
    "conversation_id": 456,
    "user_id": 1,
    "role": "user",
    "content": "...",
    "content_type": "text"
  }
}
```

ai-svc consumer 按 `data.message_id` / `data.user_id` 写库。**任何字段 rename 必须同步 ai-svc consumer + proto stub（如果走 Protobuf 迁移）**。

### 5.3 conversations.user_id 约定

`conversations.user_id` 与 `messages.user_id` **冗余存**——前者从 logic 层注入，后者从 messages 表写时冗余。两个 user_id 必须**永远一致**。chat-svc SendMessageLogic 在写 messages 时应从 conversations.user_id 取（避免 client 端塞 `userId` 字段伪造）。

### 5.4 outbox_events ↔ Kafka topic 路由

- `topic='chat-events'` 路由到 ai-svc consumer + analytics-svc consumer
- 未来新增 topic 必须在 ADR 显式记录"哪些 svc 订阅"

---

## 六、禁止事项

| ❌ 禁止 | 原因 |
|---|---|
| 在 chat-svc 表上加**跨 schema 外键**（如 `messages.user_id REFERENCES emotion_echo_user.users(id)`）| 跨 svc 强耦合，违反决策 6（user-svc 拥有 users 表）|
| 在 `messages.metadata` JSONB 字段塞业务关键数据（应用解析依赖 JSON）| 字段语义漂移；新加字段应走 schema migration + proto|
| 在 `conversations.context` JSONB 字段塞关键业务数据 | 同上 |
| 删除 `outbox_events` 行（仅允许 status='sent' 后保留归档）| ai-svc 幂等键依赖 event_id UNIQUE 约束，删除会破坏去重语义 |
| 改 `outbox_events.event_id` UNIQUE 约束为 non-UNIQUE | 同上 |
| 在 chat-svc 直接写 `emotion_analysis` 或 `user_behavior_events` 表 | 跨 schema 写违反职责划分，应通过 Kafka 或 DevEventPublisher（ADR-19） |

---

## 七、调研依据

| 项 | 文件 / 命令 |
|---|---|
| conversations DDL | `deploy/db/02-create-tables-in-schemas.sql:62-73` |
| messages DDL | `deploy/db/02-create-tables-in-schemas.sql:76-86` |
| outbox_events DDL | `emotion-echo-chat-svc/migrations/001_create_outbox_events.sql:31-42` |
| client_msg_id migration | `emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql` |
| chat-svc model | `emotion-echo-chat-svc/internal/model/conversation.go` |
| chat-svc repo ListConversations | `emotion-echo-chat-svc/internal/repository/conversation_repository.go:320` |
| outbox relay | `emotion-echo-chat-svc/internal/outbox/relay.go` |
| events payload schema | `emotion-echo-chat-svc/internal/events/events.go` |
| analytics-svc SQL 跨域聚合 | `emotion-echo-analytics-svc/internal/repository/report_repository.go:175-205` |
| msg_summary_v 视图 | `deploy/db/04-create-views.sql` |
| 跨域写入端实测 | `grep -rn "INSERT INTO emotion_echo_chat" emotion-echo-*/ --include="*.go"` 仅 chat-svc logic 命中 |

---

> 最后更新：2026-09-11 by 决策 22 起草 session
> 关联：todo-pile §D5 + stage-37-B-landing §三 + stage-38-system-status §四 + Stage 36-D Bug 2 fix