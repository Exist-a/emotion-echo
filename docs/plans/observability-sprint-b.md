---
status: planned
priority: high
owner: TBD
target-stage: Stage 44（候选）
created: 2026-09-08
depends-on:
  - docs/plans/observability-compose-gap.md（PR-OBS-1/2/3 是其细化执行版）
  - docs/plans/observability-testing-gap.md（PR-OBS-9~16 是其细化执行版）
  - docs/plans/kafka-reliability-gaps.md §1.4 consumer lag 监控（Sprint A 尾巴）
related-stages:
  - stage-28-observability.md（k8s 路径已交付，作为基线）
  - STAGE-28-LANDING.md §三/§七（k8s 已有的 4 scrape job / Loki / Grafana 模板）
  - stage-35-system-feasibility.md:78（SkyWalking dial fail 现象记录）
  - stage-38-system-status.md:46（BFF /metrics 201 series 手动验证）
  - stage-43-kafka-reliability-sprint-a.md（Kafka Sprint A 收口，§四尾巴未做）
related-decisions:
  - adr-2026-09-loki-aggregator-dev.md（dev Loki 选型）
  - decisions.md 决策 6（JSON 日志 + trace_id 串联）
supersedes:
  - docs/plans/observability-compose-gap.md（PR 拆分被本文 PR-OBS-1~7 取代；原文件 §三目标 / §四任务表作为参考保留）
  - docs/plans/observability-testing-gap.md（PR 拆分被本文 PR-OBS-9~16 取代）
---

# Plan — 观测链路 Sprint B（dev compose 三层补齐 + 测试护栏 + Kafka lag 收口）

> **本 plan 把 `observability-compose-gap.md` + `observability-testing-gap.md` + Kafka Sprint A 尾巴
> §1.4 consumer lag 监控 三件事合并成一个可执行 Sprint B**，统一 TDD 节奏、统一分支命名、
> 统一 DoD 验收。原两份 plan 作为参考保留（不再独立推进），本文为**单一执行源**。

## 〇、Sprint B 范围与不范围

### ✅ 在范围

| 类别 | 项 | 来源 |
|---|---|---|
| **基础设施** | dev compose 装 Prometheus + Grafana + Loki + Promtail + SkyWalking trace 真通 | observability-compose-gap.md §三 |
| **配置错误修复** | APISIX `skywalking.endpoint_addr` 修 host + port + 加 skywalking-logger / file-logger 插件 | observability-compose-gap.md §1.3 + §三.3 |
| **代码修复** | 7 个 svc yaml 占位符解析 helper（治本）+ SkyWalking init 失败 fail-fast（治标）| observability-compose-gap.md §四 PR-2 + 本 plan 新增 |
| **测试护栏** | 5 个 svc metrics 契约测试 + trace 透传单元测试 + 日志结构化测试 + 启动期回归守护 | observability-testing-gap.md §三 |
| **Kafka 尾巴** | kafka-exporter + scrape job + consumer lag Grafana 面板 + 告警规则 | kafka-reliability-gaps.md §1.4（Stage 43 §四尾巴）|

### ❌ 不在范围（明确推到 Sprint C 或后续）

- **Protobuf 迁移**（kafka-reliability-gaps.md §1.5）：无外部依赖，独立 Sprint C，2-3 天
- **历史数据迁移 SQL 执行**（Stage 43 §四 §3）：运维窗口 + DBA review，单独排期
- **Nacos dev 全链路启用**（nacos-enablement-dev.md）：与本 Sprint 独立，独立推进
- **gRPC 化**（grpc-inter-service-migration.md）：架构变更，独立 Sprint D
- **业务自定义告警规则**（HighErrorRate / PodOOMKilled）：STAGE-28-LANDING §九 候选，本 Sprint 只交付**基础面板 + 占位告警规则**
- **Alertmanager webhook 真实集成**：Stage 28-D placeholder，本 Sprint 不动

---

## 一、现状（与代码事实对齐 — 2026-09-08 实测）

### 1.1 现状 1：dev compose 三层全缺

`deploy/docker-compose.infra.yml` 现有 312 行服务清单：

| 组件 | 已部署 | 缺 |
|---|---|---|
| postgres / redis / kafka / skywalking-oap / skywalking-ui / nacos / etcd / apisix | ✅ | — |
| prometheus / grafana / loki / alertmanager / promtail / kafka-exporter | ❌ | 6 项缺 |

业务 svc 都启了 `SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800`（apps.yml 6 处：user/chat/assessment/analytics/ai/web-bff），值正确，**不是 plan §1.2 写的"yaml 占位符未解析"**——但**5 个 svc 的 tracer init 静默吞错**：

```go
// chat-svc/main.go:129 + assessment-svc/main.go:77 + analytics-svc/main.go:136 + web-bff/main.go:86 + ai-svc/main.go:237
tracer, _ = go2sky.NewTracer(svcName, go2sky.WithReporter(rep))
// user-svc/main.go:93 唯一记 err，但只 log 不退出
tracer, err = go2sky.NewTracer(svcName, go2sky.WithReporter(rep))
if err != nil { log.Printf("[skywalking] tracer init failed: %v", err) }
```

