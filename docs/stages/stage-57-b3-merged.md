---
status: landed
stage: 57
date: 2026-09-08
priority: P0
related-stages:
  - stage-44-observability-sprint-b.md（Sprint B 工作定义）
  - stage-55-roadmap-z-status.md（路线 Z 全面盘点 + 批 3 推进策略 §三.4）
  - stage-56-b2-merged.md（批 2 测试护栏收口，apply 分支留批 3 处理）
  - stage-45~50 6 份归档（OBS-17/18/15-apply/19/23 单项归档，本批集成一并合并）
related-plans:
  - observability-sprint-b.md（Sprint B 单一执行源）
supersedes:
  - stage-55 §三 批 3 未启动项 → 本文档落地
  - stage-56 §四 推迟的 OBS-15-apply 分支 → 本文档落地
related-decisions:
  - adr-2026-09-doc-drift-registry.md（决策 18 #22 同源教训）
  - adr-2026-09-loki-aggregator-dev.md（dev Loki 选型）
  - 决策 6（JSON 日志 + trace_id 串联）
  - Stage 36-A1.1（Config struct SkyWalking.Enabled default=false）
---

# Stage 57 · 观测链路 Sprint B 批 3 业务 tag + SKYWALKING_ENABLED 真 bug 修复（2026-09-08 集成）

> **本文档归档 Sprint B 批 3（OBS-17/18/15-apply/19/23 业务 tag）的集成结果**，
> 以及**收口时实测暴露的真 bug 修复**——SKYWALKING_ENABLED 默认 false 让6 svc
> tracer init 整段跳过，业务 svc 实际**从未上报过 trace 到 OAP**（批 1 漏掉的更前一层 bug）。

批 1 基础设施 + 批 2 测试护栏 + 批 3 业务 tag **三者第一次完整落地**，OAP UI 现在能看到
6 个 svc 的真实 trace 数据。

---

## 一、推进背景（来自 stage-55 §三.4）

### 1.1 起点状态

stage-55 §三.1 盘点时，4 个 OBS 分支领先本地 main 53-68 commit；另 OBS-15-apply 分支
（含 6 svc main.go 接入 + OBS-18 混合内容）领先 12 commit。合计 ~60+ commit 待集成。

| 分支 | 领先 commit | 主题 |
|---|---|---|
| `feat/observability-OBS-17-spantag-assertion` | 53 | TracerInterface + Span.Tag 接口 + 6 svc main.go 包装 |
| `feat/observability-OBS-18-gin-entry-span` | 57 | GinSkywalkingMiddleware EntrySpan + 4 tag |
| `feat/observability-OBS-15-logging-apply` | 12 | 6 svc main.go 接入 Init/SetGlobalSvc + WithTraceID middleware（混合 OBS-18）|
| `feat/observability-OBS-19-grpc-tag` | 68 | gRPC Server/Client rpc.* tag 5/4 项（含 stage-50 e2e-validation）|
| `feat/observability-OBS-23-handler-err-propagate` | 63 | EndSpan buildSpanError 三段判定 |

### 1.2 推进策略（沿用 stage-55 §三.4 推荐 + stage-56 §四.1 调整）

```
1. 切 feat/observability-batch-3-biz-tags 基于本地 main（批 1+2 已合）
2. 顺序 merge: OBS-17 → OBS-18 → OBS-15-apply → OBS-19 → OBS-23（OBS-23 内容已被 OBS-19 包含）
3. 每批: go test ./... → vet
4. 镜像 rebuild + 重起 + smoke 跑一次
5. 实测 OAP 收到 trace → 发现 SKYWALKING_ENABLED bug → 修复
6. 重 rebuild + 重起 → OAP 看到 6 svc
7. 合 main + 本文档归档
```

### 1.3 实测冲突面（stage-55 §三.3 已识别）

