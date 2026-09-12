---
status: superseded
superseded-by: docs/plans/observability-sprint-b.md（PR 拆分被 PR-OBS-9~16 取代；本文档 §一/§二/§三保留作为参考）
priority: medium
owner: TBD
created: 2026-09-07
superseded-at: 2026-09-08
depends-on:
  - docs/plans/observability-compose-gap.md（基础设施先装好，e2e 测试才有目标可测）
related-stages:
  - stage-28-observability.md
  - STAGE-28-LANDING.md
  - stage-38-system-status.md
related-adrs:
  - adr-2026-09-loki-aggregator-dev.md（dev Loki 选型）
  - docs/architecture/decisions.md（决策 6：审计 = 白盒化 + JSON 日志 + trace_id 串联）
---

# Plan — 可观测性链路测试完善

## 一、与 observability-compose-gap.md 的关系（必读）

本计划与 [`observability-compose-gap.md`](observability-compose-gap.md) 是**互补关系**，不重复：

| 计划 | 范围 | 做什么 |
|---|---|---|
| `observability-compose-gap.md` | **基础设施层** | 把 prometheus / grafana / loki / skywalking 装上、配好、修通（dev compose 三层全缺 → 全可用）|
| **本计划** | **测试层** | 装好了之后，用自动化测试保证 trace / metrics / logs 真的工作、数据正确、且不被后续改动意外破坏 |

**依赖**：本计划的 e2e 级测试（§三.1 trace 链路 e2e、§三.4 Loki 查询断言）依赖 observability-compose-gap.md 先落地；但单元测试级别的 metrics 契约测试、日志结构化测试不依赖基础设施，可先行。

---

## 二、现状（与代码事实对齐）

### 2.1 已有可观测性组件

- **Metrics**：6 个业务 svc + BFF 都挂了 `shared/pkg/metrics` 的 `GinMetricsMiddleware` + `PromHTTPHandler`，`/metrics` 端点已暴露；ai-svc 有额外 4 个 fusion collector（`fusion_metrics.go`）；BFF 手动验证有 201 series（`stage-38-system-status.md:46`）
- **Traces**：SkyWalking go2sky 已接入 HTTP 中间件（`shared/pkg/middleware/gin_skywalking.go`）+ gRPC 拦截器（`shared/pkg/grpcinterceptor/tracing.go`）+ Redis/GORM 钩子；ai-svc Kafka consumer 每条消息建 span
- **Logs**：决策 6 要求 JSON 结构化日志（ts/level/svc/trace_id/user_id/action），各 svc 用 go-zero logx 或自封装 logging（ai-svc `internal/logging`）

### 2.2 已有测试覆盖（单元级，非链路级）

| 维度 | 已有测试 | 证据 |
|---|---|---|
| Metrics 中间件本身 | `shared/pkg/metrics/metrics_test.go` | ✅ |
| ai-svc fusion metrics | `ai-svc/internal/fusion/fusion_metrics_test.go` | ✅ |
| BFF main（含 /metrics）| `web-bff/main_test.go` | ✅（可能覆盖，需确认断言 series 数量）|
| Trace 中间件本身 | `shared/pkg/skywalking/tracing_test.go` + `grpcinterceptor/tracing_test.go` + `middleware/gin_skywalking_test.go` | ✅（只测拦截器创建 span，不测跨服务链路）|
| Redis/GORM trace 钩子 | `shared/pkg/skywalking/redis_tracing_test.go` + `gorm_tracing_test.go` | ✅ |
| **其他 svc metrics 契约** | ❌ 无 | user/chat/assessment/analytics 无 metrics_test.go |
| **trace 链路 e2e** | ❌ 无 | 只有手动截图验证（observability-compose-gap.md §3.4 第 5 条），无自动化 |
| **日志结构化** | ❌ 无 | 无 JSON 日志字段完整性测试 |
| **可观测性回归** | ❌ 无 | 无测试防止 trace/metrics 中间件被意外移除 |

### 2.3 核心问题

