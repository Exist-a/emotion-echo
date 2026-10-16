---
status: landed
priority: medium
owner: lane-cl-f（阶段 CL-03）
created: 2026-10-11
landed: 2026-10-16（阶段 CL-03 · Kafka/可靠性闭环；逐点证据见 `docs/closure-roadmap/stages/cl-03-kafka-reliability/{plan,report}.md` 与 `evidence/`）
last-updated: 2026-10-16（**落地归档**：CL-03 收口，本文件自 `docs/plans/` 迁入 `docs/legacy-plans/landed/`；引用旧路径的文档请改指本文件）
type: implementation-plan
source: 2026-10-11 会话（用户："dlq 队列目前有问题吧，建个文档"）；问题源自 2026-09-29 E2E-22 实测（F-150）+ 2026-10-01 E2E-24（F-174/D-34）
depends-on: []
related-plans:
  - outbox-trace-continuity.md（同轮，消息链路）
---

# Plan — DLQ 硬化：消除丢消息黑洞 + 半自动回放 + 堆积可见

> **只描述要做的事，不含代码改动。**
> 现状：DLQ（`chat-events-dlq`）已能用——毒消息重试耗尽会进 DLQ、有回放工具。但仍有**一个丢消息黑洞**、**两种静默丢弃**、**只进不出**、**堆积不告警**四个问题。

> **⚠︎ 与 E2E 路线图的关系（2026-10-11 用户明确）**：本计划**独立于 E2E 路线图**——30 阶段已定稿，**不登记 E2E-F 账本编号、不归属任何阶段**。（追溯到的 E2E-F-12/150/174 引用仅作历史来源。）

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-ai-svc/internal/consumer/consumer.go:149-154` | 消费循环：`deliverWithRetry` 失败 → `publishToDLQ` → **无条件 `sess.MarkMessage(msg, "")`**（offset 前进） |
| `emotion-echo-ai-svc/internal/consumer/consumer_failure.go`（`publishToDLQ`） | DLQ.Publish 失败 → `IncDLQPublishResult(false)` + log；**消息已被 Mark，无法挽回**（注释自称"业务+DLQ 双失败黑洞，counter 让其可见"） |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go`（`publishToDLQ` + ConsumeClaim） | 同型：DLQ 失败计数 + log，`sess.MarkMessage(msg, "")` 无条件前进 |
| `emotion-echo-ai-svc/internal/consumer/dlq.go` | `NoopDLQPublisher`（DLQ 未配时**不发布、返回成功**，但调用方仍 Mark）；`KafkaDLQPublisher` 投 `chat-events-dlq` |
| `scripts/replay_dlq/main.go`（+ `main_test.go`） | **人工**回放工具（Go + sarama 字节级透传，决策 D-33）：剥 DLQ 诊断 headers、按 `x-original-topic` 重发布；幂等靠 `ON CONFLICT (event_id, occurred_at)` |
| `deploy/prometheus/rules/kafka-dlq.yml` | 只告警 **DLQ 投递失败**（`emotion_echo_dlq_publish_total{result="failure"}`）；**不告警堆积** |
| `deploy/grafana/dashboards/kafka-consumer-lag.json:124` | DLQ Depth 面板显式标 **non-alerting** |
| `deploy/docker-compose.apps.yml:203,383` | 两服务都配了 `KAFKA_DLQ_TOPIC: chat-events-dlq` |

### 0.2 相关历史决策（读 ADR/账本）

- **E2E-F-12**（已解决）：DLQ 无自动回放 → 建了 `scripts/replay_dlq`（人工工具，D-33）。
- **E2E-F-150**（已解决）：DLQ **堆积 24 条且面板永不可见**（原面板用 consumer lag 查一个没有 consumer 的 topic，恒 0 series）→ 面板改 offset 求和 + 保留 non-alerting。
- **E2E-F-174 / D-34**（已解决）：consumer 重试语义重写为**原地重试**（预算 MaxRetries+1、指数退避、耗尽 → DLQ + Mark 前进）。**明确取舍：宁可 Mark 前进不卡分区，也不让单条毒消息永久阻塞分区**（旧"按 msg.Key 计数"会让分区永久停滞）。

### 0.3 四个问题（本文范围）

