# Stage 30-B — Kafka 异步管线修复与扩展性分析

> **生成时间**: 2026-08-30
> **对应 commit**: `84ba012..f5af3df`（6 个 commits）
> **前置**: `docs/stage-30-A-sql-landing.md`（analytics-svc 数据层真 SQL）

---

## 一、Kafka 管线现状（as-built）

```
chat-svc（生产者 :8890）                       chat-events topic
  createconversationlogic ──► conversation.created ─┐
  sendmessagelogic        ──► message.created ──────┼──► ai-svc（消费者 :8892）
  deleteconversationlogic ──► conversation.closed ──┤      只消费 message.created
  （本次新增 DELETE 端点）                          │      只分析 role=user
                                                     │      → emotion_analysis 表
                                                     └──► analytics-svc（消费者 :8893）
                                                           3 种事件 → user_behavior_events
                                                           （本次修复前未接线）
```

- **事件 schema**：CloudEvents 风格 `{id, type, source, time, data}`，topic 常量 `chat-events`（chat-svc `internal/events` 定义，analytics 侧镜像一份）。
- **Producer 可靠性**：sarama SyncProducer + `WaitForAll` + 重试 5 + 事件 ID 作 key（HashPartitioner，同 id 同 partition，便于去重）；Kafka 不可达时降级 InMemory publisher。
- **Consumer 语义**：`OffsetOldest` + 消费组；处理失败不 MarkMessage → 重投；SkyWalking 每条消息建 span。

---

## 二、本次修复（探查发现的问题 → 落地）

| # | 问题 | 修复 | commit |
|---|------|------|--------|
| 1 | **analytics-svc consumer 有实现但从未接线**：`internal/kafka/consumer.go` 单测齐全，main.go 没有启动它 → `user_behavior_events` 永远不会收到数据，行为报表全空 | main.go 在 `Kafka.Enabled && evtRepo != nil` 时启动 Consumer goroutine（topic=Topics[0]、group=analytics-svc、broker 不可达 5s 重试不 crash）；config 增 `Kafka{BrokersCSV, GroupID, Enabled, Topics}`；yaml 增 Kafka 段；compose 注入 `KAFKA_BROKERS` | `84ba012` / `12b55a0` / `9bbe985` |
| 2 | **`conversation.closed` 定义了但从未发布**（chat-svc 无关闭/删除操作） | 新增 `DELETE /api/v1/conversations/:id`（前端已在调用、此前 404）：owner 校验 → repo 事务删除会话+消息 → 发布 `conversation.closed`（best-effort）。该端点成为该事件的唯一生产者 | `5a70385`(red) / `f1fbd30`(green) |
| 3 | **analytics-svc 缺 `applyEnvOverrides`**（chat/ai-svc 已有，Stage 26-Q）：go-zero 1.10 `conf.MustLoad` 不展开 `${VAR:default}` → compose 里 POSTGRES_DSN/KAFKA_BROKERS env 从未生效，Postgres 连的是字面占位符 | 补 `applyEnvOverrides`（POSTGRES_DSN / KAFKA_BROKERS / SKYWALKING_OAP_ADDR），镜像 chat-svc | `f5af3df` |
| 4 | **chat-api.yaml 占位符漂移** `${VAR:default}`（无 `-`），导致预存的 `TestYaml_HasEnvPlaceholders` 失败（`go test ./...` 一直是红的） | 对齐为 `${VAR:-default}` 项目惯例 | `5d0f66e` |

### 新增测试

- analytics：config yaml 加载默认值测试（GroupID/Enabled/Topics）+ **真实 broker 集成测试**（testcontainers Kafka + Postgres，发布 message.created → 断言写入 user_behavior_events，事件 ID 作 target）
- chat-svc：`DeleteConversationLogic` 5 场景 + InMemory repo 级联删除 + handler 200/400

### 验证

| 命令 | 结果 |
|------|------|
| `go test ./...`（analytics + chat，全包） | ✅ 全绿 |
| `go vet ./...`（两服务） | ✅ 无 warning |
| `go test -tags integration ./integration_test/`（analytics，含 Kafka E2E） | ✅ 全绿 |

---

## 三、Kafka 扩展性分析（按价值排序）

### P0 — 可靠性（建议尽快）

