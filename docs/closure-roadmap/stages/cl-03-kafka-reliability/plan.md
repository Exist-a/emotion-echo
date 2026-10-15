---
stage: cl-03
title: Kafka/可靠性闭环（outbox→relay→Kafka trace 贯通 + DLQ 硬化 + 遗留决策收尾）
type: transformation
status: in-progress
created: 2026-10-16
last-updated: 2026-10-16（建档：§A 实读 14 个文件 + 1 个只读盘点 agent 复核）
depends-on: []           # roadmap §2 写的"与 CL-01/02 共享追踪面"——两者均已 done（CL-01 #211 / CL-02 #214）
blocks: []
gate: []                 # 无开工阻塞决策门；[M] 项见 §7
source: docs/plans/outbox-trace-continuity.md（主）+ docs/plans/dlq-hardening-and-replay.md + docs/plans/kafka-pipeline-pending-decisions.md
related-plans:
  - docs/closure-roadmap/parallel-contract.md（F 轨占位 + §3.1 栈排队 + §四 握手区）
ledger: CL-F-15（outbox 静默改投内存，归本阶段）
---

# CL-03 · Kafka/可靠性闭环 — 执行计划

> **核心口径（roadmap §4 CL-03，用户 2026-10-13 拍板）**：**修完之后链路贯通** —— 一条消息的 trace 从
> **入站 → outbox → relay → Kafka → 消费端**连成一条，且能结合 SkyWalking UI 查看。**本阶段就盯这一条**，别的从简。
>
> **决策依据**：`CL-D-04` 已定 —— 三份计划（`outbox-trace-continuity` / `dlq-hardening-and-replay` /
> `kafka-pipeline-pending-decisions`）**合并为一次「异步闭环」执行**，不拆成三个阶段。
>
> **执行协议**：[RUNBOOK.md](../../RUNBOOK.md)（六步循环 / 证据有效性 / 收口契约）
> + [parallel-contract.md](../../parallel-contract.md)（F 轨占位与文件面隔离）
> + [anti-patterns.md](../../../e2e-roadmap/anti-patterns.md)（14 类失真反例）。
>
> **并行**：本阶段为 **F 轨**，与 **D 轨（CL-05 消息链路，运行中）** 并行。
> 唯一串行资源 = **dev 栈**（D 持锁中，本轨排队第 5 位）与 **`chat-svc/main.go`**（脊柱，已握手）。
> 本轨**完全不碰** 前端 / BFF / `proto/**`，与 D 轨的交集仅 `chat-svc/main.go` 一个文件。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件，本次实读亲核）

**① outbox → Kafka trace 断点（本阶段核心）**

