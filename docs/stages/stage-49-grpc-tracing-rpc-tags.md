---
status: landed
priority: high
stage: 49
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义)
related-stages:
  - stage-44-observability-sprint-b.md (Sprint B 收口,本 stage 接 §四 B 步骤 6)
  - stage-45-observability-sprint-b-regression.md (PR-OBS-17 接口前置)
  - stage-46-observability-gin-entry-span.md (PR-OBS-18 HTTP EntrySpan,本 stage 与之对称)
  - stage-47-logging-helper-apply.md (PR-OBS-15 logging 接入)
  - stage-48-handler-err-propagate.md (PR-OBS-23 handler err 透传)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id 串联)
related-commits:
  - 96287f2 test(shared): RED PR-OBS-19 gRPC server interceptor 打 rpc.* tag + Span.SetSpanLayer/Component
  - ce240dc feat(shared): GREEN PR-OBS-19 ServerTracingInterceptor 打 rpc.* + SetSpanLayer/Component
---

# Stage 49 · PR-OBS-19 gRPC interceptor rpc.* tag + OAP layer/component 收口

> **本文档归档 PR-OBS-19（stage-44 §四 B 步骤 6 收口，business 6 svc 全链路
> 观测最后一步）**。2 个 commit 把 `ServerTracingInterceptor` + `ClientTracingInterceptor`
> 升级为设置 OAP layer/component + 3 个 RPC tag，让 OAP UI 按 RPC 维度聚合。

## 一、范围 vs 落地

| 项 | 落地 | 证据 |
|---|---|---|
| `Span` 接口扩 `SetSpanLayer` + `SetComponent` | ✅ | 96287f2 |
| Go2Sky adapter 实现 `SetSpanLayer` + `SetComponent` | ✅ | 96287f2 |
| mockSpan / stubSpan 升级（向后兼容）| ✅ | 96287f2 |
| `ServerTracingInterceptor` 打 5 项（layer/component/3 tag）| ✅ | ce240dc |
| `ClientTracingInterceptor` 打 4 项（layer/component/2 tag）| ✅ | ce240dc |
| `extractUserIDFromCtx` 从 gRPC metadata 抽 x-user-id | ✅ | ce240dc |
| 6 svc `go build ./...` 零回归 | ✅ | 全干净 |

## 二、核心改动

### 2.1 Span interface 扩展

```go
type Span interface {
    EndSpan(err error)
    Tag(key, value string)
    SetSpanLayer(layer int32)     // PR-OBS-19 新增
    SetComponent(componentID int32) // PR-OBS-19 新增
}

// 配套常量(OAP enum 值)
const (
    SpanLayerHTTP  int32 = 2
    SpanLayerGRPC  int32 = 5
    SpanLayerMQ    int32 = 6  // Kafka 沿用 MQ
    SpanLayerCache int32 = 7
    SpanLayerDB    int32 = 3

    ComponentGoGRPC    int32 = 5001
    ComponentGoHTTP    int32 = 5002
    ComponentGoKafka   int32 = 5003
    ComponentGoRedis   int32 = 5008
    ComponentGoPostgre int32 = 5005
)
```

**Tag 类型决策**：`int32` 而非 `go2sky` 原生 enum，避免 shared pkg 测试依赖 `agentv3` protobuf。

### 2.2 ServerTracingInterceptor 改造

```go
ctx, span := tracer.StartEntry(ctx, info.FullMethod)

// PR-OBS-19: 设置 OAP layer/component + 3 个 RPC tag
span.SetSpanLayer(SpanLayerGRPC)   // layer=5 (GRPC)
span.SetComponent(ComponentGoGRPC) // component=5001 (Go gRPC)
span.Tag("rpc.system", "grpc")
span.Tag("rpc.method", info.FullMethod)  // 如 /emotion_query.v1.EmotionQueryService/GetEmotionByMessage
if uid := extractUserIDFromCtx(ctx); uid != "" {
    span.Tag("user_id", uid)  // APISIX jwt-auth 注入的 x-user-id
}
```