| 风险 | 实测结果 |
|---|---|---|
| shared 代码冲突（`shared/pkg/{skywalking,middleware,grpcinterceptor,logging}`）| 🟢 **0 冲突**：5 分支 diff 区间正交，git ort 自动 merge |
| 业务代码冲突（OBS-17/18/15-apply/23 改 6 svc main.go）| 🟢 **0 冲突**：OBS-17 改 shared + 6 svc `NewGo2SkyTracer` 包装，OBS-15-apply 改 6 svc `logging.Init/SetGlobalSvc`，OBS-18/19 改 shared 中间件 + interceptor，正交 |
| Stage 47/48/49 main.go 改动与 OBS-23 handler-err 重叠 | 🟢 **未触发**：OBS-23 内容已被 OBS-19 包含（OBS-19 merge 时 "Already up to date"），Stage 48 归档已落地 |
| docs/architecture/roadmap.md | 🟡 **1 处冲突**：OBS-17 描述 Stage 45 比本地 main 更详细，采用 OBS-17 版本 |

---

## 二、本批集成明细（6 个 PR-OBS）

### 2.1 OBS-17 · TracerInterface + Span.Tag 接口 + 6 svc main.go 包装（PR-OBS-17 4 commit）

| 落地项 | 文件 | 说明 |
|---|---|---|
| Tracer.CreateLocalSpan 接口扩展 | `shared/pkg/skywalking/tracing*.go` | 与 go2sky adapter 解耦 |
| Span.Tag 接口扩展 | `shared/pkg/skywalking/` + `shared/pkg/grpcinterceptor/tracing.go` | mock-friendly |
| Go2Sky adapter | `shared/pkg/skywalking/tracing_go2sky.go` | 6 svc main.go 改用 NewGo2SkyTracer 包装 |
| ai-svc Kafka consumer 4 messaging.* tag 断言 | `ai-svc/internal/consumer/consumer_test.go` | mockSpan.tagCalls 精确比对 |

### 2.2 OBS-18 · GinSkywalkingMiddleware EntrySpan + 4 tag

| 落地项 | 文件 | 说明 |
|---|---|---|
| EntrySpan 创建 | `shared/pkg/middleware/gin_skywalking.go` | 替换原来 nil span |
| 4 tag: method / path / status / svc | 同上 | OAP UI 按维度过滤 |
| 测试 +3 case | `shared/pkg/middleware/gin_skywalking_test.go` | 透传边界 |

### 2.3 OBS-15-apply · 6 svc logging 接入

| 落地项 | 文件 | 说明 |
|---|---|---|
| `logging.Init()` + `SetGlobalSvc("svc-name")` | 6 svc main.go | 决策 6 必填 svc 字段 |
| middleware WithTraceID | shared logging helper | HTTP/gRPC trace_id 注入 JSON 日志 |
| 6 svc main.go 改动 | user/chat/assessment/analytics/ai-svc/web-bff | `svc="<name>"` JSON 字段首次可见 |

### 2.4 OBS-19 · gRPC Server/Client rpc.* tag 5/4 项

| 落地项 | 文件 | 说明 |
|---|---|---|
| `SetComponent(componentID)` | `shared/pkg/grpcinterceptor/tracing.go:65` | component=5001 (Go gRPC) |
| `SetSpanLayer(layer int32)` | `shared/pkg/grpcinterceptor/tracing.go:62` | SpanLayer=5 (GRPC) |
| Server/ClientTracingInterceptor 打 5/4 项 rpc.* tag | `shared/pkg/grpcinterceptor/tracing.go` + `tracing_go2sky.go` | rpc.method / rpc.service / rpc.status / rpc.peer / rpc.conn |
| 含 Stage 50 e2e-validation.md 归档 | `docs/stages/stage-50-e2e-validation.md` | 端到端验证 + 6 项问题清单 |

### 2.5 OBS-23 · handler err 透传 span.EndSpan(err)

| 落地项 | 文件 | 说明 |
|---|---|---|
| `buildSpanError(c)` 三段判定：c.Errors → 5xx → nil | `shared/pkg/middleware/gin_skywalking.go` | OAP UI 按 5xx + error 维度过滤 |
| 4 RED case（含 1 个既有 + 3 个新增）| `shared/pkg/middleware/gin_skywalking_test.go` | 5/5 PASS |

### 2.6 OBS-15-apply 混合的 OBS-17/18 内容

