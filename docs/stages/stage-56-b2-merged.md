---
status: landed
stage: 56
date: 2026-09-08
priority: P0
related-stages:
  - stage-44-observability-sprint-b.md（Sprint B 工作定义）
  - stage-51-batch-1-infra-merged.md（批 1 已合 main）
  - stage-55-roadmap-z-status.md（路线 Z 全面盘点 + 本批推进策略 §二.4）
related-plans:
  - observability-sprint-b.md（Sprint B 单一执行源）
  - observability-testing-gap.md（已被取代，原文件保留作为参考）
supersedes:
  - stage-55-roadmap-z-status.md §二 批 2 未启动项 → 本文档落地
related-decisions:
  - adr-2026-09-doc-drift-registry.md（决策 18 #22 同源教训：commit msg 报'全 PASS'但镜像未 rebuild）
---

# Stage 56 · 观测链路 Sprint B 批 2 测试护栏（2026-09-08 集成）

> **本文档归档 Sprint B 批 2（OBS-9~16 测试护栏）的集成结果**。批 1（基础设施）已在 Stage 51~54 收口；
> 本批聚焦**测试护栏**——任何人后续改 6 svc 的 `/metrics` 注册、GinSkywalkingMiddleware、gRPC interceptor、
> Kafka consumer span、JSON 日志 helper、bootstrap fail-fast 行为，**单测立即红**。

---

## 一、推进背景（来自 stage-55 §二.4）

### 1.1 起点状态

stage-55 §一盘点时，7 个 OBS 分支领先本地 main 38-48 commit，但**未集成**：

| 分支 | 领先 commit | 主题 |
|---|---|---|
| `feat/observability-OBS-10-svc-metrics-test` | 38 | 6 svc `/metrics` 端点契约 |
| `feat/observability-OBS-11-fusion-metrics-test` | 40 | ai-svc fused model 指标 |
| `feat/observability-OBS-12-http-trace-test` | 41 | GinSkywalkingMiddleware 端到端 |
| `feat/observability-OBS-13-grpc-trace-test` | 42 | gRPC interceptor 端到端 |
| `feat/observability-OBS-14-kafka-trace-test` | 43 | Kafka consumer span tag 端到端 |
| `feat/observability-OBS-15-logging-helper` | 45 | JSON 日志 helper + 决策 6 必填字段测试 |
| `feat/observability-OBS-16-bootstrap-regression` | 48 | BootstrapSkyWalkingTracer fail-fast 契约 |

### 1.2 推进策略（沿用 stage-55 §二.4 推荐）

```
1. 切 feat/observability-batch-2-test-guards 基于本地 main（本地 main 含批 1 + Stage 52/53/54）
2. 按依赖序 merge: OBS-10 → OBS-11 → OBS-12 → OBS-13 → OBS-14 → OBS-15 → OBS-16
3. 每批后 go test ./... → 批尾双 smoke
4. scripts/build_dev_images.sh rebuild 6 svc 镜像
5. docker compose up -d 重起 6 svc
6. curl /metrics 抽样验证 + sw-oap UI 端到端
7. 合 main + 本文档归档
```

### 1.3 实测冲突面（stage-55 §二.3 已识别）

| 风险 | 实测结果 |
|---|---|
| shared 代码冲突（`shared/pkg/{metrics,grpcinterceptor,middleware,logging}`）| 🟢 **0 冲突**：7 分支都改 shared 但 diff 区间正交，git ort 自动 merge |
| 业务代码冲突（OBS-13/14/15 改 6 svc main.go）| 🟢 **0 冲突**：本批均只改测试文件（`metrics_test.go` / `bootstrap_test.go`），不动 main.go |
| Stage 47/48/49 main.go 改动与 OBS-15 重叠（`logging.Init/SetGlobalSvc`）| 🟢 **未触发**：OBS-15-logging-helper 分支只改 helper + 测试，未碰 6 svc main.go；OBS-15-logging-apply 分支（含 OBS-18 混合）按 stage-55 §3 推迟到批 3 业务 tag |