1. 🔴 **业务+DLQ 双失败黑洞（永久丢消息）**：DLQ 投递失败时，代码只计数+日志，调用方**仍 Mark 前进** ⇒ 消息已被消费、又没进 DLQ ⇒ **永久丢失**。这是当前唯一的**数据丢失**路径。
2. 🟡 **NoopDLQ 静默丢弃**：`KAFKA_DLQ_TOPIC` 未配 → `NoopDLQPublisher` 返回成功 → 毒消息静默蒸发（dev/prod 已配，但缺配置时无任何提示）。
3. 🟡 **DLQ 只进不出**：无消费者订阅 `chat-events-dlq`，回放靠**人工** `go run ./replay_dlq`。堆积没有自动/半自动处置，且没人主动跑就永远堆着。
4. 🟡 **堆积不可见（non-alerting）**：DLQ Depth 面板存在但显式不告警——"进 DLQ 是预期行为"的假设下，**"只进不出地持续堆积"这个异常反而没人叫**（F-150 的 24 条就是这么堆起来的）。

---

## §1 目标与非目标

### 目标
- **G1** 消除"双失败黑洞"：DLQ 投递失败时**要么重试到成功、要么持久化兜底、要么可见地停住**，**绝不静默丢**。
- **G2** 消除 Noop 静默：DLQ 未配时启动期可见（fail-fast 或强告警）。
- **G3** 回放从"纯人工"升级为**半自动**（DLQ 消费者 + 人工确认闸门），且复用既有 `replay_dlq` 转换逻辑。
- **G4** 堆积**可告警**：从"不告警任何死信"改为"**告警持续堆积/超阈值**"（区分"偶发死信=预期"与"堆积=异常"）。

### 非目标
- 不改 `deliverWithRetry` 的原地重试语义（D-34 的产物，不动）。
- 不为"偶发死信"加告警——那是预期行为。
- 不登记 E2E-F、不挂阶段。

---

## §2 设计

### G1 · 消除双失败黑洞（**最高优先**）

**原则**：DLQ 投递失败时**不能既 Mark 又没落地**。三条候选路线（**建议组合 ①+②**）：

1. **有界重试 DLQ 投递**：`publishToDLQ` 内部按退避重试 N 次（如 3 次），给 Kafka 短暂抖动恢复窗口。
2. **持久化兜底（推荐）**：重试仍失败 → 把 DLQ 条目写进一个**本地持久化落点**（PG 表 `dlq_fallback` 或落盘文件），并**为它保留"未处置"状态**；后台再投递到 Kafka DLQ。这样即使 Kafka DLQ 长期不可用，消息也**不丢**。
3. **（备选，不推荐单独用）不 Mark 前进**：让分区停下重投。**风险**：会**重现 E2E-F-174 的分区永久停滞**（单条毒消息卡死分区），D-34 明确否决过。若采用，必须配"分区停滞告警"。

> **决策点（需用户拍板）**：走 ①+② 持久化兜底，还是 ② 之外再叠加 ③？

### G2 · Noop 不再静默

- 启动时若 `Kafka.Enabled && DLQTopic == ""` → **fail-fast**（或至少 `Warn` + 暴露一个 gauge `emotion_echo_dlq_disabled=1` 供告警）。与"业务依赖字段不做静默 localhost fallback"的项目既有原则一致（`ai-svc/main.go:216-220` 同款校验）。

### G3 · 半自动回放

- 新增 **DLQ 消费者**（订阅 `chat-events-dlq`）：
  - 复用 `scripts/replay_dlq` 的转换逻辑（剥诊断 headers、按 `x-original-topic` 重发、保留 sw8）；
  - **人工确认闸门**：默认只"预览/计数"，需显式开关（配置/env/管理端点）才真正回放——避免"修好的 bug 又被自动重放坏消息"；
  - 仍受消费端 `ON CONFLICT` 幂等兜底，重复回放不产生重复行。
- 保留 `scripts/replay_dlq` 作为离线/应急手段。

### G4 · 堆积可告警

- 新增规则（放 `deploy/prometheus/rules/`）：
  - `DLQBacklogGrowing`：`sum(kafka_topic_partition_current_offset{topic="chat-events-dlq"}) > <阈值>` **且持续**（`for: 30m`）——"堆积且没在减少"。
  - 或按增量：`increase(...[1h]) > 0` 且深度 > N。