可观测性组件**装上了但没有测试兜底**，导致：

1. **不知道真的工作**：SkyWalking trace 跨服务是否连续？metrics 关键 series 是否存在？Loki 是否能查到日志？——全靠手动截图，无自动化断言
2. **容易回归**：某次重构删了中间件、改了 metrics 标签、换了日志格式——没有测试会失败，问题到生产才发现
3. **数据正确性无验证**：metrics 数值对不对（如 request_count 递增）、trace span 标签全不全（trace_id/user_id 是否透传）、日志字段齐不齐——无测试

---

## 三、测试完善范围（按维度）

### 3.1 Metrics 契约测试（单元级，不依赖基础设施）

**目标**：每个 svc 启动后 `/metrics` 端点暴露的关键 series 存在且数值合理，防止中间件被移除或 series 名漂移。

**任务**：

| PR | 范围 | 测试内容 |
|---|---|---|
| **PR-1** | shared metrics 包增强 | `metrics_test.go` 加：`GinMetricsMiddleware` 挂后请求一次 → 断言 `http_requests_total` series 存在 + label（method/path/status）正确；`PromHTTPHandler` 返回 200 + Content-Type text/plain |
| **PR-2** | 5 个业务 svc 各加 metrics_test.go | user/chat/assessment/analytics/ai-svc 各加：启动 test server → 请求 /health → 断言 /metrics 返回 ≥ N 个 series（N 按 svc 现状定，如 BFF=201）+ 关键 series 名存在（`http_requests_total` / `http_request_duration_seconds` / `go_goroutines`）|
| **PR-3** | ai-svc fusion metrics 数值断言 | `fusion_metrics_test.go` 加：触发一次 fusion → 断言 `emotion_fusion_calls_total` 递增 + label（modality/result）正确 |
| **PR-4** | metrics 回归守护 | 加 `metrics_contract_test.go`（shared 包）：断言 `GinMetricsMiddleware` 是 `gin.HandlerFunc` 类型 + 必须挂在 router 上（防止后续重构时漏挂）|

**验收**：`go test ./...` 包含 metrics 契约测试；任一 svc 删了 metrics 中间件 → 对应测试红。

### 3.2 Trace 链路测试（分两级）

#### 3.2.1 单元级：trace 透传断言（不依赖 SkyWalking OAP）

**目标**：验证 HTTP/gRPC 调用链中 trace_id / user_id / 操作名正确透传，span 标签完整。

**任务**：

| PR | 范围 | 测试内容 |
|---|---|---|
| **PR-5** | HTTP 中间件 trace 标签断言 | `gin_skywalking_test.go` 加：mock tracer → 请求带 `X-User-Id: 123` → 断言创建的 span 标签含 `user_id=123` + `http.method` + `http.url` + `http.status_code` |
| **PR-6** | gRPC 拦截器 trace 标签断言 | `grpcinterceptor/tracing_test.go` 加：mock tracer → gRPC 调用带 metadata `x-user-id` → 断言 span 标签含 `user_id` + `rpc.system` + `rpc.service` + `rpc.method` |
| **PR-7** | Kafka consumer trace 断言 | ai-svc `consumer_test.go` 加：mock tracer → 消费一条消息 → 断言 span 标签含 `messaging.system=kafka` + `messaging.kafka.topic` + `messaging.kafka.partition` + `event.type` |
| **PR-8** | trace_id 跨层透传 | 集成测试（testcontainers 不需要，用 in-memory）：HTTP 请求 → BFF → gRPC 调下游 → 断言下游 handler 的 ctx 里 trace_id 与入口请求一致（用 mock tracer 捕获 span parent-child 关系）|

#### 3.2.2 e2e 级：SkyWalking 真实链路断言（依赖 observability-compose-gap.md）

**目标**：dev compose 全栈启动后，真实请求经 APISIX→BFF→svc，断言 SkyWalking OAP 能查到完整 span 链。

**任务**：

