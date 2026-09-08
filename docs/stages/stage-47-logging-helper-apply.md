---
status: landed
priority: high
stage: 47
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义)
related-stages:
  - stage-45-observability-sprint-b-regression.md (PR-OBS-17 接口前置)
  - stage-46-observability-gin-entry-span.md (PR-OBS-18 HTTP EntrySpan)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id 串联)
related-commits:
  - 5437ee5 test(shared): RED PR-OBS-15 6 svc main.go Init/SetGlobalSvc 接入层契约
  - 6c33d08 feat(shared+svc): GREEN PR-OBS-15 6 svc 接入 Init/SetGlobalSvc + middleware WithTraceID
---

# Stage 47 · PR-OBS-15 logging helper 接入收口

> **本文档归档 PR-OBS-15（stage-44 §四 C 落地）**。
> 2 个 commit 把 `logging.SetGlobalSvc` 接入到 6 svc main.go + gin middleware 加
> `WithTraceID` 注入,使 Loki 日志可按 svc / trace_id 维度查询（决策 6 闭环）。

## 一、范围 vs 落地

| 项 | 落地 | 证据 |
|---|---|---|
| 6 svc main.go 调 `logging.Init()` + `SetGlobalSvc("<name>")` | ✅ | 6c33d08 |
| ai/web-bff re-export 补全 SetGlobalSvc/WithTraceID/WithAction | ✅ | 6c33d08（internal/logging/logging.go）|
| GinSkywalkingMiddleware 调 `logging.WithTraceID` 注入 Request ctx | ✅ | 6c33d08 |
| 接入层契约测试（RED→GREEN）| ✅ | TestMain_FilesInvokeInitAndSetGlobalSvc |
| middleware trace_id 注入端到端 | ✅ | TestGinSkywalkingMiddleware_InjectsTraceIDIntoRequestContext |
| 6 svc `go build ./...` 零回归 | ✅ | 全干净 |

## 二、核心改动

### 2.1 6 svc main.go（机械改动）

| svc | import 路径 | 接入方式 |
|---|---|---|
| ai-svc | `emotion-echo-ai-svc/internal/logging` (Stage 41 re-export) | `logging.SetGlobalSvc("ai-svc")` |
| web-bff | `emotion-echo-web-bff/internal/logging` (Stage 41 re-export) | `logging.SetGlobalSvc("web-bff")` |
| chat-svc | `shared/pkg/logging` (alias sharedlogging) | `sharedlogging.SetGlobalSvc("chat-svc")` |
| user-svc | `shared/pkg/logging` | `sharedlogging.SetGlobalSvc("user-svc")` |
| assessment-svc | `shared/pkg/logging` | `sharedlogging.SetGlobalSvc("assessment-svc")` |
| analytics-svc | `shared/pkg/logging` | `sharedlogging.SetGlobalSvc("analytics-svc")` |

### 2.2 GinSkywalkingMiddleware 注入 trace_id

```go
// 业务路径中:
c.Set("skywalking_tracer", tracer)

// PR-OBS-15: 把 trace_id (来自 X-Trace-Id header) 注入 Request ctx,
// 让 handler 内 slog.InfoContext(ctx, ...) 自动带 trace_id 字段(Loki 可查)
if tid := c.GetHeader("X-Trace-Id"); tid != "" {
    c.Request = c.Request.WithContext(logging.WithTraceID(c.Request.Context(), tid))
}
```

**trace_id 来源优先级**（注释中说明）：
1. X-Trace-Id header（APISIX 路径可显式注入）
2. skywalking span（若未来 span 携带 trace_id，本 PR 未启用）
3. 跳过（无 trace_id，handler 日志无此字段）

### 2.3 ai/web-bff re-export 补全

Stage 41 PR-5/PR-7 把 internal/logging 改成 re-export 时只 re-export 了 Init/Printf/Infof/Warnf/Fatalf，**漏了 PR-OBS-15 GREEN commit 加的 SetGlobalSvc/WithTraceID/WithAction**。本 PR 补全。

