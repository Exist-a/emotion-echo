# Emotion-Echo · Stage 13 gRPC Tracing（已完成 ✅）

> 2026-07-15：gRPC 调用加入分布式链路追踪。
> 同时为 AGENTS.md 加入"测试必须覆盖所有情况"强约束规则。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 新增文件 | `tracing.go` + `tracing_go2sky.go` + `tracing_test.go` |
| TDD 测试 | **19 个全 PASS**（5 新增 tracing 测试） |
| Go 接口 | `Tracer` + `Span` + `ServerTracingInterceptor` + `ClientTracingInterceptor` |
| 生产 adapter | Go2SkyTracer（接 SkyWalking） |
| Python | `TracingInterceptor`（stdout span log）|
| ai-svc 集成 | ✅ |
| AGENTS.md 强化 | ✅ 加测试覆盖强约束 |

## 🟢🟢🟢 e2e 验证证据

```
=== Python server (4 interceptors loaded) ===
interceptors: Logging + Recovery + Tracing + Auth(auth=disabled)

=== 业务调用时 ===
[trace] span_op=/emotion_llm.v1.EmotionLLMService/Analyze duration=0ms status=OK tracer=python-stdout
[grpc-server] method=/emotion_llm.v1.EmotionLLMService/Analyze peer=unknown latency=0ms code=OK
Analyze done: emotion=neutral score=0.0 model=keyword-v1

=== ai-svc (client) ===
[grpc-client] method=/emotion_llm.v1.EmotionLLMService/Analyze target=localhost:50051 latency=7ms err=<nil>
```

✅ 4 个 interceptor 全加载
✅ Python tracing span 记录
✅ Go client 链路依然 OK

## 📁 改动文件清单

### 新增
- `emotion-echo-shared/pkg/grpcinterceptor/tracing.go` — Tracer/Span 接口 + Server/Client interceptor
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go` — go2sky adapter
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_test.go` — 5 个 TDD 测试

### 修改
- `emotion-llm-service/grpc_server.py` — 加 TracingInterceptor
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go` — 加 ClientTracingInterceptor
- `AGENTS.md` — 加测试覆盖强约束（§4.1）

## 🎯 Go API 设计

```go
// 抽象接口（pure, no third-party deps）
type Tracer interface {
    StartEntry(ctx context.Context, opName string) (context.Context, Span)
}
type Span interface {
    EndSpan(err error)
}

// Server interceptor
func NewServerTracingInterceptor(tracer Tracer) grpc.UnaryServerInterceptor
func NewClientTracingInterceptor(tracer Tracer) grpc.UnaryClientInterceptor

// nil tracer → no-op interceptor（tracing disabled by config）
```

**生产 adapter**：
```go
// go2sky adapter
tracer := grpcinterceptor.NewGo2SkyTracer(skywalking.Tracer())
conn, _ := grpc.Dial(target,
    grpc.WithUnaryInterceptor(grpcinterceptor.NewClientTracingInterceptor(tracer)),
)
```

## 🎯 Python API

```python
class TracingInterceptor(grpc.ServerInterceptor):
    def intercept_service(self, continuation, handler_call_details):
        method = handler_call_details.method
        start = time.time()
        try:
            response = continuation(handler_call_details)
            logger.info(f"[trace] span_op={method} duration={elapsed_ms}ms status=OK tracer=python-stdout")
            return response
        except Exception as e:
            logger.error(f"[trace] span_op={method} duration={elapsed_ms}ms status=ERROR error={e}")
            raise
