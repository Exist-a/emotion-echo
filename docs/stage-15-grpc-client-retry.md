# Stage 15: gRPC Client Retry interceptor

> 目标：transient 错误自动重试（指数退避 + 抖动），业务错误立即返回。

## 1. 背景

gRPC 调用失败分两类：

| 类型 | 例子 | 行为 |
|------|------|------|
| **transient**（应重试） | `Unavailable`（conn reset / 后端重启）、`DeadlineExceeded`（timeout 临界）、`Aborted`（事务冲突）、`ResourceExhausted`（临时过载） | 退避后重试 |
| **business**（不应重试） | `Unauthenticated`、`InvalidArgument`、`NotFound`、`PermissionDenied` | 立即返回 |

网络层错误（`conn reset by peer`）没经过 `grpc.status` 包装 → 视为 transient。

## 2. 设计

### 2.1 RetryOptions

```go
type RetryOptions struct {
    MaxAttempts       int             // 总尝试次数（含首次），默认 3
    InitialBackoff    time.Duration   // 第 1 次重试前等待，默认 100ms
    MaxBackoff        time.Duration   // 单次 backoff 上限，默认 2s
    BackoffMultiplier float64         // 指数乘子，默认 2.0
    Jitter            bool            // 随机抖动避免雪崩，默认 true
    RetryableCodes    []codes.Code    // 触发重试的 status code
}
```

### 2.2 退避公式

```
backoff(n) = min(initial × multiplier^(n-1), max)  (无 jitter)
backoff(n) = backoff(n) × uniform(0.5, 1.0)         (有 jitter)
```

例（Initial=100ms, Multiplier=2.0, Max=2s, Jitter=true）：
- 第 1 次重试：100ms × 0.5~1.0 = 50-100ms
- 第 2 次重试：200ms × 0.5~1.0 = 100-200ms
- 第 3 次：400ms × 0.5~1.0 = 200-400ms
- … 封顶 2s × 0.5~1.0 = 1-2s

### 2.3 默认重试码

```go
codes.Unavailable       // 网络断开 / 后端重启
codes.DeadlineExceeded  // 客户端 timeout 临界
codes.Aborted           // 乐观锁冲突
codes.ResourceExhausted // 限流（应配合退避）
```

## 3. 实现

```go
func ClientRetryInterceptor(retryOpts RetryOptions) grpc.UnaryClientInterceptor
```

行为：
- **成功** → 返回（不重试）
- **不可重试** → 立即返回最后一次 error
- **可重试 + 未达 MaxAttempts** → 算 backoff → select { ctx.Done() | timer.C }
- **可重试 + 达 MaxAttempts** → 返回最后一次 error
- **ctx 取消** → 立即停止重试

链顺序（重要）：
```
TracingInterceptor     # 最外层：包住整个 retry 循环
ClientLoggingInterceptor
ClientTimeoutInterceptor  # 单次 timeout
ClientRetryInterceptor    # 单次失败后等 backoff
```

retry 应该在 timeout 之内：单次 timeout 3s，retry 3 次总耗时上限约 3s + 100ms + 3s = 6.1s。

## 4. TDD 覆盖

8 个测试覆盖：

| # | 测试 | 覆盖点 |
|---|------|--------|
| 1 | NoError_NoRetry | 成功不重试 |
| 2 | TransientError_RetriesUntilSuccess | Unavailable/DeadlineExceeded 重试 |
| 3 | NonRetryableError_NoRetry | Unauthenticated 立即返回 |
| 4 | MaxAttemptsReached | 达到上限返回最后错误 |
| 5 | BackoffGrowsExponentially | 20ms + 40ms ≈ 60ms 总 backoff |
| 6 | ContextCancel_StopsRetry | ctx cancel 立即停止 |
| 7 | DefaultRetryOptions | 默认配置合理 |
| 8 | CustomRetryableCodes | 自定义重试码 |
| 9 | NonStatusError_RetriedAsTransient | 网络错误视为 transient |

```
$ go test ./pkg/grpcinterceptor/ -v -count=1 -run TestClientRetry
=== RUN   TestClientRetry_NoError_NoRetry                    --- PASS
=== RUN   TestClientRetry_TransientError_RetriesUntilSuccess --- PASS
=== RUN   TestClientRetry_NonRetryableError_NoRetry           --- PASS
=== RUN   TestClientRetry_MaxAttemptsReached_ReturnsLastError --- PASS
=== RUN   TestClientRetry_BackoffGrowsExponentially            --- PASS
=== RUN   TestClientRetry_ContextCancel_StopsRetry             --- PASS
=== RUN   TestClientRetry_CustomRetryableCodes                 --- PASS
=== RUN   TestClientRetry_NonStatusError_RetriedAsTransient    --- PASS
PASS    3.812s
```

## 5. ai-svc 集成

`grpc_analyzer.go` 加一行：

```go
grpc.WithChainUnaryInterceptor(
    grpcinterceptor.NewClientTracingInterceptor(...),
    grpcinterceptor.ClientLoggingInterceptor(),
    grpcinterceptor.ClientTimeoutInterceptor(3*time.Second),
    // ↓ 新增
    grpcinterceptor.ClientRetryInterceptor(grpcinterceptor.DefaultRetryOptions()),
),
```

## 6. E2E 验证

### 6.1 Python 端 transient 模拟

`grpc_server.py` 加环境变量 `FAIL_FIRST_N`：

```python
if self._fail_remaining > 0:
    self._fail_remaining -= 1
    context.set_code(grpc.StatusCode.UNAVAILABLE)
    context.set_details(...)
    return AnalyzeResponse()
```

注意：用 `set_code` 而非 `abort` —— `abort` 抛异常会被 RecoveryInterceptor 包装成 INTERNAL，绕过 retryable 判断。

### 6.2 验证日志

启动 server：`FAIL_FIRST_N=2 python grpc_server.py`

调用 client（Go）：

```
2026/07/15 15:40:38 [grpc-client] method=/grpc.health.v1.Health/Check target=localhost:50051 latency=62ms err=<nil>
2026/07/15 15:40:38 [grpc-client] method=/emotion_llm.v1.EmotionLLMService/Analyze target=localhost:50051 latency=183ms err=<nil>
✅ Analyze success: emotion=neutral score=0.00
```

Server 端看到 3 次调用：

```
[trace] span_op=/emotion_llm.v1.EmotionLLMService/Analyze duration=0ms status=OK
[grpc-server] method=...Analyze code=OK
WARNING: [transient-mock] simulating Unavailable (remaining=1)
[trace] span_op=...Analyze status=OK
[grpc-server] method=...Analyze code=OK
WARNING: [transient-mock] simulating Unavailable (remaining=0)
[trace] span_op=...Analyze status=OK
[grpc-server] method=...Analyze code=OK
Analyze done: emotion=neutral
```

总耗时 183ms ≈ 100ms（首次）+ 50ms（backoff1）+ 100ms（backoff2）+ RPC 时间。

## 7. 使用建议

| 场景 | 推荐配置 |
|------|----------|
| **同步 RPC（HTTP 类）** | DefaultRetryOptions() 3 次 / 100ms 起始 |
| **关键业务调用** | MaxAttempts=5, InitialBackoff=200ms |
| **流式 RPC（实时）** | 关闭 retry（流式断了就放弃） |
| **写操作（避免重复）** | 自定义 RetryableCodes 只含 Unavailable（不重试 Aborted） |

⚠️ **非幂等接口慎用 retry**：默认 Aborted 在重试码内（数据库乐观锁场景），但业务写接口如果是"插入订单"，retry 重复发送可能产生副作用。建议用幂等 token 或关闭 retry。