## 三、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-shared/pkg/logging/logging.go:42-65`（SetGlobalSvc/WithTraceID/WithAction）
- `emotion-echo-shared/pkg/middleware/gin_skywalking.go`（PR-OBS-18 实现）
- `emotion-echo-ai-svc/internal/logging/logging.go` + `emotion-echo-web-bff/internal/logging/logging.go`（re-export）
- 6 svc `main.go` 的 import 与 main() 入口

### ② 查相关 ADR / stage

- `docs/stages/stage-44-observability-sprint-b.md §四 C`（决策 6 字段缺失）
- `docs/stages/stage-46-observability-gin-entry-span.md §四`（PR-OBS-15 标记 P1）
- `docs/architecture/decisions.md 决策 6`（JSON 日志 + trace_id）

### ③ 跑现状 smoke

| 项 | 结果 |
|---|---|
| `go test ./emotion-echo-shared/...` | 10 包全绿（含新增 3 case + 1 middleware case）|
| 6 svc `go build ./...` | 全部干净编译 |
| log format 兼容（LOG_FORMAT=json/text）| 5 既有 case PASS |

### ⑤ 列架构假设

- **假设 A**：ai/web-bff 用 internal/logging（Stage 41 re-export 路径），其他 4 svc 直接 import shared。
  **验证**：grep import 路径 ✅
- **假设 B**：trace_id 来自 X-Trace-Id header（APISIX 路径），middleware 解析后注入 Request ctx。
  **验证**：TestGinSkywalkingMiddleware_InjectsTraceIDIntoRequestContext PASS ✅
- **假设 C**：6 svc `logging.Init()` + `SetGlobalSvc` 后，所有 slog 输出自动带 svc 字段。
  **验证**：TestMain_FilesInvokeInitAndSetGlobalSvc PASS + TestLogFields_AllThreeFieldsCombined_RealisticFlow PASS ✅

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-47-logging-helper-apply.md`。2 commit message 末尾均按 AGENTS.md §〇 §⑥ 列调研依据。

## 四、未做项

| 项 | 估 | 备注 |
|---|---|---|
| PR-OBS-23 handler err 透传到 span.EndSpan(err) | 半天 | stage-46 §二.2.3 + stage-45 §四 |
| PR-OBS-19 ServerTracingInterceptor 打 rpc.* + 5 svc gRPC 接入 | 1-1.5 天 | 需先调研 go-zero 兼容性 |
| PR-OBS-19 顺带：span.SetSpanLayer/SetComponent | 半天 | 含在 #19 |
| 业务 handler 主动打业务 tag | backlog | 取决于 PR-OBS-19 |

## 五、与 Stage 44 §四 的对账

| §四 未做项 | 状态 |
|---|---|
| A. 16 分支 merge main | ⏳ 阻塞型（需先修 Nacos ephemeral）|
| B. PR-OBS-12/13/14 完整 span tag | ✅ Stage 45 (接口) + Stage 46 (HTTP) |
| **C. PR-OBS-15 6 svc 接入 logging helper** | ✅ **Stage 47（本 stage）** |
| D. sw-oap SW_TELEMETRY=prometheus | ⏳ 15 分钟 |
| E. PR-OBS-4/5 干净环境实跑 | ⏳ 阻塞型（需先修 Nacos）|
| F. Kafka Sprint C Protobuf | ⏳ 独立 Sprint C（2-3 天）|
| G. Kafka Sprint A §3 历史数据 SQL | ⏳ 运维窗口 |
| H. Sprint B 范围外 backlog | ⏳ 多 PR |

**完成度**：8 项中 1 项已收口（**C → Stage 47**），3 项需 Nacos 修复（A/E 阻塞），4 项独立 Sprint 排期（D/F/G/H）。
