---
status: landed
priority: high
stage: 45
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义)
related-stages:
  - stage-44-observability-sprint-b.md (Sprint B 收口,本 stage 接 §四 B)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id)
related-commits:
  - 348bcb8 test(shared): RED PR-OBS-17 mockSpan.Tag + mockTracer.CreateLocalSpan 边界
  - 787a461 feat(shared): GREEN PR-OBS-17 Span.Tag + Tracer.CreateLocalSpan + Go2Sky adapter
  - 8b4f8dc refactor(shared+svc): 3 处签名变更走接口 + 6 svc main.go 包装
  - 0e9c9bc test(shared+ai-svc): 完整 span tag 断言(4 tag 精确比对 + mock interface)
---

# Stage 45 · PR-OBS-17 完整 trace 抽象收口（observability-sprint-b §四 B 部分落地）

> **本文档归档 PR-OBS-17（Stage 44 §四 B 的"完整 span tag 断言"前两步）**。
> 4 个 commit 把 `Span/Tracer` 接口从"只有 `EndSpan/StartEntry`"扩展为
> 完整 mockable 形态,并写入 ai-svc Kafka consumer 4 个 messaging.* tag 精确断言。

## 一、范围 vs 落地

Stage 44 §四 B 列的完整收口需 4 步。本 stage 落地 1~2 步（接口 + mock），
3~4 步（业务路径打真实 tag）留作 PR-OBS-18+。

| 步骤 | 落地 | 证据 |
|---|---|---|
| 1. 抽 TracerInterface + SpanInterface | ✅ | 348bcb8 + 787a461（RED + GREEN adapter）|
| 2. mockSpan + mockTracer 满足接口 + tag 记录 | ✅ | 0e9c9bc（consumer 3 case + middleware 2 case）|
| 3. GinSkywalkingMiddleware 创建真实 EntrySpan + http.* tag | ⏳ PR-OBS-18 | stubTracer.StartEntry 已就位 |
| 4. ServerTracingInterceptor 打 rpc.method/rpc.system/user_id + 5 svc gRPC server 接入 shared interceptor | ⏳ PR-OBS-19 | mockSpan.Tag 已可断言 |

## 二、核心改动

### 2.1 接口扩展（`emotion-echo-shared/pkg/grpcinterceptor/tracing.go`）

```go
type Span interface {
    EndSpan(err error)
    Tag(key, value string)   // 新增 (PR-OBS-17)
}

type Tracer interface {
    StartEntry(ctx context.Context, op string) (context.Context, Span)
    CreateLocalSpan(ctx context.Context, op string) (context.Context, Span, error)  // 新增
}
```

**Tag 类型决策**：`(string, string)` 而非 `go2sky.Tag`。
理由：(1) shared pkg 测试不必 import go2sky；(2) adapter `Go2SkySpan.Tag`
内部把 `string` 转 `go2sky.Tag(key)`（go2sky v1.5 Tag 也是 string 类型，
零运行时开销）。

### 2.2 Go2Sky adapter（`tracing_go2sky.go`）

```go
func (s *Go2SkySpan) Tag(key, value string) {
    if s.span == nil { return }
    s.span.Tag(go2sky.Tag(key), value)
}

func (t *Go2SkyTracer) CreateLocalSpan(ctx, op) (context.Context, Span, error) {
    if t == nil || t.tracer == nil {
        return ctx, &Go2SkySpan{}, nil  // 容错 noop
    }
    span, nCtx, err := t.tracer.CreateLocalSpan(ctx, go2sky.WithOperationName(op))
    ...
}
```

### 2.3 三处签名变更（行为零变化）

| 文件 | 变更 |
|---|---|
| `pkg/middleware/gin_skywalking.go` | `tracer *go2sky.Tracer` → `tracer Tracer`（import grpcinterceptor）|
| `ai-svc/internal/consumer/consumer.go` | `ConsumerGroupHandler.Tracer *go2sky.Tracer` → `grpcinterceptor.Tracer` |
| `ai-svc/main.go` + 5 svc main.go | `tracer` → `sharedgrpc.NewGo2SkyTracer(tracer)` 包装（6 处机械替换）|

### 2.4 ai-svc consumer 4 tag 精确断言

新增 `TestConsumeClaim_EmitsMessagingSystemTag`：断言 mockSpan.tagCalls
精确含 `(messaging.system, kafka)` / `(messaging.kafka.topic, chat-events)` /
`(messaging.kafka.partition, "3")` / `(event.type, message.created)`，
调用总数 == 4（不多打不少打）。