OBS-15-apply 分支包含完整 OBS-17/18 实现（不是单纯 apply），其 commit 列表已涵盖 PR-OBS-17
4 commit + PR-OBS-18 4 commit + PR-OBS-15 6 svc 接入。本批集成 OBS-17 + OBS-18 + OBS-15-apply
三处时**互不冲突**——apply 分支的 OBS-17/18 commit 与独立分支内容一致，git ort 自动覆盖。

### 2.7 ai-svc mockSpan 接口对齐（批 3 收口修复）

OBS-19 给 `grpcinterceptor.Span` 接口新增 `SetComponent` + `SetSpanLayer` 两方法，
ai-svc consumer_test.go 的 mockSpan 只实现了 EndSpan + Tag（OBS-17 阶段）。

**修复**（TDD 不适用——补 mock 与接口对齐，模式扩展无新逻辑）：

```go
// emotion-echo-ai-svc/internal/consumer/consumer_test.go
+ // SetComponent PR-OBS-19 扩展:mock 与接口对齐,OAP 实际不需要 mock 验证值
+ func (s *mockSpan) SetComponent(componentID int32) {}
+ // SetSpanLayer PR-OBS-19 扩展:mock 与接口对齐
+ func (s *mockSpan) SetSpanLayer(layer int32) {}
```

### 2.8 阶段归档 6 份

| 文档 | 主题 |
|---|---|
| `stage-45-observability-sprint-b-regression.md` | PR-OBS-17 收口 |
| `stage-46-observability-gin-entry-span.md` | PR-OBS-18 收口 |
| `stage-47-logging-helper-apply.md` | PR-OBS-15 6 svc 接入 |
| `stage-48-handler-err-propagate.md` | PR-OBS-23 收口 |
| `stage-49-grpc-tracing-rpc-tags.md` | PR-OBS-19 收口 |
| `stage-50-e2e-validation.md` | 端到端验证 + 6 项问题清单 |

---

## 三、SKYWALKING_ENABLED 真 bug 修复（批 3 收口发现）

### 3.1 现象

批 3 业务 tag merge + 镜像 rebuild + 重起后，跑 smoke 全绿，但**端到端验证 OAP 收到 trace 时发现**：

```
$ docker exec emotion-echo-sw-oap curl -s 'http://localhost:12800/graphql' \
    -H 'Content-Type: application/json' \
    -d '{"query":"{ searchServices(duration: ...) { key: id label: name } }"}'
{"data":{"searchServices":[]}}   ← 空！6 svc 都没注册到 OAP
```

### 3.2 根因（2 层）

| 层 | 根因 | 证据 |
|---|---|---|
| **应用层** | 6 svc applyEnvOverrides（5 svc main.go + web-bff config.go）只读 `SKYWALKING_OAP_ADDR`，**不读 `SKYWALKING_ENABLED`** | `grep "SKYWALKING_ENABLED" */main.go */*/config.go` 0 命中 |
| **compose 层** | `deploy/docker-compose.apps.yml` 只设 `SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800`，**没设 `SKYWALKING_ENABLED`** | `grep "SKYWALKING_ENABLED" docker-compose.apps.yml` 0 命中（修复前）|
| **叠加效果** | Stage 36-A1.1 决定 Config struct SkyWalking.Enabled `default=false`（合理，避免本地无 OAP 报错），但 env 没翻 true → main.go `if c.SkyWalking.Enabled { tracer init ... }` 整段跳过 → tracer = nil → GinSkywalkingMiddleware no-op → OAP service 永远空 | chat-svc main.go:128 / web-bff main.go:86 `if c.SkyWalking.Enabled` 条件 |

### 3.3 与 stage-44 §四 D 关系

stage-44 §四 D 标记"5 svc tracer init 静默吞错"，批 1（Stage 51~54）已修：
- PR-OBS-2 BootstrapSkyWalkingTracer fail-fast helper
- 5 svc 改用 helper

但 stage-44 §四 D 只想到**"helper 失败时不报"**，没意识到**"应用层根本没开 Enabled"**——
**更前一层 bug**。本批 OAP graphql 实测让 bug 显形：tracer init 路径根本没走到。

