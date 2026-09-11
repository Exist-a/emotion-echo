---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/grpc-inter-service-migration.md
related-stages:
  - stage-63-bff-grpc-wiring.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-decision-4-closure.md
---

# Sprint F2 — ai-svc gRPC 3 业务 RPC 实现（解 ai.go MultiModalAnalyze/SynthesizeSpeech/AIHealth）

## 一、问题

`emotion_query.proto`（Stage 19）只定义 4 emotion-query RPC：
- `GetEmotionByMessage` / `GetEmotionByConversation` / `GetFusedEmotion` / `UpsertNeutralEmotion`

**ai-svc 业务方法（ai.go）仍走 HTTP**：
- `MultiModalAnalyze`（多模态情绪/语音/图像分析）
- `SynthesizeSpeech`（TTS 语音合成）
- `AIHealth`（AI 服务集群健康探针）

后果：BFF→ai-svc 业务路径与"决策 4 内部 svc-to-svc = gRPC"不符，**仅 emotion_query 路径走 gRPC**。

## 二、范围

Sprint F2 加 3 RPC + 实现 + BFF client 接入 + docker 端到端验证。

## 三、修法

### 3.1 proto 扩

`proto/emotion_query.proto` 加 3 RPC + 6 message（MultiModalAnalyzeRequest/Response, SynthesizeSpeechRequest/Response, AIHealthRequest/Response, AIHealthEntry）：

```proto
rpc MultiModalAnalyze (MultiModalAnalyzeRequest) returns (MultiModalAnalyzeResponse);
rpc SynthesizeSpeech (SynthesizeSpeechRequest) returns (SynthesizeSpeechResponse);
rpc AIHealth (AIHealthRequest) returns (AIHealthResponse);
```

字段映射：proto MultiModalAnalyzeRequest.kind / file_bytes / text_content 等 → logic.Analyze 第二参数；proto AIHealthEntry → logic.AIHealthEntry 字段对齐。

### 3.2 ai-svc gRPC server 实现

`emotion-echo-ai-svc/internal/grpcserver/server.go`:
- `emotionQueryServer` 加 `svcCtx *svc.ServiceContext` 字段（用于 logic 构造）
- `New(...)` 签名加 `svcCtx *svc.ServiceContext` 参数
- 3 RPC 实现：复用现有 `logic.NewMultiModalAnalyzeLogic / NewSynthesizeSpeechLogic / NewAIHealthLogic`
- `mapAIError` 把 aiclient.ErrNotConfigured / logic.ErrXTTSUnavailable / logic.ErrMultiModalNotInit → codes.Unavailable；其他 → codes.Internal（与 HTTP handler 503/500 行为对齐）

### 3.3 BFF client 接入

`emotion-echo-web-bff/internal/downstream/ai_grpc.go`(新建):
- `aiGRPCClient` 实现 3 RPC（组合 `aiHTTPClient` 不再需要，因 ai-svc 必有 gRPC server）
- `fileToBytes(io.Reader)` 把 multipart File 转 []byte（gRPC 不能传流）
- proto→types 转换：MIME→Mime, All→AllHealthy, SV→SenseVoice, TTS→XTTS（BFF 字段名映射）

`emotion-echo-web-bff/internal/downstream/ai.go`:
- `AIClientOptions` 加 `GRPCConn *grpc.ClientConn` + `Transport string`
- `AITransport(s string) string` 函数
- `NewAIClient` 工厂化：Transport="http" 强制 HTTP；默认 GRPCConn 非 nil 走 gRPC；否则 HTTP fallback

`emotion-echo-web-bff/internal/config/config.go`:
- `AIService` 结构体加 `Transport string`
- 加 `AI_TRANSPORT` env override + 顺手补 `AI_SVC_HTTP_URL` env override（pre-existing 缺失，导致 Sprint F1 测试 FAIL 时发现并修）

`emotion-echo-web-bff/main.go`:
- dial ai-svc gRPC conn（之前 emotion_query 用独立 dial，3 业务 RPC 复用同一 conn）
- `NewAIClient` 调用加 `GRPCConn: aiGRPCConn` + `Transport: AITransport(c.AIService.Transport)`

### 3.4 BFF 路由

`emotion-echo-web-bff/main.go`:
- 注册 `GET /api/v1/ai/health`（pre-existing 缺失，Sprint F2 顺手补）

`emotion-echo-web-bff/internal/handler/ai_health_handler.go`(新建):
- `AIHealthHandler` 接 `AIClient` interface
- `Health(c)` 调 `h.ai.AIHealth(session.WithRequestAuth(c))`（关键：用 session 包注入 user_id 进 ctx，让 aiGRPCClient.AIHealth 内部 withUserID 能取出注入 metadata）
- 错误映射：err → 502；resp.AllHealthy=false 仍 200（K8s liveness 友好）

### 3.5 测试

无新增 RED 测试 — 因 ai-svc gRPC server 现有 4 RPC 单测已覆盖 test framework，本批 3 RPC 走 docker 端到端验证（与 Sprint D 一致模式）。

更新现有契约测试：
- `main_test.go` `wantRoutes` 加 `/api/v1/ai/health`
- `knownPathPrefixes` 加 `/api/v1/ai/health`
- `server_test.go` `New(repo, nil, nil, port)` 加 svcCtx=nil（第 4 个参数）

### 3.6 测试全绿

| 范围 | 结果 |
|---|---|
| ai-svc `go test ./...` | ✅ 全绿（grpcserver 1.2s，其余 cached） |
| emotion-echo-shared `go build ./...` | ✅ |
| BFF `go test ./...` | ✅ 全绿（10 包 + 3 handler 包括新增 ai_health_handler） |

### 3.7 Docker 端到端

