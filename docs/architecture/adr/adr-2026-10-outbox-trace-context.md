# ADR · 2026-10 · outbox 承载 trace 上下文（trace_sw8 列 + relay 恢复父 ctx）

- **编号**：决策 42
- **日期**：2026-10-16
- **状态**：✅ **Accepted**
- **触发**：CL-03「Kafka/可靠性闭环」的核心判据 —— 一条消息的 trace 必须从**入站 → outbox → relay → Kafka → 消费端**连成一条（可结合 SkyWalking UI 查看）
- **关联决策**：`CL-D-15`（sw8 存取原语落点：给 shared `Tracer` 加方法）、`CL-D-16`（DLQ 失败不 Mark）、`CL-D-18`（fallback 后台重试）
- **关联 ADR**：[决策 22 chat-svc 表依赖清单 + 字段变更契约](adr-2026-09-chat-svc-table-deps.md)（本 ADR 新增的列走它的 9 步 checklist）、[ADR-19 DevEventPublisher / D1 fallback 登记](adr-2026-09-dev-publisher-user-behavior-events.md)
- **阶段档**：`docs/closure-roadmap/stages/cl-03-kafka-reliability/{plan.md,report.md}`

---

## 一、上下文（Context）

### 1.1 问题（2026-10-16 实读代码确认，非推断）

一次聊天触发的 SkyWalking trace 在 **outbox → relay** 处断掉：

| 环节 | 事实 | 证据 |
|---|---|---|
| 写入 outbox 行 | 在**请求 ctx** 内（有 gin 入口 span） | `chat-svc/internal/logic/{send,create,delete}*logic.go` 的 `DB.Transaction` 段 |
| relay 发布 | 跑在**后台根 ctx** 上 ⇒ exit span 无父 | `main.go:245` `relayCtx, _ := context.WithCancel(context.Background())` → `outbox/relay.go:99` → `events/kafka_publisher.go:133` `CreateExitSpan(ctx, ...)` |
| 消费端 | **这一段本来是连的**（从 Kafka header 的 sw8 重建父 trace） | `ai-svc/internal/consumer/consumer.go` 的 `CreateEntrySpan` + extractor |

即：断点只有一处 —— relay 那一跳没有可用的父 trace 上下文。

### 1.2 为什么不能"直接沿用 span 对象"

relay 与请求**不同生命周期**（请求早已结束、goroutine 还活着），跨 goroutine 传递
`Span` 句柄既不安全也无意义；可传递的只有**序列化后的传播头 sw8**。

### 1.3 硬约束

- `outbox_events` 是 chat-svc 自治表，字段变更必须走**决策 22 的 9 步 checklist**；
- `AGENTS §四` 红线：**span 只在有 ctx 时建，不硬造根 span**；
- 不改指标语义、不新增容器（内存红线，继承 CL-01 约束）。

---

## 二、决策（Decision）

**把 sw8 存进 outbox 行，relay 发布前用它恢复父 ctx。** 具体三条：

### 2.1 存储：`outbox_events` 新增 `trace_sw8 VARCHAR(512) NULL`

- 可空；历史行 / SkyWalking 关闭时为 `NULL`/空串（合法状态，不报错）。
- 迁移 `emotion-echo-chat-svc/migrations/c010_add_trace_sw8_to_outbox.sql`（幂等、带原因/跨域/回滚注释）。
- 跨域影响：**无**（不进 Kafka payload / 不进 proto / 无视图依赖）⇒ 无需同步 `deploy/db/02-create-tables-in-schemas.sql`（outbox_events 本就不在集中 initdb 内，见决策 22 §2.3）。

### 2.2 写入侧：shared `Tracer` 新增 `EncodeSW8(ctx, operationName) (string, error)`

- 语义：取出**当前 ctx 所在 span** 的 sw8 传播头；无 tracer / 编码失败 ⇒ `("", nil)` 降级，**绝不抛错**。
- 调用点统一走 `chat-svc/internal/outbox.EncodeTraceSW8(ctx, opName)`（读 `grpcinterceptor.GlobalTracer()`
  —— 采纳 **CL-F-14 的"请求期现读全局"** 语义：tracer 可能后台才补挂上，构造期快照会漏掉）。
- **实现形态（go2sky v1.5.0 限制，实测确认）**：该库的 `Span` 接口不暴露 `SpanContext`；
  `ReportedSpan.Context()` 返回的是 `*SegmentContext`（≠ `propagation.SpanContext`）；
  自拼 `SpanContext` 需要 `tracer` 的**私有字段** `service`/`instance`（`trace.go:181-182`）
  ⇒ 包外无法复刻编码。**唯一可行路径** = 借一次 `CreateExitSpan` 的 injector 拿到编码值，
  随即 `EndSpan`（不留悬挂 span，避免 duration 失真 —— 同 Stage 94 PR-1 踩过的坑）。
