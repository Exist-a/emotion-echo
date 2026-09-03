# Stage 16: gRPC Stream Interceptor

> 目标：为 stream RPC（unary 之外的 server/client/bidi）补全 interceptor 链。

## 1. 背景

gRPC 有 4 种 RPC 类型：

| 类型 | 模式 | 用途 |
|------|------|------|
| Unary | 1 req → 1 resp | 大部分业务接口 |
| **Server stream** | 1 req → N resp | 订阅、推送、批量分析（Stage 16） |
| **Client stream** | N req → 1 resp | 上传聚合 |
| **Bidi stream** | N req ↔ N resp | 实时双向 |

之前所有 interceptor（Logging/Recovery/Auth/Tracing/Retry）都是 unary。**stream RPC 不走这些 chain**，导致：
- 流式响应没有日志（哪个方法开了流？多少条消息？总耗时？）
- 流中 panic 会拖垮整个 server 进程
- 客户端流没有 timeout，会无限等待

## 2. 设计

### 2.1 Stream interceptor API

gRPC 提供两种 stream interceptor：

```go
// Server
type StreamServerInterceptor func(
    srv interface{},
    ss ServerStream,
    info *StreamServerInfo,
    handler StreamHandler,  // handler 签名: func(srv, stream) error
) error

// Client
type StreamClientInterceptor func(
    ctx context.Context,
    desc *StreamDesc,
    cc *ClientConn,
    method string,
    streamer Streamer,
    opts ...CallOption,
) (ClientStream, error)
```

注意：v1.80+ 移除了 handler 的 `info` 参数（v1.40 之前是 3 参数）。

### 2.2 4 个 stream interceptor

| 函数 | 角色 | 行为 |
|------|------|------|
| `NewServerStreamLoggingInterceptor` | server | 记录 stream 启动/结束 + 消息数 |
| `NewServerStreamRecoveryInterceptor` | server | 捕获 handler panic → INTERNAL status |
| `NewClientStreamLoggingInterceptor` | client | 记录 stream 启动/结束 + 消息数 |
| `NewClientStreamTimeoutInterceptor` | client | ctx timeout 自动取消流 |

### 2.3 wrappedServerStream 计数

Server stream 需要知道 handler send/recv 多少条消息来打 log：

```go
type wrappedServerStream struct {
    grpc.ServerStream
    recvCount, sendCount int
}
func (w *wrappedServerStream) SendMsg(m interface{}) error {
    err := w.ServerStream.SendMsg(m)
    if err == nil { w.sendCount++ }
    return err
}
```

## 3. Proto 更新

```proto
service EmotionLLMService {
  rpc Analyze (AnalyzeRequest) returns (AnalyzeResponse);
  rpc AnalyzeBatch (AnalyzeBatchRequest) returns (stream AnalyzeResponse);
}

message AnalyzeBatchRequest {
  repeated AnalyzeRequest items = 1;
  string user_id = 2;
}
```

重新生成 .pb.go：
```bash
protoc --go_out=emotion-echo-shared --go-grpc_out=emotion-echo-shared \
  --go_opt=Mproto/emotion_llm.proto=github.com/emotion-echo/shared/pkg/emotionllm \
  --go-grpc_opt=Mproto/emotion_llm.proto=github.com/emotion-echo/shared/pkg/emotionllm \
  -I proto proto/emotion_llm.proto
```

protoc-gen-go-grpc v1.6+ 用 generics：`grpc.ServerStreamingClient[Resp]`，需 Go 1.22+。

## 4. TDD 覆盖

4 个测试：

| # | 测试 | 覆盖点 |
|---|------|--------|
| 1 | ServerStreamLogging_LogsStartAndEnd | server 端 log 格式 + 计数 |
| 2 | ServerStreamRecovery_PanicIsRecovered | 1 条后 panic → client 收 Internal |
| 3 | ClientStreamLogging_LogsStartEndAndMsgCount | client 端 start/end log |
| 4 | ClientStreamTimeout_StopsSlowStream | ctx timeout 截断慢流 |

