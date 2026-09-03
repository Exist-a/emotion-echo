# Stage 30-C 完成报告 — A 档急需 3 项全部落地

> **生成时间**: 2026-08-30
> **对应 commit**: `31da84b..f4f581e`（10 个 commits + 3 docs baseline 来自前置阶段，详见下文）
> **对应规划**: `docs/stage-30-C-kafka-ext-backlog.md`
> **对应 backlog 状态**: A 档 3 项全部 ✅，B/C/D 档未动（按 backlog 原计划）

---

## 一、目标回顾

Stage 30-C 的定义（per `docs/stage-30-C-kafka-ext-backlog.md` §二）:

> **A 档急需 3 项** —— 修复"现在就在出问题"的可靠性 bug：
> 1. **A1 消费幂等去重**：at-least-once 重复消费会重复落库（情绪计数虚高 / 行为统计翻倍）
> 2. **A2 DLQ 死信队列**：ai-svc 毒消息卡死 partition / analytics-svc 静默丢
> 3. **A3 事务性 Outbox**：chat-svc 双写问题，事件静默丢失
>
> 按 §七 顺序：A1 → A2 → A3，每个独立 PR，严格 TDD（RED → GREEN）。

本次落地完成 **A 档全部 3 项**。B/C/D 档**未动**（按 backlog 原计划保留）：B 等排期、C 等触发、D 明确不做。

---

## 二、本阶段总产出（数据快照）

| 指标 | 值 |
|------|----:|
| 本阶段代码 commit 数（A1+A2+A3） | **10** |
| 新增代码 LOC | **+2,932 / -111** |
| 新增 migrations | **3** SQL 文件（ai-svc 001 + analytics-svc 006 + chat-svc 001） |
| 新增单元测试（`go test ./...`） | **20+ 个测试**（repo / consumer / logic / outbox / consumer 包） |
| 新增集成测试（`-tags integration`） | **4** 个（ai-svc 2 + chat-svc 1 + analytics-svc 1 — 端到端 + 幂等 + DLQ + Outbox） |
| 新增模块 | **3** 个（`consumer/dlq.go` + `kafka/dlq.go` + `outbox/relay.go`） |
| 涉及服务 | **3** 个（ai-svc / analytics-svc / chat-svc） |
| `t.Skip` 命中（AGENTS §四 禁令） | **0** |
| `go test ./...`（三个 svc 全包） | **全绿** |
| `go vet ./...` | **3 svc 全包无 warning**（baseline 1 条 warning 来自前置 ai-svc config tag，非本次引入） |
| `go test -tags integration`（三个 svc 全包） | **全绿**（ai-svc 37s + analytics-svc 100s + chat-svc 33s） |

---

## 三、A 档 3 项 — 实施矩阵

### A1 — 消费幂等去重

**改动**：