### 2.3 ClientTracingInterceptor 对称改造

```go
ctx, span := tracer.StartEntry(ctx, "client:"+method)
span.SetSpanLayer(SpanLayerGRPC)
span.SetComponent(ComponentGoGRPC)
span.Tag("rpc.system", "grpc")
span.Tag("rpc.method", method)
// client 端无 user_id(metadata 由上游 server 注入,client ctx 通常由调用方注入,
// 不在此层抽 — 留作业务层需要时扩展)
```

### 2.4 extractUserIDFromCtx helper

```go
func extractUserIDFromCtx(ctx context.Context) string {
    md, ok := metadata.FromIncomingContext(ctx)
    if !ok { return "" }
    values := md.Get("x-user-id")
    if len(values) == 0 { return "" }
    uid := values[0]
    // 防御性校验: 与 GinAuthMiddleware strconv.ParseInt 一致,确保 OAP tag 数字合法
    if _, err := strconv.ParseInt(uid, 10, 64); err != nil {
        return ""
    }
    return uid
}
```

## 三、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:21-43`（PR-OBS-17 接口）
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go`（adapter）
- `emotion-echo-ai-svc/internal/grpcserver/server.go:67-99`（唯一 gRPC server）
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go:61-96`（gRPC client）
- `emotion-echo-{chat,user,assessment,analytics,web-bff}/main.go`（无 gRPC server，gin HTTP）

### ② 查相关 ADR / stage

- `docs/stages/stage-44-observability-sprint-b.md §四 B 步骤 6`（收口范围）
- `docs/stages/stage-46-observability-gin-entry-span.md §二.2.3`（PR-OBS-19 规划）
- `docs/stages/stage-45-observability-sprint-b-regression.md §四`（PR-OBS-19 标记 P1）
- `docs/architecture/roadmap.md`（Stage 索引）

### ③ 跑现状 smoke

| 项 | 结果 |
|---|---|
| `go test ./emotion-echo-shared/...` | 10 包全绿 |
| 6 svc `go build ./...` | 全干净 |

### ⑤ 列架构假设

- **假设 A**：5 svc 中仅 ai-svc 有 gRPC server，其余是 gin HTTP。
  **验证**：grep `grpc.NewServer` 仅 ai-svc 1 处 ✅
- **假设 B**：现有 gRPC 接入点（ai-svc grpcserver + grpc_analyzer）已接 ServerTracing/ClientTracing interceptor，**新行为自动生效，无需改 ai-svc 代码**。
  **验证**：`grpcserver/server.go:76` + `grpc_analyzer.go:71` 已接入 ✅
- **假设 C**：go2sky `SetSpanLayer(agentv3.SpanLayer)` + `SetComponent(int32)` 已实现。
  **验证**：`go.sum` vendor `span.go` 接口确认 ✅
