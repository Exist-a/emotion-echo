---
status: planned
priority: high
owner: TBD
created: 2026-09-07
related-stages:
  - stage-30-B-kafka-pipeline.md
  - stage-30-C-kafka-ext-backlog.md
  - stage-36-fixes-roadmap.md
  - stage-37-fixes-roadmap.md
related-adrs:
  - adr-2026-09-dev-publisher-user-behavior-events.md（ADR-19 · DevEventPublisher 方案未落地）
  - adr-2026-09-known-gaps.md（ADR-16 · G4 Kafka 默认关导致情绪分析无数据）
related-plans:
  - todo-pile-2026-09-04.md（A1/B4 dev 模式多模态不可用）
---

# Plan — Kafka 管线健壮性缺口汇总与修复排期

## 一、现状（与代码事实对齐）

Kafka 管线生产路径（KAFKA_ENABLED=true）的可靠性骨架已搭好：

- **生产端**：chat-svc 事务性 Outbox（outbox_events 表 + relay goroutine 每 1s 扫描）+ sarama SyncProducer（WaitForAll + 重试 5 + 事件 ID 作 key）
- **消费端**：ai-svc（message.created → emotion_analysis）+ analytics-svc（3 种事件 → user_behavior_events），均为 sarama ConsumerGroup（OffsetOldest）
- **可靠性三件套**（Stage 30-C A 档已落地）：幂等去重（event_id UNIQUE）、DLQ（Kafka/Noop/InMemory 三实现）、事务性 Outbox
- **可观测性**：ai-svc 每条消息建 SkyWalking span（messaging.system/topic/partition/event.type）

但存在以下 6 项健壮性缺口，按严重度排序：

### 1.1 🔴 P0 — dev 模式 KAFKA_ENABLED=false 完全空跑

**事实**：
- chat-svc `main.go:154`：outbox relay 只在 `c.Kafka.Enabled && len(kafkaBrokersList) > 0` 时启动
- KAFKA_ENABLED=false 时：outbox_events 行永久 status=pending，relay 不启动
- ai-svc / analytics-svc consumer 同样不启动（`main.go` 均有 `if c.Kafka.Enabled` 守卫）
- 后果：`user_behavior_events` / `emotion_analysis` 无数据 → 4 个 dashboard（daily/weekly/monthly/annual）全空

**已有方案未落地**：ADR-19（`adr-2026-09-dev-publisher-user-behavior-events.md`）提出在 chat-svc 加 `DevEventPublisher`（Kafka 关时同步写 user_behavior_events），有完整 TDD 排期（PR-A1.1/1.2/1.3），但**代码尚未实现**：
- `emotion-echo-chat-svc/internal/events/` 只有 4 个文件（events.go / events_test.go / kafka_publisher.go / kafka_publisher_test.go），无 `dev_publisher.go`
- `emotion-echo-shared/` 无 `eventrow` 公共映射包

**AGENTS.md §2.4 契约 6** 已把"KAFKA_ENABLED=false 路径不空跑"列为合并前必跑验收项，当前是红的。

### 1.2 🟡 P1 — ai-svc consumer 缺外层故障重试

**事实**：两个 consumer 的故障恢复策略不对称：

| | analytics-svc | ai-svc |
|---|---|---|
| Consume 出错后 | `Run` 内置 **5s 重试**（`consumer.go:100-105`：log + sleep + continue） | `Consume` **直接 return err**（`consumer.go:234-235`），无内置重试 |
| main.go 外层 | goroutine 调 `kc.Run(appCtx)`，Run 自己无限重试 | goroutine 调 `kc.Consume(...)`，出错只 log 然后 **goroutine 死掉，不重启**（`main.go:300-311`） |

sarama 内部能处理 transient 错误（单次 fetch 失败、rebalance），不会让 Consume 返回。但会话级故障（consumer group 被关闭、broker 长期不可达超过 sarama 内部重试）时，**ai-svc consumer goroutine 直接退出，不会自动恢复，只能重启进程**。

### 1.3 🟡 P1 — DLQ 默认 Noop，毒消息静默丢弃

**事实**：
- ai-svc `main.go:285`：`var dlq consumer.DLQPublisher = consumer.NoopDLQPublisher{}`，只有配置了 `Kafka.DLQTopic` 才创建 KafkaDLQPublisher
- analytics-svc `consumer.go:65`：默认 `NoopDLQPublisher{}`，需显式 `WithDLQ()` 注入
- NoopDLQPublisher 的行为：不投任何 topic，但调用方仍 MarkMessage 让消费继续
- 后果：毒消息（永远解析失败/处理失败的消息）超过 MaxRetries 后**被静默丢弃**，不进入可回放的 DLQ topic，无积压告警，事后无法排查

"毒消息不卡死 partition"这一点是保证的（因为 Noop 也会 MarkMessage），但"可回放/可告警"缺失。

### 1.4 🟡 P2 — 无 consumer lag 监控

