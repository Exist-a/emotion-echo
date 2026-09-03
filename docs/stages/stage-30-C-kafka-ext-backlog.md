# Stage 30-C — Kafka 扩展待做选型（Backlog）

> **生成时间**: 2026-08-30
> **前置**: `docs/stage-30-B-kafka-pipeline.md` §三（扩展性分析）
> **性质**: 决策文档 —— 把"可以做的扩展"整理成可执行的 backlog，并区分**急需 / 可以做 / 待触发 / 明确不做**
> **落地报告**: `docs/stage-30-C-completion-report.md`（A 档 3 项已全部 ✅）

---

## 一、选型原则（判断"急不急"的标准）

| 判断维度 | 急 | 不急 |
|---------|----|------|
| 是否正确性/可靠性问题（数据错、数据丢、消费卡死） | ✅ 急 | — |
| 是否影响**当前已接线**的链路 | ✅ 急（consumer 已上线） | — |
| 是否只是"未来规模化"才需要 | — | 待触发 |
| 是否需专门重构 / 新依赖 / 跨服务 | — | 可以做 / 排期 |

核心逻辑：**先修"现在就在出问题"的，再修"以后才会出问题"的。**

---

## 二、分档总表

| 档 | 项 | 一句话 | 工作量 | 涉及 | 状态 |
|----|----|--------|--------|------|------|
| **A 急需** | A1 消费幂等去重 | 重复消费会重复写数据，**现在是真实行为** | 小（半天~1天） | ai-svc / analytics-svc | ✅ 完成 |
| **A 急需** | A2 DLQ（死信队列） | 毒消息：analytics 静默丢 / ai-svc 无限重投卡死 | 中（1~2 天） | ai-svc / analytics-svc | ✅ 完成 |
| **A 急需** | A3 事务性 Outbox | 事件丢失（双写问题），最经典可靠性缺口 | 中偏大（2~3 天） | chat-svc | ✅ 完成 |
| **B 可以做** | B1 Consumer lag 监控 | 没有消费滞后指标 | 小（半天） | 基建（compose/k8s） | ⏸ 待排期 |
| **B 可以做** | B2 Schema Registry / 事件 Protobuf | 契约靠人肉镜像，易漂移 | 大（独立 session） | chat-svc + analytics-svc | ⏸ 待排期 |
| **C 待触发** | C1 Kafka Streams 实时聚合 | 报表从批查改流算 | 大 | analytics-svc | 🔒 等触发 |
| **C 待触发** | C2 mental-health trigger 迁 Kafka | 进程内队列换分布式 | 中 | analytics-svc | 🔒 等触发 |
| **C 待触发** | C3 多 topic 分类 | 单 topic 拆域 | 中 | 全链路 | 🔒 等触发 |
| **明确不做** | D1 TTS 异步化 / D2 ClickHouse、ksqlDB | 违背实时需求 / v1 规模不需要 | — | — | ❌ 明确不做 |

**建议顺序：A1 → A2 → A3（每个独立 PR，TDD 节奏）→ B 档按需 → C 档等触发。**

---

## 三、A 档 — 急需（当前系统已存在的实际问题）

### A1. 消费幂等去重（最高优先）

**问题定位**：消费者已接线（Stage 30-B），Kafka 是 **at-least-once 语义**——消费失败重投、offset 回退重放都会**重复处理**：

- `emotion-echo-ai-svc/internal/logic/consumehandler.go`：`message.created` 每处理一次就 `repo.Create` 一行 `emotion_analysis` → **重复消息会写重复的情绪分析行**，报表 emotion counts 虚高
- `emotion-echo-analytics-svc/internal/kafka/consumer.go`：`handleOne` 每处理一次就 `Create` 一行 `user_behavior_events` → 行为统计翻倍

**为什么急**：不是"万一发生"，而是 at-least-once 下**必然偶发**（broker 抖动、consumer 重启、offset 回退）。事件已带 `ID` 且 chat-svc 用 ID 作 Kafka key（同 id 同 partition）——**为幂等铺的路已就位，只差消费侧落地**，收尾成本很小。

**业内对应**：Idempotent Consumer（幂等消费者），at-least-once 的标配配套；支付系统的 Idempotency-Key 同理。

**做法（二选一，推荐 ①）**：
1. `emotion_analysis` / `user_behavior_events` 加 `event_id` 列 + 唯一约束，写入 `INSERT ... ON CONFLICT (event_id) DO NOTHING`（DB 级幂等，最可靠）
2. 消费侧 Redis `SETNX event_id`（需要 Redis，多一个依赖）

**DoD**：消费同一事件两次只落一行（集成测试断言）；`go test ./...` 绿；vet 绿。

### A2. DLQ（死信队列）

**问题定位**：两个 consumer 的失败语义**都不对**，且相反：

| consumer | 失败时 | 后果 |
|----------|--------|------|
| ai-svc（`consumer.go:98-101`） | 不 MarkMessage → **无限重投** | 毒消息（解析永远失败）**卡死整个 partition**，后续正常消息全部堵住 |
| analytics（`consumer.go:118-122`） | 打日志后 MarkMessage → **静默跳过** | 毒消息被永久丢弃，**数据静默丢失**（注释说 "will skip" 但没人看日志） |

ai-svc 的 `consumehandler.go` 注释写着"最终入 DLQ"——**注释与实现不符**，属于"自以为有保障"。

**为什么急**：消费是 Stage 30-B 刚接线的核心链路，两种失败模式都直接影响数据正确性；且这是标准 SRE 问题，拖得越久越难查。

