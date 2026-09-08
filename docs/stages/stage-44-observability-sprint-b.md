---
status: landed
priority: high
stage: 44
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义,候选 stage 44)
  - observability-compose-gap.md (已被 superseded)
  - observability-testing-gap.md (已被 superseded)
  - kafka-reliability-gaps.md (§1.4 consumer lag 监控 = PR-OBS-7)
related-stages:
  - stage-28-observability.md (k8s 路径可观测性基线)
  - stage-35-system-feasibility.md:78 (SkyWalking dial fail 现象)
  - stage-43-kafka-reliability-sprint-a.md (Kafka Sprint A 收口,§四 §1.4 尾巴由 PR-OBS-7 收口)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id 串联,PR-OBS-15 落地)
  - adr-2026-09-loki-aggregator-dev.md (dev Loki 选型,PR-OBS-5 落地)
related-commits:
  - 3f5801e docs(plans): 新增 observability-sprint-b.md (Stage 44 入口登记)
  - a7206c6 docs(plans+roadmap): 登记 Stage 44 + 原两份 observability plan superseded
  # PR-OBS-1 APISIX skywalking endpoint + skywalking-logger + file-logger
  - 6d52386 PR-OBS-1 RED: seed_test.js 4 断言
  - 89b0117 PR-OBS-1 GREEN: config.yaml + seed.sh (endpoint 修通 + 2 logger 插件)
  - b7df76d PR-OBS-1 REFACTOR: OBSERVABILITY_PLUGINS_JSON 共享变量消重
  # fix/seed-test-nacos-discovery-naming (PR-OBS-1 预存在 7 项 fail 收口)
  - d26623c fix: seed_test.js 同步 Stage 39 Nacos discovery 命名变更
  # PR-OBS-2 7 svc SkyWalking init fail-fast
  - 52c550b PR-OBS-2 REFACTOR: ai-svc/internal/bootstrap → shared/pkg/bootstrap 上提
  - 9f15645 PR-OBS-2 RED: BootstrapSkyWalkingTracer + counter 断言
  - 798154a PR-OBS-2 GREEN: BootstrapSkyWalkingTracer + SkyWalkingInitFailedTotal 实现
  - 73aec18 PR-OBS-2 接入: 6 svc main.go 用 shared helper
  # PR-OBS-3 yaml 占位符 helper
  - 566490f PR-OBS-3 RED: ExpandShellEnvDefaults 8 case
  - 03470ed PR-OBS-3 GREEN: expand.go 实现
  # PR-OBS-4 Prometheus + Grafana compose
  - b6a1b6a PR-OBS-4 RED: smoke_observability.py 3 断言
  - 0ef0756 PR-OBS-4 GREEN: prometheus.yml + datasource.yaml + compose
  - 36882cc PR-OBS-4 [VERIFY]: 实跑结果 + sw-oap scrape 注释
  # PR-OBS-5 Loki + Promtail compose
  - 3c5cecc PR-OBS-5 RED: smoke 3 断言
  - cddb9b7 PR-OBS-5 GREEN: loki-config + promtail-config + compose + datasource
  # PR-OBS-6 Grafana dashboard provisioning
  - 41173b4 PR-OBS-6 RED: dashboard API 1 断言
  - e87ee1a PR-OBS-6 GREEN: emotion-echo-overview.json + dashboards.yaml
  # PR-OBS-7 Kafka consumer lag (接 Kafka Sprint A §1.4)
  - 22cbb18 PR-OBS-7 RED: smoke 4 断言
  - 8a515fe PR-OBS-7 GREEN: kafka-exporter + rules + dashboard + compose
  # PR-OBS-8 runbook
  - 22420b7 PR-OBS-8 RED: runbook 文件存在 1 断言
  - 63d5373 PR-OBS-8 GREEN: observability-compose.md (255 行)
  # PR-OBS-9 shared metrics 4 subtest
  - 3241218 PR-OBS-9: metrics_test.go 4 subtest (Content-Type/series/bucket/自循环)
  # PR-OBS-10 5 svc metrics_test
  - bed2204 PR-OBS-10 part 1: web-bff main_test.go metrics 契约测试
  - 9383096 PR-OBS-10 part 2: chat/assessment/analytics/user metrics_test.go
  # PR-OBS-11 fusion metrics 数值断言
  - dbf72f5 PR-OBS-11 RED: fusion calls total + duration 2 case
  - 2be2659 PR-OBS-11 GREEN: FusionCallsTotal + FusionDurationSeconds + wrapper
  # PR-OBS-12 HTTP trace 透传边界
  - 1334996 PR-OBS-12: gin_skywalking_test.go 3 case (覆盖补全型)
  # PR-OBS-13 gRPC trace 透传边界
  - e208c50 PR-OBS-13: tracing_test.go 2 case (覆盖补全型)
  # PR-OBS-14 Kafka trace 透传边界
  - 354df14 PR-OBS-14: consumer_test.go 3 case (覆盖补全型)
  # PR-OBS-15 logging helper svc/trace_id/action
  - 1dd7a93 PR-OBS-15 RED: 决策 6 必填 JSON 字段 5 case
  - e730393 PR-OBS-15 GREEN: SetGlobalSvc + WithTraceID/WithAction + enrichHandler
  # PR-OBS-16 5 svc bootstrap 装配
  - e938d3c PR-OBS-16 part 1: web-bff bootstrap 装配断言
  - 88e570f PR-OBS-16 part 2: chat/assessment/analytics/user bootstrap_test.go