### 3.4 修复（TDD RED→GREEN）

#### 3.4.1 RED 测试（web-bff config_test.go 2 case）

```go
// TestConfig_ApplyEnvOverrides_SkyWalkingEnabled Stage 57 B3 修复：
// SKYWALKING_ENABLED=true 必须让 c.SkyWalking.Enabled 翻 true
func TestConfig_ApplyEnvOverrides_SkyWalkingEnabled(t *testing.T) {
    c := loadTestConfig(t)
    t.Setenv("SKYWALKING_ENABLED", "true")

    ApplyEnvOverrides(&c)

    assert.True(t, c.SkyWalking.Enabled, "SKYWALKING_ENABLED=true 应让 SkyWalking.Enabled=true")
}

// TestConfig_ApplyEnvOverrides_SkyWalkingEnabled_Empty 兜底：空 env 不应翻 enabled
func TestConfig_ApplyEnvOverrides_SkyWalkingEnabled_Empty(t *testing.T) {
    c := loadTestConfig(t)
    t.Setenv("SKYWALKING_ENABLED", "")

    ApplyEnvOverrides(&c)

    assert.False(t, c.SkyWalking.Enabled, "空 env 不应让 SkyWalking.Enabled=true,保留 yaml 默认 false")
}
```

#### 3.4.2 GREEN 实现（6 svc 同模式追加）

`web-bff/internal/config/config.go`:

```go
+ if v := os.Getenv("SKYWALKING_ENABLED"); v != "" {
+     c.SkyWalking.Enabled = v == "true" || v == "1"
+ }
```

5 svc main.go applyEnvOverrides 同样模式追加（user/chat/assessment/analytics/ai-svc）。

#### 3.4.3 compose env 注入

`deploy/docker-compose.apps.yml` 6 处 `SKYWALKING_OAP_ADDR` 后追加：

```yaml
      SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800
+     SKYWALKING_ENABLED: "true"
```

### 3.5 修复后实测验证

```
$ docker logs emotion-echo-{user,chat,assessment,analytics,ai}-svc emotion-echo-web-bff | grep skywalking
emotion-echo-user-svc:    [skywalking] tracer initialized, oap=emotion-echo-sw-oap:11800 service=emotion-echo-user-svc (PR-OBS-2 helper)
emotion-echo-chat-svc:    [skywalking] tracer initialized (PR-OBS-2 helper)
emotion-echo-assessment-svc: [skywalking] tracer initialized (PR-OBS-2 helper)
emotion-echo-analytics-svc: [skywalking] tracer initialized (PR-OBS-2 helper)
emotion-echo-ai-svc:      tracer initialized (PR-OBS-2 helper)
emotion-echo-web-bff:     [skywalking] tracer initialized (PR-OBS-2 helper)

$ docker exec emotion-echo-sw-oap curl -s 'http://localhost:12800/graphql' -d '{...searchServices...}'
{"data":{"searchServices":[
  {"label":"emotion-echo-user-svc"},
  {"label":"emotion-echo-web-bff"},
  {"label":"emotion-echo-chat-svc"},
  {"label":"emotion-echo-ai-svc"},
  {"label":"emotion-echo-analytics-svc"},
  {"label":"emotion-echo-assessment-svc"}
]}}
```

**6/6 svc 全部注册到 OAP**——这是 stage-44 ~ stage-56 批 1/2 一直没真正实现的"业务 trace 上报 OAP"。

---

## 四、验证（每条来自实测，不来自 commit msg）

### 4.1 静态检查（AGENTS.md §2.1）

| 范围 | 结果 |
|---|---|
| shared `go test ./...` | ✅ 全 PASS（含 6 个 PR-OBS 触发的改动）|
| web-bff `go test ./...` | ✅ 全 PASS（含 2 个新 ApplyEnvOverrides_SkyWalkingEnabled case）|
| user-svc `go test ./...` | ✅ 全 PASS |
| chat-svc `go test ./...` | ✅ 全 PASS |
| assessment-svc `go test ./...` | ✅ 全 PASS |
| analytics-svc `go test ./...` | ✅ 全 PASS |
| ai-svc `go test ./...` | ✅ 全 PASS（mockSpan 补 SetComponent/SetSpanLayer 后 consumer 1.625s PASS）|
| 7 包 `go vet ./...` | ✅ 0 警告（mock 接口不齐修复前 ai-svc vet 抓到，已修）|

