---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/bff-grpc-error-mapping-backlog.md
related-stages:
  - stage-63-grpc-closure-report.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-decision-4-closure.md
---

# Sprint G — BFF 全局 gRPC error → HTTP code 映射（解 tts/synthesize 502 bug）

## 一、问题

Sprint F2 端到端实测发现：`POST /api/v1/tts/synthesize`（BFF→ai-svc gRPC SynthesizeSpeech）在 XTTS 容器不可用时返 **HTTP 502**。

**根因**：5 个 gRPC client（user/chat/assessment/analytics/ai）的错误处理是 ad-hoc 风格——`fmt.Errorf("downstream: %w", err)` 简单包装，handler 侧 `if err != nil { Fail(c, http.StatusBadGateway, ...) }` 硬编码 502，**完全忽略 gRPC error code**。

**正确语义**（gRPC → HTTP 映射）：
- `gRPC codes.Unavailable` → HTTP **503 Service Unavailable**（与 HTTP handler 503 行为对齐）
- `gRPC codes.Unauthenticated` → HTTP **401 Unauthorized**
- `gRPC codes.PermissionDenied` → HTTP **403 Forbidden**
- `gRPC codes.NotFound` → HTTP **404 Not Found**
- `gRPC codes.InvalidArgument` → HTTP **400 Bad Request**
- `gRPC codes.DeadlineExceeded` → HTTP **504 Gateway Timeout**
- 其他 gRPC（Internal/Unknown/Canceled）→ HTTP **500 Internal Server Error**

## 二、修法

### 2.1 抽 `MapGRPCError` helper

`emotion-echo-web-bff/internal/downstream/error.go` 加：

```go
// MapGRPCError 把 gRPC error 或普通 error 映射为 (httpStatus, code, message)。
// 7 类 gRPC code + context.DeadlineExceeded + 其他非 gRPC error fallback + nil 全覆盖。
func MapGRPCError(err error) (httpStatus int, code int, message string) { ... }
```

### 2.2 `wrapGRPCError` helper

`emotion-echo-web-bff/internal/downstream/ai_grpc.go` 加（其他 client 共用同一函数，因为同包）：

```go
// wrapGRPCError 把 gRPC error 包装为 APIError（用 MapGRPCError 决定 StatusCode）。
// Sprint G（2026-09-11）：所有 5 个 gRPC client 用此统一包装。
// handler 侧调 StatusCodeOf(err) 拿到正确 HTTP code（gRPC Unavailable → 503）。
func wrapGRPCError(err error, op string) error {
    if err == nil {
        return nil
    }
    status, _, msg := MapGRPCError(err)
    return &APIError{
        StatusCode: status,
        Msg:        fmt.Sprintf("downstream: %s: %s", op, msg),
    }
}
```

### 2.3 5 个 gRPC client 改造

`user_grpc.go` / `chat_grpc.go` / `assessment_grpc.go` / `analytics_grpc.go` / `ai_grpc.go` 共 **24 处** `fmt.Errorf` → `wrapGRPCError`：

| 文件 | 替换处数 |
|---|---|
| user_grpc.go | 7（GetMe/UpdateMe/GetByID/ResetPassword/Logout/Login/Register） |
| chat_grpc.go | 7（CreateConv/SendMessage/ListMessages/ListConversations/DeleteConv/PinConv/StreamMessages） |
| assessment_grpc.go | 3（ListSurveys/GetSurvey/SubmitSurvey） |
| analytics_grpc.go | 6（DailyReport/TrendReport/DayNightPattern/InteractionDepth/FrequencyTrend/MentalAssessment） |
| ai_grpc.go | 3（MultiModalAnalyze/SynthesizeSpeech/AIHealth） |

### 2.4 handler 侧零改动

`statusFor(err error) int`（`handler/status.go`）已存在并调 `downstream.StatusCodeOf(err)`：
- 原来 `StatusCodeOf` 只能解析 `APIError`（HTTP client 包装路径）
- `wrapGRPCError` 把 gRPC error 也包装为 `*APIError{StatusCode: ...}`，`StatusCodeOf` 自动解析
- **handler 一行不改**，全部 4 svc 的 handler（multimodal_handler.go / tts_handler.go / ai_health_handler.go / chat 等）自动用 `statusFor` 拿到正确 HTTP code

### 2.5 测试

`emotion-echo-web-bff/internal/downstream/error_sprint_g_test.go`(新建, 10 个测试):

