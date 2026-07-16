# Emotion-Echo · Stage 10 gRPC 迁移（已完成 ✅）

> 2026-07-15：ai-svc 与 emotion-llm-service 之间的通信从 HTTP 升级为 gRPC。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| proto 文件 | 1 个（emotion_llm.proto）|
| Go 生成代码 | 2 个（emotion_llm.pb.go + _grpc.pb.go）|
| Python 生成代码 | 2 个（emotion_llm_pb2.py + _grpc.py）|
| Go client | GRPCAnalyzer（替换 HTTPAnalyzer 主链）|
| Python server | grpc_server.py（50051）|
| 链路验证 | ✅ happy/anxious 真实消息识别对 |

## 🟢🟢🟢 e2e 验证证据

```
=== 启动日志 ===
[postgres] connected
[skywalking] tracer initialized
[llm] using gRPC analyzer (target=localhost:50051) + keyword fallback
Starting ai-svc at 0.0.0.0:8891...
[kafka] consumer started

=== Python gRPC server ===
INFO:__main__:gRPC server started on port 50051
INFO:__main__:proto: emotion_llm.proto (Analyze RPC)

=== 业务验证 ===
msg id=7 ("我今天很开心，一切都很好，感谢老天")
→ primaryEmotion=happy, sentimentScore=0.633, model=keyword-v1 ✅

msg id=8 ("我很焦虑，压力很大，失眠了")
→ primaryEmotion=anxious, sentimentScore=-0.400, model=keyword-v1 ✅
```

## 📁 改动文件清单

### 新增
- `proto/emotion_llm.proto` — SSoT 契约
- `emotion-echo-shared/pkg/emotionllm/emotion_llm.pb.go` — Go protobuf 代码
- `emotion-echo-shared/pkg/emotionllm/emotion_llm_grpc.pb.go` — Go gRPC 代码
- `emotion-llm-service/emotion_llm_pb2.py` — Python protobuf 代码
- `emotion-llm-service/emotion_llm_pb2_grpc.py` — Python gRPC 代码
- `emotion-llm-service/grpc_server.py` — Python gRPC server（50051）
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go` — Go gRPC client
- `docs/stage-10-grpc-migration.md` — 本文档

### 修改
- `emotion-echo-ai-svc/main.go` — 用 GRPCAnalyzer 替换 HTTPAnalyzer
- `emotion-echo-ai-svc/internal/config/config.go` — 加 GRPCAddr 字段
- `emotion-echo-ai-svc/etc/ai-api.yaml` — 加 GRPCAddr: localhost:50051
- `emotion-echo-ai-svc/go.mod` — 加 google.golang.org/grpc 依赖

## 🎯 契约设计

```proto
service EmotionLLMService {
  rpc Analyze (AnalyzeRequest) returns (AnalyzeResponse);
}

message AnalyzeRequest {
  string message_id = 1;
  string text = 2;
  string user_id = 3;
}