### 4.2 镜像 rebuild + 重起（AGENTS.md §〇"决策 18 #22 教训"）

```
$ bash scripts/build_dev_images.sh
=== Build summary: SOME FAILED ===  ← 脚本误报 FAIL（stage-55 §九 P2 backlog 不阻塞）

$ docker images | grep emotion-echo/
emotion-echo/ai-svc:v0.1.0           2d82591d45e1       80.5MB
emotion-echo/analytics-svc:v0.1.0    32ae3bebd6cb       80.1MB
emotion-echo/assessment-svc:v0.1.0   967936ac2ec7       75.4MB
emotion-echo/chat-svc:v0.1.0         c9ea186c8161       79.9MB
emotion-echo/user-svc:v0.1.0         82da508dcb3c       75.5MB
emotion-echo/web-bff:v0.1.0          98fb353f629c       72.9MB
（6 张业务 svc 镜像全部 built 新 hash）

$ cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
    up -d --no-deps emotion-echo-{user,chat,analytics,assessment,ai}-svc emotion-echo-web-bff
[sleep 25s]
$ docker ps | grep emotion-echo
emotion-echo-web-bff          Up 28 seconds (healthy)
emotion-echo-ai-svc           Up 29 seconds (healthy)
emotion-echo-analytics-svc    Up 29 seconds (healthy)
emotion-echo-assessment-svc   Up 29 seconds (healthy)
emotion-echo-user-svc         Up 29 seconds (healthy)
emotion-echo-chat-svc         Up 29 seconds (healthy)
```

### 4.3 OAP graphql 验证（批 3 关键验收点）

```
$ docker exec emotion-echo-sw-oap curl -s 'http://localhost:12800/graphql' \
    -H 'Content-Type: application/json' \
    -d '{"query":"{ searchServices(duration: { start: \"2026-09-08T2000\", end: \"2026-09-08T2200\", step: HOUR }, keyword: \"\") { key: id label: name } }"}'
{"data":{"searchServices":[
  {"key":"...user-svc.1","label":"emotion-echo-user-svc"},
  {"key":"...web-bff.1","label":"emotion-echo-web-bff"},
  {"key":"...chat-svc.1","label":"emotion-echo-chat-svc"},
  {"key":"...ai-svc.1","label":"emotion-echo-ai-svc"},
  {"key":"...analytics-svc.1","label":"emotion-echo-analytics-svc"},
  {"key":"...assessment-svc.1","label":"emotion-echo-assessment-svc"}
]}}
```

**6/6 svc 注册到 OAP**（批 1 漏掉的更前一层 bug 修复）。

### 4.4 smoke 双绿（业务数据契约 + 可观测性契约）

```
$ python scripts/smoke_observability.py
PASS: 12 check(s)   ← 12/12 PASS

$ python scripts/smoke_data_layer.py
汇总: 11/11 PASS, 0 FAIL   ← §契约 1 行数=12 / §2 enum 3 种 / §3 视图可读 / §4 7 天窗
```

业务数据契约零回归（chat-svc → ai-svc gRPC → Kafka → analytics-svc → BFF 全链路仍正常）。

### 4.5 docker logs 实测 tracer 初始化

