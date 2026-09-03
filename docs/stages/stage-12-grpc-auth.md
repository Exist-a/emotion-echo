# Emotion-Echo · Stage 12 gRPC Internal Auth（已完成 ✅）

> 2026-07-15：gRPC 跨进程调用加 internal API key 鉴权。
> ai-svc ↔ emotion-llm-service 内部通信加 metadata 鉴权层。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 新增文件 | auth.go（server auth + client helper）+ auth_test.go（7 个测试）|
| TDD 测试 | **14 个 interceptor 测试全 PASS**（6+7+1 round-trip）|
| 三种模式 | disabled / enabled+matched / enabled+mismatch |
| e2e 验证 | ✅ 全部通过 |

## 🟢🟢🟢 e2e 验证证据

### 场景 1：auth disabled（双方都不配 key）
```
ai-svc: [llm] using gRPC analyzer (target=..., auth=disabled)
Python: interceptors: ... + Auth(auth=disabled)
调用 → 200 OK ✅
```

### 场景 2：auth enabled + 双方同 key
```
ai-svc: [llm] using gRPC analyzer (target=..., auth=enabled)
Python: interceptors: ... + Auth(auth=enabled)
ai-svc 启动 yaml: InternalAPIKey: "emotion-echo-internal-2026"
Python env var:   INTERNAL_API_KEY=emotion-echo-internal-2026

调用 → 200 OK ✅
```

### 场景 3：auth enabled + 错 key
```
ai-svc 启动 yaml: InternalAPIKey: "wrong-key-9999"
Python env var:   INTERNAL_API_KEY=emotion-echo-internal-2026

Python server:
  WARNING [grpc-server] AUTH REJECTED: invalid api key for /Analyze

ai-svc:
  [grpc-client] method=... err=rpc error: code = Unauthenticated desc = invalid api key
  [analyzer] primary failed, fallback to secondary ✅ 不崩溃
```

## 📁 改动文件清单

### 新增
- `emotion-echo-shared/pkg/grpcinterceptor/auth.go` — ServerAuth + WithInternalAPIKey
- `emotion-echo-shared/pkg/grpcinterceptor/auth_test.go` — 7 个 TDD 测试
- `emotion-echo-ai-svc/internal/analyzer/auth_wrapped.go` — AuthWrappedAnalyzer（auto-inject key）

### 修改
- `emotion-llm-service/grpc_server.py` — Python AuthInterceptor
- `emotion-echo-ai-svc/main.go` — 集成 AuthWrappedAnalyzer + auth status 日志
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go` — 加 AnalyzeWithAuth 方法
- `emotion-echo-ai-svc/internal/config/config.go` — 加 InternalAPIKey 字段
- `emotion-echo-ai-svc/etc/ai-api.yaml` — 配 InternalAPIKey

## 🎯 3 种部署模式

| 模式 | server expectedKey | client apiKey | 结果 |
|------|-------------------|---------------|------|
| **disabled** | `""` | `""` | ✅ 直接通过（dev mode）|
| **enabled match** | `"secret-123"` | `"secret-123"` | ✅ 通过 |
| **enabled mismatch** | `"secret-123"` | `"wrong-key"` | ❌ UNAUTHENTICATED |

## 🎯 Go API

```go
// Server-side: enforce auth
server := grpc.NewServer(
    grpc.UnaryInterceptor(
        grpc.ChainUnaryInterceptor(
            grpcinterceptor.NewServerAuthInterceptor("expected-key"),
            grpcinterceptor.ServerLoggingInterceptor(),
            grpcinterceptor.ServerRecoveryInterceptor(),
        ),
    ),
)

// Client-side: inject key
ctx := grpcinterceptor.WithInternalAPIKey(context.Background(), "secret-123")
resp, err := client.Analyze(ctx, req)
```

## 🎯 Python API

```python
# Server-side
class AuthInterceptor(grpc.ServerInterceptor):
    def __init__(self, expected_api_key: str = ""):
        self.expected_api_key = expected_api_key
    
    def intercept_service(self, continuation, handler_call_details):
        if not self.expected_api_key:
            return continuation(handler_call_details)
        # ... check metadata, return deny handler if invalid ...