实测现象（stage-35-system-feasibility.md:78）：dev 启动后 OAP/UI 跑着，6 svc 全连不上 → trace / metrics / log 三层失效。

### 1.2 现状 2：APISIX 配置错误（3 处）

**错误 1**：skywalking endpoint 容器内回环 + 端口错（config.yaml:171）

```yaml
skywalking:
  endpoint_addr: http://127.0.0.1:12800  # ← 错：①容器内 127.0.0.1 ②端口应为 11800（gRPC reporter）还是 12800（HTTP/Log）需区分
```

**错误 2**：seed.sh 主入口路由 plugins 缺 skywalking-logger（seed.sh:265-298）

```bash
# 当前有: jwt-auth / limit-count / limit-req / api-breaker / cors / prometheus
# 缺: skywalking-logger / file-logger
```

**错误 3**：access_log 未挂 volume（config.yaml:290）

```yaml
access_log: logs/access.log  # 容器内路径，无 volume mount，容器销毁即丢
```

### 1.3 现状 3：metrics 中间件已全挂但**没测试护栏**

| svc | GinMetricsMiddleware | /metrics 端点 | metrics_test.go |
|---|---|---|---|
| emotion-echo-shared/pkg/metrics | ✅ 实现 | — | ✅ 4 case（基线）|
| emotion-echo-web-bff | ✅ main.go:94 | ✅ main.go:96+ | ❌ 无 |
| emotion-echo-ai-svc | ✅ main.go:352 | ✅ main.go:375 | ❌ 无（fusion metrics 测试存在）|
| emotion-echo-user-svc | ✅ main.go:127 | ✅ main.go:150 | ❌ 无 |
| emotion-echo-chat-svc | ✅ main.go:196 | ✅ main.go:204 | ❌ 无 |
| emotion-echo-assessment-svc | ✅ main.go:89 | ✅ main.go:96 | ❌ 无 |
| emotion-echo-analytics-svc | ✅ main.go:188 | ✅ main.go:244 | ❌ 无 |

**结论**：5 svc 都暴露了 /metrics 但 0 个测试守护——任何人删了 `r.Use(GinMetricsMiddleware(...))` 都不会被发现（与 PR-4c-4 reset-password 同源教训，决策 18 §二 #22）。

### 1.4 现状 4：trace 中间件已全挂但链路测试缺失

| svc | GinSkywalkingMiddleware | tracer 失败行为 |
|---|---|---|
| shared/pkg/middleware/gin_skywalking.go | ✅ 实现 + 测试 | — |
| web-bff / user / chat / assessment / analytics / ai-svc | ✅ main.go 都挂了 | 静默吞错（5 svc）或 log 不退出（1 svc） |

trace 单元测试覆盖：`shared/pkg/skywalking/{tracing,redis_tracing,gorm_tracing}_test.go` + `shared/pkg/middleware/gin_skywalking_test.go` + `shared/pkg/grpcinterceptor/tracing_test.go` 都覆盖了"创建 span"维度，但**没测**：

- 业务 svc 启动后真的连上了 OAP
- trace_id 跨服务透传（HTTP→BFF→chat-svc）
- 业务操作（如一次 login）的 span 标签含 user_id

### 1.5 现状 5：Kafka Sprint A 尾巴 §1.4（consumer lag）

Stage 43 §四明确："未做原因 = Sprint B 范围，**前置：observability-compose-gap PR-3（Prometheus 安装）落地**"。本 Sprint 把这条并进来。

---

## 二、目标（dev compose "真正可用" + 测试护栏 + Kafka lag 三件事一起验收）

### 2.1 Metrics：dev compose 装 prometheus + scrape

- [ ] `deploy/docker-compose.infra.yml` 加 `prometheus:9090` + `grafana:3000` 服务
- [ ] 配 4 个 scrape job（与 k8s 路径对齐）：
  - `kubernetes-pods` → 简化为静态 target（6 svc + APISIX + skywalking-oap）
  - `apisix` → `emotion-echo-apisix:9091`，metrics_path `/apisix/prometheus/metrics`
  - `skywalking-oap` → `emotion-echo-sw-oap:1234`（需 OAP 启用 SW_TELEMETRY）
  - `prometheus-self` → `localhost:9090`
- [ ] Grafana 加 Prometheus datasource sidecar
- [ ] dev compose 启动后 `curl :9090/targets` ≥ 6/6 UP

### 2.2 Logs：dev compose 装 Loki + Promtail + APISIX file-logger 落盘

依据 `adr-2026-09-loki-aggregator-dev.md`：