| 文件 | 关键事实（file:line） |
|---|---|
| `emotion-echo-chat-svc/internal/repository/outbox.go` | `OutboxEvent` 结构体 `:42-53`（`ID/EventID/EventType/Topic/Payload/Status/Attempts/LastError/CreatedAt/SentAt`）—— **无任何 trace 字段**；`OutboxRepo` 接口 `:59-83`；`CreateInTx(tx, e)` `:62` 由调用方开事务；Postgres 实现 `:250-255`、InMemory 实现 `:102-122` |
| `emotion-echo-chat-svc/migrations/c001_create_outbox_events.sql` | 建表 DDL：`id/event_id/event_type/topic/payload/status/attempts/last_error/created_at/sent_at` —— **无 trace 列**；索引 `idx_outbox_pending`（partial `status='pending'`）+ `idx_outbox_attempts` |
| `emotion-echo-chat-svc/main.go:449-472` | `runOutboxMigration()` **内联硬编码了一份 outbox DDL**（`const sql = ... CREATE TABLE IF NOT EXISTS ...`）—— 与 `c001_*.sql` **两份事实源**，且已漂移（内联版**缺** `idx_outbox_attempts`）。本阶段改表时**必须一并决定这两份怎么收敛**（§6 R3） |
| `emotion-echo-chat-svc/internal/outbox/relay.go` | `Run(ctx)` `:80-95` 循环 → `FlushOnce(ctx)` `:98-134` → `publishOne(ctx, e)` `:141-150`；失败走 `MarkFailed` + `MarkDead`（`attempts >= currentMaxAttempts()`）`:111-126`；成功 `MarkSent` `:129-131`。**`ctx` 就是 `Run` 的入参** |
| `emotion-echo-chat-svc/main.go:244-259` | relay 启动：`relayCtx, relayCancel := context.WithCancel(context.Background())`（`:245`）→ `outbox.NewRelay(outboxRepo, pub, 1*time.Second, 100)`（`:247`）→ `go relay.Run(relayCtx)`（`:256-259`）⇒ **relay 全程跑在"后台根 ctx"上，与任何请求无关** |
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go` | `Publish(ctx, topic, e)` `:80-184`；`:133` `_, span, err = p.tracer.CreateExitSpan(ctx, "kafka-publish", topic, injector)` ⇒ **ctx 里没有父 span 时 go2sky 新建「根」span**；`:138-142` 把 injector 捕获到的 `sw8` 写进 Kafka header；`tracer == nil` 时整段降级跳过（`:117`） |
| `emotion-echo-ai-svc/internal/consumer/consumer.go:97-169` | 消费端 `ConsumeClaim` 里 `CreateEntrySpan` + sw8 extractor ⇒ **从 Kafka header 往后这一段是连的**（消费侧不是断点） |
| `emotion-echo-chat-svc/internal/logic/sendmessagelogic.go:186-214` | `DB.Transaction(...)` 内 `AppendMessageTx` + `IncrementMessageCountTx` + `OutboxRepo.CreateInTx(tx, &repository.OutboxEvent{EventID, EventType, Topic, Payload})`（`:202-207`）—— **写 outbox 行时身处请求 ctx**（该 ctx 里有 gin 入口 span） |
| `emotion-echo-chat-svc/internal/logic/createconversationlogic.go:127` / `deleteconversationlogic.go:102,117` | 另外两条写 outbox 行的路径（同样在请求 ctx 内；`delete` 还有一处 `CreateInTx(nil, ...)` `:117`） |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:101-135` | `Tracer` 接口**只有 4 个方法**：`StartEntry` / `CreateLocalSpan` / `CreateExitSpan(ctx, op, peer, injector)` / `CreateEntrySpan(ctx, op, extractor)` —— **没有"把当前 ctx 的 sw8 取出来"或"用 sw8 恢复父 ctx"的原语** |
| `emotion-echo-shared/pkg/grpcinterceptor/sw8.go:36-56` | 已有 `TraceIDFromSW8(sw8) string`（只解析 trace_id，**不做父 ctx 恢复**） |
| go2sky v1.5.0 `propagation/propagation.go` | `SpanContext.EncodeSW8() string`（147 行）/ `DecodeSW8(header) error`（101 行）—— 正是"存取两端"要用的原语（A 轨 CL-04 阶段的调研亦引此两行） |

**② DLQ 硬化四问题**

