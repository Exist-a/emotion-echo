---
status: landed
priority: high
stage: 46
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义)
related-stages:
  - stage-45-observability-sprint-b-regression.md (PR-OBS-17 接口抽象前置)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id)
related-commits:
  - af85835 test(shared): RED PR-OBS-18 GinSkywalkingMiddleware EntrySpan + 4 tag 边界
  - 57f6d07 feat(shared): GREEN PR-OBS-18 GinSkywalkingMiddleware 创建 EntrySpan + 4 tag
  - 22208fd refactor(shared): PR-OBS-18 span 挂 ctx 契约固化 + 1 case 覆盖补全
---

# Stage 46 · PR-OBS-18 GinSkywalkingMiddleware EntrySpan + 4 tag 收口

> **本文档归档 PR-OBS-18（stage-44 §四 B 收口第二步）**。
> 3 个 commit 把 `GinSkywalkingMiddleware` 从"ctx 透传"升级为"创建 EntrySpan
> 并打 4 个 http.* / user_id tag"，使 SkyWalking UI 能按 HTTP 维度聚合查询。

## 一、范围 vs 落地

| 项 | 落地 | 证据 |
|---|---|---|
| GinSkywalkingMiddleware 创建 EntrySpan | ✅ | 57f6d07（GREEN）|
| 4 tag: http.method / http.url / http.status_code / user_id | ✅ | 57f6d07 |
| span 挂到 gin ctx (`skywalking_span`) | ✅ | 22208fd（REFACTOR 固化契约）|
| 不破坏现有 PR-OBS-17 mock + PR-OBS-12 边界 case | ✅ | 42 PASS / 0 FAIL |
| 6 svc `go build ./...` 零回归 | ✅ | chat/analytics/user/assessment/ai/web-bff 全干净 |

## 二、核心改动

### 2.1 实现（`emotion-echo-shared/pkg/middleware/gin_skywalking.go`）

```go
func GinSkywalkingMiddleware(tracer grpcinterceptor.Tracer) gin.HandlerFunc {
    return func(c *gin.Context) {
        // /health + /internal/* 仍跳过(避免 scrape 制造无意义 span)
        path := c.Request.URL.Path
        if path == "/health" || strings.HasPrefix(path, "/internal/") {
            c.Next(); return
        }
        c.Set("skywalking_tracer", tracer)  // 旧:仅挂 ctx

        var span grpcinterceptor.Span
        if tracer != nil {
            _, span = tracer.StartEntry(c.Request.Context(), c.FullPath())
        }
        if span != nil {
            c.Set("skywalking_span", span)  // NEW:下游可读 span
            span.Tag("http.method", c.Request.Method)              // NEW
            span.Tag("http.url", c.Request.URL.Path)               // NEW
            if uid := c.GetHeader("X-User-Id"); uid != "" {
                if _, err := strconv.ParseInt(uid, 10, 64); err == nil {
                    span.Tag("user_id", uid)                         // NEW
                }
            }
        }

        c.Next()

        if span != nil {
            span.Tag("http.status_code", strconv.Itoa(c.Writer.Status()))  // NEW
            span.EndSpan(nil)                                              // NEW
        }
    }
}
```

### 2.2 设计取舍

- **opName 用 `c.FullPath()`** = 路由注册路径（用于 span 聚合,避免每条消息都成新 opName）
- **http.url 用 `c.Request.URL.Path`** = 实际请求路径（区分 `/api/v1/chat/42` vs `/api/v1/chat/43`）
- **4 tag 分两段写**：c.Next 前 3 个（method/url/user_id），c.Next 后 1 个（status_code），同步顺序确保 tag 全写入后才 EndSpan（不用 defer）
- **user_id 防御性校验**：仅当 `X-User-Id` header 存在且 `strconv.ParseInt` 成功时打（沿用 `jwt_auth.go:50-56` 解析逻辑，避免非法 uid 进入 trace）
- **nil tracer 容错**：保持 PR-OBS-17 兼容，直接 c.Next() 不挂 span（PR-OBS-12 5 case 验证）

### 2.3 不在本 PR 范围（明确划线）

| 项 | 留作 |
|---|---|
| `span.SetSpanLayer(agentv3.SpanLayer_HTTP)` | PR-OBS-19（需扩 Span 接口）|
| `span.SetComponent(int32)` | PR-OBS-19 |
| handler err 透传到 `span.EndSpan(err)` | PR-OBS-23（需 c.Next wrap 拿 c.Errors() + 5xx 判定）|
| 业务 handler 主动打业务 tag（如 ai-svc fusion kind）| PR-OBS-20+ |