- [ ] 加 `loki:3100` + `promtail`（compose 路径用 service 替代 k8s DaemonSet）
- [ ] 镜像版本 / retention / config 模式与 k8s Loki values 严格对齐
- [ ] promtail config 采集 docker container stdout（`docker.sock:ro` 卷）
- [ ] APISIX `seed.sh` 主入口路由 plugins 加 `file-logger` + `access_log` 挂 volume
- [ ] volume mount: `/tmp/apisix-access.log → host ./tmp/apisix-access.log`，由 promtail 推 Loki

### 2.3 Traces：修 3 处错（host / port / 缺失插件 + 7 svc init 失败 fail-fast）

**修 1**：APISIX skywalking endpoint 改容器 DNS + 正确端口

```yaml
# before (config.yaml:171)
endpoint_addr: http://127.0.0.1:12800
# after
endpoint_addr: http://emotion-echo-sw-oap:12800
# 注：12800 = OAP HTTP/Log receiver；11800 = gRPC trace receiver（go2sky 用 11800）
# APISIX skywalking-logger 插件用 HTTP 上报，所以是 12800。
# 业务 svc 用 go2sky + reporter.NewGRPCReporter 用的是 11800（apps.yml 已对）
```

**修 2**：seed.sh 主入口路由 plugins 加 `"skywalking-logger"` + `"file-logger"`

**修 3**：业务 svc SkyWalking init 失败 → fail-fast + 加 metrics 计数器

- 7 svc（user/chat/assessment/analytics/ai-svc/web-bff + llm-service Python）tracer init 失败时：
  - 默认 fail-fast 退出（`bootstrap.ShouldFailFast()` 已有模式，ai-svc 用了）
  - 加 metrics counter `skywalking_init_failed_total{service="..."}` 让 smoke 抓得到
- 提供 `SKY_FAILURE_MODE=fail-fast|warn` env 开关（dev 默认 warn，本地 IDE 调试用；CI/集成测试默认 fail-fast）

### 2.4 测试护栏：5 svc 都加 metrics/trace/日志契约测试

- 5 个 svc 各加 `metrics_test.go`：启动 test server → 请求 /health → 断言 /metrics 返回 ≥ N 个 series + 关键 series 名（`emotion_echo_http_requests_total` / `emotion_echo_http_request_duration_seconds` / `go_goroutines`）+ /metrics Content-Type `text/plain`
- 5 个 svc 各加 `bootstrap_test.go`：启动 svc → 断言 GinSkywalkingMiddleware + GinMetricsMiddleware 都已挂在 router 上（防止后续重构漏挂）
- shared logging helper 统一 JSON 日志格式（含 trace_id 透传），各 svc 加 logging_test.go 断言 ts/level/svc/trace_id/action 字段

### 2.5 Kafka lag 收口

- [ ] compose 加 `kafka-exporter`（`danielqsj/kafka-exporter:latest`，暴露 :9308）
- [ ] Prometheus scrape job `kafka-exporter` 指向 :9308/metrics
- [ ] Grafana 加 consumer lag 面板（3 个 consumer group：ai-svc / analytics-svc + DLQ）
- [ ] Alertmanager 加告警规则：`kafka_consumergroup_lag{consumergroup=~"ai-svc|analytics-svc"} > 10000 for 5m`
- [ ] dead 状态行不进告警（已 dead 是预期，不该告警；alert 只看 pending/lag）

---

## 三、任务拆分（按 TDD 循环，PR-OBS-1 ~ PR-OBS-16）

> **命名约定**：PR-OBS-X（X=1~16），对应原 observability-compose-gap.md + observability-testing-gap.md
> 的 PR 编号做 +10 偏移（避免与原 plan PR 编号混淆）。
>
> **总：16 PR / 估 30-35 commit / 估 12-15 天**

### 3.1 PR-OBS-1 — APISIX skywalking endpoint 修通 + skywalking-logger 插件挂上

**RED 测试**：`deploy/apisix/seed_test.js` 加 4 项断言

1. `endpoint_addr` 含 `emotion-echo-sw-oap`（非 127.0.0.1）
2. `endpoint_addr` 端口 = 12800（HTTP/Log receiver，APISIX skywalking-logger 用）
3. catch-all 主入口路由 plugins 含 `skywalking-logger`
4. catch-all 主入口路由 plugins 含 `file-logger`

**GREEN 改动**：
- `deploy/apisix/config.yaml:171` 改 1 行
- `deploy/apisix/seed.sh:265-298` `PLUGINS_JSON` 与 `CATCHALL_PLUGINS_JSON` 都加 `"skywalking-logger": {"endpoint_addr":"http://emotion-echo-sw-oap:12800"}` + `"file-logger": {"path":"/tmp/apisix-access.log","log_format":"..."}`

**估 commit**：2（test + feat）

### 3.2 PR-OBS-2 — 7 svc SkyWalking init 失败 fail-fast + 计数器 + 启动顺序封装

**RED 测试**：7 svc 各加 `bootstrap_test.go`

- 用 `bootstrap.ShouldFailFast()`（ai-svc 已有）统一 fail-fast 行为
- `shared/pkg/bootstrap` 加 helper：`BootstrapTracer(reporterAddr) (*go2sky.Tracer, error)` 把 5 处重复代码抽出来
- 加 metrics counter `emotion_echo_skywalking_init_failed_total{service}`（shared metrics 包）