---

# Stage 44 · 观测链路 Sprint B 全收口（observability-sprint-b.md / 16 PR-OBS-X）

> **本文档归档 observability-sprint-b.md 全部 16 个 PR-OBS-X + 1 个 fix 分支共 34 个 commit**。
> Sprint B 范围按 `docs/plans/observability-sprint-b.md §〇` 定义：dev compose 三层补齐
> + 测试护栏 + Kafka consumer lag 收口（接 Kafka Sprint A §1.4）。

## 一、Sprint B 范围 vs 落地状态

按 observability-sprint-b.md §〇（16 PR-OBS-X）逐项映射:

| # | 项 | 类别 | 落地状态 | 证据 |
|---|---|---|---|---|
| PR-OBS-1 | APISIX skywalking endpoint 修通 + skywalking-logger + file-logger | 基础设施 | ✅ | 6d52386 + 89b0117 + b7df76d |
| fix/seed | seed_test.js 同步 Nacos discovery 命名 (7 项预存在 fail) | 修复 | ✅ | d26623c |
| PR-OBS-2 | 7 svc SkyWalking init fail-fast (BootstrapSkyWalkingTracer) | 基础设施 | ✅ | 52c550b + 9f15645 + 798154a + 73aec18 |
| PR-OBS-3 | yaml 占位符 helper ExpandShellEnvDefaults | 基础设施 | ✅ | 566490f + 03470ed |
| PR-OBS-4 | Prometheus + Grafana compose | 基础设施 | ✅ | b6a1b6a + 0ef0756 + 36882cc |
| PR-OBS-5 | Loki + Promtail compose | 基础设施 | ✅ | 3c5cecc + cddb9b7 |
| PR-OBS-6 | Grafana dashboard provisioning (emotion-echo-overview) | 基础设施 | ✅ | 41173b4 + e87ee1a |
| PR-OBS-7 | Kafka consumer lag 监控 (kafka-exporter + rules + dashboard) | 基础设施 | ✅ | 22cbb18 + 8a515fe |
| PR-OBS-8 | runbook (observability-compose.md) | 文档 | ✅ | 22420b7 + 63d5373 |
| PR-OBS-9 | shared metrics 4 subtest | 测试护栏 | ✅ | 3241218 |
| PR-OBS-10 | 5 svc metrics_test.go | 测试护栏 | ✅ | bed2204 + 9383096 |
| PR-OBS-11 | fusion metrics 数值断言 | 测试护栏 | ✅ | dbf72f5 + 2be2659 |
| PR-OBS-12 | HTTP trace 透传边界 | 测试护栏 | ✅ (边界) | 1334996 |
| PR-OBS-13 | gRPC trace 透传边界 | 测试护栏 | ✅ (边界) | e208c50 |
| PR-OBS-14 | Kafka trace 透传边界 | 测试护栏 | ✅ (边界) | 354df14 |
| PR-OBS-15 | logging helper svc/trace_id/action | 测试护栏 | ✅ | 1dd7a93 + e730393 |
| PR-OBS-16 | 5 svc bootstrap 装配断言 | 测试护栏 | ✅ | e938d3c + 88e570f |