## 三、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:21-43`（扩前接口）
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go`（adapter）
- `emotion-echo-shared/pkg/middleware/gin_skywalking.go:21-32`（中间件）
- `emotion-echo-ai-svc/internal/consumer/consumer.go:106-122`（4 tag 设置点）
- `emotion-echo-shared/pkg/skywalking/{tracing.go, skywalking.go}`（无现成抽象可复用）

### ② 查相关 ADR / stage

- `docs/stages/stage-44-observability-sprint-b.md §四 B`（收口范围定义）
- `docs/architecture/decisions.md 决策 6`（JSON 日志 + trace_id 串联）

### ③ 跑现状 smoke

| 包 | 状态 |
|---|---|
| `emotion-echo-shared/...` 10 包 | 全绿（含 8 case 新增）|
| `emotion-echo-ai-svc/...` 11 包 | 全绿（含 3 case 新增）|
| 6 svc `go build ./...` | 全部干净编译 |

### ⑤ 列架构假设

- **假设 A**：go2sky v1.5 `Tag` 是 `string` 类型（不是 int）。
  **验证**：从 `go.sum` vendor 路径 `span.go` 读取确认。✅
- **假设 B**：mockSpan/mockTracer 加方法不会破坏其他调用方。
  **验证**：grep 验证两类型仅在 3 个 _test.go 内使用。✅
- **假设 C**：6 svc main.go 机械改 `NewGo2SkyTracer` 包装无遗漏。
  **验证**：`go build ./...` 全绿。✅
- **假设 D**：ai-svc consumer 的 4 tag 是 ui 聚合关键，迁移后字面量不变。
  **验证**：mock 断言精确比对（messaging.system/topic/partition/event.type）。✅

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-45-observability-sprint-b-regression.md`。
4 commit message 末尾均按 AGENTS.md §〇 §⑥ 列调研依据。

## 四、未做项（继续收口清单）

| 项 | 估 | 依赖 |
|---|---|---|
| ~~PR-OBS-18 GinSkywalkingMiddleware 创建 EntrySpan + http.method/url/status_code/user_id tag~~ | ~~半天~~ ✅ Stage 46 | — |
| ~~PR-OBS-15 6 svc 接入 logging helper SetGlobalSvc + WithTraceID/WithAction~~ | ~~半天~~ ✅ Stage 47 | — |
| PR-OBS-19 ServerTracingInterceptor 打 rpc.method/rpc.system/user_id + 5 svc gRPC server 接入 shared interceptor | 1-1.5 天 | 接口已就位（Stage 45）|
| PR-OBS-19 顺带：span.SetSpanLayer(agentv3.SpanLayer_HTTP/GRPC) + SetComponent | 半天 | 需扩 Span 接口 |
| PR-OBS-23 handler err 透传到 span.EndSpan(err) | 半天 | c.Next wrap 拿 c.Errors() |
| 业务 handler 主动打业务相关 tag（如 ai-svc fusion kind、chat-svc message length）| backlog | 取决于 PR-OBS-19 落地 |

## 五、与 Stage 44 §四 B 的对账

| §四 B 列的步骤 | 本 stage 状态 |
|---|---|
| 抽 TracerInterface（含 CreateLocalSpan）| ✅ Step 1 |
| 抽 SpanInterface（含 Tag）| ✅ Step 2 |
| GinSkywalkingMiddleware 改用接口 + 创建真实 span | 🟡 接口切换 ✅, 创建 span ⏳ PR-OBS-18 |
| ServerTracingInterceptor 改用接口 + 多次 Tag | 🟡 接口已扩展（Span.Tag）⏳ PR-OBS-19 |
| ConsumerGroupHandler 改用接口 + mock 实现 | ✅ Step 3（含 4 tag 精确断言）|
| 完整 span tag 断言（user_id / rpc.method / messaging.*）| 🟡 messaging.* 4 tag ✅, user_id / rpc.method ⏳ PR-OBS-18/19 |

**完成度**：3/6 步（接口 + consumer 4 tag），剩余 3 步都是"在接口上扩展业务 tag"。

## 六、下一步（启动顺序建议）

| 优先级 | 项 | 备注 |
|---|---|---|
| **P1** | PR-OBS-18 GinSkywalkingMiddleware 创建 EntrySpan + http.method/url/status_code/user_id tag | 接口已就位,无外部依赖 |
| **P1** | PR-OBS-19 ServerTracingInterceptor 打 rpc.method/rpc.system/user_id + 5 svc gRPC server 接入 shared interceptor | 需改 5 svc main.go（破坏性大但机械）|
| **P2** | PR-OBS-15 6 svc 接入 logging helper SetGlobalSvc + WithTraceID/WithAction | Stage 44 §四 C 独立 P1 |