**业内对应**：Dead Letter Queue（AWS SQS 原生 Redrive；Confluent/Kafka 生态自实现），重试 N 次后投 `chat-events-dlq` topic + 告警。

**做法**：Consumer 加"重试计数"（或基于时间/次数），超过阈值 → 发 `chat-events-dlq`（消息带原始 payload + 失败原因）→ MarkMessage 继续前进；健康检查暴露 DLQ 积压数。

**DoD**：毒消息进 DLQ 不阻塞消费（集成测试）；DLQ 消息可回放；`go test ./...` 绿。

### A3. 事务性 Outbox（chat-svc）

**问题定位**：**双写问题**（dual write）——`createconversationlogic.go` / `sendmessagelogic.go` / `deleteconversationlogic.go` 都是"先落库 → 后 Publish，失败只打日志"：

- 事件丢失是**静默的**（只有日志，下游永远不知道少了这条）
- 后果：analytics 行为统计漏数、ai-svc 漏分析某条消息

**为什么急**：这是"消息落库与事件发布原子性"的经典缺口，也是所有下游数据一致性的源头。**A1 做完后做 A3 正好**：Outbox relay 重发天然会重复 → A1 的幂等去重兜底，两个模式是配套的（业内也总是成对出现）。

**业内对应**：Transactional Outbox（Chris Richardson 微服务模式）；大厂落地常用 Debezium（CDC），本项目规模用"goroutine 轮询 outbox 表"即可。

**做法**：chat-svc 加 `outbox_events` 表（同 schema，`id / event_type / payload / status / created_at`）→ 业务逻辑在**同一 DB 事务**里写业务数据 + outbox 行 → relay goroutine 读未发送行 → Publish → 标记已发。

**DoD**：业务落库与事件入 outbox 原子（事务）；relay 发送失败重试不丢；事件消费幂等（依赖 A1）；`go test ./...` 绿。

---

## 四、B 档 — 可以做（值得做，无紧迫性）

### B1. Consumer lag 监控

- **为什么不急**：本地/学习环境没有真实流量；但**部署到任何真实环境前**应完成。
- **业内对应**：kafka-exporter + Prometheus（本项目已有 Prometheus/Grafana，接入成本≈一行 compose）+ Burrow（LinkedIn，专做 lag 检测）。
- **做法**：compose/k8s 加 kafka-exporter；Grafana 面板 + lag 告警（lag 持续 > 阈值 5 分钟）。
- **DoD**：`/metrics` 出现 `kafka_consumergroup_lag`；Grafana 面板可查。

### B2. Schema Registry / 事件 Protobuf

- **为什么不急**：契约漂移是"慢性病"，现在只有 2 个消费者、1 个人维护，风险可控；但**第三个消费者或跨团队前**必须做。
- **业内对应**：Confluent Schema Registry（Avro 事实标准）/ Apicurio；Netflix、Uber、LinkedIn 全在用。
- **做法**：事件结构迁到 `proto/`（项目已有 protobuf 基建）→ 生产者/消费者用同一份生成的 Go 代码（消灭 `internal/events` 镜像拷贝）→ 可选接 Schema Registry 做兼容性校验。
- **DoD**：`events.go` 镜像拷贝删除；事件序列化改 Protobuf；兼容性测试（加字段不破坏旧消费者）。

---

## 五、C 档 — 待触发（有明确触发条件才做）

| 项 | 触发条件（来自既有文档） | 说明 |
|----|------------------------|------|
| C1 Kafka Streams 实时聚合 | 日数据量 > 10M 行 或 聚合端点 p95 > 2s（stage-30-A §三.1） | 从"事件落表后 SQL 批查"改为"边进边算"，报表延迟从秒级到近实时 |
| C2 trigger 迁 Kafka | K8s 部署 > 1 副本（进程内 TriggerQueue 不跨实例） | consumer group 天然分片，顺带承载"跨服务调 assessment-svc"的 defer |
| C3 多 topic 分类 | AI / assessment 域也开始产生事件 | 按域拆 `chat-events / ai-events / assessment-events`，独立保留策略 |

---

## 六、明确不做（记录理由，避免反复纠结）

| 项 | 理由 |
|----|------|
| D1 TTS 异步化 | XTTS 是实时流式需求（`/tts/stream`），Kafka 引入延迟违背产品需求 |
| D2 ClickHouse / ksqlDB 全套 | v1 单 Postgres 足够，按 stage-30-A §九 触发条件再上 |

---

## 七、落地建议与学习关联

- **顺序**：A1（幂等）→ A2（DLQ）→ A3（Outbox）。A1+A3 是业内成对出现的"可靠性双件套"，A2 是消费健壮性底线。
- **每个 A 档项**：独立 PR，严格 TDD（RED → GREEN），DoD 见上表。
- **学习视角**：A1 练"幂等键 + 唯一约束"、A2 练"消费者状态机 + 告警"、A3 练"本地事务 + 后台 relay"——三个做完，你对 Kafka 可靠性的理解就超过多数初级工程师。
- 更完整的业界背景见 [stage-30-B-kafka-pipeline.md](/docs/stages/stage-30-B-kafka-pipeline.md) §三。

---

## 八、变更记录

| 日期 | 变更 |
|------|------|
| 2026-08-30 | 初版：从 stage-30-B 扩展性分析整理为分档 backlog；A/B/C/D 四档 |
| 2026-08-30 | A 档 3 项全部落地：13 commit 完成 A1 幂等去重 + A2 DLQ + A3 事务性 Outbox；详见 `docs/stage-30-C-completion-report.md` |

---

> 生成者: ZCode agent
> 对应 git tag: main @ 当前 HEAD