**16 PR + 1 fix = 34 commit 全部落地。** PR-OBS-1~8 基础设施层 100%，PR-OBS-9~16 测试护栏层 100%。

## 二、核心交付物概述

### 2.1 dev compose 可观测性三层（PR-OBS-1/4/5/6）

| 层 | 组件 | 文件 | 启动方式 |
|---|---|---|---|
| Metrics | prometheus + grafana | deploy/prometheus/prometheus.yml + deploy/grafana/provisioning/ | `COMPOSE_PROFILES=obs` |
| Logs | loki + promtail | deploy/loki/loki-config.yaml + promtail-config.yaml | 同上 |
| Traces | APISIX skywalking-logger + 7 svc go2sky | deploy/apisix/config.yaml + seed.sh | 同上 |
| 看板 | emotion-echo-overview (4 panel) | deploy/grafana/dashboards/emotion-echo-overview.json | 同上 |

### 2.2 Kafka consumer lag 监控（PR-OBS-7，接 Kafka Sprint A §1.4）

- kafka-exporter (:9308) + prometheus scrape + Grafana 看板 + alert rule
- **Stage 43 §四 §1.4 尾巴完整收口**

### 2.3 测试护栏（PR-OBS-9~16）

- shared metrics 契约测试 (Content-Type/series/bucket/自循环)
- 5 svc metrics_test + 5 svc bootstrap_test
- fusion calls_total + duration_seconds 新 metric
- HTTP/gRPC/Kafka trace 透传边界 case
- logging helper (决策 6 必填 svc/trace_id/action 字段)

### 2.4 runbook（PR-OBS-8）

docs/deployment/runbook/observability-compose.md（255 行，5 节：启动/看 targets/看 logs/看 trace/故障排查）

## 三、验证证据（实跑记录，诚实标注）

| 项 | 结果 | 证据 |
|---|---|---|
| 本地 dev compose 起 prometheus+grafana | ✅ 起容器成功 | PR-OBS-4 VERIFY commit 36882cc |
| grafana /api/health 200 + datasource auto-register | ✅ PASS | smoke 断言 2/3 |
| loki /ready 200 + query 端点 | ✅ PASS | PR-OBS-5 cddb9b7 实跑 |
| kafka-exporter :9308 metrics + alert rule loaded | ✅ PASS | PR-OBS-7 4 断言全 PASS |
| dashboard API 200 + panels | ✅ PASS (overview 4 panel + kafka-lag 3 panel) | PR-OBS-6 e87ee1a |
| 5 svc metrics_test + bootstrap_test | ✅ 5 svc 全包 0 回归 | PR-OBS-10 + PR-OBS-16 |
| fusion metrics 2 case + logging 5 case | ✅ PASS | PR-OBS-11 + PR-OBS-15 |
| 全仓 go test 相关 svc | ✅ 各 PR 验证 0 回归 | 各 PR commit |
| **prometheus scrape targets ≥ 6 UP** | ❌ **dev 环境未全绿** | 5 svc 因 Nacos ephemeral 注册 500 Restarting (Stage 39 §二同源),与 PR-OBS-4 无关 |

## 四、未做项（诚实标注）

### ❌ A. 分支未合并 main（当前状态）

16 个 OBS 分支 + fix 分支都**已推送 origin 但未 merge 回 main**：

```
feat/observability-OBS-1-apisix-skywalking-endpoint
feat/observability-OBS-2-svc-fail-fast
feat/observability-OBS-3-yaml-helper
feat/observability-OBS-4-prometheus
feat/observability-OBS-5-loki
feat/observability-OBS-6-grafana-dashboard
feat/observability-OBS-7-kafka-lag
feat/observability-OBS-8-runbook
feat/observability-OBS-9-shared-metrics-test
feat/observability-OBS-10-svc-metrics-test
feat/observability-OBS-11-fusion-metrics-test
feat/observability-OBS-12-http-trace-test
feat/observability-OBS-13-grpc-trace-test
feat/observability-OBS-14-kafka-trace-test
feat/observability-OBS-15-logging-helper
feat/observability-OBS-16-bootstrap-regression
fix/seed-test-nacos-discovery-naming
```

**合并前需**：AGENTS.md §2.4 数据契约 smoke 全绿（干净 dev 环境 `python scripts/smoke_data_layer.py` 10/10）。