**事实**：
- 没有 kafka-exporter / Prometheus JMX exporter，无 consumer group lag 指标
- Stage 30-C B1 已列为"可以做"但待排期
- 无法感知消费滞后，毒消息/慢消费导致 lag 堆积时无告警
- SkyWalking 有 kafka span，但无 lag 维度

### 1.5 🟡 P2 — 无 Schema Registry，事件 schema 人肉镜像

**事实**：
- 事件 schema 是 ad-hoc JSON，`chat-svc/internal/events/events.go` 定义，`analytics-svc/internal/events/events.go` 手动镜像一份
- Stage 37-A 刚修过 event_type 细分（conversation.created/closed 不再合并成 "conversation"）和 target 语义（用 message.id/conversation.id 而非 Event.ID）——就是 schema 漂移的实例
- 项目已用 protobuf（proto/ + gRPC），但 Kafka 事件未迁 Protobuf
- Stage 30-C B2 已列为"可以做"但待排期

### 1.6 🟢 P3 — outbox relay 单线程轮询，无最大重试上限

**事实**：
- `outbox/relay.go`：单 goroutine，每 1s 扫一批 100 条，单线程顺序发送
- `MarkFailed` 只递增 attempts + 记录 last_error，status 保留 pending，下次 relay 再试——**无最大重试次数上限**，永久失败的消息会无限重试
- v1 规模够用，高吞吐场景会成瓶颈
- 有 attempts 计数和 last_error 可人工排查，但无自动告警

---

## 二、修复优先级与建议顺序

| 优先级 | 项 | 工作量 | 依赖 |
|---|---|---|---|
| **P0** | DevEventPublisher 落地（ADR-19 PR-A1.1/1.2/1.3） | 1-2 天 | 无 |
| **P1** | ai-svc consumer 外层 5s 重试（对齐 analytics-svc） | 0.5 天 | 无 |
| **P1** | DLQ 默认启用真实 Kafka topic（compose 加 chat-events-dlq） | 0.5-1 天 | 无 |
| **P2** | consumer lag 监控（kafka-exporter + Prometheus + Grafana 面板 + 告警） | 1 天 | observability-compose-gap.md（prometheus 先装好）|
| **P2** | 事件 schema 迁 Protobuf + 共享 eventrow 包（消灭两份镜像） | 2-3 天 | DevEventPublisher 落地后（eventrow 包是 ADR-19 §E 缓解措施）|
| **P3** | outbox relay 最大重试上限 + 死信 outbox 行告警 | 0.5 天 | 无 |

**建议顺序**：P0 DevEventPublisher → P1 ai-svc 重试对齐 + DLQ 真实 topic → P2 lag 监控 → P2 Protobuf/eventrow → P3 relay 上限。

---

## 三、各修复项要点

### 3.1 P0 — DevEventPublisher 落地（执行 ADR-19）

直接按 ADR-19 已规划的 TDD 节奏执行：

- **PR-A1.1（RED）**：`dev_publisher_test.go` + integration test（KAFKA_ENABLED=false 发消息 → user_behavior_events 有行 + outbox status=sent）
- **PR-A1.2（GREEN）**：`events.DevEventPublisher` 实现 `EventPublisher` 接口 + main.go 在 `Kafka.Enabled==false` 时注入（替代 nil）
- **PR-A1.3（REFACTOR）**：抽 `shared/pkg/eventrow/mapper.go` 公共映射函数，DevEventPublisher + analytics-svc consumer 共用

验收：`KAFKA_ENABLED=false` 启动 dev compose → 发消息 → dashboard 有数据；`KAFKA_ENABLED=true` 路径无回归。

### 3.2 P1 — ai-svc consumer 外层重试对齐

修改 `emotion-echo-ai-svc/internal/consumer/consumer.go` 的 `Consume` 方法：

- 当前：Consume 出错 → `return err`
- 改为：与 analytics-svc `Run` 对齐——出错时 log + sleep 5s + continue（除非 `sarama.ErrClosedConsumerGroup` 或 ctx 取消）
- main.go 的 goroutine 调用无需改（Consume 内部自循环）

测试：`consumer_test.go` 加用例——模拟 `group.Consume` 返回 transient error → 断言 5s 后重试（用 clock 接口注入，避免真实 sleep）。

### 3.3 P1 — DLQ 默认启用真实 topic

- compose `docker-compose.apps.yml` 给 ai-svc / analytics-svc 加 `KAFKA_DLQ_TOPIC=chat-events-dlq` env
- ai-svc `main.go:286` 已有 DLQTopic 配置分支，只需 compose 注入
- analytics-svc 需在 main.go 加 `WithDLQ(kafka.NewKafkaDLQPublisher(...))` 注入（当前默认 Noop）
- infra compose 确保 `chat-events-dlq` topic 存在（或依赖 Kafka auto.create.topics.enable）

验收：发一条毒消息（如 malformed JSON）→ 断言 `chat-events-dlq` topic 有消息 + 原消费继续前进。