| 文件 | 关键事实（file:line） |
|---|---|
| `emotion-echo-ai-svc/internal/consumer/consumer.go:154-164` | `attempts, handlerErr := deliverWithRetry(...)`（`:154`）→ 失败则 `h.publishToDLQ(...)`（`:155-157`）→ **无条件 `sess.MarkMessage(msg, "")`（`:159`）** ⇒ **DLQ 投递成败与 offset 前进无关** |
| `emotion-echo-ai-svc/internal/consumer/consumer_failure.go:69-115` | `publishToDLQ`：建 `kafka-publish-dlq` exit span → `h.DLQ.Publish` → 按 `dlqErr` 调 `IncDLQPublishResult(true/false)`；**失败分支 `:104-111` 只有 `IncDLQPublishResult(false)` + `slog.ErrorContext`** —— 不阻断、不重投 ⇒ **业务事件 + DLQ 双丢，仅 counter 可见** |
| `emotion-echo-ai-svc/internal/consumer/dlq.go:53-63` | `NoopDLQPublisher.Publish` **`return nil`** ⇒ 未配 DLQ 时"发布成功"且无任何指标/log |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go:220-277` | 同型：`deliverWithRetry`（`:261`）→ `publishToDLQ`（`:263`）→ **`sess.MarkMessage`（`:267`）**；`:316-355` `publishToDLQ` 同构 |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go:75-76` | `NewConsumer` **默认装配 `NoopDLQPublisher{}`**（`WithDLQ` 才替换 `:83-89`）；`internal/kafka/dlq.go:41-43` Noop 同样 `return nil` |
| `emotion-echo-ai-svc/internal/consumer/consumer_failure.go:33-52` | `deliverWithRetry`：同条消息原地重试 `maxRetries+1` 次，`attempt` 是**循环局部变量**（无持久化）→ D3「attempts 不跨重启」现状成立 |
| `scripts/replay_dlq/{main.go,main_test.go}` + `scripts/test_replay_dlq.sh` | 回放工具**已存在但与 relay 同属人工**：`go run ./replay_dlq -brokers ... -limit 0 [-dry-run]`（`main.go:63-70`），无消费组、`OffsetOldest` 读 DLQ（`:74-113`）、剥诊断 header 重发（`:128-148`）；幂等靠 **consumer 侧 DB 约束** `ON CONFLICT (event_id, occurred_at) DO NOTHING`（`analytics-svc/internal/repository/event_repository.go:240-242`）；3 例单测 + 独立守卫脚本 |
| `deploy/prometheus/rules/kafka-lag.yml:5,14-15` | 文件头注释**自述「DLQ 不告警」**；仅 `KafkaConsumerGroupLagHigh`（`kafka_consumergroup_lag{consumergroup=~"ai-svc\|analytics-svc"} > 10000` for 5m） |
| `deploy/prometheus/rules/kafka-dlq.yml:26-27,39-40` | 只有**投递失败**告警（`AIDLQPublishFailure` / `AnalyticsDLQPublishFailure`，`rate(...{result="failure"}[5m]) > 0.1`）—— **无一条按 DLQ 深度/偏移量的告警** |
| `deploy/prometheus/rules/cl01-health-business.yml:83-86` | `DLQPublishFailing`：`increase(...{result="failure"}[10m]) > 0`（for 0m, critical）—— CL-01 新增，仍是"投递失败"而非"堆积" |
| `deploy/grafana/dashboards/kafka-consumer-lag.json:124,132,139` | 面板 title **显式标 `non-alerting`**；expr `sum(kafka_topic_partition_current_offset{topic="chat-events-dlq"}) or vector(0)` ⇒ **堆积可见但刻意不告警**（四问题之四） |
| `charts/emotion-echo/charts/prometheus/templates/configmap-rules.yaml` | k8s 侧**内联**全部 5 个规则文件（`outbox-dead.yml:29` / `kafka-lag.yml:48` / `kafka-dlq.yml:67` / `observability-self.yml:101` / `cl01-health-business.yml:148`）⇒ **新增告警必须同步这份内联副本** |
| `deploy/obs_monitoring.test.js:182-204` | group ⑤「k8s 侧规则挂载」：把 `deploy/prometheus/rules/*.yml` 的 `alert:` 名集合与 chart 内联集合求差，断言 **双向为空** ⇒ 漏同步 = 该测试红（现成的机器守卫，本阶段直接复用） |

**③ 遗留决策（`kafka-pipeline-pending-decisions.md` D1~D8）**

| 文件 | 关键事实（file:line） |
|---|---|
| `emotion-echo-chat-svc/main.go:163-183` | `:164` 先给 `var pub events.EventPublisher = events.NewInMemoryEventPublisher()`；`:166-173` Kafka producer 构造失败 → log + `outbox.IncSentViaFallback()`，**`pub` 仍留 InMemory** ⇒ 此后 relay 每轮把行 `MarkSent` 而事件只进内存（**CL-F-15**） |
| `emotion-echo-chat-svc/internal/outbox/metrics.go:43-49` | `OutboxSentViaFallbackTotal{Name: "emotion_echo_outbox_sent_via_fallback_total"}` + `IncSentViaFallback()`；拉远看：该 counter **只在启动失败那一次 +1**，不逐条计数 |
| `docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md:183-244` | **ADR-19「D1 fallback 路径登记」段**：明确把"fallback 到 InMemory"登记为**短期选项 C**（指标可见），**长期选项 B（relay 收紧）留 open** ⇒ 本阶段若动 fallback 语义，**必须回写本 ADR** |
| `emotion-echo-shared/pkg/messaging/sw8_header.go:12,23-33` | **D8-2 实际已收敛**：`Sw8HeaderName` + `ExtractSw8Header` 真实现已在 shared；`ai-svc/internal/consumer/consumer_failure.go:125-127` 与 `analytics-svc/internal/kafka/consumer.go:368-370` **只剩转发的薄包装** ⇒ **计划文档写的"D8-2 仍 open"是文档漂移**（本阶段修文档） |
| `emotion-echo-chat-svc/internal/outbox/cleanup.go:33-79` + `internal/config/config.go:118` | D2 已落：`CleanupOnce` + `DeleteOlderThan`；`OUTBOX_CLEANUP_ENABLED` **默认 false**（`:118` 注释：prod 显式启用） |
| `emotion-echo-analytics-svc/internal/config/config.go:29,106-108` + `main.go:59-64` | D4 已落：`Kafka.MaxRetries` + `KAFKA_MAX_RETRIES` env |
| `emotion-echo-chat-svc/internal/outbox/relay.go:99-105` + `repository/outbox.go:257-268` | D5 现状：`ListPending` 无 `FOR UPDATE SKIP LOCKED`、relay 无租约 ⇒ 多副本会重复发（`event_id` UNIQUE 兜底不产生重复数据） |

