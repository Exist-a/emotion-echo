# Emotion-Echo · Stage 11 gRPC Interceptor（已完成 ✅）

> 2026-07-15：gRPC 跨进程调用增加 logging + recovery interceptor。
> 跨语言（Go + Python）复用相同的拦截模式。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 新增包 | `emotion-echo-shared/pkg/grpcinterceptor` |
| Server interceptors | 2 个（Logging + Recovery）|
| Client interceptors | 2 个（Logging + Timeout）|
| TDD 测试 | **6 个全 PASS** |
| Python interceptor | 2 个（Logging + Recovery）|
| ai-svc 集成 | ✅ |
| e2e 验证 | ✅ client + server 日志都看到 |

## 🟢🟢🟢 e2e 验证证据

```
=== 启动日志（关键行）===
[llm] using gRPC analyzer (target=localhost:50051) + keyword fallback
gRPC server started on port 50051
interceptors: Logging + Recovery

=== 业务调用时 ===

[grpc-client] method=/emotion_llm.v1.EmotionLLMService/Analyze
              target=localhost:50051 latency=4ms err=<nil>

[grpc-server] method=/emotion_llm.v1.EmotionLLMService/Analyze
              peer=unknown latency=0ms code=OK
```

✅ Client interceptor 工作
✅ Server interceptor 工作
✅ latency 4ms（gRPC 比 HTTP 快）

## 📁 改动文件清单

### 新增
- `emotion-echo-shared/pkg/grpcinterceptor/server.go` — Go server interceptors
- `emotion-echo-shared/pkg/grpcinterceptor/client.go` — Go client interceptors
- `emotion-echo-shared/pkg/grpcinterceptor/interceptor_test.go` — 6 个 TDD 测试

### 修改
- `emotion-llm-service/grpc_server.py` — 加 LoggingInterceptor + RecoveryInterceptor
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go` — 用 WithChainUnaryInterceptor 集成
- `emotion-echo-shared/go.mod` — 加 google.golang.org/grpc v1.80.0

## 🎯 interceptor 设计

### Server（Go）

```go
// 日志
[grpc-server] method=/emotion_llm.v1.EmotionLLMService/Analyze
              peer=127.0.0.1:54321 latency=42ms code=OK err=<nil>

// panic 恢复（避免单请求崩溃整个 server）
defer func() {
    if r := recover(); r != nil {
        log.Printf("[grpc-server] PANIC in %s: %v\n%s", method, r, debug.Stack())
        err = status.Errorf(13, "internal error: %v", r)  // 13 = Internal
    }
}()
```

### Client（Go）

```go
// 日志
[grpc-client] method=/emotion_llm.v1.EmotionLLMService/Analyze
              target=localhost:50051 latency=42ms err=<nil>

// 超时（防止永久阻塞）
if _, ok := ctx.Deadline(); !ok {
    ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
}
```

### Python server（同名同语义）

```python
# LoggingInterceptor
logger.info(f"[grpc-server] method={method} peer={peer_addr} latency={elapsed_ms}ms code=OK")

# RecoveryInterceptor
try:
    return continuation(handler_call_details)
except Exception as e:
    logger.error(f"...PANIC...")
    raise grpc.RpcError(f"internal error: {e}") from e
```

## 🎯 ai-svc 集成方式

```go
conn, err := grpc.DialContext(ctx, target,
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithBlock(),
    grpc.WithChainUnaryInterceptor(
        grpcinterceptor.ClientLoggingInterceptor(),
        grpcinterceptor.ClientTimeoutInterceptor(3*time.Second),
    ),
)
```

**一行 chain** 集成两个 interceptor。

## 🎯 TDD 测试覆盖（6 个全 PASS）

```
TestServerLogging_PassesThroughSuccess           ✓
TestServerLogging_PropagatesError                ✓
TestServerRecovery_PanicRecovered                ✓
TestServerRecovery_NormalCallNotAffected         ✓
TestClientTimeout_AddsTimeoutWhenNoDeadline      ✓
TestClientTimeout_PreservesExistingDeadline      ✓
```

**关键场景**：
- ✅ panic 被 recover，不让测试进程崩溃
- ✅ ctx 有 deadline 时不覆盖
- ✅ ctx 无 deadline 时加上 50ms 超时
- ✅ err 透传
- ✅ 不影响正常调用

## 🎓 关键设计洞察

### 1. **跨语言 interceptor 同构**

| Go | Python | 功能 |
|----|--------|------|
| ServerLoggingInterceptor | LoggingInterceptor | 记 method/peer/latency |
| ServerRecoveryInterceptor | RecoveryInterceptor | panic 异常转 INTERNAL |
| ClientLoggingInterceptor | - | 记 client 端 method/target |
| ClientTimeoutInterceptor | - | 默认 deadline |

**同一个 proto 契约 + 同一套 interceptor 模式 = 跨语言一致体验**

### 2. **panic 恢复的必要性**

gRPC server 默认行为：
- 一个 handler panic → **整个 server 崩溃**（不可接受）

加 RecoveryInterceptor 后：
- panic 被 catch → 转 INTERNAL → 单请求失败 + server 继续运行

### 3. **client timeout 防止阻塞**

Kafka consumer 调用 ai-svc → ai-svc 调 gRPC。如果 gRPC server 卡住：
- 没有 timeout：consumer 永远阻塞，Kafka offset 不提交
- 有 timeout：3s 后超时，consumer fallback 到 keyword analyzer

### 4. **复用 chain 模式**

```go
grpc.WithChainUnaryInterceptor(
    interceptor1,
    interceptor2,
    interceptor3,
)
```

未来加 ServerTracing / ServerAuth / ClientRetry 都直接 append 到 chain。

## 📊 项目总进度

```
Phase 0-7    ████████████████████ 100% ✅
Phase 8      ████████████████░░░░  80% （legacy 迁移）
Phase 10     ████████████████████ 100% ✅ （gRPC 迁移）
Phase 11     ████████████████████ 100% ✅ ← 当前（interceptor）
Phase 9 K8s  ░░░░░░░░░░░░░░░░░░░░   0%（按你要求暂缓）
```

## 🎯 跨语言 interceptor 完整调用图

```
[chat-svc POST /conversations/N/messages]
   ↓
[Kafka: chat-events]
   ↓
[ai-svc Kafka consumer]
   ↓
[GRPCAnalyzer.Analyze(ctx, text)]
   ↓ ClientLogging: "[grpc-client] method=... target=... latency=4ms"
   ↓ ClientTimeout(3s)
   ↓ (gRPC, HTTP/2, protobuf)
[emotion-llm-service grpc_server.py]
   ↓ LoggingInterceptor: "[grpc-server] method=... peer=... latency=0ms"
   ↓ RecoveryInterceptor (catches exceptions)
   ↓
[analyze(text) → keyword + sentiment]
   ↓
[AnalyzeResponse → gRPC 响应]
   ↓
[ai-svc 写库 + Kafka offset commit]
```

## 🎯 后续可选

| 任务 | 工作量 |
|------|--------|
| **ServerTracing interceptor**（SkyWalking 集成）| 2h |
| **ServerAuth interceptor**（JWT 校验）| 1h |
| **ClientRetry interceptor**（自动重试）| 1h |
| **Stream interceptor**（流式 RPC）| 2h |
| **OpenTelemetry 集成**（统一 trace）| 1d |

**推荐**：先做 **ServerTracing**（复用已有 SkyWalking tracer），形成完整调用链。

要继续吗？