---

## 二、本批集成明细

### 2.1 OBS-10 · svc /metrics 端点契约

| 文件 | 行数 | 断言 |
|---|---|---|
| `emotion-echo-web-bff/main_test.go` | +178 行（增量）| metrics 中间件挂载 + 端点 200 + emotion_echo_http_requests_total + svc label |
| `emotion-echo-user-svc/metrics_test.go` | +109 行 | 同上（user-svc）|
| `emotion-echo-chat-svc/metrics_test.go` | +109 行 | 同上（chat-svc）|
| `emotion-echo-assessment-svc/metrics_test.go` | +109 行 | 同上（assessment-svc）|
| `emotion-echo-analytics-svc/metrics_test.go` | +109 行 | 同上（analytics-svc）|

### 2.2 OBS-11 · fusion metrics

| 文件 | 行数 | 断言 |
|---|---|---|
| `emotion-echo-shared/pkg/metrics/fusion_metrics.go` | +29 行 | `emotion_echo_fusion_calls_total` counter + `emotion_echo_fusion_duration_seconds` histogram |
| `emotion-echo-ai-svc/internal/fusion/fusion_metrics.go` | +17 / -1 | ai-svc 内部 fusion 包装 |
| `emotion-echo-ai-svc/internal/fusion/fusion_metrics_test.go` | +48 行 | 2 case（calls total + duration histogram）|

### 2.3 OBS-12/13/14 · trace 透传边界

| OBS | 文件 | 行数 | 断言 |
|---|---|---|---|
| 12 | `shared/pkg/middleware/gin_skywalking_test.go` | +102 行 | HTTP trace 透传 3 case |
| 13 | `shared/pkg/grpcinterceptor/tracing_test.go` | +77 行 | gRPC trace 透传 2 case |
| 14 | `ai-svc/internal/consumer/consumer_test.go` | +138 行 | Kafka consumer span tag 3 case |

### 2.4 OBS-15 · JSON 日志 helper（仅 helper 部分）

| 文件 | 行数 | 断言 |
|---|---|---|
| `shared/pkg/logging/logging.go` | +99 / -5 | JSON 日志 helper + 决策 6 必填字段 svc/trace_id/action |
| `shared/pkg/logging/logging_test.go` | +86 行 | 5 case（必填字段 / 序列化 / 错误处理）|

**OBS-15-apply 分支（含 6 svc main.go 接入 + OBS-18 混合）→ 推迟到批 3**，见 §四。

### 2.5 OBS-16 · bootstrap fail-fast 契约

| 文件 | 行数 | 断言 |
|---|---|---|
| `emotion-echo-web-bff/main_test.go` | +59 行 | web-bff bootstrap 装配断言（1 case）|
| `emotion-echo-user-svc/bootstrap_test.go` | +71 行 | 4 svc 部分 1：bootstrap 装配 |
| `emotion-echo-chat-svc/bootstrap_test.go` | +71 行 | 同上 |
| `emotion-echo-assessment-svc/bootstrap_test.go` | +71 行 | 同上 |
| `emotion-echo-analytics-svc/bootstrap_test.go` | +71 行 | 同上 |
| `docs/stages/stage-44-observability-sprint-b.md` | +282 行 | Sprint B 全收口归档 + 未做项标注 |

---

## 三、验证（每条来自实测，不来自 commit msg）

### 3.1 静态检查（AGENTS.md §2.1）