### A.2 相关 ADR / 决策

- 已查：[`adr-2026-09-chat-svc-table-deps.md`](../../../architecture/adr/adr-2026-09-chat-svc-table-deps.md)（**决策 22**）—— chat-svc 拥有 `outbox_events`；**§四 定死「字段变更 checklist」9 步**（migration / 集中 initdb / model / types / proto / 跨域影响 / 集成测试 / 回归守护 / 文档同步），**任何加列都必须按它走**。
- 已查：[`adr-2026-09-dev-publisher-user-behavior-events.md`](../../../architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)（**ADR-19**）—— D1 fallback 的**已登记**状态 + 长期项 B。
- 已查：`docs/closure-roadmap/decisions.md` —— 相关只有 `CL-D-04`（合并执行）；`CL-D-08~10` 属 C 轨（已随 #214 落地），**本阶段新裁定从 `CL-D-15` 起**。
- 已查：`docs/closure-roadmap/discovered-unresolved.md` —— **CL-F-15** 归本阶段；`CL-F-16`（health readiness）归 CL-09、`CL-F-17`（数字人 VRM 竞态）待立项，**均不属本阶段**。

### A.3 架构假设清单（本文假设 vs 现状核对）

| 假设 | 现状核对 |
|---|---|
| 「trace 断点在 outbox，不在消费端」 | ✅ 成立。断点在 `relay` 用后台根 ctx 发布（`main.go:245` → `relay.go:99` → `kafka_publisher.go:133`）；消费端已有 sw8 extractor（`ai-svc/consumer.go:120-140`） |
| 「outbox 表能存 trace 上下文」 | ❌ **不成立**：表与 model 都无 trace 列（`c001_*.sql` / `outbox.go:42-53`）⇒ 本阶段要加列 |
| 「shared tracer 现成支持 sw8 存取」 | ⚠️ **部分**：有 `CreateExitSpan(injector)` / `CreateEntrySpan(extractor)`，**没有**"直接取当前 ctx 的 sw8 / 用 sw8 造父 ctx"的原语（`tracing.go:101-135`）⇒ 取法要设计（§7 M1） |
| 「DLQ 投递失败时消息不会丢」 | ❌ **不成立**：`MarkMessage` 无条件前进（`ai-svc/consumer.go:159`、`analytics/consumer.go:267`）⇒ 双失败即黑洞 |
| 「DLQ 堆积有告警」 | ❌ **不成立**：面板自标 non-alerting（`kafka-consumer-lag.json:124`），规则里无一条 depth 告警 |
| 「D8-2（sw8 解析双份）还没收敛」 | ❌ **文档过期**：代码已收敛到 shared（`messaging/sw8_header.go`），两 svc 只剩薄包装 |
| 「outbox 表 DDL 只有一份事实源」 | ❌ 两份：`migrations/c001_*.sql` 与 `main.go:449-472` 内联 DDL（且已漂移）|
| 「改 outbox 表只需加 migration」 | ⚠️ 还要走决策 22 的 9 步 checklist（含 `deploy/db/` 同步判定、model、跨域影响、schema 漂移守护） |

### A.4 未跑 smoke 声明（如适用）