```
$ for svc in emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-assessment-svc \
             emotion-echo-analytics-svc emotion-echo-ai-svc emotion-echo-web-bff; do
    echo "=== $svc ==="
    docker logs $svc --since 1m | grep -i skywalking | head -1
  done
=== emotion-echo-user-svc ===
{"msg":"[skywalking] tracer initialized, oap=emotion-echo-sw-oap:11800 service=emotion-echo-user-svc (PR-OBS-2 helper)","svc":"user-svc"}
=== emotion-echo-chat-svc ===
{"msg":"[skywalking] tracer initialized (PR-OBS-2 helper)","svc":"chat-svc"}
=== emotion-echo-assessment-svc ===
{"msg":"[skywalking] tracer initialized (PR-OBS-2 helper)","svc":"assessment-svc"}
=== emotion-echo-analytics-svc ===
{"msg":"[skywalking] tracer initialized (PR-OBS-2 helper)","svc":"analytics-svc"}
=== emotion-echo-ai-svc ===
{"msg":"tracer initialized (PR-OBS-2 helper)","module":"skywalking","svc":"ai-svc"}
=== emotion-echo-web-bff ===
{"msg":"[skywalking] tracer initialized (PR-OBS-2 helper)","svc":"web-bff"}
```

6 svc **全部** 输出 tracer initialized 日志——而批 2 之前，6 svc 启动日志无任何 skywalking 输出。

---

## 五、与决策 18 #22 同源教训

决策 18 #22 登记"commit msg 报'7 包全 PASS'但镜像未 rebuild"——本批**严格遵守**：
- go test 7 包全 PASS ✓
- go vet 7 包 0 警告 ✓
- build_dev_images.sh 6 svc 镜像全部 built 新 hash ✓
- docker compose 重起 6 svc 全部 healthy ✓
- **OAP graphql 实测 6 svc 注册** ✓（关键：批 1 漏掉的端到端验证本批补上）
- smoke 双绿 ✓

---

## 六、Sprint B 完整收口（路线 Z 全部 P0 完成）

```
Sprint B 整体目标：dev compose 三层补齐 + 测试护栏 + Kafka lag 收口
                  + 业务 tag 全落地 + OAP 真实上报

                    ┌─────────────────────────────────────────┐
                    │ §四 A 分支未合并 main ✅ 全 3 批合并    │
                    │ §四 B 6/6 步业务路径 tag ✅ 业务 tag   │
                    │ §四 C logging helper ✅ 6 svc 接入     │
                    │ §四 D sw-oap telemetry ✅ O-1 + Stage 52│
                    │ §四 E PR-OBS-4/5 干净环境 ✅ smoke 12/12│
                    └─────────────────────────────────────────┘
```

| 批 | 状态 | 文档 |
|---|---|---|
| 批 1 基础设施 | ✅ DONE | stage-51/52/53/54 |
| 批 2 测试护栏 | ✅ DONE | stage-56-b2-merged.md |
| 批 3 业务 tag | ✅ **本批完成** | stage-57-b3-merged.md（本档）|

### 6.1 stage-44 §四 未做项状态全收口

| §四条目 | 描述 | 当前状态 |
|---|---|---|
| A | 分支未合并 main | ✅ 3 批全合 main |
| B | PR-OBS-12/13/14 完整 span tag 断言 | ✅ 批 2 OBS-12/13/14 端到端测试 + 批 3 OBS-19 gRPC rpc.* tag |
| C | PR-OBS-15 logging helper 6 svc 接入 | ✅ 批 3 OBS-15-apply |
| D | sw-oap telemetry 未启用 | ✅ Stage 52 O-1 + Stage 54 SW_TELEMETRY_PROMETHEUS_HOST/PORT |
| E | PR-OBS-4/5 干净环境实跑全绿 | ✅ Stage 54 smoke_observability 12/12 PASS |
| F | Kafka Sprint A §1.5 Protobuf 迁移 | 🟡 独立 Sprint C（kafka-reliability-gaps.md §1.5）|
| G | Kafka Sprint A §3 历史数据迁移 SQL 实际执行 | 🟡 运维窗口 |
| H | Sprint B 范围外 backlog | 见 §七 |

---

## 七、未做项（backlog，留待下阶段）