### 关键坑：typed client 不走 interceptor

grpc-go 1.80 的 `ServerStreamingClient[Resp]` 接口（typed generic）**不**走 stream client interceptor chain。必须用底层 `ClientConn.NewStream(ctx, desc, method)` 直接调。

参考 [grpc-go#6700](https://github.com/grpc/grpc-go/issues/6700) — 已知 issue。

```go
// 走 interceptor
cs, _ := conn.NewStream(ctx, &grpc.StreamDesc{
    StreamName:    "AnalyzeBatch",
    ServerStreams: true,
}, "/emotion_llm.v1.EmotionLLMService/AnalyzeBatch")

// typed client 不走 interceptor
stream, _ := cli.AnalyzeBatch(ctx, req)  // generic wrapping breaks chain
```

## 5. Python 端 stream 实现

`grpc_server.py`：

```python
def AnalyzeBatch(self, request, context):
    """Server-streaming 批量分析（Stage 16）"""
    items = list(request.items)
    logger.info(f"AnalyzeBatch request: items={len(items)}")
    for item in items:
        http_resp = http_analyze(item.text)
        yield emotion_llm_pb2.AnalyzeResponse(
            message_id=item.message_id,
            primary_emotion=http_resp.primaryEmotion,
            ...
        )
```

用 `yield` 实现 Python 的 server generator。

## 6. E2E 验证

### 6.1 Python 客户端

```python
for resp in stub.AnalyzeBatch(req):
    print(f'recv msg_id={resp.message_id} emotion={resp.primary_emotion}')
```

输出：
```
recv msg_id=1 emotion=neutral score=0.00
recv msg_id=2 emotion=neutral score=0.00
recv msg_id=3 emotion=neutral score=0.00
Total: 3 messages streamed
```

### 6.2 Go 客户端（带 client interceptor）

```
2026/07/15 16:47:20 [stream-client] method=/emotion_llm.v1.EmotionLLMService/AnalyzeBatch stream-start desc.ServerStreams=true
2026/07/15 16:47:20 [stream-client] method=/emotion_llm.v1.EmotionLLMService/AnalyzeBatch close-send duration=110ms sent=1 recv=0
recv msg_id=1 emotion=neutral score=0.00
recv msg_id=2 emotion=neutral score=0.00
recv msg_id=3 emotion=neutral score=0.00
2026/07/15 16:47:20 [stream-client] method=/emotion_llm.v1.EmotionLLMService/AnalyzeBatch stream-end duration=114ms sent=1 recv=3 code=EOF
✅ Streamed 3 messages
```

server 端：
```
[stream-server] method=/emotion_llm.v1.EmotionLLMService/AnalyzeBatch stream-start
AnalyzeBatch emit: msg_id=1 emotion=neutral
AnalyzeBatch emit: msg_id=2 emotion=neutral
AnalyzeBatch emit: msg_id=3 emotion=neutral
[stream-server] method=/emotion_llm.v1.EmotionLLMService/AnalyzeBatch stream-end duration=... sent=3 recv=1 code=OK
```

## 7. 已知限制

- **Bidi stream** 未单独测试（API 相同，可复用 client/server stream interceptor）
- **typed generic client** 不走 stream interceptor（grpc-go 已知 bug）
- **未封装 stream 级 retry**：流中断后不重试（断流通常意味着业务放弃）
- **Python 端 stream interceptor 缺失**：Python 端 `ServerInterceptor.intercept_service` 不直接支持 stream（需要 `ServerStreamInterceptor` 单独实现，grpcio 1.50+ 才稳定）

## 8. 后续 TODO

- Python 端加 `ServerStreamInterceptor`（包装 RPC 的 handler 在 stream Send/Recv 旁加日志）
- bidi stream 单元测试
- stream 级重试（流中断后从断点续传，目前太复杂未做）