**本轮建档未起栈、未跑 `scripts/smoke_data_layer.py`**。原因：dev 栈由 **D 轨（CL-05）持锁运行中**（`deploy/.devmode-session` owner=`lane-cl-d`，2026-10-16 06:16 取锁），并行契约 §3.1 明确「不得强占持锁方」。
⇒ 本文所有现状结论**均来自实读代码 / 配置 / 既有测试文件**（上表逐条附 file:line），**不含运行时观测**；凡需运行时证据的测试点一律标 🔴 并排队等栈。

---

## §1 目标与非目标

**目标（对齐 roadmap §4「链路贯通」）**
1. **trace 贯通**：一条消息的 trace 从**入站 HTTP → chat-svc → outbox → relay → Kafka → 消费端**连成一条，可在 SkyWalking UI 查看（当前 relay 段是**根 span** ⇒ 断）。
2. **DLQ 不再有"黑洞 / 静默"**：四问题逐条有结论（双失败黑洞 / Noop 静默 / 只进不出 / 堆积不告警）。
3. **遗留决策收尾**：D1（fallback，并入 CL-F-15）/ D8-2（文档修正）/ D3·D5·D6·D7（**只做登记与触发条件**，不扩范围）。

**非目标（明确不做）**
- ❌ **不动** 前端 / BFF / `proto/**`（与 D 轨（CL-05）的文件面硬隔离）。
- ❌ 不做 D5 的 `FOR UPDATE SKIP LOCKED` 与 D3 的 attempts 持久化 —— 二者**触发条件 = chat-svc/consumer 扩多副本**，当前单副本 ⇒ 本阶段只登记触发条件（与计划文档既有口径一致）。
- ❌ 不重写回放工具（`scripts/replay_dlq` 已可用且有人工守卫）—— 本阶段只判"半自动"要做到什么程度（§7 M3）。
- ❌ 不改指标语义（继承 CL-01 约束）；不新增容器（内存红线）。

---

## §2 测试点清单（每点标 `[A]`/`[V]`/`[M]`）

> 结果四值 `PASS / FAIL / BLOCKED / N/A` + 证据（RUNBOOK §4）。🔴 = 需 dev 栈（本轨排队第 5 位）。

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-01 | `outbox_events` 新增 `trace_sw8` 列，migration 幂等可重跑 | [A] | 新增 `c010_*.sql` + `IF NOT EXISTS`；决策 22 checklist 逐条过 | |
| T-02 | 写 outbox 行时把**当前请求 ctx 的 sw8** 落进 `trace_sw8`（三条 logic 路径） | [A] | Go 单测：注入 fake tracer → 断言列非空且等于注入值 | |
| T-03 | relay 发布前**从列恢复父 ctx**，`kafka-publish` exit span 不再是根 | [A] | Go 单测：断言 publisher 拿到的 ctx 携带父 sw8（或断言注入 header 与列值一致） | |
| T-04 | 🔴 端到端：发一条消息 → SkyWalking UI 里入站与 Kafka 段**同一条 trace** | [A] | 起栈 → 经网关发消息 → 查 OAP trace（需栈） | |
| T-05 | 向后兼容：`trace_sw8` 为空（历史行 / tracer=nil）时 relay 行为**与改前逐字一致** | [A] | Go 单测：空列 ⇒ 不注入 header、不建父 span | |
| T-06 | DLQ 投递失败**不再静默丢**（双失败黑洞）| [A] | Go 单测：DLQ publisher 返错 ⇒ 断言 `MarkMessage` **未被调用**（或按 §7 M2 裁定的语义断言） | |
| T-07 | 未配置 DLQ 时 `NoopDLQPublisher` **不再静默** | [A] | Go 单测：断言走 Noop 时 counter/log 被触发 | |
| T-08 | 新增 **DLQ 堆积告警**，且 k8s 内联副本同步 | [A] | 新规则 + `charts/.../configmap-rules.yaml` 同步 + `node deploy/obs_monitoring.test.js` 双向断言绿 | |
| T-09 | 🔴 DLQ 堆积告警**负向对照**：真灌消息 → `firing` → 送达 | [A] | 起栈 → 造堆积 → 查 alertmanager（需栈） | |
| T-10 | 回放"半自动"程度落实（按 §7 M3 裁定） | [A]/[M] | 按裁定执行 | |
| T-11 | DLQ 深度/累计指标**可被面板与规则消费**（非"定义了没人用"） | [A] | 断言新 expr 返回非空 series（需栈的部分标 🔴） | |
| T-12 | **CL-F-15**：producer init 失败**不再静默改投内存**（按 §7 M4 裁定） | [A] | Go 单测 + 起栈对照（🔴） | |
| T-13 | ADR-19「D1 fallback」段按最终语义回写 | [A] | 文档 diff | |
| T-14 | `kafka-pipeline-pending-decisions.md` 的 **D8-2 状态修正**（文档写 open，代码已收敛） | [A] | 文档 diff + 引用 shared 文件行号 | |
| T-15 | D3 / D5 / D6 / D7 **逐条终态**（落 / 登记触发条件），不留"待定" | [A] | 计划文档 §7 + 决策表 | |
| T-16 | 本阶段新增/修改的告警与面板**都在 dev 侧真被消费**（承接 CL-01 口径，"埋了要有人看"） | [A] | 规则 expr 有 series / 面板有值 | |