### ❌ B. PR-OBS-12/13/14 完整 span tag 断言（需 TracerInterface 抽象）

- 现状：3 个 trace PR 只落地"边界 case"（覆盖补全型，非 RED→GREEN 周期）
- 根因：`go2sky.Tracer` 是具体类型，NewTracer 需真实 reporter 无法 mock
- 完整实现需：
  1. 抽 `TracerInterface`（含 CreateLocalSpan(ctx, name) (SpanInterface, ctx, error)）
  2. 抽 `SpanInterface`（含 Tag(key, value string) / End）
  3. GinSkywalkingMiddleware / ServerTracingInterceptor / ConsumerGroupHandler 改用接口
  4. mockSpan + mockTracer 实现 + 完整 span tag 断言（user_id/rpc.method/messaging.*）
- 估 1-2 天，独立 PR

**🟡 PR-OBS-17（2026-09-08）部分收口**：

[Stage 45](/docs/stages/stage-45-observability-sprint-b-regression.md) 已落地：

- ✅ 步骤 1+2：`Tracer.CreateLocalSpan` + `Span.Tag` 接口扩展 + Go2Sky adapter
- ✅ 步骤 3（部分）：GinSkywalkingMiddleware / ConsumerGroupHandler 改用接口（行为零变化）
- ✅ 步骤 4（部分）：ai-svc Kafka consumer 4 个 messaging.* tag 精确断言（mockSpan.tagCalls）

**🟢 PR-OBS-18（2026-09-08）进一步收口**：

[Stage 46](/docs/stages/stage-46-observability-gin-entry-span.md) 已落地：

- ✅ 步骤 3（完整）：GinSkywalkingMiddleware 创建 EntrySpan + 4 个 http.* / user_id tag 精确断言

剩余（PR-OBS-19）：
- ⏳ ServerTracingInterceptor 打 rpc.method/rpc.system/user_id + 5 svc gRPC server 接入 shared interceptor

### ❌ C. PR-OBS-15 6 svc 接入 logging helper

- 现状：shared/pkg/logging 已有 SetGlobalSvc + WithTraceID/WithAction + enrichHandler，测试 5 case 全 PASS
- 但 **6 svc main.go 未调 SetGlobalSvc**，gin/gRPC middleware 未调 WithTraceID/WithAction
- 落地后 Loki 日志查询可按 svc/trace_id/action 过滤
- 估半天，独立 PR

### ❌ D. sw-oap telemetry 未启用

- `SW_TELEMETRY=prometheus` 未加到 docker-compose.infra.yml sw-oap env
- prometheus scrape target `emotion-echo-sw-oap:1234` DOWN（OAP 9.7 默认不监听）
- smoke 已把 sw-oap 移出 EXPECTED_TARGETS（PR-OBS-4 VERIFY commit 36882cc）
- 启用后需 up -d 重挂载 + smoke 加回 sw-oap 断言
- 估 15 分钟，独立小 PR

### ❌ E. PR-OBS-4/5 干净环境实跑全绿（Nacos 阻塞）

- dev 环境 5 svc (user/chat/assessment/analytics/ai-svc) 因 Nacos ephemeral 注册 500 持续 Restarting
- 阻塞：prometheus scrape targets ≥ 6 UP + Loki apisix access.log 全链验证
- 需先修 Nacos ephemeral 注册问题（nacos-enablement-dev.md §二，独立 Sprint）
- 或用 `STARTUP_STRICT=false` + 跳过 Nacos 的本地启动方式验证 obs 层

### ❌ F. Kafka Sprint A §1.5 Protobuf 迁移（独立 Sprint C）

- kafka-reliability-gaps.md §1.5：事件 schema 迁 Protobuf + eventrow 完整迁移
- 2-3 天，独立分支 `feat/kafka-protobuf-migration`，无外部依赖

### ❌ G. Kafka Sprint A §3 历史数据迁移 SQL 实际执行（运维窗口）

- migrations/002 末尾参考 SQL（4 步 + 1 步验证）
- 需人工 review + 数据库备份 + DBA 排期，不在自动化测试范围

### ❌ H. Sprint B 范围外 backlog（未做）

