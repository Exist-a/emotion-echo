---
status: landed
priority: high
stage: 48
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义)
related-stages:
  - stage-46-observability-gin-entry-span.md (PR-OBS-18 中间件 EntrySpan)
  - stage-45-observability-sprint-b-regression.md (Span.Tag 接口)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id 串联)
related-commits:
  - 0c509fc test(shared): RED PR-OBS-23 handler err 透传 span.EndSpan(err) 边界
  - 9b849a8 feat(shared): GREEN PR-OBS-23 handler err 透传 span.EndSpan(err)
---

# Stage 48 · PR-OBS-23 handler err 透传 span.EndSpan(err) 收口

> **本文档归档 PR-OBS-23**。2 个 commit 把 `GinSkywalkingMiddleware` 从"总是
> `EndSpan(nil)`"升级为按 `buildSpanError(c)` 规则透传 err，让 OAP UI 能直接
> 过滤"5xx + error"维度（stage-46 §二.2.3 落地）。

## 一、范围 vs 落地

| 项 | 落地 | 证据 |
|---|---|---|
| `buildSpanError(c)` 三段判定：c.Errors → 5xx → nil | ✅ | 9b849a8 |
| 4 RED case（含 1 个既有 + 3 个新增）| ✅ | 0c509fc / 5/5 PASS |
| 中间件 EndSpan 调用点改造 | ✅ | 9b849a8 |
| 6 svc `go build ./...` 零回归 | ✅ | 全干净 |
| shared 10 包 test 全绿 | ✅ | 包括 middleware 5/5 + grpcinterceptor + logging |

## 二、核心改动

### 2.1 实现（`emotion-echo-shared/pkg/middleware/gin_skywalking.go`）

```go
// 中间件 EndSpan 调用点
- span.EndSpan(nil)
+ span.EndSpan(buildSpanError(c))

// 三段判定
func buildSpanError(c *gin.Context) error {
    // 1. 优先 handler 显式 c.Error(err)
    if lastErr := c.Errors.Last(); lastErr != nil {
        if unwrapped := lastErr.Unwrap(); unwrapped != nil {
            return unwrapped
        }
        return lastErr
    }
    // 2. 兜底 status >= 500
    if status := c.Writer.Status(); status >= 500 {
        return fmt.Errorf("http %d", status)
    }
    // 3. 4xx / 200 + 无 c.Error → 不视为 err
    return nil
}
```

### 2.2 设计取舍

- **Unwrap 优先**：`gin.Error.Unwrap()` 返回底层 `error`，让 `errors.Is/As` 兼容；同时让 go2sky `span.Error()` 收到原始 err 信息
- **Unwrap 为 nil 回退**：`gin.Error` 自身（防止 handler 误用 `c.Error(&struct{})` 无 err 字段）
- **status >= 500 兜底**：chat/user/assessment/analytics/ai/web-bff 6 svc handler 现状都是 `c.JSON(500, ...)` 而非 `c.Error(err)`，所以中间件需要从 status 推断
- **不改 handler 现状**：通过 status 兜底兼容，不需要 6 svc 全部改写为 `c.Error` 模式
- **4xx 不视为 err**：业务正常（401/403/404 不算服务端错误），保持 OAP UI error 列语义清晰

### 2.3 与现有 handler 模式的契合

| svc | handler 错误模式 | buildSpanError 触发路径 |
|---|---|---|
| chat-svc | `c.JSON(500, gin.H{"error": err.Error()})` | 路径 2（status 兜底）|
| user-svc | `c.JSON(500, gin.H{"error": err.Error()})` | 路径 2 |
| assessment-svc | 类似 c.JSON 模式 | 路径 2 |
| analytics-svc | 类似 c.JSON 模式 | 路径 2 |
| ai-svc | 类似 c.JSON 模式 | 路径 2 |
| web-bff | 类似 c.JSON 模式 | 路径 2 |
| 未来 handler 显式 `c.Error(err)` 软警告 | c.Error + 200 | 路径 1 |