**GREEN 改动**：
- 7 svc main.go（user/chat/assessment/analytics/ai/web-bff/llm-service Python）调用 `bootstrap.BootstrapTracer`
- 失败时根据 `SKY_FAILURE_MODE` env 决定：fail-fast → log.Fatal；warn → log + counter
- 加 env `SKY_FAILURE_MODE` 默认 `warn`（dev 友好），CI 用 `fail-fast`

**估 commit**：3（shared helper + 7 svc 改造 — 拆分小 PR：shared 先，6 Go svc 一个，llm-service 一个）

### 3.3 PR-OBS-3 — shared 占位符 helper `ExpandShellEnvDefaults` + 7 svc 接入

**RED 测试**：`shared/pkg/config/expand_test.go` 覆盖 8 case

| 输入 | 期望 |
|---|---|
| `${HOME}/x` HOME=/root | `/root/x` |
| `${HOME:-/tmp}/x` HOME=/root | `/root/x` |
| `${UNSET:-/tmp}/x` 无 UNSET | `/tmp/x` |
| `${UNSET}/x` 无 UNSET | error |
| `$${HOME}` 转义 | 字面 `${HOME}` |
| `${X:-hello world}/y` 多词 default | `hello world/y` |
| `${A}-${B}` 多占位符 | `1-2` |
| `price=$10` 不替换无关 `$` | `price=$10` |

**GREEN 改动**：
- `emotion-echo-shared/pkg/config/expand.go` 新增 `ExpandShellEnvDefaults(s string) (string, error)`
- 7 svc main.go 加载顺序：raw read → expand → yaml parse → ApplyEnvOverrides
- 现状：compose apps.yml 已直接字面注入 `SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800`，**未用占位符**——但其他字段如 `KAFKA_BROKERS: ${KAFKA_BROKERS:-emotion-echo-kafka:9092}` 仍是字面（本 plan §一.1 提到"yaml 占位符"指的就是这类）

**估 commit**：3（test + feat + refactor 7 svc）

### 3.4 PR-OBS-4 — compose 加 prometheus + scrape config + Grafana datasource

**RED 测试**：`scripts/smoke_observability.py` 加 3 断言

- `curl :9090/targets` ≥ 6 个 scrape target UP（apisix + 6 svc + skywalking-oap）
- `curl :3000/api/health` 200
- `curl :3000/api/datasources` 含 `prometheus` 默认 datasource

**GREEN 改动**：
- `deploy/docker-compose.infra.yml` 加 `prometheus` + `grafana` 服务
- 新增 `deploy/prometheus/prometheus.yml`（4 scrape job）
- 新增 `deploy/grafana/provisioning/datasources/datasource.yaml`（自动挂 Prometheus）
- 新增 `deploy/grafana/provisioning/dashboards/dashboards.yaml`（sidecar 占位）

**估 commit**：2（test + feat）

### 3.5 PR-OBS-5 — compose 加 Loki + Promtail + APISIX file-logger 落盘卷

**RED 测试**：`scripts/smoke_observability.py` 加 3 断言

- `curl :3100/ready` 200
- `curl :3100/loki/api/v1/query?query={job="apisix"}` 返非空（触发访问后再查）
- `cat ./tmp/apisix-access.log` 非空（卷挂载生效）

**GREEN 改动**：
- `deploy/docker-compose.infra.yml` 加 `loki` + `promtail` 服务
- 新增 `deploy/loki/loki-config.yaml`（retention 24h + filesystem store）
- 新增 `deploy/promtail/promtail-config.yaml`（docker.sock:ro 卷）
- APISIX access_log 加 volume mount：`./tmp/apisix-access.log:/tmp/apisix-access.log`

**估 commit**：2

### 3.6 PR-OBS-6 — Grafana dashboard provisioning（基础 1 张看板）

**RED 测试**：`scripts/smoke_observability.py` 加 1 断言

- `curl :3000/api/dashboards/uid/emotion-echo-overview` 200

**GREEN 改动**：
- 新增 `deploy/grafana/dashboards/emotion-echo-overview.json`（HTTP 请求速率 / 错误率 / p95 延迟 / Goroutine 数 4 个 panel）

**估 commit**：1

### 3.7 PR-OBS-7 — Kafka consumer lag 监控（kafka-exporter + scrape + 面板 + 告警）

**RED 测试**：`scripts/smoke_observability.py` 加 4 断言

- `curl :9308/metrics` 含 `kafka_consumergroup_lag` series
- Prometheus targets 含 `kafka-exporter` UP
- Grafana 含 `kafka-consumer-lag` 看板
- Alertmanager rules 含 `KafkaConsumerGroupLagHigh` 规则