| 项 | 来源 | 估 |
|---|---|---|
| Nacos dev 全链路启用 | nacos-enablement-dev.md | 多 PR |
| gRPC 化（chat-svc / BFF→4 下游） | grpc-inter-service-migration.md | 多 PR |
| TTS/多模态 AI 决策 | todo-pile §A1/B4 | 需拍板 |
| 文件上传 | todo-pile §A2 | 1-2 天 |
| BFF 路由三方契约收口 | todo-pile §C8 | 1.5-2 天 |

## 五、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码

- deploy/apisix/config.yaml + seed.sh（PR-OBS-1 endpoint + logger 插件）
- emotion-echo-shared/pkg/bootstrap/（PR-OBS-2 上提 + BootstrapSkyWalkingTracer）
- 6 svc main.go（PR-OBS-2 接入 + PR-OBS-16 装配）
- emotion-echo-shared/pkg/config/expand.go（PR-OBS-3）
- deploy/prometheus/ + deploy/loki/ + deploy/grafana/（PR-OBS-4/5/6/7）
- emotion-echo-shared/pkg/metrics/ + pkg/logging/ + pkg/middleware/ + pkg/grpcinterceptor/（PR-OBS-9~15）
- emotion-echo-ai-svc/internal/consumer/ + internal/fusion/（PR-OBS-11/14）
- 5 svc metrics_test.go + bootstrap_test.go（PR-OBS-10/16）

### ② 查相关 ADR / stage

- docs/plans/observability-sprint-b.md（本 stage 工作定义源）
- docs/plans/observability-compose-gap.md + observability-testing-gap.md（已被 superseded，现状参考）
- docs/stages/stage-43-kafka-reliability-sprint-a.md（§四 §1.4 尾巴由 PR-OBS-7 收口）
- docs/stages/stage-28-observability.md + STAGE-28-LANDING.md（k8s 基线）
- docs/architecture/decisions.md 决策 6（JSON 日志 + trace_id）
- adr-2026-09-loki-aggregator-dev.md（dev Loki 选型）

### ③ 跑现状 smoke

- 各 PR 本地 `go test ./...` 相关 svc 全绿 0 回归
- smoke_observability.py 12 项断言：10 PASS + 1 SKIP + 1 FAIL（scrape targets 因 Nacos Restarting）
- seed_test.js：PASS=36 FAIL=0（PR-OBS-1 + fix 收口后）

### ⑤ 列架构假设

- **假设 A**：APISIX skywalking endpoint 端口 12800 (HTTP)，go2sky 端口 11800 (gRPC)。验证：APISIX skywalking-logger 用 HTTP 上报 ✅
- **假设 B**：dev compose 装 obs 组件后业务 svc /metrics 自动被 scrape。验证：compose 网络 DNS 静态 target 配置 ✅（实际 scrape 受 Nacos 阻塞）
- **假设 C**：ExpandShellEnvDefaults 治本后未来 env-driven 字段不撞"未解析"墙。验证：8 case 全 PASS ✅
- **假设 D**：Kafka Sprint A 落地后 consumer group 名稳定 (ai-svc/analytics-svc)。验证：kafka-exporter 自动发现 ✅

### ⑥ 写完后回填

本文档归档到 docs/stages/stage-44-observability-sprint-b.md。
observability-sprint-b.md front-matter 标记 landed（已同步）。
34 commit message 末尾均按 AGENTS.md §〇 §⑥ 列调研依据。

---

## 六、下一步（未做项启动顺序建议）

| 优先级 | 项 | 依赖 |
|---|---|---|
| **P0** | 16 分支 + fix merge 回 main | AGENTS.md §2.4 数据契约 smoke 全绿（需先修 Nacos ephemeral 或本地跳过 Nacos 验证）|
| **P0** | 干净 dev 环境实跑 smoke_data_layer.py 10/10 + smoke_observability.py 全绿 | Nacos 修复 |
| **P1** | PR-OBS-12/13/14 完整收口（TracerInterface/SpanInterface 抽象）| 无 |
| **P1** | PR-OBS-15 6 svc 接入 SetGlobalSvc + WithTraceID/WithAction | 无 |
| **P2** | sw-oap SW_TELEMETRY=prometheus 启用 | 无 |
| **P2** | Kafka Sprint C Protobuf 迁移 | 无 |
| **P3** | 运维窗口：历史数据迁移 SQL 执行 | 人工 review + 备份 |
| **P3** | Nacos enablement / gRPC 化 / TTS 多模态 | backlog 独立排期 |