## 三、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-shared/pkg/middleware/gin_skywalking.go:91`（原 EndSpan(nil)）
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go:30-38`（Go2SkySpan.EndSpan 调 span.Error）
- `emotion-echo-{chat,user}-svc/internal/handler/*_handler.go`（现状：c.JSON 模式）
- `/c/Users/LENVOV/go/pkg/mod/github.com/gin-gonic/gin@v1.9.1/errors.go:118`（errorMsgs.Last）
- `/c/Users/LENVOV/go/pkg/mod/github.com/gin-gonic/gin@v1.9.1/errors.go:95`（Error.Unwrap）

### ② 查相关 ADR / stage

- `docs/stages/stage-44-observability-sprint-b.md §四 B`（整体收口范围）
- `docs/stages/stage-46-observability-gin-entry-span.md §二.2.3`（PR-OBS-23 规划）
- `docs/stages/stage-45-observability-sprint-b-regression.md §四`（PR-OBS-23 标记 P1）

### ③ 跑现状 smoke

| 项 | 结果 |
|---|---|
| `go test ./emotion-echo-shared/...` | 10 包全绿 |
| 6 svc `go build ./...` | 全干净 |

### ⑤ 列架构假设

- **假设 A**：6 svc handler 都不用 `c.Error(err)`，只用 `c.JSON(500, ...)`。
  **验证**：grep `c.Error` 仅 1 处测试代码，handler 实际模式是 c.JSON ✅
- **假设 B**：go2sky `span.Error(time.Now(), err.Error())` 让 OAP UI error 列可见。
  **验证**：tracing_go2sky.go:36 + go2sky 文档确认 ✅
- **假设 C**：4xx 不视为服务端 err（业务正常）。
  **验证**：HTTP RFC 7231 + 行业共识（OAP UI 默认不把 4xx 计入 error rate）✅

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-48-handler-err-propagate.md`。2 commit message 末尾均按 AGENTS.md §〇 §⑥ 列调研依据。

## 四、未做项

| 项 | 估 | 备注 |
|---|---|---|
| PR-OBS-19 ServerTracingInterceptor 打 rpc.* + 5 svc gRPC 接入 | 1-1.5 天 | 需先调研 go-zero 兼容性 |
| PR-OBS-19 顺带：span.SetSpanLayer/SetComponent | 半天 | 含在 #19 |
| 业务 handler 主动打业务 tag | backlog | 取决于 PR-OBS-19 |
| handler 主动改用 c.Error(err) | backlog | 当前 c.JSON 模式够用，无需改造 |

## 五、与 Stage 44 §四 的对账

| §四 未做项 | 状态 |
|---|---|
| A. 16 分支 merge main | ⏳ 阻塞型 |
| B. PR-OBS-12/13/14 完整 span tag | ✅ 5/6 步（Stage 45 + 46 + 47 + 48 收口）|
| C. PR-OBS-15 6 svc logging 接入 | ✅ Stage 47 |
| **新增** PR-OBS-23 handler err 透传 | ✅ **Stage 48（本 stage）** |
| D. sw-oap SW_TELEMETRY | ⏳ 15 分钟 |
| E. PR-OBS-4/5 干净环境实跑 | ⏳ 阻塞型 |
| F. Kafka Sprint C Protobuf | ⏳ 独立 Sprint |
| G. Kafka Sprint A §3 历史 SQL | ⏳ 运维窗口 |
| H. Sprint B 范围外 backlog | ⏳ 多 PR |

**完成度**：业务功能 6 svc 全链路观测落地（HTTP + Kafka consumer + 决策 6 字段 + err 透传）。剩余仅 PR-OBS-19（gRPC interceptor，需 go-zero 桥接）+ Nacos 阻塞项 + 独立 Sprint 排期项。
