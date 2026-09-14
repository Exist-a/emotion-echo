---
status: planned
priority: high
owner: TBD
created: 2026-09-14
source: observability-edge-gaps-from-code-review §A（P1）
depends-on:
  - stage-44-observability-sprint-b.md
  - stage-50-e2e-validation.md
related-stages:
  - stage-91-file-understanding-pdf-prompt-directive-2026-09-14.md
related-adrs:
  - adr-2026-09-doc-drift-registry.md
---

# Plan — Stage 92 Kafka sw8 透传（observability P1，Stage 92 收口 scope = PR-1+PR-2）

## 0. 来源

来自 [`docs/plans/observability-edge-gaps-from-code-review.md`](observability-edge-gaps-from-code-review.md) §A（P1，最值得修复）。
2026-09-14 Stage 91 收口后立刻接 Stage 92——P1 项 user value 最高（跨服务全链路 trace）。

## 1. 问题陈述

chat-svc 用 sarama 发消息时，**ProducerMessage.Headers 只写 `content-type`，不写 SkyWalking sw8 trace header**：

```go
// emotion-echo-chat-svc/internal/events/kafka_publisher.go:47-54
msg := &sarama.ProducerMessage{
    Topic: topic, Key: sarama.StringEncoder(e.ID), Value: sarama.ByteEncoder(body),
    Headers: []sarama.RecordHeader{
        {Key: []byte("content-type"), Value: []byte(ContentTypeHeaderProto)},
    },
}
```

ai-svc consumer（`emotion-echo-ai-svc/internal/consumer/consumer.go:111-122`）调 `h.Tracer.CreateLocalSpan(sess.Context(), "kafka-consume")` —— **这是新 trace tree，没父 span**。

**后果**：SkyWalking UI 上 chat-svc HTTP 处理完 trace 就断；ai-svc 情绪分析是独立 trace；用户排查"消息发出去了但情绪分析迟迟不返回"只能 grep 日志 + 手动对齐 timestamp。

## 2. 根因分析

| 缺失 | 影响 |
|---|---|
| chat-svc 不抽 ctx 里的 sw8 写到 Kafka header | sw8 在跨进程边界丢失 |
| shared `Tracer` 接口无 `CreateEntrySpan` | consumer 无法从 header 重建上游 trace context |
| ai-svc consumer 用 `CreateLocalSpan` 而非 `CreateEntrySpan` | 即使有 sw8 header 也无从认父 |

