---
status: planned
priority: medium
owner: TBD
created: 2026-10-11
type: implementation-plan
source: 2026-10-11 会话（用户："关于 kafka 的观测链路断了这件事也落地一个计划文档"）；问题源自 2026-10-03 E2E-26 #4 运行时观察
depends-on: []
related-plans:
  - observability-business-metrics-wiring.md（同轮，监控栈补齐）
---

# Plan — 修复 outbox → relay → Kafka 的 trace 断点（异步链路贯通）

> **只描述要做的事，不含代码改动。**
> 问题：一次聊天触发的 SkyWalking trace 里，Kafka 发布 span 是**独立根**，接不回最初的 HTTP 入站 trace——因为**投递在后台异步执行、且 outbox 表没有存 trace 上下文**。

> **⚠︎ 与 E2E 路线图的关系（2026-10-11 用户明确）**：本计划**独立于 E2E 路线图**——30 阶段已定稿，**不登记 E2E-F 账本编号、不归属任何阶段**。（追溯到的 E2E-26 #4 引用仅作历史来源。）

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-chat-svc/main.go:238-253` | relay 启动：`relayCtx, _ := context.WithCancel(context.Background())` → **后台根 ctx**，与请求无关 |
| `emotion-echo-chat-svc/internal/outbox/relay.go` | `Run` 循环 → `FlushOnce` → `publishOne(ctx, e)` → `r.publisher.Publish(ctx, e.Topic, evt)`（`ctx` 就是那个后台根 ctx） |
| `emotion-echo-chat-svc/internal/repository/outbox.go:42-62` | `OutboxEvent` model + `CreateInTx(tx, e)`；**无 trace 字段** |
| `emotion-echo-chat-svc/migrations/c001_create_outbox_events.sql` | 表结构：`id/event_id/event_type/topic/payload/status/attempts/last_error/created_at/sent_at`——**无 trace 上下文列** |
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go:114-143` | `p.tracer.CreateExitSpan(ctx, "kafka-publish", topic, injector)` —— ctx 里没有父 span ⇒ go2sky 新建**根** span；sw8 写进 Kafka header 供消费者提取 |
| `emotion-echo-ai-svc/internal/consumer/consumer.go:120-140` | 消费端 `CreateEntrySpan(ctx, "kafka-consume", extractor)`——从 Kafka header 的 sw8 重建父 trace（**这一段是连的**） |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing.go` | 仓库级 tracer 抽象：`CreateEntrySpan → (ctx, Span, err)`、`CreateExitSpan → (ctx, Span, err)` |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go` | go2sky 适配：`CreateEntrySpan`/`CreateExitSpan` 内部调 go2sky 原语 |
| go2sky v1.5.0（`propagation/propagation.go`） | **`SpanContext.EncodeSW8() string`（147 行）/ `DecodeSW8(header) error`（101 行）**——正是存取两端要用的 API |
| go2sky v1.5.0（`trace.go:94`） | `CreateEntrySpan(ctx, op, extractor) (Span, context.Context, error)`——**可用 extractor 从外部 sw8 重建父子关系** |

### 0.2 根因

一次 `SendMessage` 的链路形如：
```
HTTP 入站(entry) → chat-svc 业务逻辑 → [同事务写 outbox] ⇢ ...时间隔断... ⇢ relay(后台 ctx) → kafka-publish(exit)
```
- 业务逻辑拿到的是**请求 ctx**（带当前 span）；
- 但写 outbox 行时**只存了业务 payload，没存 trace 上下文**；
- relay 跑在**后台根 ctx** 上，`CreateExitSpan` 找不到父 → 新建根 span（新 traceId）。
- ⇒ 入站 entry span 与 kafka-publish 及下游消费 span **分成两条 trace**。断言"同 traceId + ref 互指"不成立。

### 0.3 与既有机制的关系

- Kafka **生产→消费**这一段本身是连的（生产端写 sw8 到 header，消费端 extract）。断点**只在**"入站 → 写 outbox → relay 投递"之间。
- 因此修复只需**把入站 trace 上下文带过 outbox 这道异步边界**，下游全部沿用现有机制。

---

## §1 目标与非目标

