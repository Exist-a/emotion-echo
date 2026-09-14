---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/bff-grpc-error-mapping-backlog.md
related-stages:
  - stage-63-bff-grpc-wiring.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-decision-4-closure.md §八 backlog
---

# Plan — BFF 全局 gRPC error → HTTP code 映射（已 landed Sprint G）

## 〇、Sprint G 收口（2026-09-11）

**commit**: `f874d3f`

**修复结果**（docker 端到端实测）：
- `POST /api/v1/tts/synthesize` 走 ai-svc gRPC SynthesizeSpeech，XTTS 容器不可用时：
  - 原（Sprint F2）：HTTP 502（Bad Gateway） ❌
  - 修复后（Sprint G）：HTTP 503（Service Unavailable） ✅
- 7 路径 Final E2E 回归：users/me / conversations / surveys / reports/daily / multimodal/analyze / ai/health 全部 200
- message 文本也更清晰：`rpc error: code = Unavailable desc = ...` → `upstream unavailable: ...`

---

# Plan — BFF 全局 gRPC error → HTTP code 映射 backlog

## 一、问题

Sprint F2 收口时 docker 端到端实测发现：`POST /api/v1/tts/synthesize`（BFF→ai-svc gRPC SynthesizeSpeech）在 XTTS 容器不可用时返 **HTTP 502**。

**实际行为**：
- gRPC 调用通（ai-svc gRPC server 收到请求）
- ai-svc 调 XTTS 容器失败（DNS lookup emotion-echo-xtts 失败）
- ai-svc gRPC server 返 `codes.Unavailable`（与 HTTP 503 语义对齐）
- **BFF 把 gRPC error 统一标 502 Bad Gateway**——错误语义错位

**正确语义**：
- `gRPC codes.Unavailable` → HTTP **503 Service Unavailable**
- `gRPC codes.Unauthenticated` → HTTP **401 Unauthorized**
- `gRPC codes.PermissionDenied` → HTTP **403 Forbidden**
- `gRPC codes.NotFound` → HTTP **404 Not Found**
- `gRPC codes.InvalidArgument` → HTTP **400 Bad Request**
- `gRPC codes.DeadlineExceeded` → HTTP **504 Gateway Timeout**
- 其他 → HTTP **500 Internal Server Error**

## 二、当前 BFF 错误处理机制

`emotion-echo-web-bff/internal/downstream/` 各 gRPC client 文件的错误处理是 ad-hoc 风格：

| 文件 | 错误处理 |
|---|---|
| `user_grpc.go` | `fmt.Errorf("downstream: user getMe: %w", err)` 直接抛 |
| `chat_grpc.go` | 同上 |
| `assessment_grpc.go` | 同上 |
| `analytics_grpc.go` | 同上 |
| `ai_grpc.go` | 同上（**Sprint F2 新增**） |

handler 侧（`multimodal_handler.go` / `tts_handler.go` 等）调用 client 收到 error 后：

```go
resp, err := h.ai.SynthesizeSpeech(session.WithRequestAuth(c), req)
if err != nil {
    Fail(c, http.StatusBadGateway, 1, err.Error())  // ← 统一 502
    return
}
```

**问题**：HTTP status 写死 502（Bad Gateway），不管 gRPC error code 是什么。

## 三、影响范围

Sprint F2 端到端实测确认（2026-09-11）：
- `POST /api/v1/tts/synthesize` 走 ai-svc gRPC → 502（应 503）
- 其他 6 路径（users/me / conversations / surveys / reports/daily / multimodal / ai/health）全部 200 ✅

**预估影响**：chat-svc / assessment-svc / analytics-svc 4 svc gRPC client 都有相同问题（user-svc Login 401 例外——Sprint E/F1 已用 `mapAuthError` 部分处理 InvalidCredentials，但**也只覆盖了 auth 场景，其他错误仍走 502**）。

## 四、修复方案

### 4.1 抽 `mapGRPCError` helper 到 `web-bff/internal/downstream/error.go`

```go
// mapGRPCError 把 gRPC error 转为 (HTTP status, error message)，
// 供 4 svc gRPC client 共用。语义对齐 gRPC → HTTP code mapping 标准。
//
// gRPC codes.Unavailable → 503（与 HTTP handler 503 行为一致）
// gRPC codes.Unauthenticated → 401
// gRPC codes.PermissionDenied → 403
// gRPC codes.NotFound → 404
// gRPC codes.InvalidArgument → 400
// gRPC codes.DeadlineExceeded → 504
// 其他（Internal / Unknown / Canceled）→ 500
//
// 非 gRPC error（context.DeadlineExceeded / net.Error）→ 504 / 502
//
// 返回 (httpStatus, code, message) 三元组，handler 用 OK/Fail 辅助函数包装。
func mapGRPCError(err error) (httpStatus int, code int, message string) {
    if err == nil {
        return http.StatusOK, 0, ""
    }
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.Unavailable:
            return http.StatusServiceUnavailable, 1, "upstream unavailable: " + st.Message()
        case codes.Unauthenticated:
            return http.StatusUnauthorized, 1, "unauthenticated: " + st.Message()
        case codes.PermissionDenied:
            return http.StatusForbidden, 1, "permission denied: " + st.Message()
        case codes.NotFound:
            return http.StatusNotFound, 1, "not found: " + st.Message()
        case codes.InvalidArgument:
            return http.StatusBadRequest, 1, "invalid argument: " + st.Message()
        case codes.DeadlineExceeded:
            return http.StatusGatewayTimeout, 1, "timeout: " + st.Message()
        default:
            return http.StatusInternalServerError, 1, "internal: " + st.Message()
        }
    }
    // 非 gRPC error（如 net.Error 或 context 超时）
    if errors.Is(err, context.DeadlineExceeded) {
        return http.StatusGatewayTimeout, 1, err.Error()
    }
    return http.StatusBadGateway, 1, err.Error()
}
```