---

### 2.1 执行结果（滚动更新 · 逐点四值）

> 四值口径见 RUNBOOK §4；证据以"命令 + 输出"为准（"已创建"不算证据）。完整输出见 [report.md](report.md)。

| # | 结果 | 证据（2026-10-16） |
|---|---|---|
| T-01 | ✅ **PASS** | Phase 1：`migrations/c010_add_trace_sw8_to_outbox.sql` + `TestC010_TraceSw8Migration_*` 4 项绿；`go test ./migrations/` ok |
| T-02 | ⏸ **PENDING**（等 M1 裁定） | 写侧捕获 sw8 需先定原语落点（plan §7 M1）；未动代码 |
| T-03 | ⏸ **PENDING**（等 M1 裁定） | relay 恢复父 ctx 同上 |
| T-05 | 🟡 **PARTIAL** | 已验：InMemory repo 空值合法（`TestOutboxEvent_TraceSw8_EmptyIsAllowed` 绿）；**未验**：relay 侧空列时行为逐字不变（需 Phase 3 落地后才有对象可测） |
| T-08 | ✅ **PASS** | Phase 4a：`node deploy/obs_monitoring.test.js` group ⑦ 6/6 绿（含 k8s 内联同步 + 面板标签）+ `bash scripts/test_helm_prometheus_render.sh` **PASS=8 FAIL=0**（两侧 14 条告警一致） |
| T-14 | ✅ **PASS** | Phase 6：`kafka-pipeline-pending-decisions.md` D8 行/两处摘要更正为"已收敛"，附 `shared/pkg/messaging/sw8_header.go:23` 等行号证据 |
| T-15 | ✅ **PASS** | Phase 6：新增「CL-03 处置」表 —— D1 并入 CL-F-15 / D2·D4 ✅ / D3·D5·D6 登记触发条件 / D7 等产品语义拍板 / D8 ✅ 全部落地（无"待定"残留） |
| T-04 / T-06 / T-07 / T-09 / T-10 / T-11 / T-12 / T-13 / T-16 | ⏸ **PENDING** | T-04/T-09/T-11/T-16 需栈（排队第 5 位）；T-06/T-07/T-10 等 M2/M3 裁定；T-12/T-13 等 M4 裁定 |

**Phase 4a 附带新账**：`CL-F-22`（内联 DDL 缺 `idx_outbox_attempts`）· `CL-F-23`（DLQ 无真实深度指标，`oldest_offset` 可采性未实测）—— 均已登 `discovered-unresolved.md`。

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

> 铁律：**先写会失败的测试**（RED）→ 最小实现（GREEN）→ 重构。每个 Phase 一个 TDD 循环，一个 PR 内多个 commit。

