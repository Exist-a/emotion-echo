# Stage 19: ai-svc gRPC Server（HTTP + gRPC 共存）

> 目标：让 ai-svc 同时支持 HTTP（前端）和 gRPC（内部 svc-to-svc）两种协议。

## 1. 背景

ai-svc 之前是纯 Gin HTTP server（:8891）：
- 前端 Nuxt 用 HTTP
- 内部 svc 无法调（其他服务走 Kafka 异步管道）

Stage 19 让 ai-svc **额外**起一个 gRPC server（:8892）：
- 前端继续用 HTTP（:8891）
- 内部 svc 用 gRPC（:8892）直接查情绪

实际意义：业务服务可走同步 RPC 而不必等 Kafka 消费。

## 2. 设计

### 2.1 双协议共存

```
┌──────────────────┐
│  Nuxt (前端)     │  ──HTTP/JSON──>  :8891 (Gin)
└──────────────────┘

┌──────────────────┐
│  chat-svc        │  ──gRPC/proto─>  :8892 (gRPC)
│  analytics-svc   │  ──gRPC/proto─>  :8892
│  assessment-svc  │  ──gRPC/proto─>  :8892
└──────────────────┘

                 ┌─────────────────────┐
                 │      ai-svc         │
                 │  ┌───────────────┐  │
                 │  │ HTTP (Gin)    │  │  :8891
                 │  │ gRPC server   │  │  :8892
                 │  │ EmotionRepo   │  │  ←共享同一份 repo
                 │  └───────────────┘  │
                 └─────────────────────┘
```

### 2.2 Proto 定义

`proto/emotion_query.proto`：

```proto
service EmotionQueryService {
  rpc GetEmotionByMessage (GetEmotionByMessageRequest) returns (Emotion);
  rpc GetEmotionByConversation (GetEmotionByConversationRequest) returns (EmotionList);
}

message Emotion {
  int64  id              = 1;
  int64  message_id      = 2;
  int64  conversation_id = 3;
  string primary_emotion = 4;
  double sentiment_score = 5;
  double confidence      = 6;
  string model           = 7;
  int64  created_at_ms   = 8;
}
```

### 2.3 Interceptor 链

ai-svc gRPC server 复用 `pkg/grpcinterceptor`：

```go
grpc.ChainUnaryInterceptor(
    grpcinterceptor.NewServerTracingInterceptor(grpcinterceptor.NewGo2SkyTracer(tracer)),
    grpcinterceptor.ServerLoggingInterceptor(),
    grpcinterceptor.ServerRecoveryInterceptor(),
)
```

跟 emotion-llm-service 一样。

### 2.4 config

```yaml
GRPC:
  Enabled: true   # 启用 ai-svc gRPC server（:8892），HTTP（:8891）共存
  Port:    8892
```

## 3. 实现

### 3.1 ai-svc gRPC server 启动

`main.go`：
```go
if c.GRPC.Enabled {
    gs := grpcserver.New(emoRepo, c.GRPC.Port)
    go func() {
        if err := gs.Start(context.Background()); err != nil {
            log.Printf("[grpc] server failed: %v", err)
        }
    }()
}

if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil { ... }
```

两个 server 并行在两个 goroutine，共享 `emoRepo`。

### 3.2 业务实现

```go
func (s *emotionQueryServer) GetEmotionByMessage(ctx context.Context, req *emotionquery.GetEmotionByMessageRequest) (*emotionquery.Emotion, error) {
    if req.MessageId == 0 {
        return nil, status.Error(codes.InvalidArgument, "message_id is required")
    }
    e, err := s.repo.GetByMessageID(ctx, req.MessageId)
    if err != nil {
        return nil, status.Errorf(codes.Internal, "query failed: %v", err)
    }
    if e == nil {
        return nil, status.Error(codes.NotFound, "emotion not found for this message")
    }
    return toProtoEmotion(e), nil
}
```

复用 HTTP 层用的 `EmotionRepo`（PostgresEmotionRepo 或 InMemoryEmotionRepo）— 不重复实现。

### 3.3 Health check

ai-svc gRPC server 顺便注册了 `grpc.health.v1.Health`：
```go
healthSrv := health.NewServer()
healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
healthSrv.SetServingStatus("emotion.AI", healthpb.HealthCheckResponse_SERVING)
healthpb.RegisterHealthServer(gs, healthSrv)
```

## 4. E2E 验证

### 4.1 启动 ai-svc（HTTP + gRPC 双 server）

```bash
$ ./ai-svc.exe
2026/07/15 17:24:02 Starting ai-svc at 0.0.0.0:8891...
2026/07/15 17:24:02 [grpc] ai-svc gRPC server listening on :8892
2026/07/15 17:24:02 [grpc] services: EmotionQueryService + grpc.health.v1
```

### 4.2 调 gRPC client

```bash
$ go run querye2e.go
[grpc-client] method=/grpc.health.v1.Health/Check target=localhost:8892 latency=57ms err=<nil>
Health/Check('emotion.AI') = 1, err=<nil>                            # SERVING
[grpc-client] method=/emotion_ai.v1.EmotionQueryService/GetEmotionByMessage target=localhost:8892 latency=22ms
GetEmotionByMessage(999999) = NotFound                                # 标准错误码
GetEmotionByConversation(1) = total=4 err=<nil>                       # 4 条情绪
[grpc-client] method=/emotion_ai.v1.EmotionQueryService/GetEmotionByConversation target=localhost:8892 latency=3ms
```

## 5. 与 HTTP API 对照

| 功能 | HTTP（:8891） | gRPC（:8892） |
|------|----------------|----------------|
| 按 message_id 查 | `GET /api/v1/emotion/message/:id` | `GetEmotionByMessage(message_id)` |
| 按 conversation_id 查 | `GET /api/v1/emotion/conversation/:id` | `GetEmotionByConversation(conv_id, limit)` |
| 健康检查 | `GET /health` | `Health/Check("")` / `Health/Check("emotion.AI")` |
| 鉴权 | `x-user-id` JWT | 暂无（Stage 19 范围） |
| 拦截器 | gin Recovery + SkyWalking + JWT | Logging + Recovery + Tracing |

前端继续用 HTTP（JWT 鉴权 + 浏览器 cookie 友好），内部 svc 用 gRPC（低延迟 + 强契约）。

## 6. 已知限制

- **gRPC 端无鉴权**：HTTP 端有 JWT，gRPC 端当前没有（Stage 19 范围外）
- **mTLS 留作可选开关**：当前 gRPC server 走 insecure
- **Proto 字段类型**：message_id 用 int64（与 Postgres 表 column 一致），未来支持 UUID 需改 string
- **未在 ai-svc 暴露 LLM service**：gRPC 端只读 emotion，不调 LLM（避免循环依赖）

## 7. 后续 TODO

- gRPC 端加 AuthInterceptor（与 HTTP 端 JWT 同步）
- chat-svc 实际调 ai-svc gRPC 替换部分 Kafka 同步场景
- mTLS for ai-svc gRPC server
- gRPC reflection（方便 grpcurl 调试）