| 文件 | 类型 | 描述 |
|------|------|------|
| `emotion-echo-ai-svc/migrations/001_add_event_id_to_emotion_analysis.sql` | 新建 | `emotion_echo_ai.emotion_analysis` 加 `event_id` 列 + UNIQUE 约束（老数据 `'legacy-'||id` 回填） |
| `emotion-echo-ai-svc/internal/model/emotion.go` | 改 | `EmotionAnalysis` struct 加 `EventID` 字段 |
| `emotion-echo-ai-svc/internal/repository/emotion_repository.go` | 改 | InMemory 加 `byEventID` 索引；Postgres 加 `clause.OnConflict{DoNothing}` |
| `emotion-echo-ai-svc/internal/logic/consumehandler.go` | 改 | `row` 加 `EventID: evt.ID`（事件 ID 透传） |
| `emotion-echo-ai-svc/integration_test/grpc_health_integration_test.go` | 改 | 内联 DDL 漂移同步 |
| `emotion-echo-ai-svc/integration_test/emotion_idempotency_integration_test.go` | 新建 | PG 端 ON CONFLICT 双写测试 |
| `emotion-echo-ai-svc/internal/repository/emotion_repository_test.go` | 改 | 3 个 InMemory 单测（Duplicate/Distinct/Empty） |
| `emotion-echo-ai-svc/internal/logic/consumehandler_test.go` | 改 | EventID 透传断言 |
| `deploy/db/02-create-tables-in-schemas.sql` | 改 | 集中式 DDL 漂移同步 |
| `emotion-echo-analytics-svc/migrations/006_add_event_id_to_user_behavior_events.sql` | 新建 | 同上结构（`user_behavior_events` 表） |
| `emotion-echo-analytics-svc/internal/model/event.go` | 改 | `UserBehaviorEvent` struct 加 `EventID` |
| `emotion-echo-analytics-svc/internal/repository/event_repository.go` | 改 | InMemory + Postgres 同上模式 |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go` | 改 | `handleOne` 构造 `be` 时加 `EventID: ev.ID` |
| `emotion-echo-analytics-svc/internal/{repository,kafka}/*_test.go` | 改 | 幂等断言 |
| `emotion-echo-analytics-svc/integration_test/kafka_integration_test.go` | 改 | Kafka 端到端幂等测试 [evt-dup, evt-dup, sentinel] |

**Commit 历史**：

| Commit | 描述 |
|--------|------|
| `fc03fb0` | test(ai-svc): red — EventID 字段 + 幂等断言 |
| `5232548` | feat(ai-svc): green — migration + repo ON CONFLICT |
| `62f82ba` | test(analytics-svc): red — EventID 字段 + 幂等断言 |
| `67c23ba` | feat(analytics-svc): green — migration + repo ON CONFLICT |

### A2 — DLQ 死信队列

**改动**：

| 文件 | 类型 | 描述 |
|------|------|------|
| `emotion-echo-ai-svc/internal/consumer/dlq.go` | 新建 | `DLQEntry` / `DLQPublisher` / `NoopDLQPublisher` / `InMemoryDLQPublisher` / `KafkaDLQPublisher` |
| `emotion-echo-ai-svc/internal/consumer/consumer.go` | 改 | `ConsumerGroupHandler` 加 `DLQ` / `MaxRetries` / `attempts` 字段；新增 `handleFailure` + `attemptKey` helper；`ConsumeClaim` 改走 handleFailure |
| `emotion-echo-ai-svc/internal/consumer/consumer_test.go` | 改 | 5 个新单测（RetriesBeforeDLQ / DLQReceivesOriginalPayload / DLQReceivesErrorReason / DLQNilIsSafe / HandlerSuccessClearsAttempts） |
| `emotion-echo-ai-svc/internal/config/config.go` | 改 | `Kafka` 加 `DLQTopic` + `MaxRetries` 字段 |
| `emotion-echo-ai-svc/etc/ai-api.yaml` | 改 | Kafka 段加 DLQTopic / MaxRetries 配置（默认 `chat-events-dlq` / 3） |
| `emotion-echo-ai-svc/main.go` | 改 | 装配 DLQ publisher（与 main producer 同 brokers，DLQ topic 独立）；`KafkaConsumer.Consume` 签名扩展 dlq + maxRetries |
| `emotion-echo-ai-svc/integration_test/dlq_integration_test.go` | 新建 | 真 Kafka broker + 真 DLQ publisher；发 2 条同 key → 第 2 次投 DLQ；断言 DLQ 收到 1 条 + headers |
| `emotion-echo-ai-svc/go.mod` | 改 | 加 testcontainers-go/modules/kafka 依赖 |
| `emotion-echo-analytics-svc/internal/kafka/dlq.go` | 新建 | 同 ai-svc 同构（但保留在本包内，不抽到 shared — 见 §六设计决策） |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go` | 改 | `Consumer` 加 `dlq` + `maxRetries` 字段 + `WithDLQ` / `WithMaxRetries` builder；`chatEventHandler` 加 dlq/maxRetries/attempts；新增 `handleFailure` + `attemptKey` |
| `emotion-echo-analytics-svc/internal/kafka/consumer_test.go` | 改 | 3 个新单测（DLQ_NoOpWhenSuccess / DLQ_RetriesThenMarks / NoopDLQ_RetriesAndMarks） |

**Commit 历史**：

| Commit | 描述 |
|--------|------|
| `87f641f` | test(ai-svc): red — DLQ 接口 + 5 个测试 |
| `c679767` | feat(ai-svc): green — DLQ publisher 注入 + handleFailure + 集成测试 |
| `551d74d` | feat(analytics-svc): DLQ publisher + handleFailure + 3 个测试（合并 RED+GREEN） |

### A3 — 事务性 Outbox

**改动**：

| 文件 | 类型 | 描述 |
|------|------|------|
| `emotion-echo-chat-svc/migrations/001_create_outbox_events.sql` | 新建 | `emotion_echo_chat.outbox_events` 表 + 2 个 partial index |
| `emotion-echo-chat-svc/internal/repository/outbox.go` | 新建 | `OutboxEvent` struct + `OutboxRepo` 接口 + `InMemoryOutboxRepo` + `PostgresOutboxRepo` |
| `emotion-echo-chat-svc/internal/repository/outbox_test.go` | 新建 | 8 个 InMemory 单测 |
| `emotion-echo-chat-svc/internal/repository/conversation_repository.go` | 改 | `ConversationRepo` 接口加 4 个 Tx 方法（CreateConversationTx / AppendMessageTx / DeleteConversationTx / IncrementMessageCountTx） + InMemory / Postgres 双实现 |
| `emotion-echo-chat-svc/internal/svc/servicecontext.go` | 改 | `ServiceContext` 加 `DB *gorm.DB` + `OutboxRepo` 字段 + `WithDB` / `WithOutboxRepo` builder |
| `emotion-echo-chat-svc/internal/logic/createconversationlogic.go` | 改 | 重构为 `persistWithOutbox` helper，三路径策略：DB+OutboxRepo（生产事务）→ 仅 OutboxRepo（无事务退化）→ 原行为 EventPublisher（向后兼容） |
| `emotion-echo-chat-svc/internal/logic/sendmessagelogic.go` | 改 | 同上模式 + 修复 AppendMessage 后 `msg.ID` 未回填 bug |
| `emotion-echo-chat-svc/internal/logic/deleteconversationlogic.go` | 改 | 同上模式（DELETE + outbox 同事务） |
| `emotion-echo-chat-svc/internal/logic/deleteconversationlogic_test.go` | 改 | `failingDeleteRepo` mock 补 4 个 Tx 方法 |
| `emotion-echo-chat-svc/internal/outbox/relay.go` | 新建 | `Relay` struct + `Run` 阻塞循环 + `FlushOnce` 单轮 |
| `emotion-echo-chat-svc/internal/outbox/relay_test.go` | 新建 | 5 个 relay 单测 |
| `emotion-echo-chat-svc/main.go` | 改 | `openPostgres` 改返 `(repo, *gorm.DB, error)`；启动 outbox relay goroutine（间隔 1s，batch 100，SIGTERM 优雅退出）；启动时跑 migration 建 outbox_events 表 |
| `emotion-echo-chat-svc/integration_test/outbox_integration_test.go` | 新建 | 真 PG + 真 Kafka broker + 真 repo；业务写入 + outbox 同事务；Relay.FlushOnce 验证；断言 Kafka topic 收到事件 |
| `emotion-echo-chat-svc/go.mod` | 改 | 加 testcontainers-go/modules/kafka 依赖 |

**Commit 历史**：

| Commit | 描述 |
|--------|------|
| `e7a66b7` | feat(chat-svc): outbox 表 + OutboxRepo 接口 + CreateConversationLogic 事务化（RED+GREEN 合并） |
| `113cf81` | feat(chat-svc): SendMessage + DeleteConversation 事务化（RED+GREEN 合并） |
| `f4f581e` | feat(chat-svc): Outbox relay goroutine + 端到端集成测试 |

---

## 四、验证矩阵（DoD）

| 项 | ai-svc | analytics-svc | chat-svc |
|----|--------|---------------|---------|
| `go test ./...` | ✅ 全绿 | ✅ 全绿 | ✅ 全绿 |
| `go vet ./...` | ⚠️ 1 baseline warning（不在本次范围） | ✅ 无 warning | ✅ 无 warning |
| `go test -tags integration` | ✅ 36.7s | ✅ 100.3s | ✅ 33.3s |
| A1 幂等去重集成 | ✅ PG ON CONFLICT 端到端 | ✅ Kafka 端到端（[evt-dup, evt-dup, sentinel]） | — |
| A2 DLQ 集成 | ✅ 真 broker + 真 DLQ publisher；MaxRetries=1 第 2 次投 DLQ | — | — |
| A3 Outbox 集成 | — | — | ✅ PG + Kafka 端到端；业务 + outbox 同事务 → relay → Kafka topic |
| 旧 logic 单测向后兼容 | ✅ | ✅ | ✅ |

**DoD 核心断言全部满足**：
- A1：同 event_id 两次 Create 只落 1 行（PG 端 ON CONFLICT DO NOTHING）；InMemory 与 PG 语义对齐
- A2：attempt > MaxRetries 后投 DLQ topic + MarkMessage；DLQ message 保留原 payload + x-original-topic / x-error-reason / x-attempts headers
- A3：业务表写入与 outbox_events 行写入在同一 DB 事务（commit 后两边可见）；relay 异步发送；Kafka topic 收到事件

---

## 五、可靠性改进（业务影响）

| 问题（落地前） | 改进（落地后） |
|----------------|---------------|
| ai-svc 重复消息 → emotion_analysis 重复行，emotion counts 虚高 | `event_id` UNIQUE + ON CONFLICT DO NOTHING，重复消息不落库 |
| analytics-svc 重复消息 → user_behavior_events 重复行，行为统计翻倍 | 同上 |
| ai-svc 毒消息（永远解析失败）→ 不 MarkMessage，无限重投，partition 卡死 | 重试 3 次后投 `chat-events-dlq` topic + MarkMessage，让消费继续 |
| analytics-svc 失败 → log + MarkMessage，**静默丢** | 走 attempt 计数 + DLQ，运维可观测（Kafka 监控 DLQ 积压） |
| chat-svc 业务落库成功 + Publish 失败 → 事件**静默丢失**（仅日志） | 业务写入 + outbox 行同事务；relay goroutine 异步发布，**事件不丢**（即使 Kafka 不可达，事件也在 outbox 表里待 relay 重发） |

**A1 + A3 业内成对**（Chris Richardson 微服务模式）：A3 relay 重发天然会重复，A1 消费者侧 `event_id` UNIQUE 兜底幂等。

---

## 六、设计决策（与 §一规划的差异 / 补全）

| 决策 | 规划 | 实际 | 理由 |
|------|------|------|------|
| 幂等策略 | 二选一：① DB UNIQUE + ON CONFLICT（推荐）② Redis SETNX | ✅ ① | DB 级幂等最可靠；不引入新依赖 |
| UNIQUE 类型 | 规划未明确（partial vs 完整） | ✅ 完整 UNIQUE + 老数据 `'legacy-'||id` 回填 | GORM `clause.OnConflict{Columns: [event_id]}` 生成的 SQL 不带 WHERE，与 partial index arbiter 不匹配；完整约束让 ON CONFLICT 直接命中 |
| DLQ topic | 规划建议 `chat-events-dlq` | ✅ `chat-events-dlq`（ai-svc / analytics-svc 默认）+ 集成测试用 `chat-events-dlq-test` | 集成测试用独立 topic 避免污染 |
| DLQ 失败语义 | 规划二选一 | ✅ 重试 N 次 + DLQ | N=3（可配置 `MaxRetries`） |
| DLQ message 格式 | 规划建议 "原始 payload + 失败原因" | ✅ 原始 payload + sarama headers (`x-original-topic` / `x-error-reason` / `x-attempts`) | 不重写 payload，保持回放可行 |
| DLQ publisher 共享 | 规划未明确 | ❌ 不抽到 shared 包（ai-svc / analytics-svc 各一份 ~60 行镜像） | 接口签名不同（sarama.ConsumerGroupHandler vs chatEventHandler）；B2 Schema Registry 阶段再合并 |
| Outbox 颗粒度 | 规划 "三 logic + relay + integration" | ✅ 三 logic + relay + 1 个集成测试 | 与规划一致 |
| Outbox 表 schema | 规划未明确 | ✅ 放在 emotion_echo_chat（与 conversations 同 schema，便于单事务） | 业务表与 outbox 表同 schema 是 Outbox 模式标准 |
| Outbox EventID 与 A1 复用 | 规划提及 | ✅ 双层 unique 保护（outbox 表 + consumer 表） | Outbox 表 event_id UNIQUE 防止 relay 重发；consumer 表 event_id UNIQUE（A1）防止跨服务重复消费 |
| Outbox 重试策略 | 规划未明确 | ✅ MarkFailed attempts++ status 保留 pending；下次 relay 再试 | 与 A2 DLQ 失败语义对齐；attempts > 阈值时人工介入 |
| Relay 间隔 | 规划未明确 | ✅ 1s 生产 / 200ms 集成测试 | 可配置 `interval` |
| logic 改造范围 | 规划"三 logic 事务化" | ✅ 三 logic 全部走 outbox，但保留 EventPublisher.Publish 路径作为退化（向后兼容） | 旧 logic 单测不破坏；新增 `OutboxRepo` 字段触发事务路径 |
| main.go migration | 规划未明确 | ✅ `runOutboxMigration` 用 gorm.Exec 直接跑 inline SQL（学习/开发期够用） | 生产建议引入 `golang-migrate` 工具 |

---

## 七、未做（B/C/D 档，按 backlog 原计划保留）

| 档 | 项 | 状态 | 触发条件 |
|----|----|------|---------|
| B | B1 Consumer lag 监控 | ⏸ 待排期 | 部署到真实环境前 |
| B | B2 Schema Registry / Protobuf | ⏸ 待排期 | 第三个消费者或跨团队前 |
| C | C1 Kafka Streams 实时聚合 | 🔒 等触发 | 日数据量 > 10M 行 或 聚合端点 p95 > 2s |
| C | C2 mental-health trigger 迁 Kafka | 🔒 等触发 | K8s 部署 > 1 副本 |
| C | C3 多 topic 分类 | 🔒 等触发 | AI / assessment 域也开始产生事件 |
| D | D1 TTS 异步化 | ❌ 明确不做 | XTTS 实时流式需求，Kafka 引入延迟违背产品需求 |
| D | D2 ClickHouse / ksqlDB | ❌ 明确不做 | v1 单 Postgres 足够 |

---

## 八、与下游规划衔接

A 档完成意味着：
- chat-svc **不再有事件丢失**（outbox relay 兜底）→ analytics-svc / ai-svc 行为统计 / 情绪分析数据可信度提升
- 消费者侧 **at-least-once 安全**（A1 + A2 双重保障）→ analytics-svc 报表 emotion counts / 行为频次不再虚高
- **可靠性边界前移** → 后续 B/C 档不必先解决数据正确性问题，可专注规模化 / 契约治理

下一步候选（按你确认优先级）：
1. **业务完善**：user-svc 真实登录 / API spec 同步 / 多模态串联 / XTTS 流式 — 见调研 §六
2. **BFF 层**：`docs/stage-30-web-bff.md` 规划 67 commit / ~2-3 周独立 session
3. **容器化测试补全**：prometheus / grafana / loki / alertmanager 在 docker-compose service 块 / 全栈 smoke

---

## 九、变更记录

| 日期 | 变更 |
|------|------|
| 2026-08-30 | 初版：A 档 3 项完成总结 + 验证矩阵 + 设计决策 |