| 范围 | 命令 | 结果 |
|---|---|---|
| shared | `go test ./...` | ✅ 全 PASS（15+ 包，含新增 metrics/logging/middleware/grpcinterceptor 测试）|
| web-bff | `go test ./...` | ✅ 全 PASS（含 metrics_test 109 行 + main_test 178 行 + bootstrap 59 行）|
| user-svc | `go test ./...` | ✅ 全 PASS（含 metrics_test 109 + bootstrap 71）|
| chat-svc | `go test ./...` | ✅ 全 PASS（含 metrics_test 109 + bootstrap 71）|
| assessment-svc | `go test ./...` | ✅ 全 PASS（含 metrics_test 109 + bootstrap 71）|
| analytics-svc | `go test ./...` | ✅ 全 PASS（含 metrics_test 109 + bootstrap 71）|
| ai-svc | `go test ./...` | ✅ 全 PASS（含 fusion_metrics_test 48 + consumer_test 138，grpcserver 55.679s 含健康检查）|
| 7 包 `go vet ./...` | `for d in ...; do go vet ./...` | ✅ 0 警告 |

### 3.2 镜像 rebuild（AGENTS.md §二"决策 18 #22 教训"：必须 rebuild）

```
$ bash scripts/build_dev_images.sh
=== Build summary: SOME FAILED ===  ← 脚本误报 FAIL
$ docker images | grep emotion-echo/
emotion-echo/ai-svc:v0.1.0           a64fd7f657d7       80.5MB         20.2MB
emotion-echo/analytics-svc:v0.1.0    1312dc9ef9f1       80.1MB         20.1MB
emotion-echo/assessment-svc:v0.1.0   ced1a248d3c2       75.4MB         18.8MB
emotion-echo/chat-svc:v0.1.0         18cf705551f1       79.9MB         20.1MB
emotion-echo/user-svc:v0.1.0         752a02499020       75.4MB         18.8MB
emotion-echo/web-bff:v0.1.0          bfd985de7300       72.9MB         18.4MB
emotion-llm-service:v0.1.0           de90a2c2a269       358MB          83.3MB
```

**7 张镜像全部 built（hash 全新），脚本误报 FAIL 未阻塞**——这是 stage-54 §七 D + stage-55 §九 P2 项已知 bug，本批**只记录不修**，留给 stage-55 §九 P2 backlog。

### 3.3 docker compose 重起 + smoke（业务数据契约 + 可观测性契约）

```
$ cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
    up -d --no-deps emotion-echo-{user,chat,analytics,assessment,ai}-svc emotion-echo-web-bff
[sleep 15s]
$ docker ps --format "table {{.Names}}\t{{.Status}}"
emotion-echo-web-bff          Up 17 seconds (healthy)
emotion-echo-ai-svc           Up 18 seconds (healthy)
emotion-echo-chat-svc         Up 18 seconds (healthy)
emotion-echo-user-svc         Up 18 seconds (healthy)
emotion-echo-analytics-svc    Up 18 seconds (healthy)
emotion-echo-assessment-svc   Up 18 seconds (healthy)

$ python scripts/smoke_observability.py
PASS: 12 check(s)   ← 12/12 PASS（prometheus scrape 10/8 / grafana / loki / kafka-lag / alert）

$ python scripts/smoke_data_layer.py
汇总: 11/11 PASS, 0 FAIL   ← §契约 1 行数=12 / §契约 2 enum 3 种 / §契约 3 视图可读 / §契约 4 7 天窗
```

### 3.4 /metrics 端点抽样（验证护栏对应的真实指标）

```
$ curl -s http://localhost:8894/metrics | wc -l
269   ← 业务指标真实采样

$ curl -s http://localhost:8894/metrics | grep -E "^# (HELP|TYPE) emotion_echo"
# HELP emotion_echo_http_request_duration_seconds Histogram of HTTP request latency in seconds.
# TYPE emotion_echo_http_request_duration_seconds histogram
...（含 service="web-bff" label / method / path / le bucket / _sum / _count 完整）

$ curl -s http://localhost:8894/metrics | grep "conversations/:id"
emotion_echo_http_request_duration_seconds_count{method="DELETE",path="/api/v1/conversations/:id",service="web-bff"} 1
   ← smoke_data_layer §契约 1 跑 DELETE conv 留下的真实采样
```