| Phase | 内容 | RED 断言 | 需栈 |
|---|---|---|---|
| **0** | 基线取证（若拿到栈窗口）：一条消息的 trace 现状——`kafka-publish` 为根 span 的**实拍证据** | — （现状取证，非 TDD） | 🔴 |
| **1** | `trace_sw8` 列：migration + model + 决策 22 checklist | 断言 model 有该字段 / migration 幂等可重跑 / DDL 漂移守护 | 🟢 |
| **2** | 写侧捕获 sw8（三条 logic 路径） | 断言 `OutboxEvent.TraceSW8` 非空且等于注入值 | 🟢 |
| **3** | relay 侧恢复父 ctx | 断言 exit span 的父 sw8 == 列值；空列时行为不变 | 🟢 |
| **4** | DLQ 硬化（四问题）：黑洞 / Noop 可见化 / 堆积告警 + k8s 同步 / 回放程度 | 每条一个 RED（详见 T-06~T-11） | 🟢（T-09 需栈） |
| **5** | CL-F-15 fallback 语义 + ADR-19 回写 | 断言 producer init 失败时的行为 == 裁定语义 | 🟢 |
| **6** | 遗留决策收尾（D3/D5/D6/D7 登记 + D8-2 文档修正） | 文档断言（状态列无"待定"残留） | 🟢 |
| **7** | 🔴 端到端贯通 + 负向对照 + 收口冒烟 | T-04 / T-09 / T-11 | 🔴 |

**排序理由**：Phase 1~3 是"链路贯通"的**编码主体**，全部可单测（🟢）；Phase 4~6 是可靠性与文档；Phase 7 集中用一次栈窗口，避免反复排队。

---

## §4 收口判据（"做到什么程度"，对齐 roadmap §4）

**功能判据**
- 一条消息的 trace 在 SkyWalking UI 里**入站 → outbox → relay → Kafka → 消费端连成一条**（T-04）。
- DLQ 四问题逐条有终态：**不再有静默丢失路径**、堆积**有告警**并真的 firing 过一次（T-06~T-09）。
- `CL-F-15` 关闭：fallback 不再无声（T-12）。

**机器判据**
- 新增/修改包 `go test ./...` 全绿（ai-svc / analytics-svc / chat-svc / shared）。
- `node deploy/obs_monitoring.test.js` 绿（告警与 k8s 内联双向一致）。
- 本阶段新增的每个告警/面板 expr 在 dev 侧返回**非空 series**（T-11/T-16）。
- 回归钉落地：见 §5。

---

## §5 回归钉

- **Go 单测（本阶段主体）**：outbox trace 往返（写→读→注入）、relay 空列兼容、DLQ 失败不 Mark、Noop 可见化、fallback 语义。
- **契约/静态钉**：`deploy/obs_monitoring.test.js`（告警 ↔ k8s 内联双向）、migration 结构契约（`scripts/test_migrations_contract.sh`）、schema 漂移守护（决策 22 §四 第 8 步）。
- **运行时钉（🔴 需栈）**：一次端到端 trace 贯通取证（OAP trace JSON）+ DLQ 堆积告警 firing 一次（alertmanager 截图/JSON）。

---

## §6 风险与坑

- **R1（最大）：`Tracer` 接口若无 sw8 原语，取"当前 ctx 的 sw8"会走非常规路子。** 候选：① 给接口加 `EncodeSW8(ctx)`/`ContextWithSW8(ctx, sw8)`（要同步所有实现 + 接口契约测试）；② 复用 `CreateExitSpan(ctx, "outbox-enqueue", ...)` 的 ip 捕获 sw8 后再 `EndSpan`（零接口改动但语义别扭，会多出 span）。**需用户裁定（§7 M1）**。
- **R2：改 outbox 表 = 触决策 22 的 9 步 checklist**，漏步即"架构改动无 ADR/无跨域评估"。特别是第 8 步"schema 漂移守护"与第 9 步"本 ADR §二 表清单更新"。
- **R3：outbox DDL 有两份事实源且已漂移**（`migrations/c001_*.sql` vs `main.go:449-472` 内联）。改表必须一并处置：要么内联版改为"只做幂等兜底并指向 migration"，要么删内联改由 migrate 统一。**否则新列在"内联先建表"的路径上会缺**。
- **R4：DLQ 失败不 Mark 会引入"重投"，与既有幂等假设耦合。** 消费端幂等靠 DB UNIQUE（`ON CONFLICT`），但**不是所有 handler 都幂等**（如外部副作用）⇒ 语义要用户确认（§7 M2）。
- **R5：告警新增必须同步 k8s 内联副本**（`obs_monitoring.test.js` group ⑤ 会红）——这是**好事**（现成守卫），但容易漏。
- **R6：栈窗口稀缺**（本轨排队第 5 位，D 轨持锁中）⇒ 🔴 项集中到 Phase 7，不在中间反复起栈。
- **R7：CL-F-15 的"正确修法"有分歧**：① 后台重试初始化 Kafka（与 CL-F-14 的 tracer 后台补挂同构）；② 失败即不让 relay `MarkSent`（行留 pending，Kafka 恢复后自然重发）。两者都改行为（§7 M4）。