- 该 exit span 的 peer 取固定哨兵 `"trace-context"`：go2sky 校验 exit span 的 peer 非空
  （`trace.go:156`，空串 ⇒ `errParameter "parameter are nil"`），而本 span 不指向真实对端。

### 2.3 恢复侧：relay 用**既有** `CreateEntrySpan` + extractor（不新增方法）

```go
// outbox/relay.go resumeTraceCtx（示意）
nCtx, span, err := tr.CreateEntrySpan(ctx, "outbox-relay", func(key string) (string, error) {
    if key == grpcinterceptor.SW8HeaderName { return e.TraceSW8, nil }
    return "", nil
})
```

- tracer 来源：`Relay.Tracer`（显式注入，单测用）优先，否则回落 `grpcinterceptor.GlobalTracer()`。
- 空列 / 无 tracer ⇒ 原样 ctx + no-op end（**行为与改前逐字一致**，向后兼容）。
- 发布 err **透传** `EndSpan(err)`，让 OAP 把这一跳标红。

---

## 三、后果（Consequences）

### 3.1 收益

- 断点接上：请求入口 span → `outbox-enqueue` → `outbox-relay` → `kafka-publish` → 消费端 entry span，
  **同一条 trace**（在进程内已用 `TestSW8RoundTrip_EncodeThenRestore_SameTraceID` 钉死；
  端到端 UI 证据归 CL-03 Phase 7，需起栈）。
- 成本可控：一列 + 一个接口方法；写入侧对业务**零影响**（任何失败都降级为空串）。

### 3.2 代价（如实登记，勿当零成本）

1. **每个 trace 会多两个短 span**：`outbox-enqueue-*`（为编码而建）与 `outbox-relay`（为恢复而建）。
   前者的 peer 是哨兵 `"trace-context"` ⇒ 拓扑图上会出现指向该哨兵的边。语义上仍成立
   （"把 trace 上下文写进异步载体"确实是一次出站动作），但**是观测噪音**。
2. **接口变更外溢**：`Tracer` 加方法后，6 处测试 mock（跨 5 个模块）必须同步实现
   —— 这是 `CL-D-15=A` 的已知代价，已一次性付清（同一提交内）。
3. **列可空 ⇒ 存在"有行无 trace"的状态**（历史行、SkyWalking 关闭、编码失败）。
   消费方（relay / 排查者）必须按"可空"处理，不得假定非空。

### 3.3 已知限制与触发条件

- **outbox DDL 有两份事实源**（`migrations/c0NN_*.sql` 与 `main.go runOutboxMigration` 内联），
  本 ADR 只保证了"新列两侧同步"（`TestC010_TraceSw8_InlineDDLNotDrifted`）＋内联路径补了
  幂等 `ADD COLUMN IF NOT EXISTS`；**两份是否合并**未决，已登账 **`CL-F-22`**（含 `idx_outbox_attempts` 漂移）。
- **`EncodeSW8` 的编码代价**依赖 go2sky 内部实现（借 exit span）。若未来 go2sky 暴露
  `SpanContext`，应改成直接编码并删掉哨兵 peer —— 判据：round-trip 测试仍绿且不再产生额外 span。
- **多副本**：relay 多副本互斥（D5，`FOR UPDATE SKIP LOCKED`）**不在本 ADR 范围**，
  触发条件 = chat-svc 决定扩副本（见 `kafka-pipeline-pending-decisions.md` D5）。

---

## 四、被否决的备选（Alternatives considered）

| 备选 | 为什么否决 |
|---|---|
| **A. 在调用点用 `CreateExitSpan` 的 injector 旁路捕获 sw8**（即 `CL-D-15` 的 B 选项） | 语义散落在 3 个业务 logic 里、易漂移；同一 hack 会被写三遍。用户裁定取 A（封装成接口原语） |
| **B. 只存 trace_id（不存 sw8）** | 只有 trace_id **无法恢复父子关系**（还缺 segment/span id 与 service/instance）⇒ relay 仍会起根 span |
| **C. 复制 go2sky 的编码逻辑自己拼 `SpanContext`** | 需要 `tracer.service` / `tracer.instance` 私有字段（`trace.go:181-182`）⇒ 包外不可行（已实测） |
| **D. relay 用 `CreateLocalSpan` 造"假父"** | 违背 `AGENTS §四`「不硬造根 span」，且 local span 无传播头可注入 ⇒ 消费端仍接不上 |
| **E. 让 relay 在请求 ctx 内同步发布** | 直接推翻 outbox 模式（异步解耦 + 至少一次投递）的立项前提 |