**GREEN 改动**：
- `deploy/docker-compose.infra.yml` 加 `kafka-exporter`（`danielqsj/kafka-exporter:latest`，env `KAFKA_BROKERS=emotion-echo-kafka:9092`）
- `deploy/prometheus/prometheus.yml` 加 scrape job `kafka-exporter`
- `deploy/grafana/dashboards/kafka-consumer-lag.json`（3 panel：ai-svc / analytics-svc / DLQ 各自的 lag）
- `deploy/prometheus/rules/kafka-lag.yml` Alertmanager rule 文件

**估 commit**：3（test + feat + alertmanager config）

### 3.8 PR-OBS-8 — runbook 文档

**GREEN 改动**：`docs/deployment/runbook/observability-compose.md`（dev 调试手册）

- 启动后第一件事（看 targets / 看 logs / 看 trace 步骤）
- 常见故障排查（trace 不通 / metrics 漏 / Loki 查不到）

**估 commit**：1

### 3.9 PR-OBS-9 — shared metrics 包增强测试

**RED 测试**：`shared/pkg/metrics/metrics_test.go` 加 4 case

- 断言 `http_requests_total` series 存在 + label 完整
- 断言 `http_request_duration_seconds` histogram series 存在
- 断言 `/metrics` 返回 200 + Content-Type `text/plain; version=0.0.4`
- 断言 /metrics 自身不计入（自循环保护）

**GREEN 改动**：纯测试增强

**估 commit**：1

### 3.10 PR-OBS-10 — 5 业务 svc 各加 metrics_test.go

**RED 测试**：5 svc 各加 `metrics_test.go`

- 启动 test server（httptest）
- 请求 /health
- 请求 /metrics
- 断言：① ② ③ 关键 series 名存在 + 数值 ≥ 1

**GREEN 改动**：5 个新测试文件

**估 commit**：2（拆 2 批：web-bff+user 一批，chat+assessment+analytics 一批）

### 3.11 PR-OBS-11 — ai-svc fusion metrics 数值断言

**RED 测试**：`emotion-echo-ai-svc/internal/fusion/fusion_metrics_test.go` 加 2 case

- 触发一次 fusion → 断言 `emotion_fusion_calls_total{modality, result}` 递增
- 断言 `emotion_fusion_duration_seconds` histogram 存在

**GREEN 改动**：纯测试增强

**估 commit**：1

### 3.12 PR-OBS-12 — HTTP trace 透传断言（5 svc + BFF）

**RED 测试**：5 svc + BFF 各加 `gin_skywalking_test.go` 或合并到现有

- mock tracer → 请求带 `X-User-Id: 123` → 断言 span 标签含 `user_id=123` + `http.method` + `http.url` + `http.status_code`

**GREEN 改动**：纯测试增强

**估 commit**：2

### 3.13 PR-OBS-13 — gRPC trace 拦截器断言（ai-svc）

**RED 测试**：`shared/pkg/grpcinterceptor/tracing_test.go` 加 2 case

- mock tracer → gRPC 调用带 metadata `x-user-id` → 断言 span 标签含 `user_id` + `rpc.system` + `rpc.service` + `rpc.method`

**GREEN 改动**：纯测试增强

**估 commit**：1

### 3.14 PR-OBS-14 — Kafka consumer trace 断言（ai-svc + analytics-svc）

**RED 测试**：2 svc 各加 `consumer_trace_test.go`

- mock tracer → 消费一条消息 → 断言 span 标签含 `messaging.system=kafka` + `messaging.kafka.topic` + `messaging.kafka.partition` + `event.type`

**GREEN 改动**：纯测试增强

**估 commit**：2

### 3.15 PR-OBS-15 — 共享 logging helper 统一 JSON + 各 svc logging_test.go

**RED 测试**：

- `shared/pkg/logging/json_test.go` 加 5 case（字段必填、trace_id 透传、level 映射、嵌套字段、时间格式 ISO8601）
- 5 svc + BFF 各加 `logging_test.go`：捕获 stdout → 触发一次业务操作 → 断言 JSON 日志含 ts/level/svc/trace_id/action

**GREEN 改动**：

- `shared/pkg/logging/json.go` 新增 `JSONFormatter`（如果各 svc 格式不统一）
- 5 svc 改用 shared formatter

**风险**：各 svc 现状格式差异大（stage-43 之前用 logx，现在用 slog）。本 PR 不要求一次统一，**只测现状格式**——记录差异，留作 Sprint C/D 统一项

**估 commit**：3（shared + 6 svc 测试）

### 3.16 PR-OBS-16 — 可观测性回归守护（CI 集成 + 启动期断言）

**RED 测试**：

- `scripts/ci_smoke.sh` 加：跑完 `go test ./...` 后断言 `metrics_test.go` 和 `bootstrap_test.go` 都有跑过（不能被 skip）
- 5 svc `bootstrap_test.go` 加：启动 svc → 断言 /metrics 端点 200 + router 挂了 GinMetricsMiddleware + GinSkywalkingMiddleware

**GREEN 改动**：

- 5 svc bootstrap_test.go 新增
- scripts/ci_smoke.sh 加断言