- **原则**：不告警"有死信"，告警"死信堆积不消化"。同步更新 Grafana DLQ 面板标注（non-alerting → 堆积告警说明）。

---

## §3 分阶段实施（TDD）

### Phase 0 — 复现双失败黑洞（先证明问题真在）
- 单测/集成：令 `DLQ.Publish` 必失败（fake publisher），断言当前实现下 **offset 仍前进且消息不在任何落点**（复现 §0.3 #1）。

### Phase 1 — G1 RED → GREEN
- RED：断言"DLQ 投递最终失败时，消息可被找回"（持久化兜底里能查到）。
- GREEN：`publishToDLQ` 有界重试 + 持久化兜底表/文件 + 后台补投。
- 负向：DLQ 正常时行为不变（不产生多余兜底行）。

### Phase 2 — G2
- RED：DLQTopic 为空时启动不应静默成功（断言启动期报错或 gauge=1）。
- GREEN：fail-fast / 告警 gauge。

### Phase 3 — G3
- RED：DLQ 消费者在"闸门关"时只计数不回放；"闸门开"时按 `x-original-topic` 回放且幂等。
- GREEN：实现 + 复用 replay 转换逻辑（建议把 `scripts/replay_dlq` 的转换抽到可复用包）。

### Phase 4 — G4
- 新增规则 + promtool 校验 + 负向（偶发 1 条不触发、持续堆积触发）。

### Phase 5 — 文档
- runbook 增"DLQ 处置流程"（谁看、怎么看、怎么回放闸门开）；同步本计划状态。

---

## §4 验收与回归钉

- **G1**：单测断言"DLQ 失败时消息可从兜底找回"；负向"DLQ 正常时不产生兜底行"。
- **G2**：单测断言缺 topic 的启动行为。
- **G3**：consumer 单测（闸门关/开）+ 幂等复验（重复回放行数不变）。
- **G4**：promtool + 负向对照（偶发 vs 堆积）。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §5 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **改"不 Mark"会重现分区停滞** | E2E-F-174/D-34 明确否决过"不 Mark"（单条毒消息卡分区） | 优先用**持久化兜底**而非"停住"；若用停住必须配停滞告警 |
| **DLQ 与主 topic 同 broker** | Kafka 挂时"DLQ 投递失败"与"消费中断"同时发生，兜底其实很难被触发 | 兜底主要为"topic 不存在/鉴权/配额"等**局部故障**兜底，别指望它扛整机 Kafka 宕 |
| **自动回放放大故障** | 若毒消息是"必然失败"的，自动回放会无限循环 | 必须**人工确认闸门**，默认关 |
| **幂等** | 回放重复 | 依赖消费端 `ON CONFLICT (event_id, occurred_at)`（F-149 已修） |
| **落盘兜底的位置** | 容器重启丢文件 | 用 **PG 表**优先于文件（本项目 PG 是持久化的） |

---

## §6 待拍板

| # | 事项 | 说明 |
|---|---|---|
| D1 | G1 用"持久化兜底"还是"停住不 Mark"（或两者） | 建议持久化兜底为主 |
| D2 | G2 缺 DLQ 配置时：fail-fast 还是仅告警 | 建议 fail-fast（与既有"业务依赖不静默降级"原则一致） |
| D3 | G3 半自动回放的"闸门"形态：env 开关 / 管理端点 / 配置中心 | 建议配置中心（可热更，与项目 Nacos 机制一致） |

---

## §7 调研依据（本次实读）

- `emotion-echo-ai-svc/internal/consumer/{consumer.go, consumer_failure.go, dlq.go}`
- `emotion-echo-analytics-svc/internal/kafka/{consumer.go, dlq.go}`
- `scripts/replay_dlq/{main.go, main_test.go}`
- `deploy/prometheus/rules/kafka-dlq.yml`、`deploy/grafana/dashboards/kafka-consumer-lag.json`
- `deploy/docker-compose.apps.yml`（DLQ topic 配置）
- 账本 `docs/e2e-roadmap/discovered-unresolved.md`（E2E-F-12 / F-150 / F-174 原文）

**未能确认**（需实机）：当前 dev 栈 DLQ 里的实际存量条数；`emotion_echo_dlq_publish_total{result="failure"}` 是否有非零值（Phase 0 核对）。