| PR | 范围 | 测试内容 |
|---|---|---|
| **PR-9** | SkyWalking e2e 测试脚本 | `scripts/test_trace_e2e.sh`：启动 dev compose → 登录获取 token → 调 `/api/v1/conversations` → 等 5s → 调 SkyWalking OAP GraphQL API 查询 trace → 断言：① trace 存在 ② span 数 ≥ 3（APISIX + BFF + chat-svc）③ span 间 parent-child 关系连续 ④ 每个 span 标签含 `user_id` |
| **PR-10** | trace 拓扑断言 | 同上脚本加：查 SkyWalking 拓扑图 → 断言存在 `emotion-echo-web-bff → emotion-echo-chat-svc` 边 |

**验收**：`bash scripts/test_trace_e2e.sh` 全绿；任一服务移除 trace 中间件 → 脚本断言 span 数不足而红。

### 3.3 日志结构化测试（单元级）

**目标**：验证各 svc 输出的 JSON 日志包含决策 6 要求的必填字段（ts/level/svc/trace_id/user_id/action），防止日志格式漂移。

**任务**：

| PR | 范围 | 测试内容 |
|---|---|---|
| **PR-11** | shared 日志结构化 helper + 测试 | 如果各 svc 日志格式不统一，先在 `shared/pkg/logging/` 加 `JSONLogFormatter` + 字段校验 helper；`logging_test.go` 断言：格式化后的日志可被 `encoding/json` 解析 + 含必填字段 + trace_id 从 ctx 透传 |
| **PR-12** | 各 svc 日志字段断言 | user/chat/assessment/analytics/ai-svc/BFF 各加 `logging_test.go`：捕获 stdout → 触发一次业务操作 → 断言输出的 JSON 日志含 `ts` / `level` / `svc` / `trace_id` / `action` 字段（user_id 可选，仅在有用户上下文时）|
| **PR-13** | 日志级别断言 | 加：error 级别日志含 `error` 字段 + stack trace（如配置）；debug 级别在 prod 配置下不输出 |

**验收**：`go test ./...` 包含日志结构化测试；任一 svc 改了日志字段名或去掉 trace_id → 测试红。

### 3.4 可观测性回归守护（集成级）

**目标**：防止后续重构意外移除可观测性中间件/拦截器/钩子。

**任务**：

| PR | 范围 | 测试内容 |
|---|---|---|
| **PR-14** | svc 启动可观测性断言 | 各 svc `main_test.go` 或 `bootstrap_test.go` 加：启动 svc → 断言 ① `/metrics` 端点 200 ② HTTP router 挂了 `GinMetricsMiddleware` + `GinSkyWalkingMiddleware`（通过 router 内部断言或 test server 请求后查 metrics）③ gRPC server（如有）挂了 tracing + userid 拦截器 |
| **PR-15** | 配置可观测性开关断言 | 加：`SKYWALKING_OAP_ADDR` 为空时 svc 启动不 crash（降级）+ 日志 warn；`KAFKA_ENABLED=false` 时 consumer 不启动但 HTTP server 正常 |
| **PR-16** | CI 集成 | `.github/workflows/` 或 `scripts/ci.sh` 加：`go test ./...` 必须包含可观测性测试标签；e2e trace 脚本作为 nightly job（不阻塞 PR，因为需要全栈启动）|

---

## 四、优先级与建议顺序

| 优先级 | 项 | 依赖 | 工作量 |
|---|---|---|---|
| **P0** | PR-1/2 metrics 契约测试（5 svc） | 无 | 1 天 |
| **P0** | PR-5/6/7 trace 透传单元测试 | 无 | 1 天 |
| **P1** | PR-11/12 日志结构化测试 | 无（可能需先统一日志 helper）| 1-2 天 |
| **P1** | PR-14/15 可观测性回归守护 | PR-1/2/5/6 | 0.5 天 |
| **P2** | PR-3/4 fusion metrics + metrics 回归 | PR-1 | 0.5 天 |
| **P2** | PR-8 trace_id 跨层透传集成测试 | PR-5/6 | 0.5 天 |
| **P3** | PR-9/10 SkyWalking e2e 脚本 | observability-compose-gap.md 全落地 | 1 天 |
| **P3** | PR-13/16 日志级别 + CI 集成 | PR-11/12 | 0.5 天 |