**估 commit**：2

---

## 四、PR 全景与依赖

```
PR-OBS-1 (APISIX trace)         ──→ 不依赖任何 OBS PR
PR-OBS-2 (SkyWalking fail-fast) ──→ 不依赖（独立 PR）
PR-OBS-3 (yaml helper)          ──→ 不依赖
PR-OBS-4 (Prometheus + Grafana) ──→ 不依赖
PR-OBS-5 (Loki + Promtail)      ──→ 依赖 PR-OBS-1（file-logger）
PR-OBS-6 (Grafana 看板)         ──→ 依赖 PR-OBS-4
PR-OBS-7 (Kafka lag)            ──→ 依赖 PR-OBS-4（Prometheus 装好）
PR-OBS-8 (runbook)              ──→ 依赖 PR-OBS-1/4/5/7
PR-OBS-9~16 (测试护栏)           ──→ 互相独立
```

**推荐执行顺序**（按依赖 + 价值）：

1. **第一批（基础）**：PR-OBS-1（APISIX trace） + PR-OBS-2（fail-fast） + PR-OBS-3（yaml helper） + PR-OBS-9（shared metrics 测试）
2. **第二批（infra）**：PR-OBS-4（Prometheus） + PR-OBS-5（Loki） + PR-OBS-6（Grafana 看板） + PR-OBS-7（Kafka lag）
3. **第三批（测试）**：PR-OBS-10~16（5 svc 测试 + CI）
4. **收口**：PR-OBS-8（runbook）

---

## 五、DoD（Done Definition — Sprint B 验收清单）

### 5.1 基础设施全绿