## 三、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-shared/pkg/middleware/gin_skywalking.go:21-32`（PR-OBS-17 实现）
- `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:21-43`（Span.Tag 接口）
- `emotion-echo-shared/pkg/middleware/jwt_auth.go:50-56`（X-User-Id header 解析）
- `emotion-echo-chat-svc/main.go:200-205`（中间件顺序）
- `emotion-echo-web-bff/main.go:104-110`（APISIX jwt-auth 注 X-User-Id）

### ② 查相关 ADR / stage

- `docs/stages/stage-44-observability-sprint-b.md §四 B`（收口范围）
- `docs/stages/stage-45-observability-sprint-b-regression.md`（PR-OBS-17 前置）

### ③ 跑现状 smoke

| 项 | 结果 |
|---|---|
| `go test ./emotion-echo-shared/...` | 全绿 10 包 |
| `go test ./emotion-echo-ai-svc/...` | 全绿 11 包 |
| 6 svc `go build ./...` | 全部干净 |
| middleware 测试 | 42 PASS / 0 FAIL |

### ⑤ 列架构假设

- **假设 A**：中间件顺序 Recovery → Metrics → Skywalking → Auth（6 svc 约定）
  → Skywalking 跑时 X-User-Id header 已注入 Request（APISIX 路径），
  可直接 `c.GetHeader` 读。**验证**：web-bff/main.go:104-110 + chat-svc/main.go:200-205 ✅
- **假设 B**：`c.FullPath()` 在中间件里可用（gin 路由匹配后才入 handler chain）。
  **验证**：测试 POST /api/v1/chat/42 → opName = /api/v1/chat/:id ✅
- **假设 C**：`c.Writer.Status()` 在 c.Next 后立即可读（gin 不在 c.Next 中 flush）。
  **验证**：TestGinSkywalkingMiddleware_TagsHTTPMethodURLStatus PASS ✅
- **假设 D**：6 svc 不需要改 main.go（PR-OBS-17 已用 `NewGo2SkyTracer` 包装）。
  **验证**：`go build ./...` 全绿 ✅

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-46-observability-gin-entry-span.md`。
3 commit message 末尾均按 AGENTS.md §〇 §⑥ 列调研依据。

## 四、未做项

| 项 | 估 | 依赖 |
|---|---|---|
| PR-OBS-19 ServerTracingInterceptor 打 rpc.method/rpc.system/user_id tag + 5 svc gRPC server 接入 shared interceptor | 1-1.5 天 | 需先在 chat/assessment/analytics/user-svc gRPC server 加 `NewServerTracingInterceptor`（go-zero 路径需 zap → slog 桥接）|
| PR-OBS-19 顺带：`span.SetSpanLayer(agentv3.SpanLayer_HTTP/GRPC)` + `SetComponent` | 半天 | 需扩 Span 接口加 SetSpanLayer/SetComponent |
| ~~PR-OBS-23 handler err 透传到 span.EndSpan(err)~~ | ~~半天~~ ✅ Stage 48 | — |
| ~~PR-OBS-19 ServerTracingInterceptor 打 rpc.* + 5 svc gRPC 接入~~ | ~~1-1.5 天~~ ✅ Stage 49 | — |
| ~~PR-OBS-19 顺带：span.SetSpanLayer(GRPC) + SetComponent(5001)~~ | ~~半天~~ ✅ Stage 49 | — |
| PR-OBS-20 业务层 tag（ai-svc fusion kind / chat-svc message length / analytics-svc chart source）| backlog | 业务 handler 拿 span 现已就位 |
| ~~PR-OBS-15 6 svc 接入 logging helper SetGlobalSvc + WithTraceID/WithAction~~ | ~~半天~~ ✅ Stage 47 | — |

## 五、与 Stage 44 §四 B 的对账

| §四 B 列的步骤 | 状态 |
|---|---|
| 1. 抽 TracerInterface（含 CreateLocalSpan）| ✅ Stage 45 |
| 2. 抽 SpanInterface（含 Tag）| ✅ Stage 45 |
| 3. GinSkywalkingMiddleware 改用接口 | ✅ Stage 45 |
| 4. mockSpan + mockTracer + 完整 span tag 断言 | ✅ Stage 45 (ai-svc Kafka) + Stage 46 (HTTP) |
| 5. **GinSkywalkingMiddleware 创建真实 EntrySpan + http.* tag** | ✅ **Stage 46（本 stage）** |
| 6. ServerTracingInterceptor 改用接口 + 多次 Tag | 🟡 接口已扩展（Span.Tag）⏳ PR-OBS-19 |

**完成度**：5/6 步（HTTP 全链落地），剩余 1 步是 gRPC 拦截器（需 go-zero 桥接，估 1-1.5 天）。