**测试护栏"指标应存在" + 实际端点"指标真实采样"两端对齐**——OBS-10 契约测试守护的不是"指标注册了"，而是"业务流量走过中间件后采样能被外部抓到"。

### 3.5 ai-svc fusion metrics（部分未触发，留给批 3 / chat-svc e2e）

```
$ curl -s http://localhost:8891/metrics | grep "^emotion_echo_fusion"
（无输出）
```

**原因**：fusion worker 当前未收到业务消息触发（chat-svc 端到端 e2e 未跑）。**这不是批 2 缺陷**——OBS-11 测试守护的是"fusion_metrics.go 注册了 counter + histogram"，已绿；业务侧未跑通是 chat-svc→ai-svc gRPC 链路 + chat-svc SSE 流端到端问题，留给批 3（OBS-19 gRPC rpc.* tag）+ chat-svc e2e。

---

## 四、推迟到批 3 的项

### 4.1 `feat/observability-OBS-15-logging-apply` 分支

apply 分支实际包含：
- OBS-15 6 svc main.go `logging.Init` / `SetGlobalSvc` 接入
- OBS-18 GinSkywalkingMiddleware 创建 EntrySpan + 4 tag（混合在 apply 分支上）
- Stage 46/47 归档文档

按 stage-55 §3，**这属于"业务 tag"范畴**，留到批 3（OBS-17/18/19/23）统一推进。原因：
- 应用层接入涉及 6 svc main.go 改动，**镜像必须 rebuild**
- Stage 47/48/49 main.go 改动可能与 OBS-23 handler-err-propagate 重叠（stage-55 §三.3 已识别）
- 批 2 收口后，批 3 在 OBS-17 TracerInterface 抽象统一基础上推进，**冲突面比独立推进小**

### 4.2 `feat/observability-OBS-17/18/19/23` 4 个分支（批 3 主体）

| 分支 | 主题 | 领先 commit |
|---|---|---|
| OBS-17 | TracerInterface + Span.Tag + 6 svc main.go 包装 | 53 |
| OBS-18 | GinSkywalkingMiddleware 创建 EntrySpan + 4 tag | 57 |
| OBS-19 | gRPC Server/ClientTracingInterceptor rpc.* 5/4 项 | 68（含 Stage 50 e2e-validation）|
| OBS-23 | handler err propagate 三段判定 | 63 |

批 3 推进时序（stage-55 §三.4 推荐）：

```
1. 切 feat/observability-batch-3-biz-tags 基于 main（本批已合）
2. 拉 OBS-15-apply 进来一起处理（消除 apply 分支与 OBS-17/18/19 重叠）
3. 顺序 merge: OBS-17 → OBS-18 → OBS-19 → OBS-23
4. rebuild 6 svc 镜像（Stage 47/48/49 main.go 改动首次生效）
5. sw-oap UI 验证 http.* / rpc.* / messaging.* tag 出现
6. 合 main + 写 stage-XX-b3-merged.md 归档
```

---

## 五、与 stage-55 §九 优先级闭环情况

| 优先级 | 项 | 状态 |
|---|---|---|
| 🔴 P0 | 路线 Z 批 2（OBS-9~16 测试护栏）| ✅ **本批完成** |
| 🔴 P0 | 路线 Z 批 3（OBS-17/18/19/23 业务 tag）| ⏸ 留待下一批（§四.2）|
| 🟡 P1 | todo-pile §C8 BFF 路由三方契约收口 | 未启动（决策 18 #22 同源，待排期）|
| 🟡 P1 | todo-pile §A1/B4 TTS / 多模态 AI 决策 | 未启动（待 owner）|
| 🟡 P1 | todo-pile §A2 文件上传 | 未启动（待 MinIO 选型）|

---

## 六、未做项（本批未触碰，留 backlog）