---

## §7 需用户裁定项（[M]，进阶段逐条提出）

| 编号 | 事项 | 选项 | 我的建议 |
|---|---|---|---|
| **M1** | sw8 存取原语的落点 | A. 给 shared `Tracer` 接口加 `EncodeSW8`/`ContextWithSW8`（+ 同步实现与接口契约测试）<br>B. 用 `CreateExitSpan` 的 injector 旁路捕获（零接口改动，多一个无用 span） | ✅ **裁定 = A**（多轮追问后由用户拍板，见 `CL-D-15`） |
| **M2** | DLQ 投递失败时的语义 | A. **不 `MarkMessage`**（消息留待重投，靠消费端幂等去重）<br>B. 仍然 Mark，但落库/落 counter 成"可逐条观测的黑洞" | ✅ **裁定 = A**（见 `CL-D-16`；落地前须逐 handler 核实幂等）|
| **M3** | 回放"半自动"到什么程度 | A. 保持人工工具（只补文档/守卫）<br>B. 加一个"一键回放全部积压"的脚本 + 回放计数指标<br>C. 做定时自动回放 | ✅ **裁定 = B**（见 `CL-D-17`；不做 C，防二次投毒）|
| **M4** | CL-F-15 fallback 改法 | A. Kafka 初始化**后台重试**（对齐 CL-F-14 tracer 补挂模式）<br>B. init 失败时**不让 relay MarkSent**（行留 pending）<br>C. A+B 都做 | ✅ **裁定 = C**（见 `CL-D-18`；须回写 ADR-19）|

> **裁定状态（2026-10-16）**：四项**均已由用户拍板**（全部采纳推荐项），已登记为 `CL-D-15`~`CL-D-18`。
> 编号前已按 `decisions.md 使用约定` 执行 `grep` 核对（main 最大号为 14；在飞分支 #219/#231/#229 均未占用 15+）。
> ⇒ **Phase 2/3/4b/4c/5 解除阻塞，可开工**。

---

## §8 调研依据（本次实读）

**文件（实读，非文档转述）**
- `emotion-echo-chat-svc/`：`internal/repository/outbox.go`、`internal/outbox/{relay.go,metrics.go,cleanup.go}`、`internal/events/kafka_publisher.go`、`internal/logic/{sendmessagelogic.go,createconversationlogic.go}`（outbox 写入段）、`main.go`（`:128-267` relay/fallback、`:449-472` 内联 DDL）、`migrations/c001_create_outbox_events.sql`
- `emotion-echo-ai-svc/internal/consumer/{consumer.go,consumer_failure.go,dlq.go}` + 三个测试文件
- `emotion-echo-analytics-svc/internal/kafka/{consumer.go,dlq.go}` + `internal/config/config.go` + `main.go`
- `emotion-echo-shared/pkg/{grpcinterceptor/{tracing.go,sw8.go},messaging/sw8_header.go}`
- `scripts/replay_dlq/{main.go,main_test.go}`、`scripts/test_replay_dlq.sh`、`scripts/test_migrations_contract.sh`
- `deploy/prometheus/rules/{kafka-lag.yml,kafka-dlq.yml,outbox-dead.yml,cl01-health-business.yml}`、`deploy/grafana/dashboards/kafka-consumer-lag.json`、`deploy/obs_monitoring.test.js`、`charts/emotion-echo/charts/prometheus/templates/configmap-rules.yaml`、`deploy/db/migrate.sh`
- `docs/architecture/adr/{adr-2026-09-chat-svc-table-deps.md,adr-2026-09-dev-publisher-user-behavior-events.md}`
- `docs/plans/{outbox-trace-continuity.md,dlq-hardening-and-replay.md,kafka-pipeline-pending-decisions.md}`（**仅作任务来源**，其现状描述已按实读校正）

**工具**：一个只读盘点子代理复核了 ②③ 两块（DLQ/遗留项/deploy 观测面）的 file:line 事实，结论与本人实读一致。

**未做**：未起栈、未跑 smoke（dev 栈被 D 轨持锁，见 §A.4）。