go2sky v1.5 原生**有** `CreateEntrySpan(ctx, operationName, extractor)`（[go2sky trace.go:94](https://github.com/SkyAPM/go2sky/blob/v1.5.0/trace.go)）——只是 shared `Go2SkyTracer` adapter 没暴露这个方法。

## 3. 方案

### 3.1 PR-1 chat-svc producer 注入 sw8

`KafkaEventPublisher` 加 `tracer` 字段（可选，nil 时降级原行为）。`Publish` 时：
1. 构造临时 `propagation.SpanContext` 注入器，收集 sw8 string
2. 调 `tracer.CreateExitSpan(ctx, "kafka-publish", topic, injector)` —— go2sky 自动填 SpanContext + 调注入器
3. 把 sw8 string 写到 ProducerMessage.Headers

新增字段：`tracer grpcinterceptor.Tracer`（**新接口扩**，见 PR-2）。

### 3.2 PR-1 (shared) `Tracer.CreateEntrySpan` 接口扩展

`emotion-echo-shared/pkg/grpcinterceptor/tracing.go`：

```go
type Tracer interface {
    StartEntry(ctx context.Context, operationName string) (context.Context, Span)
    CreateLocalSpan(ctx context.Context, operationName string) (context.Context, Span, error)
    // Stage 92 新增：用于 Kafka consumer 从 msg header 重建上游 trace
    CreateExitSpan(ctx context.Context, operationName, peer string,
        injector propagation.Injector) (context.Context, Span, error)
}
```

`Go2SkyTracer` adapter 实现：
- `CreateEntrySpan` → `t.tracer.CreateEntrySpan(ctx, name, extractor)`，extractor 从 `map[string]string` 抽 sw8 header
- `CreateExitSpan` → `t.tracer.CreateExitSpanWithContext(...)`，跟 StartEntry 同等容错

### 3.3 PR-2 ai-svc consumer 用 CreateEntrySpan 替代 CreateLocalSpan

`emotion-echo-ai-svc/internal/consumer/consumer.go:111-122`：

```go
// Stage 92 PR-2：抽 sw8 header → CreateEntrySpan 重建上游 trace
sw8 := extractHeader(msg.Headers, "sw8")
extractor := func(key string) (string, error) {
    if key == "sw8" { return sw8, nil }
    return "", nil
}
_, span, err := h.Tracer.CreateEntrySpan(sess.Context(), "kafka-consume", extractor)
if span != nil { defer span.EndSpan(nil) }
```

`extractHeader` 是新 helper：sarama header 是 `[]RecordHeader`，返回 `map[string]string` 给 extractor 用。

### 3.4 不在 Stage 92 范围

| 项 | 原因 | 去向 |
|---|---|---|
| **analytics-svc consumer sw8 透传** | analytics-svc 完全没有 Tracer 集成（grep `CreateLocalSpan` 零命中），加 sw8 透传需要先加整个 tracer 链路，工作量跳跃 | Stage 93（独立）或本期 §7 候选 |
| **sw8 落库** | 与"跨进程 trace"主题无关 | 未来按需 |
| **trace 采样率配置（prod 抽样）** | 业务侧独立计划 | 已记 observability-edge-gaps §H |
| **SkyWalking OAP 升级** | 与 go2sky 升级耦合 | 已记 §H |

## 4. 工作量估算

| PR | 工作量 |
|---|---|
| PR-1 (shared) 接口扩 + adapter | 1.5h |
| PR-1 (chat-svc) producer sw8 注入 + RED/GREEN | 1.5h |
| PR-2 (ai-svc) consumer sw8 抽取 + RED/GREEN | 1h |
| docker e2e SkyWalking UI 跨进程 trace 实证 | 1h |
| **总计** | **半天（5h）** |

## 5. DoD（Stage 92 关闭条件）

1. ✅ 单元测试全绿（含 Stage 50 全量回归）
2. ✅ docker e2e：用户发消息 → SkyWalking UI 看到 chat-svc → ai-svc 跨进程 trace（一棵树，traceID 一致）
3. ✅ ai-svc consumer span 上能看到 `parent_span_id` / 上游 `trace_id`（与 chat-svc producer 端 traceID 相同）
4. ✅ 没有 sw8 header 的旧消息降级（不破坏）—— CreateEntrySpan 在 extractor 无 sw8 时 refSc.Valid=false，新 span 是新 trace 起点
5. ✅ ai-svc HTTP → llm-service 链路 trace 也保留（不破坏现有 PR-OBS-19 gRPC interceptor）
6. ✅ Plan 落地后从 `docs/plans/` 迁 `legacy-plans/landed/`

## 6. 风险与缓解

| 风险 | 等级 | 缓解 |
|---|---|---|
| 扩 shared `Tracer` 接口影响其他 caller（gRPC interceptor） | 低 | StartEntry/CreateLocalSpan 签名不变；只新增 CreateExitSpan；现有 caller 不需改 |
| `extractHeader` 在 sarama header 大小写/编码上踩坑 | 中 | sarama `RecordHeader.Key/Value` 是 `[]byte`，统一转 string；与 go2sky propagation.Header 常量 `sw8` 字符串比较 |
| sarama `ProducerMessage.Headers` 顺序敏感 | 低 | 与现有 `content-type` header 共存；新 sw8 append 即可 |
| PR-1 扩接口让 chat-svc 强制依赖 go2sky propagation | 低 | producer 端只 import `propagation.Header`（常量字符串）+ `propagation.Injector` 类型，无其他 go2sky 依赖 |
| e2e SkyWalking UI 跨进程 trace 没显示 | 中 | 验证 OAP reporter 已起来；`docker logs sw-oap` 检查接收；sample ratio 默认 1.0 |

## 7. 候选后续（Stage 93+）

| 项 | 说明 |
|---|---|
| analytics-svc consumer 加 tracer + sw8 透传 | 半天。先在 Consumer 加 Tracer 字段，再用 PR-2 同模式 |
| 其他 5 observability-edge-gaps issue（B-F） | 半天；按需排期 |
| SkyWalking OAP 升级到 9.7+ | 独立工程 |

## 8. 调研依据

| 文件 | 调研内容 |
|---|---|
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go:47-54` | ProducerMessage.Headers 实际内容（仅 content-type） |
| `emotion-echo-ai-svc/internal/consumer/consumer.go:111-122` | Tracer.CreateLocalSpan 调用位置 |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go` | 无 Tracer 字段、无 CreateLocalSpan 调用（移出 Stage 92） |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:75-86` | 现有 Tracer 接口（只有 StartEntry/CreateLocalSpan） |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go:81-130` | Go2SkyTracer adapter（需扩 CreateExitSpan + 加 CreateEntrySpan） |
| go2sky v1.5.0 `trace.go:93-111` `CreateEntrySpan` | go2sky 原生支持，需 adapter 暴露 |
| go2sky v1.5.0 `propagation/propagation.go:32` `Header = "sw8"` | sw8 header 名常量 |
| go2sky v1.5.0 `propagation/propagation.go:147` `EncodeSW8` | SpanContext → sw8 string |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing_test.go` | 现有测试结构（mock Tracer） |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky_test.go` | Go2SkyTracer 现有测试结构 |