```
TestMapGRPCError_Unavailable           → 503
TestMapGRPCError_Unauthenticated       → 401
TestMapGRPCError_PermissionDenied      → 403
TestMapGRPCError_NotFound              → 404
TestMapGRPCError_InvalidArgument       → 400
TestMapGRPCError_DeadlineExceeded      → 504
TestMapGRPCError_Internal             → 500
TestMapGRPCError_ContextDeadlineExceeded → 504 (非 gRPC)
TestMapGRPCError_OtherNonGRPC          → 502 (非 gRPC fallback)
TestMapGRPCError_Nil                   → 200
```

## 三、DoD 验证

| # | 项 | 结果 |
|---|---|---|
| 1 | `MapGRPCError` helper + 10 个 RED unit test | ✅ 全 GREEN |
| 2 | 5 个 gRPC client 24 处用 helper | ✅ |
| 3 | handler 侧零改动（自动生效） | ✅ |
| 4 | BFF `go test ./...` 全包绿（10 包 + handler） | ✅ |
| 5 | BFF v0.1.10 rebuild | ✅ |
| 6 | docker 端到端 tts/synthesize 502 → 503 | ✅ |
| 7 | 7 路径 Final E2E 回归全 200 | ✅ |

## 四、Docker 端到端实测（2026-09-11）

| # | 路径 | gRPC 调用链 | HTTP | 评注 |
|---|---|---|---|---|
| 1 | `GET /api/v1/users/me` | BFF→user-svc gRPC GetMe | **200** ✅ | userId=1 |
| 2 | `GET /api/v1/conversations` | BFF→chat-svc gRPC ListConversations | **200** ✅ | 空 list |
| 3 | `GET /api/v1/surveys` | BFF→assessment-svc gRPC ListSurveys | **200** ✅ | 空 `total=0` |
| 4 | `GET /api/v1/reports/daily?user_id=1` | BFF→analytics-svc gRPC ReportsDaily | **200** ✅ | 空 summary |
| 5 | `GET /api/v1/ai/health` | BFF→ai-svc gRPC AIHealth | **200** ✅ | 3 AI 模块 health |
| 6 | `POST /api/v1/multimodal/analyze` (kind=text) | BFF→ai-svc gRPC MultiModalAnalyze | **200** ✅ | emotion=neutral |
| 7 | `POST /api/v1/tts/synthesize` | BFF→ai-svc gRPC SynthesizeSpeech | **503** ✅ | **原 502,Sprint G 修复** |

## 五、变更清单

| 文件 | 变更 |
|---|---|
| `emotion-echo-web-bff/internal/downstream/error.go` | + `MapGRPCError` helper + 7 类 gRPC code 注释 |
| `emotion-echo-web-bff/internal/downstream/error_sprint_g_test.go` | 新建（10 个 unit test） |
| `emotion-echo-web-bff/internal/downstream/ai_grpc.go` | + `wrapGRPCError` helper + 3 RPC 改用 |
| `emotion-echo-web-bff/internal/downstream/user_grpc.go` | 7 处 `fmt.Errorf` → `wrapGRPCError` + 删 `fmt` import |
| `emotion-echo-web-bff/internal/downstream/chat_grpc.go` | 7 处 `fmt.Errorf` → `wrapGRPCError` |
| `emotion-echo-web-bff/internal/downstream/assessment_grpc.go` | 3 处 `fmt.Errorf` → `wrapGRPCError` |
| `emotion-echo-web-bff/internal/downstream/analytics_grpc.go` | 6 处 `fmt.Errorf` → `wrapGRPCError` + 删 `fmt` import |
| `deploy/docker-compose.apps.yml` | BFF v0.1.9 → v0.1.10 |
| `docs/plans/bff-grpc-error-mapping-backlog.md` | frontmatter `status: planned` → `landed` + §〇 Sprint G 收口 |

## 六、剩余 backlog（决策 4 收口 ADR §八已同步）

- **#32** chat-svc HTTP `/api/v1/conversations` 500 bug 根因排查
- chat-svc PinConversation/StreamMessages gRPC 化（功能未触发）
- 错误码统一映射（chat-svc `mapLogicError` / user-svc `mapAuthError` / analytics-svc 等各自分散）
- gRPC mTLS（dev 用 insecure，prod 必做）

## 七、调研依据

- `docs/plans/bff-grpc-error-mapping-backlog.md`（原始 backlog）
- `emotion-echo-web-bff/internal/handler/status.go`（已存在的 `statusFor` helper）
- `emotion-echo-web-bff/internal/downstream/error.go`（已存在的 `APIError` + `StatusCodeOf`）
- 决策 4 收口 ADR §八 backlog 已加 BFF error mapping 项并标"已 Sprint G 关闭"
- Stage 63 grpc 收口报告 §七 加 Sprint G 关闭