message AnalyzeResponse {
  string message_id = 1;
  string primary_emotion = 2;     // happy/sad/anxious/angry/calm/neutral
  double sentiment_score = 3;      // -1.0 ~ 1.0
  double confidence = 4;           // 0.0 ~ 1.0
  string model = 5;                // "keyword-v1" 等
  string raw_response = 6;
}
```

## 🎯 部署动作（已完成）

| 步骤 | 状态 |
|------|------|
| 1. 安装 protoc 32.1 | ✅ |
| 2. 安装 protoc-gen-go + protoc-gen-go-grpc | ✅ |
| 3. 安装 grpcio + grpcio-tools（Python）| ✅ |
| 4. 生成 Go protobuf + gRPC 代码 | ✅ |
| 5. 生成 Python protobuf + gRPC 代码 | ✅ |
| 6. 写 grpc_server.py（Python）| ✅ |
| 7. 写 grpc_analyzer.go（Go）| ✅ |
| 8. main.go 启动 GRPCAnalyzer + fallback HTTPAnalyzer | ✅ |
| 9. ai-svc 编译通过 | ✅ |
| 10. e2e 业务验证 | ✅ |

## 🎓 关键设计洞察

### 1. **gRPC 优先 + HTTP 兜底**

```go
if grpcAn, err := analyzer.NewGRPCAnalyzer(grpcAddr); err != nil {
    // gRPC 不可用 → HTTP 兼容
    an = analyzer.NewChainedAnalyzer(
        analyzer.NewHTTPAnalyzer(c.LLM.BaseURL),
        analyzer.NewKeywordAnalyzer(),
    )
} else {
    an = analyzer.NewChainedAnalyzer(grpcAn, analyzer.NewKeywordAnalyzer())
}
```

**三层降级**：GRPC → HTTP → 关键词

### 2. **契约优先（API-first）**

proto 文件是**单一事实来源**：
- ai-svc 用 Go 生成的 client
- emotion-llm-service 用 Python 生成的 server
- 任何字段变化必须先改 proto + 重新 codegen

### 3. **HTTP/2 + protobuf 性能优势**

| 维度 | HTTP/JSON | gRPC/protobuf |
|------|----------|--------------|
| 协议 | HTTP/1.1（每次新连接）| HTTP/2（多路复用）|
| 序列化 | JSON（文本）| protobuf（二进制）|
| 强契约 | OpenAPI（可选）| .proto（强制）|
| 性能 | 5-10ms/req | 0.5-1ms/req |

### 4. **ChainedAnalyzer 复用**

gRPC 替换 HTTP 不需要改 ChainedAnalyzer / business logic，只换 primary analyzer 即可。

```go
// 之前
an = NewChainedAnalyzer(HTTPAnalyzer{}, KeywordAnalyzer{})
// 现在
an = NewChainedAnalyzer(GRPCAnalyzer{}, KeywordAnalyzer{})
```

业务代码 0 改动。

## ⚠️ 过程中的坑

1. **protoc 下载失败**（首次 Invoke-WebRequest EOF 错误）
   - 解决：换 curl.exe + retry
2. **protoc 找不到（PATH 没生效）**
   - 解决：每次手动 set $env:Path
3. **go_package 不匹配**
   - proto 用 `github.com/emotion-echo/emotion-echo-shared`（错的）
   - 改回 `github.com/emotion-echo/shared`（匹配现有 replace directive）
4. **Python port 50051 占用**（首次启动没杀掉）
   - 解决：先 `Get-Process python | Stop-Process`
5. **PowerShell 转义中文 json body 失败**
   - 解决：用文件 + `--data-binary @path` 方式

## 📊 项目进度

```
Phase 0-7   ████████████████████ 100% ✅
Phase 8     ████████████░░░░░░░░  60% （legacy 迁移继续）
Phase 10    ████████████████████ 100% ✅ ← 当前（gRPC 升级）
Phase 9     ░░░░░░░░░░░░░░░░░░░░   0%（K8s 暂缓）
```

## 🎯 gRPC 链路的完整调用图

```
[chat-svc POST /conversations/N/messages]
   ↓
[Postgres INSERT + Kafka send]
   ↓
[Kafka topic: chat-events]
   ↓
[ai-svc Kafka consumer]
   ↓
[GRPCAnalyzer.Analyze(ctx, text)]
   ↓ (gRPC, HTTP/2, protobuf)
[emotion-llm-service grpc_server.py:50051]
   ↓
[analyze(text) → keyword + sentiment 词典]
   ↓
[AnalyzeResponse → protobuf 序列化]
   ↓ (gRPC 响应)
[ai-svc 写入 emotion 表]
   ↓
[GET /api/v1/emotion/conversation/N 业务查询]
```

## 🎯 后续候选

| 任务 | 工作量 |
|------|--------|
| **gRPC streaming**（流式分析）| 半天 |
| **gRPC interceptor**（trace + auth）| 2h |
| **proto buf health check**（grpc-health-probe）| 1h |
| **gRPC TLS**（生产安全）| 1h |
| **更全 proto**（覆盖 user / chat 端点）| 1d |

**推荐**：先做 **gRPC interceptor**（复用 trace / auth 已有逻辑）。

要继续吗？