### 4.2 5 个 gRPC client 改用 helper

`user_grpc.go` / `chat_grpc.go` / `assessment_grpc.go` / `analytics_grpc.go` / `ai_grpc.go`：

```go
// 当前
return nil, fmt.Errorf("downstream: user getMe: %w", err)

// 改为
return nil, &GRPCError{Status: httpStatus, Code: code, Message: message, Err: err}
```

或保留简单语义：直接 return grpc status error，handler 调 `mapGRPCError` 统一处理。

### 4.3 handler 侧统一

```go
// 当前（multimodal_handler.go / tts_handler.go / ai_health_handler.go 等）
resp, err := h.ai.SynthesizeSpeech(session.WithRequestAuth(c), req)
if err != nil {
    Fail(c, http.StatusBadGateway, 1, err.Error())  // ← 硬编码 502
    return
}

// 改为
resp, err := h.ai.SynthesizeSpeech(session.WithRequestAuth(c), req)
if err != nil {
    status, code, msg := downstream.MapGRPCError(err)
    Fail(c, status, code, msg)  // ← 动态 status
    return
}
```

### 4.4 测试

`web-bff/internal/downstream/error_test.go`(新建):
- `TestMapGRPCError_Unavailable → 503`
- `TestMapGRPCError_Unauthenticated → 401`
- `TestMapGRPCError_NotFound → 404`
- `TestMapGRPCError_InvalidArgument → 400`
- `TestMapGRPCError_DeadlineExceeded → 504`
- `TestMapGRPCError_Internal → 500`
- `TestMapGRPCError_NonGRPCError → 502`（向后兼容：非 gRPC error 仍走 502）

## 五、DoD

| # | 项 | 验证 |
|---|---|---|
| 1 | `mapGRPCError` helper 实现 | 单测覆盖 7 类 gRPC code |
| 2 | 5 个 gRPC client 改用 helper | 单测覆盖错误路径 |
| 3 | 4 svc handler 改用 helper | docker 端到端 tts/synthesize 503 而非 502 |
| 4 | 回归测试：users/me / conversations / surveys / reports/daily 仍 200 | docker |
| 5 | 回归测试：multimodal/analyze 仍 200 | docker |
| 6 | 回归测试：ai/health 仍 200 | docker |

## 六、风险评估

| 风险 | 等级 | 缓解 |
|---|---|---|
| 现有调用方依赖 502 行为 | 🟡 中 | grep 全仓 `http.StatusBadGateway`，逐处确认是否需调整 |
| gRPC error 嵌套（gRPC 内套 HTTP 错误）| 🟢 低 | MapGRPCError 只看顶层 gRPC code，message 含原始 err |
| DeadlineExceeded 与 context 错误混淆 | 🟢 低 | 优先用 grpc status code，回退到 errors.Is 检查 |

## 七、工作量估算

- `mapGRPCError` helper + 单测：2 小时
- 5 gRPC client 改造：1-2 小时
- handler 侧改造：1 小时
- docker 端到端回归：1 小时

**总计**：半天（4-5 小时）

## 八、不在本 backlog 范围

- chat-svc / assessment-svc / analytics-svc svc 内部错误码统一（chat-svc `mapLogicError` / user-svc `mapAuthError` / analytics-svc 等各自定义）—— 这是 svc 内部错误码枚举统一，下次 sprint 单独排期
- gRPC mTLS（dev 用 insecure，prod 必做）—— 决策 18 已记录
- chat-svc PinConversation / StreamMessages gRPC 化（chat-svc 缺底层功能）
- #32 chat-svc HTTP `/api/v1/conversations` 500 bug 根因排查

## 九、调研依据

- Sprint F2 landed: `docs/legacy-plans/landed/sprint-f2-ai-grpc-business-rpcs.md` §七 "剩余 backlog"
- 决策 4 收口 ADR §八: `docs/architecture/adr/adr-2026-09-decision-4-closure.md`
- Final E2E 实测：tts/synthesize 502（gRPC Unavailable 应转 503），docker 验证 2026-09-11
- 各 gRPC client 文件：`emotion-echo-web-bff/internal/downstream/{user,chat,assessment,analytics,ai}_grpc.go`