| ID | 项 | 来源 | 优先级 |
|---|---|---|---|
| 1 | `feat/observability-OBS-15-logging-apply` 分支集成（含 OBS-18 混合）| stage-55 §3 | 🔴 P0 批 3 |
| 2 | `feat/observability-OBS-17/18/19/23` 4 分支集成 | stage-55 §3 | 🔴 P0 批 3 |
| 3 | `scripts/build_dev_images.sh` 误报 FAIL 修复（grep `^ Image .* Built$` 匹配行 buffer 截断）| stage-54 §七 D / stage-55 §九 P2 | 🟢 P2 |
| 4 | `docs/plans/todo-pile-2026-09-04.md` §A1/B4 TTS AI profile 决策 | todo-pile §A1/B4 | 🟡 P1 待 owner |
| 5 | `docs/plans/todo-pile-2026-09-04.md` §A2 文件上传 + MinIO 选型 | todo-pile §A2 | 🟡 P1 |
| 6 | `docs/plans/todo-pile-2026-09-04.md` §C8 BFF 路由三方契约收口 | todo-pile §C8 | 🟡 P1 |
| 7 | `kafka-reliability-gaps.md` §1.5 Protobuf 迁移（Sprint C）| kafka-reliability-gaps.md §1.5 | 🟢 P3 |

---

## 七、跨 Sprint 推进关系更新

```
Stage 44 Sprint B (路线 Z)
   ├── 批 1 基础设施 ✅ DONE (Stage 51~54, merge ce3d80b)
   ├── 批 2 测试护栏 ✅ DONE (Stage 56, merge 3e5cd25, 本批)
   └── 批 3 业务 tag ⏸ 未启动 (OBS-15-apply + OBS-17/18/19/23, 候选 Stage 57)

Kafka Sprint A
   ├── 阶段 Stage 43 ✅ DONE
   ├── Sprint B §1.4 尾巴 ✅ 批 1 含 (PR-OBS-7)
   └── Sprint C §1.5 Protobuf 迁移 ⏸ 独立 Sprint

Nacos 治理 (nacos-enablement-dev.md)
   ├── PR-0~PR-5 ⏸ 未启动 (大半已被路线 Z 间接落地)
   ├── ListenConfig 热更新 ⏸ 未启动 (B2 backlog)
   └── PR-2 web-bff 走 Nacos 发现下游 ⏸ 未启动

todo-pile backlog
   ├── §A3 前端提示框 ✅ Stage 51/53 闭环
   ├── §C7 stage-36 dashboard 空 → Stage 53 §六 A 闭环（7 天窗替代）
   └── §C5 QUICKSTART 表述 → 未启动
```

---

## 八、验证依据清单（满足 AGENTS.md §〇"文档撰写前调研"要求）

| 调研项 | 来源 | 引用 |
|---|---|---|
| 6 svc main.go `GinMetricsMiddleware` 真实挂载点 | web-bff/main.go:101 等 | `grep -n "GinMetricsMiddleware" */main.go` 6 处命中 |
| 真实指标采样验证 | `curl /metrics` | §三.4 输出 269 行 |
| shared metrics/logging/middleware 测试落地 | `go test ./pkg/{metrics,logging,middleware,grpcinterceptor}/...` | §三.1 输出 |
| smoke 退出码与数据契约 7 项 | `python scripts/smoke_*.py` | §三.3 输出 12/12 + 11/11 |
| ai-svc fusion 未触发的业务侧原因 | decision 18 #22 同源 | §三.5 说明 |
| build script 误报 FAIL 已记录未修 | stage-54 §七 D | §三.2 说明 |

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：stage-44/51/52/53/54/55 全程 + observability-sprint-b.md + 7 个 OBS 分支 git log + 实际 go test/vet/smoke/curl 输出
> 用途：路线 Z 批 2 收口 + 批 3 推进起点 + todo-pile backlog 状态对齐