1. **事务性 Outbox 模式（chat-svc）**
   现状：消息落库后单独发 Kafka，publish 失败只写日志 → **事件丢失**（例如删除会话时 conversation.closed 丢了，行为统计就漏数）。Outbox：同一 DB 事务写 `outbox_events` 表，独立 relay 进程读表 → 发 Kafka → 标记已发。保证"业务落库 ↔ 事件发布"原子性，还天然支持重放。这是 chat-svc 当前最值得做的升级。

2. **DLQ + 幂等去重（consumer 侧）**
   - ai-svc consumer 注释说"最终入 DLQ"但实现是无限重投：毒消息（永远解析失败）卡住该 offset 持续重试。扩展：解析失败 N 次后发到 `chat-events-dlq`，健康检查暴露 DLQ 积压。
   - 事件已带 ID + hash partitioner（同 id 同 partition），但 consumer 没去重：重放/重投会重复写 `emotion_analysis` / `user_behavior_events`。扩展：事件 ID 唯一约束或消费侧幂等键（INSERT ON CONFLICT DO NOTHING）。

### P1 — 契约与可观测性

3. **Schema Registry（Protobuf/Avro）**
   现状：事件 schema 是 ad-hoc JSON，`internal/events/events.go` 在 chat-svc 定义、analytics-svc 手动镜像一份（**容易漂移**，本次修 migration 001 时就踩过类似坑）。项目已用 protobuf（proto/ + gRPC），扩展：事件改 Protobuf + Schema Registry（或 Confluent / Apicurio），单一契约来源 + 兼容性校验 + 跨服务类型安全。

4. **Consumer lag 监控 + 告警**
   现状：无 lag 指标。扩展：kafka-exporter / Prometheus JMX exporter 暴露 consumer group lag，Grafana 告警（如 lag > 阈值 5 分钟）。SkyWalking 已有 kafka span，可补 lag 维度。

### P2 — 架构演进（对应既有 defer）

5. **Kafka Streams 实时聚合**（对应 stage-30-A §九 "实时事件聚合 5 秒延迟"）
   现状：行为/报表是批量 SQL 查 `user_behavior_events`。扩展：Kafka Streams 维护近实时聚合（day-night / frequency / depth），报表端点读聚合结果，替代批查。触发条件：日数据量 > 10M 或 p95 > 2s。

6. **mental-health trigger 从进程内 TriggerQueue 迁到 Kafka**
   现状：`POST /mental-health/trigger` 用进程内 buffered channel（单实例、重启丢失）。K8s 多副本时队列不共享。扩展：trigger 事件 → Kafka → consumer group 分布式执行（正好承载 defer 的"worker 跨服务调用 assessment-svc"）。

7. **多 topic / 事件分类**
   现状：单 topic `chat-events`。当 AI / assessment 域也有事件时，可按域拆 topic（`chat-events` / `ai-events` / `assessment-events`），消费隔离 + 独立保留策略。

### 明确不做 / 低价值

- **TTS 异步化**：XTTS 是实时流式需求（`/tts/stream`），走 Kafka 引入延迟，不值得。
- **ClickHouse / ksqlDB 全套**：v1 规模（单 Postgres 足够），按 stage-30-A §九 的触发条件再上。

### 建议落地顺序

```
短（1 个 session）: Outbox(chat-svc) → DLQ+去重(ai/analytics consumer)
中（1-2 个 session）: Schema Registry(事件 Protobuf) → consumer lag 监控
长（Stage-2 触发时）: Kafka Streams 实时聚合 → trigger 迁 Kafka → 多 topic
```

---

## 四、提交链

```
84ba012 feat(analytics-svc): Kafka consumer config (BrokersCSV/GroupID/Enabled/Topics)
12b55a0 test(analytics-svc): integration — chat-events consumer E2E over real broker
9bbe985 feat(analytics-svc): wire chat-events consumer in main.go + yaml + compose
5a70385 test(chat-svc): red — DeleteConversationLogic + repo delete + handler
f1fbd30 feat(chat-svc): green — DELETE /conversations/:id publishes conversation.closed
5d0f66e fix(chat-svc): align chat-api.yaml env placeholders to ${VAR:-default}
f5af3df fix(analytics-svc): applyEnvOverrides — env was never reaching config
```

---

## 五、相关文档

- [stage-30-A-sql-landing.md](/docs/stages/stage-30-A-sql-landing.md) — analytics 数据层真 SQL（前置工作）
- [stage-2-async-pipeline.md](/docs/stages/stage-2-async-pipeline.md) — 异步管线设计意图
- [stage-30-A-analytics-business.md](/docs/stages/stage-30-A-analytics-business.md) — analytics 业务端点规划（§三.3 事件采集）