- **假设 D**：`extractUserIDFromCtx` 与 `GinAuthMiddleware` 解析 `X-User-Id` 模式一致（仅 metadata vs header 区别）。
  **验证**：`jwt_auth.go:50-56` + 防御性 strconv.ParseInt 校验 ✅

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-49-grpc-tracing-rpc-tags.md`。2 commit message 末尾均按 AGENTS.md §〇 §⑥ 列调研依据。

## 四、⚠️ 与 Stage 46 §四 描述的偏差（诚实标注）

stage-46 §四 原话：
> "PR-OBS-19 ServerTracingInterceptor 打 rpc.method/rpc.system/user_id tag + **5 svc gRPC server 接入 shared interceptor**（需先在 chat/assessment/analytics/user-svc gRPC server 加 NewServerTracingInterceptor）（go-zero 路径需 zap → slog 桥接）"

**与代码现状不符**：

1. **5 svc 中仅 ai-svc 有 gRPC server**：grep 全仓 `grpc.NewServer` 仅 `ai-svc/internal/grpcserver/server.go:81` 1 处。chat/assessment/analytics/user/web-bff 都是纯 gin HTTP，没有 gRPC server。
2. **go-zero 路径不存在**：Stage 41 gozero-removal 已彻底移除 `zrpc`，全仓 `go.mod` 无 go-zero 依赖。
3. **go-zero → slog 桥接不需要**：所有 svc 都是 stdlib slog + shared/pkg/logging（PR-OBS-15 Stage 47）。

**实际工作**：本 PR **不需要改任何 svc main.go**，因为 ai-svc 已经是唯一接入点（grpcserver + grpc_analyzer），interceptor 升级自动让 ai-svc 受益。

**stage-46 §四 B 6/6 步全部完成**。

## 五、未做项

| 项 | 估 | 备注 |
|---|---|---|
| web-bff grpc.Dial 调 ai-svc 时接入 NewClientTracingInterceptor | 1h | web-bff → ai-svc 调用目前无 trace 关联；可单独 PR |
| 业务层 handler 主动打业务 tag | backlog | 取决于各 handler 拿到 span 的便捷性 |
| gRPC stream interceptor（Stage 19 没覆盖）| backlog | 业务路径未用 stream RPC |

## 六、Stage 50 端到端验证 + 问题发现（落地）

[Stage 50](/docs/stages/stage-50-e2e-validation.md) 已完成本轮 5 stage 的端到端验证 + 6 项问题发现：

**验证**：
- 单元测试层：grpcinterceptor 41/41 + middleware 24/24 + logging 10/10 + ai-svc consumer 14/14 全 PASS
- Stage 47 logging helper 本机直接 run 验证：JSON 输出含 svc / trace_id / action / msg / msg_id 6 字段
- dev compose smoke_observability.py：10/12 PASS（2 项 Nacos/warmup 阻塞与本轮无关）

**问题清单**（按严重程度排序）：
1. 🔴 5 svc 镜像滞后于代码改动（阻塞本轮验证完整性，需 16 OBS 分支 merge main + 重建镜像）
2. 🟡 5 svc 因 Nacos ephemeral 注册 500 持续 Restarting（预存问题，与本轮无关）
3. 🟡 promtail volume mount 配置错误（预存问题，与本轮无关）
4. 🟡 Loki Ingester warmup 时序（smoke 加 sleep 即可）
5. 🟢 smoke 未覆盖 svc 日志 svc/trace_id/action 字段断言
6. 🟢 smoke 未覆盖 OAP UI 收到 rpc.* tag 断言

**解锁最短路径**：修问题 4 → 3 → 1 → 2，过程中补问题 5/6 作为护栏。

## 六、与 Stage 44 §四 的对账

| §四 未做项 | 状态 |
|---|---|
| A. 16 分支 merge main | ⏳ 阻塞型 |
| **B. PR-OBS-12/13/14 完整 span tag** | ✅ **6/6 步全部完成（Stage 45+46+47+48+49）** |
| C. PR-OBS-15 6 svc logging 接入 | ✅ Stage 47 |
| D. sw-oap SW_TELEMETRY | ⏳ 15 分钟 |
| E. PR-OBS-4/5 干净环境实跑 | ⏳ 阻塞型 |
| F. Kafka Sprint C Protobuf | ⏳ 独立 Sprint |
| G. Kafka Sprint A §3 历史 SQL | ⏳ 运维窗口 |
| H. Sprint B 范围外 backlog | ⏳ 多 PR |

**业务功能层面：观测链路 Sprint B 6/6 步 100% 收口**。

- HTTP：Stage 46（EntrySpan + 4 tag）+ Stage 48（err 透传）
- gRPC：Stage 49（server/client 5+4 项）+ ai-svc 自动受益
- Kafka consumer：Stage 45（4 个 messaging.* tag）
- 决策 6 字段：Stage 47（svc/trace_id/action Loki 可查）

**剩余非业务功能项**：Nacos ephemeral 修复（阻塞 16 分支 merge）、运维 SQL（DBA 排期）、独立 Sprint（Protobuf / gRPC 化 / 多模态 / 文件上传）。