### 3.4 P2 — consumer lag 监控

- compose 加 kafka-exporter（或启用 Kafka JMX exporter）
- Prometheus 加 scrape job（与 observability-compose-gap.md PR-3 对齐）
- Grafana 加 consumer lag 面板
- 告警规则：lag > 阈值持续 5 分钟 → alertmanager（dev 可先只做面板，告警走 P3）

### 3.5 P2 — 事件 schema 迁 Protobuf + eventrow 共享包

- 在 `proto/` 加 `chat_events.proto`（定义 Event + 3 种 Data 消息）
- 生成代码到 `shared/pkg/chat_events/`
- chat-svc producer 改 Protobuf 序列化，analytics/ai consumer 改 Protobuf 反序列化
- `shared/pkg/eventrow/mapper.go` 统一事件→DB 行映射（DevEventPublisher + analytics consumer 共用）
- 兼容性：加字段不破坏旧消费者（Protobuf 向后兼容特性）

### 3.6 P3 — outbox relay 最大重试上限

- `outbox/repository.go` MarkFailed 加判断：attempts > MaxAttempts（如 100）→ status 改为 `dead`，不再被 ListPending 扫到
- 加 `dead` 状态的告警（log error + 可选 metrics）
- relay.go ListPending 只查 pending，dead 行保留供人工排查/回放

---

## 四、风险与缓解

| 风险 | 缓解 |
|---|---|
| DevEventPublisher 让 chat-svc 跨服务写 user_behavior_events（prod 不命中，但 dev 路径引入耦合） | ADR-19 §E 已接受此代价；eventrow 共享包缓解 schema 漂移 |
| DLQ 真实 topic 引入额外 broker 资源消耗 | dev 单节点够用；DLQ 消息量预期极低 |
| Protobuf 迁移期间新旧消费者并存 | 分阶段：先加 Protobuf producer 兼容 JSON consumer（双写），再切 consumer，最后删 JSON |
| ai-svc consumer 重试改造可能引入无限循环 | 5s 退避 + ctx 取消守卫；与 analytics-svc 已验证的实现对齐 |

---

## 五、不在本计划范围

- Kafka Streams 实时聚合（Stage 30-C C1，触发条件：日数据量 >10M）
- mental-health trigger 迁 Kafka（Stage 30-C C2，触发条件：K8s 多副本）
- 多 topic 分类（Stage 30-C C3，触发条件：AI/assessment 域产生事件）
- TTS 异步化（明确不做，实时流式需求）
- ClickHouse / ksqlDB（v1 规模不需要）

---

## 六、调研依据

> 满足 AGENTS.md §〇 "写文档前先调研"

| 文件 | 调研内容 |
|---|---|
| `emotion-echo-chat-svc/main.go:84-167,255-283` | outbox relay 启动条件（Kafka.Enabled 守卫）+ outbox 表迁移 |
| `emotion-echo-chat-svc/internal/outbox/relay.go` | relay 实现（单线程、1s 间隔、MarkFailed 无上限）|
| `emotion-echo-chat-svc/internal/repository/outbox.go` | outbox_events 表结构 + Postgres/InMemory 实现 |
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go` | sarama SyncProducer 配置（WaitForAll + 重试5 + HashPartitioner）|
| `emotion-echo-ai-svc/internal/consumer/consumer.go:229-241` | ai-svc Consume 出错直接 return err（无外层重试）|
| `emotion-echo-ai-svc/internal/consumer/dlq.go` | DLQPublisher 接口 + Kafka/Noop/InMemory 三实现 |
| `emotion-echo-ai-svc/main.go:248-313` | ai-svc consumer 启动（goroutine 无重启 + DLQ 默认 Noop）|
| `emotion-echo-analytics-svc/internal/kafka/consumer.go:94-112` | analytics-svc Run 内置 5s 重试（对比基准）|
| `emotion-echo-analytics-svc/internal/kafka/consumer.go:208-263` | handleOne 事件映射（event_type 细分 + target 语义）|
| `deploy/db/01-create-schemas.sql:121-143` | emotion_analysis event_id 列 + UNIQUE 约束 |
| `deploy/db/test_migrations_contract.sh:63-65` | user_behavior_events.event_id 契约测试 |
| `docs/stages/stage-30-B-kafka-pipeline.md` | Kafka 管线 as-built + 扩展性分析 |
| `docs/stages/stage-30-C-kafka-ext-backlog.md` | A/B/C/D 分档 backlog（A 档已落地）|
| `docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md` | ADR-19 DevEventPublisher 方案（未落地）|
| `docs/architecture/adr/adr-2026-09-known-gaps.md` | ADR-16 G4（Kafka 默认关导致情绪分析无数据）|
| `docs/plans/todo-pile-2026-09-04.md` | A1/B4 dev 模式多模态不可用 |
| `AGENTS.md §2.4` | 数据契约验收清单（契约 6：KAFKA_ENABLED=false 路径不空跑）|