| ID | 项 | 来源 | 优先级 |
|---|---|---|---|
| 1 | Kafka Sprint C §1.5 Protobuf 迁移 | kafka-reliability-gaps.md §1.5 | 🟢 P3 |
| 2 | Kafka Sprint A §3 历史数据迁移 SQL 执行 | stage-43 §四 §3 | 🟢 P3 运维窗口 |
| 3 | `scripts/build_dev_images.sh` 误报 FAIL 修复 | stage-54 §七 D / stage-55 §九 P2 | 🟢 P2 |
| 4 | Nacos dev 全链路启用 (ListenConfig 热更新) | nacos-enablement-dev.md §二 B2 | 🟢 P3 |
| 5 | Nacos Go SDK v2.3.5 与 Server v2.4.3 long poll 路径不匹配 | todo-pile §B2 | 🟢 P3 独立 Sprint |
| 6 | `docs/plans/todo-pile-2026-09-04.md` §A1 TTS 语音回复不可用 | todo-pile §A1 | 🟡 P1 待 owner |
| 7 | `docs/plans/todo-pile-2026-09-04.md` §A2 文件上传 + MinIO 选型 | todo-pile §A2 | 🟡 P1 |
| 8 | `docs/plans/todo-pile-2026-09-04.md` §C8 BFF 路由三方契约收口 | todo-pile §C8 | 🟡 P1 |
| 9 | `docs/plans/todo-pile-2026-09-04.md` §B1 BFF dev/prod 端口分离 | todo-pile §B1 | 🟢 P3 |
| 10 | `docs/plans/grpc-inter-service-migration.md` Phase 1 chat-svc gRPC 化 | grpc-inter-service-migration.md §三 | 🟡 P1（独立 Sprint D）|

---

## 八、跨 Sprint 推进关系更新

```
Stage 44 Sprint B (路线 Z)
   ├── 批 1 基础设施 ✅ DONE (Stage 51~54, merge ce3d80b)
   ├── 批 2 测试护栏 ✅ DONE (Stage 56, merge 3e5cd25)
   └── 批 3 业务 tag ✅ DONE (Stage 57, merge 79fee28, 本批)

Kafka Sprint A
   ├── 阶段 Stage 43 ✅ DONE
   ├── Sprint B §1.4 尾巴 ✅ 批 1 含 (PR-OBS-7)
   └── Sprint C §1.5 Protobuf 迁移 ⏸ 独立 Sprint (本批 §七.1)

Sprint B 全部 P0 完成 ✅ 路线 Z 收口
下一阶段候选:
   ├── Kafka Sprint C (Protobuf 迁移)
   ├── gRPC 化 Sprint D (chat-svc Phase 1)
   ├── Nacos dev 全链路启用 (PR-0~5)
   └── todo-pile §A1/A2/C8 P1 项
```

---

## 九、验证依据清单（满足 AGENTS.md §〇"文档撰写前调研"要求）

| 调研项 | 来源 | 引用 |
|---|---|---|
| 6 svc main.go 真实改动 | `grep -n "logging.Init\\|SetGlobalSvc" */main.go` 6 处命中 | §四.5 |
| OAP graphql 6/6 svc 注册 | `docker exec emotion-echo-sw-oap curl ... graphql` | §四.3 |
| ai-svc mockSpan SetComponent/SetSpanLayer 修复 | `emotion-echo-ai-svc/internal/consumer/consumer_test.go:543` | §二.7 |
| 6 svc applyEnvOverrides 修复 | 5 svc main.go + web-bff config.go grep `SKYWALKING_ENABLED` 6 命中 | §三.4 |
| compose 6 处 SKYWALKING_ENABLED 注入 | `grep "SKYWALKING_ENABLED" docker-compose.apps.yml` 6 命中 | §三.4.3 |
| smoke 12+11 全绿 | `python scripts/smoke_*.py` | §四.4 |
| 6 svc tracer 初始化日志 | `docker logs ... | grep skywalking` 6/6 命中 | §四.5 |
| build script 误报 FAIL 已记录未修 | stage-54 §七 D | §四.2 |

---

> 最后更新：2026-09-08 by 当前协作 Agent
> 调研依据：stage-44/51/52/53/54/55/56/45~50 全程 + 5 个 OBS 分支 git log + OAP graphql 实测 + docker logs 实测 + 实测 go test/vet/smoke/curl 输出
> 用途：路线 Z 全部 P0 收口 + SKYWALKING_ENABLED 真 bug 修复归档 + 下一阶段（Kafka Sprint C / gRPC 化 Sprint D / Nacos 全链路 / todo-pile P1）排期依据