### 目标
- **G1** outbox 行**持久化 trace 上下文**（sw8），入队时从请求 ctx 捕获。
- **G2** relay 投递时**恢复到该上下文**，使 `kafka-publish` span 正确挂到原始 trace 下。
- **G3** 端到端**同 traceId**：入站 `SendMessage` entry span、`kafka-publish` exit span、下游 `kafka-consume` entry span 属**同一 traceId**。
- **G4** 完全**向后兼容**：无 sw8 的旧行、tracer 为 nil 时行为与现状一致（新根 span，不报错）。
- **G5** 回归钉：机器可校验的"同 traceId"断言。

### 非目标
- 不改下游消费端（已连）。
- 不改 HTTP 出站 / DB / Redis 埋点（属另一条）。
- 不引入 W3C traceparent（沿用 sw8）。
- 不登记 E2E-F 账本、不挂阶段。

---

## §2 设计

### 2.1 数据模型：outbox 新增 trace 列

- 新增 migration（如 `c00X_add_outbox_trace_sw8.sql`）：
  ```sql
  ALTER TABLE emotion_echo_chat.outbox_events
    ADD COLUMN IF NOT EXISTS trace_sw8 VARCHAR(512) NULL;
  ```
- **可空**：旧行 / 无 tracer 场景为 NULL ⇒ relay 走原行为（新根 span），不破坏兼容。
- `OutboxEvent` model 增字段 `TraceSW8 string \`gorm:"column:trace_sw8"\``，`CreateInTx` 一并写入。

### 2.2 捕获端（入队时，chat-svc 三个 logic）

- 在 `createconversationlogic` / `sendmessagelogic` / `deleteconversationlogic` 调 `CreateInTx(tx, e)` 前，设 `e.TraceSW8 = skywalking.CaptureSW8(l.ctx)`。
- **`CaptureSW8(ctx) string` 实现**（放 `shared/pkg/skywalking`）：
  - go2sky 的 `Span` 接口**不导出 `Context()`**，故不能直接读当前 span 的 sw8。
  - 采用**注入器捕获**：`tracer.CreateExitSpan(ctx, "outbox-enqueue", "outbox", injector)`，injector 里收集 `sw8` 值 → 立即 `span.End()` → 返回该 sw8。
  - 语义上这也成立：入队**确实是一次"投递给 outbox"的出站动作**，留一个 exit span 合理。
  - tracer 为 nil ⇒ 返回 ""（不埋点，走降级）。

### 2.3 恢复端（投递时，relay）

- `Relay` 增字段 `Tracer grpcinterceptor.Tracer`（或注入一个 `restore func(ctx, sw8) (context.Context, func())` 钩子，便于单测）。
- `publishOne`：
  ```
  pctx := ctx
  if r.Tracer != nil && e.TraceSW8 != "" {
      extractor := func(key string) (string, error) {
          if key == "sw8" { return e.TraceSW8, nil }
          return "", nil
      }
      if c, s, err := r.Tracer.CreateEntrySpan(ctx, "outbox-relay", extractor); err == nil {
          pctx = c
          if s != nil { defer s.End() }
      }
  }
  return r.publisher.Publish(pctx, e.Topic, evt)
  ```
  - 这样 `kafka-publish` 的 ExitSpan 就挂在 `outbox-relay` entry 之下，而 `outbox-relay` 又父于存储的 sw8 ⇒ 接回原始 trace。
- sw8 为空 / tracer nil ⇒ `pctx` 保持原样，行为与今天完全一致。

### 2.4 装配

- chat-svc `main.go`：构造 relay 时把已装配的 tracer 传进去（`relay.Tracer = ...`）。tracer 来源与 kafka_publisher 同源（`skywalking.Tracer()`）。

### 2.5 兼容与降级矩阵

| 场景 | 期望行为 |
|---|---|
| tracer 正常、sw8 有值 | 全链路同 traceId（**本计划目标**） |
| 旧 outbox 行（trace_sw8 为 NULL） | 与现状一致：新根 trace，不报错 |
| tracer 为 nil（dev/未配） | 与现状一致：不埋点 |
| 采样未命中（sw8 缺失） | 新根 trace，不报错 |
| migration 未跑 | `CreateInTx` 写 NULL（列不存在会报错 ⇒ 需 migration 先跑，见 §5） |

---

## §3 分阶段实施（TDD）