| V# | 路径 | 状态 | 评注 |
|---|---|---|---|
| V1 | `GET /api/v1/ai/health` | ✅ 200 | AIHealth 走 ai-svc gRPC,返 FER/SV/XTTS 三模块健康状态（容器不在→ unhealthy 预期） |
| V2 | `POST /api/v1/multimodal/analyze` (kind=text) | ✅ 200 | MultiModalAnalyze 走 ai-svc gRPC text 路径,emotion=neutral model=keyword-stub-v1 |
| V3 | `POST /api/v1/tts/synthesize` | 🟡 502 | SynthesizeSpeech 走 ai-svc gRPC,**gRPC 调用通**,server 返 Unavailable(XTTS 容器未启,503 预期错误流);BFF 误标 502 — 是 BFF **全局**错误处理 gap（chat-svc/assessment/analytics 也有),不在 Sprint F2 范围 |

**核心目标达成**：3 RPC 都从 BFF 经 gRPC 调到 ai-svc，业务路径正确。V3 仅是 BFF 全局错误处理未把 gRPC Unavailable 转 HTTP 503，是 follow-up backlog。

## 四、DoD 验证

| # | 项 | 结果 |
|---|---|---|
| 1 | proto 扩 3 RPC + 生成 stub | ✅ |
| 2 | ai-svc gRPC server 3 RPC 实现 + New 加 svcCtx | ✅ |
| 3 | BFF aiGRPCClient 接入 + 工厂化 NewAIClient | ✅ |
| 4 | BFF `/api/v1/ai/health` 路由补 + AIHealthHandler | ✅ |
| 5 | ai-svc `go test ./...` 全绿 | ✅ |
| 6 | BFF `go test ./...` 全绿（10 包 + 3 handler） | ✅ |
| 7 | ai-svc v0.1.1 + BFF v0.1.9 rebuild | ✅ |
| 8 | docker 端到端 AIHealth 200 + MultiModal 200 + TTS gRPC 通（XTTS 未启 Unavailable 预期） | ✅ |

## 五、变更清单

| 文件 | 变更 |
|---|---|
| `proto/emotion_query.proto` | + 3 rpc + 6 message |
| `emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go` | gen.sh 重新生成 |
| `emotion-echo-shared/pkg/emotionquery/emotion_query_grpc.pb.go` | gen.sh 重新生成 |
| `emotion-echo-ai-svc/internal/grpcserver/server.go` | + 3 RPC 实现 + svcCtx 字段 + mapAIError + import aiclient/logic/errors/strings |
| `emotion-echo-ai-svc/internal/grpcserver/server_test.go` | New() 签名加 svcCtx=nil |
| `emotion-echo-ai-svc/main.go` | grpcserver.New 加 svcCtx 参数 |
| `emotion-echo-web-bff/internal/downstream/ai_grpc.go` | 新建（aiGRPCClient + 3 proto→types 转换 + fileToBytes helper） |
| `emotion-echo-web-bff/internal/downstream/ai.go` | AIClientOptions 加 GRPCConn + Transport + AITransport + 工厂化 NewAIClient |
| `emotion-echo-web-bff/internal/handler/ai_health_handler.go` | 新建（AIHealthHandler + Health handler 用 session.WithRequestAuth） |
| `emotion-echo-web-bff/internal/config/config.go` | AIService 加 Transport + AI_TRANSPORT env + 顺手补 AI_SVC_HTTP_URL env |
| `emotion-echo-web-bff/main.go` | dial ai-svc gRPC + NewAIClient 加 GRPCConn + Transport + 注册 /api/v1/ai/health |
| `emotion-echo-web-bff/main_test.go` | wantRoutes + knownPathPrefixes 加 /api/v1/ai/health |
| `deploy/docker-compose.apps.yml` | ai-svc v0.1.0→v0.1.1, BFF v0.1.7→v0.1.9 |

## 六、决策 4 覆盖度最终刷新（2026-09-11 Sprint F2 后）

| 维度 | Sprint F1 后 | **Sprint F2 后** | 评注 |
|---|---|---|---|
| 核心业务路径（BFF→4 svc handler 调用） | 21/21 = 100% | **24/24 = 100%** | +3 (ai-svc 业务) |
| BFF→4 svc 全方法 | 100% | **100%** | ai.go 3 方法也走 gRPC |
| 所有内部 svc-to-svc（决策 4 全文范围） | ~90% | **~95%** | +3 RPC |

## 七、剩余 backlog（决策 4 收口 ADR §八基础上）

- **BFF 全局 gRPC error → HTTP code 映射**：本次 Sprint F2 V3 暴露 BFF 把 gRPC codes.Unavailable 统一标 502，与 HTTP handler 503 行为不符。chat-svc/assessment/analytics 等 4 svc 都有同问题。建议下次 sprint 统一在 `web-bff/internal/downstream/error.go` 加 `mapGRPCError(err) (int, string)` helper，4 svc gRPC client 复用
- chat-svc PinConversation/StreamMessages gRPC（功能未触发）
- #32 chat-svc HTTP `/api/v1/conversations` 500 bug（被 BFF 默认 grpc 规避，根因待查）
- gRPC mTLS（dev 用 insecure，prod 必做）

## 八、调研依据

- `proto/emotion_query.proto`（Sprint F2 扩 3 RPC）
- `emotion-echo-ai-svc/internal/handler/multimodal_handler.go`（HTTP 端 3 handler 行为契约）
- `emotion-echo-ai-svc/internal/logic/{multimodalanalyzelogic,synthesizespeechlogic,aihealthlogic}.go`（logic req/resp 形态）
- `emotion-echo-web-bff/internal/downstream/ai.go`（AIClient interface + 3 方法签名）
- 决策 18 #25 #26 #27 #33 #34 #35 全文