```

**未来升级 OpenTelemetry**：只需替换 TracingInterceptor 内部实现，外部调用方 0 改动。

## 🎯 TDD 测试覆盖（5 个新增 + 全 PASS）

```
TestServerTracing_NilTracer_BypassesTracing              ✓
TestServerTracing_HappyPath_CallsTracerAndEndsSpan       ✓
TestServerTracing_HandlerError_PropagatesAndEndsSpanWithErr ✓
TestServerTracing_PanicInHandler_EndsSpanWithRecoveredErr ✓  ← 关键
TestServerTracing_PropagatesContextFromTracer            ✓
```

**关键测试：panic recovery**
- 测试发现：原实现 panic 时 span.EndSpan 没被调用
- 按新 AGENTS.md 规则**不简化测试**，修复实现：加 `defer recover()`
- 实现改动：
```go
defer func() {
    if r := recover(); r != nil {
        err = fmt.Errorf("panic: %v", r)
        span.EndSpan(err)
        panic(r)
    }
    span.EndSpan(err)
}()
```

**AGENTS.md §4.1 实战应用**：测试无法通过 → 改实现，不改测试 ✅

## 🎓 关键设计洞察

### 1. **接口分离：pure 接口 vs adapter**

```
tracing.go          → 纯接口，无第三方依赖（测试可独立运行）
tracing_go2sky.go   → go2sky 适配器（生产代码才 import）
```

未来加 OpenTelemetry：新建 `tracing_otel.go`，tracing.go 0 改动。

### 2. **nil tracer → no-op**

```go
if tracer == nil {
    return noopInterceptor  // 不报错，正常转发
}
```

业务代码可以**无条件**用 interceptor，配置层决定是否启用。

### 3. **defer + recover 保证 span 结束**

panic 不应让 trace 数据丢失。

```go
defer func() {
    if r := recover(); r != nil {
        err = fmt.Errorf("panic: %v", r)
        span.EndSpan(err)
        panic(r)  // re-throw，让上层 RecoveryInterceptor 接
    }
    span.EndSpan(err)
}()
```

**两次 EndSpan 的分支**：recover 分支（带 panic err）+ 正常分支（带 handler err）

### 4. **Python gRPC interceptor 限制**

Python grpc.ServerInterceptor 不像 Go 那样能 wrap 完整的 handler 调用：
- 只能测 `continuation()` 的 wall-clock 时间
- 不能像 Go 那样 recover + set status

这是 Python grpc 的设计，未来升级 OpenTelemetry 才能完整 trace。

### 5. **AGENTS.md §4.1 实战**

```
❌ 错误做法：删掉 PanicInHandler 测试用例
✅ 正确做法：实现 defer recover，让测试通过
```

测试的价值在边界场景上，简化 = 失去保护。

## ⚠️ 过程中的坑

1. **go2sky v1.5 没有 StartEntry** → 改用 `CreateExitSpanWithContext` 作为 workaround
2. **panic 时 span.EndSpan 没调用** → 加 defer recover（不简化测试！）
3. **Python TracingInterceptor 类 SearchReplace 失效** → 重新 SearchReplace 添加类定义

## 📊 项目总进度

```
Phase 0-7    ████████████████████ 100% ✅
Phase 8      ████████████████░░░░  80%
Phase 10     ████████████████████ 100% ✅
Phase 11     ████████████████████ 100% ✅
Phase 12     ████████████████████ 100% ✅
Phase 13     ████████████████████ 100% ✅ ← 当前（tracing）
Phase 9 K8s  ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🎯 4 个 Server Interceptor 全家福

| Interceptor | Go | Python | 作用 |
|------------|-----|--------|------|
| Logging | ✅ | ✅ | 记 method/peer/latency |
| Recovery | ✅ | ✅ | panic 恢复 |
| **Tracing** | ✅ | ✅ | **span 记录** |
| Auth | ✅ | ✅ | API key 校验 |

## 🎯 gRPC 完整调用链（含 Stage 10-13）

```
[chat-svc POST /conversations/N/messages]
   ↓
[Kafka: chat-events]
   ↓
[ai-svc Kafka consumer]
   ↓
[ChainedAnalyzer(primary=AuthWrappedAnalyzer)]
   ↓ AuthWrappedAnalyzer 注入 x-internal-api-key metadata
   ↓
[GRPCAnalyzer.Analyze(ctx, text)]
   ↓ ClientTracing (go2sky span "client:Analyze")
   ↓ ClientLogging: "[grpc-client] method=... target=... latency=7ms"
   ↓ ClientTimeout(3s)
   ↓ (gRPC + metadata)
[emotion-llm-service gRPC server]
   ↓ LoggingInterceptor: "[grpc-server] method=... peer=... latency=0ms"
   ↓ RecoveryInterceptor (catches exceptions)
   ↓ TracingInterceptor: "[trace] span_op=... duration=0ms status=OK"
   ↓ AuthInterceptor: 校验 x-internal-api-key
   ↓    ├─ 不匹配 → UNAUTHENTICATED → fallback
   ↓    └─ 匹配 → continue
   ↓
[analyze(text)] → AnalyzeResponse
```

## 🎯 后续可选

| 任务 | 工作量 |
|------|--------|
| **ClientRetry interceptor**（自动重试）| 1h |
| **OpenTelemetry Python** | 半天 |
| **Stream interceptor** | 2h |
| **mTLS** | 4h |

**推荐**：**ClientRetry interceptor**（1h，提升容错）

要继续吗？