# Usage
api_key = os.environ.get("INTERNAL_API_KEY", "")
server = grpc.server(
    interceptors=(
        LoggingInterceptor(),
        RecoveryInterceptor(),
        AuthInterceptor(expected_api_key=api_key),
    ),
)
```

## 🎯 TDD 测试覆盖（7 个新增）

```
TestServerAuth_EmptyKey_AllowsAll              ✓
TestServerAuth_MissingMetadata_Rejects         ✓
TestServerAuth_WrongKey_Rejects                ✓
TestServerAuth_CorrectKey_Allows               ✓
TestWithInternalAPIKey_EmptyKey_NoChange       ✓
TestWithInternalAPIKey_NonEmpty_SetsMetadata   ✓
TestWithInternalAPIKey_PreservesExistingMetadata ✓
TestServerAuth_EndToEnd_RoundTrip              ✓ (8 个合计)
```

## 🎓 关键设计洞察

### 1. **disabled 模式 for dev**

```go
if expectedKey == "" {
    return handler(ctx, req)  // bypass auth
}
```

开发环境不配 key 就自动跳过 auth，零配置启动。

### 2. **AppendToOutgoingContext 而非 NewOutgoingContext**

```go
// ✗ 会丢失已有 metadata（如 trace-id）
md := metadata.New(...)
return metadata.NewOutgoingContext(ctx, md)

// ✓ 保留并追加
return metadata.AppendToOutgoingContext(ctx, key, value)
```

OpenTelemetry / SkyWalking 等 trace header 需要保留。

### 3. **AuthWrappedAnalyzer 自动注入**

```go
// 业务代码完全无感
an = analyzer.NewChainedAnalyzer(authWrapped, analyzer.NewKeywordAnalyzer())
// 内部自动调用 grpcinterceptor.WithInternalAPIKey(ctx, apiKey)
```

未来加新 gRPC client，只要 wrap 一次即可。

### 4. **ChainedAnalyzer 让鉴权失败不致命**

鉴权失败 → grpc 401 → ChainedAnalyzer fallback → keyword analyzer

**业务永远不中断**。这是 ChainedAnalyzer 设计的核心价值。

### 5. **gRPC metadata key 强制小写**

```go
const InternalAPIKeyMetadataKey = "x-internal-api-key"
```

gRPC spec 要求 metadata key 全小写，否则传输会归一化导致不一致。

## ⚠️ 过程中的坑

1. `metadata.NewOutgoingContext` 覆盖已有 metadata → 改用 `AppendToOutgoingContext`
2. `apiKeyStatus` 函数实现丢失（SearchReplace 在中文注释中不稳定）
3. `analyzer.NewAuthWrappedAnalyzer` 编译时找不到 → 直接用 Write 重写 main.go

## 📊 项目总进度

```
Phase 0-7    ████████████████████ 100% ✅
Phase 8      ████████████████░░░░  80% （legacy 迁移继续）
Phase 10     ████████████████████ 100% ✅ （gRPC 迁移）
Phase 11     ████████████████████ 100% ✅ （interceptor: log+recover）
Phase 12     ████████████████████ 100% ✅ ← 当前（auth）
Phase 9 K8s  ░░░░░░░░░░░░░░░░░░░░   0%（按你要求暂缓）
```

## 🎯 auth 完整调用链

```
[chat-svc POST /conversations/N/messages]
   ↓
[Kafka: chat-events]
   ↓
[ai-svc Kafka consumer]
   ↓
[ChainedAnalyzer(primary=AuthWrappedAnalyzer, secondary=KeywordAnalyzer)]
   ↓ AuthWrappedAnalyzer 注入 x-internal-api-key metadata
   ↓
[GRPCAnalyzer.Analyze(ctx, text)]
   ↓ [grpc-client] method=... err=...
   ↓ (gRPC + metadata)
[emotion-llm-service gRPC server]
   ↓ LoggingInterceptor
   ↓ RecoveryInterceptor
   ↓ AuthInterceptor: 校验 x-internal-api-key
   ↓    ├─ key 不匹配 → UNAUTHENTICATED → client 收到 → fallback
   ↓    └─ key 匹配 → 继续 → AnalyzeResponse
   ↓
[ai-svc 写库]
```

## 🎯 后续可选

| 任务 | 工作量 |
|------|--------|
| **ServerTracing interceptor**（SkyWalking）| 2h |
| **ClientRetry interceptor**（自动重试）| 1h |
| **mTLS**（生产级）| 4h |
| **per-svc API key**（识别 caller）| 2h |

**推荐**：ServerTracing（统一调用链可视化）。

要继续吗？