**建议顺序**：先做不依赖基础设施的 P0（metrics 契约 + trace 透传单元测试），再 P1（日志结构化 + 回归守护），最后等 observability-compose-gap.md 落地后做 P3 e2e。

---

## 五、风险与缓解

| 风险 | 缓解 |
|---|---|
| metrics series 数量断言太脆弱（加个中间件 series 数就变） | 断言关键 series 名存在而非总数；总数设下限（≥ N）而非精确等于；N 取当前值的 80% |
| trace 单元测试 mock tracer 复杂 | 复用 `shared/pkg/skywalking` 已有 mock；测试只断言 span 被创建 + 标签正确，不断言 SkyWalking 内部行为 |
| 日志结构化测试各 svc 格式不统一 | PR-11 先抽 shared logging helper 统一格式；如果当前格式差异太大，先记录现状再逐步对齐，不要求一次统一 |
| e2e trace 测试需要全栈启动，慢且不稳定 | 作为 nightly job 不阻塞 PR；脚本加重试（SkyWalking 上报有延迟）+ 超时保护；失败时输出 OAP 查询结果便于排查 |
| 测试本身成为维护负担 | 只测关键路径（health + 一次业务请求），不测所有端点；测试放在 shared 包减少重复 |
| SkyWalking OAP GraphQL API 版本差异 | 脚本用 OAP 官方 REST API（`/api/v2/segment`）而非 GraphQL，兼容性更好 |

---

## 六、不在本计划范围

- 可观测性基础设施安装/配置（prometheus/grafana/loki/skywalking）→ 见 `observability-compose-gap.md`
- 业务自定义告警规则（HighErrorRate / PodOOMKilled）→ STAGE-28-LANDING §九 候选
- Alertmanager webhook 真实集成 → Stage 28-D placeholder
- 跨服务 trace sampling 策略 → 独立 ADR
- consumer lag 监控面板 → 见 `kafka-reliability-gaps.md` §3.4
- DLQ 积压告警 → 见 `kafka-reliability-gaps.md` §3.3

---

## 七、调研依据

> 满足 AGENTS.md §〇 "写文档前先调研"

| 文件 | 调研内容 |
|---|---|
| `emotion-echo-shared/pkg/metrics/metrics.go` + `metrics_test.go` | metrics 中间件实现 + 现有测试 |
| `emotion-echo-shared/pkg/metrics/fusion_metrics.go` + `fusion_metrics_test.go` | ai-svc fusion metrics |
| `emotion-echo-shared/pkg/skywalking/tracing_test.go` + `skywalking_test.go` | skywalking 单元测试现状 |
| `emotion-echo-shared/pkg/skywalking/redis_tracing_test.go` + `gorm_tracing_test.go` | Redis/GORM trace 钩子测试 |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing_test.go` + `tracing_go2sky_test.go` | gRPC trace 拦截器测试 |
| `emotion-echo-shared/pkg/middleware/gin_skywalking_test.go` | HTTP trace 中间件测试 |
| `emotion-echo-web-bff/main_test.go` | BFF 主测试（含 /metrics 可能覆盖）|
| `emotion-echo-ai-svc/internal/consumer/consumer.go:106-116` | Kafka consumer SkyWalking span 创建（标签 messaging.system/topic/partition/event.type）|
| `docs/plans/observability-compose-gap.md` | 基础设施补齐计划（本计划的前置依赖）|
| `docs/stages/stage-38-system-status.md:46` | BFF /metrics 201 series 手动验证记录 |
| `docs/stages/STAGE-28-LANDING.md §三/§七` | k8s 路径可观测性已交付内容 |
| `docs/architecture/decisions.md` 决策 6 | JSON 日志 + trace_id 串联的架构决策 |
| `docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md` | dev Loki 选型 |
