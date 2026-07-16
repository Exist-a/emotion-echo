# Stage 14: gRPC Health Check（health/v1）

> 目标：实现 gRPC 标准 health/v1 协议，让 ai-svc 启动时探活 emotion-llm-service 的"业务就绪"状态。

## 1. 背景

gRPC 健康检查有两种粒度：

| 层级 | 检查方式 | 表达 |
|------|----------|------|
| **TCP 连通** | `grpc.DialContext` + `WithBlock` | "能连上" |
| **业务就绪** | gRPC `health/v1` 协议 | "能处理请求" |

`WithBlock` 只保证 TCP 握手成功，不保证业务 handler 可用。LLM 启动需要：
- 加载模型（10-30s）
- 初始化 tokenizer
- 预热 GPU/CPU

期间 TCP 通了但 RPC 全失败。**health check 协议** 就是为这种"业务级就绪"设计的：server 主动声明 `SERVING` / `NOT_SERVING` / `SERVICE_UNKNOWN`。

参考：[gRPC Health Checking Protocol](https://github.com/grpc/grpc/blob/master/doc/health-checking.md)

## 2. 设计

### 2.1 协议

标准 health/v1 定义了 4 个状态：

```
UNKNOWN          服务状态未知（默认）
SERVING          服务正常
NOT_SERVING      服务不可用（warmup、shutdown 中）
SERVICE_UNKNOWN  该 service 名未注册
```

API：
- `Check(HealthCheckRequest) → HealthCheckResponse`：单次查询
- `Watch(HealthCheckRequest) → stream<HealthCheckResponse>`：流式订阅（状态变化时推送）

### 2.2 包结构

新增 `emotion-echo-shared/pkg/healthcheck/`：

```
pkg/healthcheck/
├── server.go          # Server 封装（grpc_health_v1 + 自维护 status map）
├── client.go          # Client 封装（Check / WaitForReady）
└── health_test.go     # 8 个 TDD 测试
```

### 2.3 为什么自维护 status map？

grpc-go 的 `health.Server` 内部 `statusMap` 未导出，业务侧需要：
- 启动后立刻 `GetServingStatus("")` 断言默认状态
- 单元测试断言"set 之后 get 拿到"
- shutdown 时遍历所有 service 设为 NOT_SERVING

所以包一层 `Server`，自己维护一份 map，再 sync 给 `health.Server` 实际处理 RPC。

## 3. 实现

### 3.1 Server API

```go
// 创建默认 SERVING 的 health server（空 service = server liveness）
srv := healthcheck.NewServer()

// 注册到 *grpc.Server
srv.RegisterWith(gs)

// 多 service 独立管理
srv.SetServingStatus("emotion.LLM", healthcheck.ServingStatusServing)
srv.SetServingStatus("emotion.FER",  healthcheck.ServingStatusNotServing)

// 查询本地状态
st := srv.GetServingStatus("emotion.LLM")

// graceful shutdown
srv.Shutdown()  // 所有 service → NOT_SERVING
srv.Resume()    // 所有 service → SERVING
```

### 3.2 Client API

```go
// 单次探活
cli := healthcheck.NewClient(conn)
st, err := cli.Check(ctx, "emotion.LLM")
// st: ServingStatus / err: 网络错误或 NOT_FOUND

// 阻塞等就绪（启动时自检）
err := cli.WaitForReady(ctx, "emotion.LLM", 5*time.Second)
// 每 200ms 轮询一次，直到 SERVING 或 ctx 超时
```

### 3.3 ai-svc 集成

修改 `analyzer.NewGRPCAnalyzer`：

```go
// 旧：阻塞 dial（仅 TCP 连通）
conn, err := grpc.DialContext(ctx, target, grpc.WithBlock(), ...)

// 新：非阻塞 dial + 业务 health check
conn, err := grpc.NewClient(target, ...)        // 不等连接
if err != nil { return nil, err }
if err := healthCli.WaitForReady(ctx, "emotion.LLM", 5*time.Second); err != nil {
    return nil, fmt.Errorf("health check failed: %w", err)
}
```

效果：ai-svc 启动时会卡在 health check 直到 LLM 真正"能处理请求"，避免 30 秒 warmup 期的雪崩重试。

## 4. TDD 覆盖

8 个测试覆盖：

| # | 测试 | 覆盖点 |
|---|------|--------|
| 1 | NewServer_DefaultServingStatus | 默认 SERVING |
| 2 | Server_CheckReturnsServing | RPC 真实返回 SERVING |
| 3 | SetServingStatus_NotServing | 状态可变更 |
| 4 | PerServiceStatus | 多 service 独立 + 未注册返回 UNKNOWN |
| 5 | Client_CheckReturnsCurrentStatus | 客户端读真实状态 |
| 6 | Client_CheckUnknownService | 不存在 service 映射 ServiceUnknown |
| 7 | Client_WaitForReady_Succeeds | 阻塞等就绪（500ms 翻 SERVING） |
| 8 | Client_WaitForReady_Timeout | 持续 NOT_SERVING 超时 |

```bash
$ go test ./pkg/healthcheck/ -v -count=1
=== RUN   TestNewServer_DefaultServingStatus    --- PASS
=== RUN   TestServer_CheckReturnsServing         --- PASS
=== RUN   TestServer_SetServingStatus_NotServing --- PASS
=== RUN   TestServer_PerServiceStatus            --- PASS
=== RUN   TestClient_CheckReturnsCurrentStatus    --- PASS
=== RUN   TestClient_CheckUnknownService         --- PASS
=== RUN   TestClient_WaitForReady_Succeeds       --- PASS
=== RUN   TestClient_WaitForReady_Timeout        --- PASS
PASS    4.468s
```

## 5. Python 服务端

emotion-llm-service 加 `grpcio-health-checking`：

```python
# requirements.txt
grpcio>=1.60.0
grpcio-health-checking>=1.60.0

# grpc_server.py
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

health_servicer_impl = health.HealthServicer()
health_servicer_impl.set("", health_pb2.HealthCheckResponse.SERVING)
health_servicer_impl.set("emotion.LLM", health_pb2.HealthCheckResponse.SERVING)

# ... 注册到 grpc.Server
health_pb2_grpc.add_HealthServicer_to_server(health_servicer_impl, server)
```

KeyboardInterrupt 时把 health 设为 NOT_SERVING，让 client 提前停止发新请求。

## 6. E2E 验证

### 6.1 Python 客户端直连

```python
# _test_health.py
Check("")              = SERVING
Check("emotion.LLM")   = SERVING
Check("not.exist")     = NOT_FOUND
Analyze("happy text")  = emotion: neutral
```

### 6.2 ai-svc 启动日志

```
2026/07/15 15:28:53 [postgres] connected
2026/07/15 15:28:53 [skywalking] tracer initialized
2026/07/15 15:28:53 [grpc-client] method=/grpc.health.v1.Health/Check target=localhost:50051 latency=20ms err=<nil>
2026/07/15 15:28:53 [grpc-health] target=localhost:50051 service=emotion.LLM status=SERVING
2026/07/15 15:28:53 [llm] using gRPC analyzer (target=localhost:50051, auth=enabled) + keyword fallback
2026/07/15 15:28:53 [kafka] consumer started: ...
2026/07/15 15:28:53 Starting ai-svc at 0.0.0.0:8891...
```

可以看到 `Health/Check` RPC 走 ClientLoggingInterceptor 输出 20ms 延迟，然后 ai-svc 才开始后续逻辑。

## 7. 使用场景

- **K8s liveness probe**：`grpc_health_probe -addr=svc:50051 -service=`
- **服务发现**：consul / etcd 注册时 health/v1 探活
- **LB 流量摘除**：后端不健康时 LB 自动剔除
- **客户端自检**：ai-svc 启动时 WaitForReady 避免 30s warmup 雪崩

## 8. 已知限制

- **无重试**：Client.WaitForReady 内部 poll 200ms 一次，但 RPC 本身不重试（Stage 15 处理）
- **不持久化**：Server 是内存状态，进程重启后状态丢失（符合 spec）
- **Watch API 未封装**：暂只暴露 Check，需要的话 stage XX 加