### Phase 0 — 验证 API 与断点复现（**先复现再修**）
- 起 dev 栈，发一条消息；在 SkyWalking/OAP（或 OAP 日志）确认**当前** kafka-publish 与入站 `SendMessage` **不同 traceId**（复现 §0.2）。
- 确认 go2sky v1.5.0 `propagation.SpanContext.EncodeSW8/DecodeSW8` 与 `grpcinterceptor.Tracer` 包装的返回顺序（**以实际导出面为准**）。

### Phase 1 — RED：写"同 traceId"断言
- 单测（relay）：注入 fake tracer，断言 `publishOne` 传给 `Publish` 的 ctx 携带了由 `e.TraceSW8` 恢复的父（或断言 `CreateEntrySpan` 被以该 sw8 调用）；**当前实现下应为 RED**。
- 单测（capture）：断言入队时产出的 `OutboxEvent.TraceSW8` 非空（fake tracer 下）。

### Phase 2 — GREEN：migration + model + 捕获端
- 加 migration + model 字段 + `CreateInTx` 写入 + `CaptureSW8` 助手 + 三处 logic 捕获。

### Phase 3 — GREEN：relay 恢复端 + 装配
- `Relay.Tracer` + `publishOne` 恢复逻辑 + main 装配。

### Phase 4 — 端到端验收（**关键**）
- 发一条消息，查询 trace：断言
  1. 入站 `SendMessage` entry span、`kafka-publish` exit span、下游 `kafka-consume` entry span **同 traceId**；
  2. `kafka-publish` 的父 = `outbox-relay`（或与入站 span 有 ref 关系）。
- **负向对照**：手动插一行 `trace_sw8=NULL` 的 outbox，确认 relay 照常投递（不报错、走新根 trace）。

### Phase 5 — 文档同步
- 更新可观测性 runbook 的 trace 说明（"异步链路已贯通"）；同步本计划状态。

---

## §4 验收与回归钉

- **机器可判据（首选）**：加一个脚本/断言（可复用现有 smoke 形态），触发一次聊天后从 OAP 或日志拉取 traceId 做**同 traceId**比较。这是 E2E-26 当初用的判据，直接沿用。
- **单测**：capture 非空 + relay 恢复父（fake tracer）。
- **负向**：NULL sw8 行照常投递。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §5 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **migration 顺序** | 列不存在时 `CreateInTx` 写该字段会报错（INSERT 显式列） | migration 与代码同 PR、部署前先跑（既有 db-migrate 容器机制）；或字段用 `default ''` 过渡 |
| **捕获多出一个 span** | `CaptureSW8` 造了一个 `outbox-enqueue` exit span | 可接受（语义成立）；若不想要，改存"入站 sw8（请求头）"——但那样父是调用方、因果略偏，**推荐保留 enqueue span** |
| **go2sky API 假设** | `Span` 接口不导出 `Context()`，只能走 injector 捕获 | Phase 0 先核实导出面；injector 方案已在生产端用过、可靠 |
| **采样** | 未采样的 trace 无 sw8 | 降级为新根 trace，不报错 |
| **relay 批量** | 一批里每行 sw8 不同 | 逐行构造 ctx（`publishOne` 已是逐行） |
| **热点** | 每行多一次 `CreateEntrySpan` | 可忽略（relay 1s 一轮、批量百条） |

---

## §6 调研依据（本次实读）

- `emotion-echo-chat-svc/{main.go, internal/outbox/relay.go, internal/repository/outbox.go, internal/events/kafka_publisher.go, migrations/c001_create_outbox_events.sql, internal/grpcclient/ai_client_grpc.go}`
- `emotion-echo-ai-svc/internal/consumer/consumer.go`（消费端 extract 范式）
- `emotion-echo-shared/pkg/grpcinterceptor/{tracing.go, tracing_go2sky.go}`、`emotion-echo-shared/pkg/skywalking/tracing.go`
- go2sky **v1.5.0** 源码（`$GOMODCACHE/.../go2sky@v1.5.0`）：`propagation/propagation.go`（`EncodeSW8`/`DecodeSW8`）、`trace.go:94`（`CreateEntrySpan` 签名）、`span.go:42`（Span 接口无 `Context()`）

**未能确认**（需实机）：OAP 侧查询 traceId 的具体 API/查询方式；当前 dev 栈是否能复现（Phase 0）。