| # | 项 | 验证方式 |
|---|----|---------|
| 1 | dev compose 启动后 `curl :9090/targets` ≥ 6 个 scrape target UP | smoke 脚本断言 |
| 2 | dev compose 启动后 `curl :3000` Grafana 可登录 + Prometheus datasource 自动挂上 | smoke |
| 3 | dev compose 启动后 `curl :3100/ready` Loki ready | smoke |
| 4 | 浏览一次 `/api/v1/conversations`，Loki 5s 内查到 access.log 行 | e2e |
| 5 | 浏览器请求 /api/v1/* 后，SkyWalking UI 服务拓扑出现 web-bff → chat-svc 边 | 手动截图 |
| 6 | Kafka consumer lag 面板能显示 ai-svc / analytics-svc / DLQ 三个 group 的当前 lag | 手动截图 |
| 7 | dev 启动后所有 svc 日志有 trace_id 字段（JSON 格式）| smoke 解析一行日志 |
| 8 | OAP 内存模式跑 5 分钟不 OOM（dev compose 默认 SW_STORAGE=h2）| 手动观察 |

### 5.2 测试护栏全绿

| # | 项 | 验证方式 |
|---|----|---------|
| 1 | `go test ./...` 7 svc 全绿，含 5 svc metrics_test.go + bootstrap_test.go | 脚本 |
| 2 | 任一 svc 删 `r.Use(GinMetricsMiddleware(...))` → 对应 metrics_test.go 红 | 验证（手动破坏一次再 revert）|
| 3 | 任一 svc 删 `r.Use(GinSkywalkingMiddleware(tracer))` → 对应 bootstrap_test.go 红 | 同上 |
| 4 | 任一 svc 日志格式字段名漂移 → 对应 logging_test.go 红 | 同上 |
| 5 | `bash scripts/smoke_observability.py` 全绿 | 脚本 |

### 5.3 Kafka lag 收口

| # | 项 | 验证方式 |
|---|----|---------|
| 1 | `curl :9308/metrics` 含 `kafka_consumergroup_lag` series | smoke |
| 2 | Prometheus 含 `kafka-exporter` scrape target UP | smoke |
| 3 | Grafana 看板能查 ai-svc / analytics-svc / DLQ lag | 手动 |
| 4 | 模拟一次 consumer 停摆 → 5min 后 Alertmanager 触发 `KafkaConsumerGroupLagHigh` | 手动（可选）|

### 5.4 文档全到位

| # | 项 |
|---|---|
| 1 | `docs/deployment/runbook/observability-compose.md` 存在且覆盖：启动 / 看 metrics / 看 logs / 看 trace / 故障排查 |
| 2 | `docs/architecture/decisions.md` 决策 6（JSON 日志）补"dev compose 已实施"批注 |
| 3 | `docs/stages/stage-XX-observability-sprint-b.md` 收口归档（落地后建）|

---

## 六、风险与缓解

| 风险 | 缓解 |
|------|------|
| Promtail 需要 docker.sock 权限 | dev compose `volumes: - /var/run/docker.sock:/var/run/docker.sock:ro`；prod 走 k8s 已有方案 |
| Grafana provisioning 重启会丢临时面板 | 用 `dashboards sidecar` ConfigMap 模式（与 Stage28-B 一致）|
| APISIX file-logger 写性能 | dev 默认够用；高频访问 prod 前置 nginx access_log |
| SkyWalking OAP 9.7 默认 1.5GB；dev compose 已用 `SW_STORAGE=h2` 内存模式 | 已确认，不变 |
| 三层全加后 dev compose 资源翻倍 | 默认 profile 不启动 AI 模型（fer/sensevoice/xtts）；observability 单独 profile 化（PR-OBS-8 runbook 注明）|
| `ExpandShellEnvDefaults` 引入新 helper 影响 7 svc 启动路径 | 单元测试覆盖（PR-OBS-3）+ 加载顺序封装 helper |
| `${VAR}` 未定义时 fail-fast 阻断本地 IDE 调试（无 env 注入）| helper 对未定义且无 default 静默保留字面 `${VAR}` 作为占位，仅对**显式带 default** 的占位符 fail-fast |
| `tracer init` fail-fast 阻断 CI（SkyWalking OAP 没起就退）| `SKY_FAILURE_MODE` env 控制：dev `warn`，CI `fail-fast` |
| 7 svc 同步改 fail-fast 行为回归风险 | PR-OBS-2 拆小 PR（shared helper 先，6 Go svc 一个 commit，llm-service 一个 commit），每步单独 `go test` |
| 各 svc 日志格式差异大，PR-OBS-15 不强求统一 | 只测现状格式，记录差异，**不强改**——避免本 Sprint 范围爆炸 |

---

## 七、调研依据（AGENTS.md §〇）

### ① 读相关代码

- `emotion-echo-shared/pkg/metrics/{metrics,fusion_metrics}.go` + `metrics_test.go` — metrics 中间件实现 + 现有测试
- `emotion-echo-shared/pkg/middleware/gin_skywalking.go` + `_test.go` — HTTP trace 中间件
- `emotion-echo-shared/pkg/skywalking/{tracing,skywalking,gorm_tracing,redis_tracing}.go` + `_test.go` — 外部组件 trace 钩子
- `emotion-echo-shared/pkg/grpcinterceptor/tracing*.go` + `_test.go` — gRPC trace 拦截器
- `emotion-echo-shared/pkg/logging/` — slog JSON 下沉
- 6 svc `main.go`（user/chat/assessment/analytics/ai/web-bff）— 验证 metrics + skywalking 都挂了
- `deploy/apisix/config.yaml:95-300` — APISIX 全配置
- `deploy/apisix/seed.sh:1-300` — 插件链与 routes
- `deploy/docker-compose.infra.yml:1-312` — infra 服务清单
- `deploy/docker-compose.apps.yml:91,145,203,255,377,597` — 业务 svc SkyWalking env 注入
- `emotion-echo-ai-svc/internal/bootstrap/deps.go` — fail-fast helper 现状（ai-svc 唯一用了）
- `emotion-echo-ai-svc/main.go:229-353` — ai-svc skywalking + fail-fast 参考实现

### ② 查相关 ADR / stage

- `docs/plans/observability-compose-gap.md` — 本 plan 前身（PR 拆分被本文取代）
- `docs/plans/observability-testing-gap.md` — 本 plan 前身（PR 拆分被本文取代）
- `docs/stages/stage-28-observability.md` + `STAGE-28-LANDING.md` §三/§七 — k8s 路径已交付
- `docs/stages/stage-35-system-feasibility.md:78` — dev SkyWalking dial fail 现象
- `docs/stages/stage-38-system-status.md:46` — BFF /metrics 201 series 手动验证
- `docs/stages/stage-43-kafka-reliability-sprint-a.md` §四 §1.4 — Kafka Sprint A 尾巴
- `docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md` — dev Loki 选型
- `docs/architecture/decisions.md` 决策 6 — JSON 日志 + trace_id 串联

### ③ 跑现状 smoke

- 7 svc `go test ./...` 全绿（Stage 43 收口后 0 FAIL）
- 当前分支 `feat/chat-dev-event-publisher-a1-1-red` 工作树干净
- dev compose 当前缺 prometheus/grafana/loki/kafka-exporter 5 个组件（infra.yml grep 确认）

### ④ 网上信息

- APISIX `skywalking-logger` 插件配置格式：https://apisix.apache.org/docs/apisix/plugins/skywalking-logger/
- SkyWalking OAP 端口：11800 (gRPC receiver) / 12800 (HTTP/Log receiver) — 官方文档
- `danielqsj/kafka-exporter` 配置：https://github.com/danielqsj/kafka_exporter
- go2sky `reporter.NewGRPCReporter(addr)` — 用 11800；`skywalking-logger` APISIX 插件用 HTTP 上报 12800

### ⑤ 列架构假设

- **假设 A**：APISIX skywalking endpoint 端口 = 12800（HTTP），go2sky 端口 = 11800（gRPC）。
  - **验证**：依据 SkyWalking 官方文档端口表 — 已写入 §2.3 修 1。
- **假设 B**：dev compose 装 prometheus/grafana/loki 后，业务 svc /metrics 自动被 scrape（因为 scrape job 用 compose 网络 DNS）
  - **验证**：compose 网络 DNS 与 k8s Pod DNS 等价，scrape target 静态配置即可
- **假设 C**：`ExpandShellEnvDefaults` 治本后，未来新增 env-driven 字段不会撞"未解析"墙
  - **验证**：PR-OBS-3 落地后所有 svc 走 `raw → expand → parse → overrides` 统一加载顺序
- **假设 D**：5 svc metrics 中间件挂的位置都是 router 全局（不是局部 group），删任意一处 → bootstrap_test.go 全红
  - **验证**：grep 5 svc main.go 确认（§一.3 表）
- **假设 E**：Kafka Sprint A 落地后 consumer group 名稳定（`ai-svc` / `analytics-svc`），kafka-exporter 自动发现
  - **验证**：grep `sarama.NewConfig` + `ConsumerGroup` 在两 svc 找 group 名

### ⑥ 写完后回填

- 本文档归档到 `docs/plans/observability-sprint-b.md`
- 落地 stage（Stage 44 候选）归档到 `docs/stages/stage-44-observability-sprint-b.md`
- 各 PR commit message 末尾列调研依据（按 AGENTS.md §〇 §⑥）

---

## 八、分支命名与执行计划

```
main
└── feat/observability-sprint-b          # 主分支（包含 PR-OBS-1 ~ 16）
    ├── feat/observability-OBS-1-apisix-skywalking-endpoint
    ├── feat/observability-OBS-2-svc-fail-fast
    ├── feat/observability-OBS-3-yaml-helper
    ├── feat/observability-OBS-4-prometheus
    ├── feat/observability-OBS-5-loki
    ├── feat/observability-OBS-6-grafana-dashboard
    ├── feat/observability-OBS-7-kafka-lag
    ├── feat/observability-OBS-8-runbook
    ├── feat/observability-OBS-9-shared-metrics-test
    ├── feat/observability-OBS-10-5svc-metrics-test
    ├── feat/observability-OBS-11-fusion-metrics-test
    ├── feat/observability-OBS-12-http-trace-test
    ├── feat/observability-OBS-13-grpc-trace-test
    ├── feat/observability-OBS-14-kafka-trace-test
    ├── feat/observability-OBS-15-logging-test
    └── feat/observability-OBS-16-bootstrap-regression
```

**Sprint B 启动流程**（按本 plan 执行）：

1. 建分支 `feat/observability-sprint-b`（基于 `feat/chat-dev-event-publisher-a1-1-red` 或 main，按合并状态）
2. 按 §四 推荐顺序（第一批/第二批/第三批/收口）逐 PR 提交
3. 每 PR 都 RED → GREEN → REFACTOR 严格 TDD 循环
4. 全 PR 落地后建 `docs/stages/stage-44-observability-sprint-b.md` 收口归档
5. 同步 `docs/architecture/roadmap.md` 登记 Sprint B 入口

---

## 九、用户决策点（需拍板）

| # | 决策 | 备选 | 推荐 |
|---|------|------|------|
| 1 | `SKY_FAILURE_MODE` 默认值 | warn（dev 友好）vs fail-fast（CI 友好） | **warn**（dev 默认）+ CI 用 fail-fast |
| 2 | Kafka lag 告警阈值 | 10000（plan 默认）vs 5000 vs 自适应 | **10000**（与 kafka-reliability-gaps.md §3.4 对齐）|
| 3 | Loki retention | 24h vs 7d vs 30d | **24h**（dev 单机，避免磁盘爆；prod 走 ADR-2 候选 S3）|
| 4 | Prometheus retention | 7d vs 15d vs 30d | **7d**（dev 默认）|
| 5 | observability 是否独立 compose profile | 不分（dev 启动就装）vs 独立 profile `obs` | **独立 profile**（`COMPOSE_PROFILES=obs` 启用，与 ai profile 同模式）|
| 6 | Grafana admin 默认密码 | admin/admin（dev 默认）vs env 强制改 | **admin/admin dev 默认**，compose 文件注释明确 prod 必须改 |

---

## 十、参考资料

- [SkyWalking OAP 端口与配置](https://skywalking.apache.org/docs/main/next/en/setup/backend/backend-deployment/)
- [APISIX skywalking-logger plugin](https://apisix.apache.org/docs/apisix/plugins/skywalking-logger/)
- [APISIX file-logger plugin](https://apisix.apache.org/docs/apisix/plugins/file-logger/)
- [kafka_exporter 文档](https://github.com/danielqsj/kafka_exporter)
- [Promtail 配置](https://grafana.com/docs/loki/latest/clients/promtail/configuration/)
- [Loki 单机模式](https://grafana.com/docs/loki/latest/operations/storage/filesystem/)
- [AGENTS.md §〇](/AGENTS.md)（本 plan 的 TDD 与调研约定）
- [Stage 43 Kafka 收口 §四](/docs/stages/stage-43-kafka-reliability-sprint-a.md)（本 plan 的 §1.4 来源）
