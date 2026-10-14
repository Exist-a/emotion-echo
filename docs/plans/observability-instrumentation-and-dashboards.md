---
status: planned
priority: medium
owner: TBD
created: 2026-10-13
type: implementation-plan
source: 2026-10-13 会话（用户："还有关于埋点和可视化面板的问题"）
depends-on: []
related-plans:
  - observability-business-metrics-wiring.md（**面板/告警实施版**——本计划与之互补：那份**不改 Go 埋点**，本计划**正是改埋点代码**，见 §7）
  - grafana-dashboard-gaps-2026-09-30.md（缺口分析 B1~B9；本计划承接其 B5/B6/B7 的**代码/采集侧**增量）
  - outbox-trace-continuity.md（异步 trace 断点，本计划 §2.5 引用不再重复设计）
  - legacy-plans/landed/prompt-pipeline-redesign.md（已落地）（同批）
---

# Plan — 可观测埋点补全（代码侧）+ 面板/采集增量

> **只描述要做的事，不含代码改动。**
> 现状：埋点是"**名义五层、实际两层**"——gRPC 双向 + Kafka 双向真通；**DB 层埋点挂错了 processor（等于全废）**、**Redis 层零调用**、**HTTP 出站零埋点**、**异步 trace 在 outbox 处断**。面板侧：19 个自定义指标只画了 2 个——**该部分已有专门计划**（`observability-business-metrics-wiring.md`），本计划**不重复**，只做**埋点代码修复** + 采集侧增量。

> **⚠︎ 与 E2E 路线图的关系**：本计划**独立于 E2E 路线图**——**不登记 E2E-F 账本、不归属任何阶段**（沿用既定口径）。

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件）—— 埋点现状

| 层 | 现状 | 证据 |
|---|---|---|
| gRPC 服务端 | ✅ 5 svc 全挂 `NewServerTracingInterceptor`（仅 unary，**无 stream**） | ai `grpcserver/server.go:124`、analytics `:106`、assessment `:106`、chat `:111`、user `:136`；守卫 `skywalking/svc_wiring_test.go:20` |
| gRPC 客户端 | ✅ 统一 helper `ClientDialOptions` 挂 `NewClientTracingInterceptor`（sw8 注入） | `shared/pkg/grpcinterceptor/client.go:133-138`；接线 ai `grpc_analyzer.go:37`、chat→ai `ai_client_grpc.go:59`、BFF `main.go:375` |
| HTTP 入站 | ✅ gin 中间件（跳过 `/health`、`/internal/`） | `shared/pkg/middleware/gin_skywalking.go:61`；6 svc 挂载 |
| Kafka 生产 | ✅ 仅 chat-svc（`CreateExitSpan`）；dev publisher 无 span | `chat-svc/internal/events/kafka_publisher.go:133`；`dev_publisher.go` 无 |
| Kafka 消费 | ✅ ai/analytics 从 header sw8 重建父 trace | ai `consumer.go:134`、analytics `consumer.go:241` |
| **DB（GORM）** | ❌ **错挂 `Raw()` processor → ORM 全无 span** | `shared/pkg/skywalking/gorm_tracing.go:27-32`（详见 §0.2） |
| **Redis** | ❌ **`InitRedis` 零生产调用** | `shared/pkg/skywalking/init.go:46`（grep 仅定义 + 测试） |
| **HTTP 出站** | ❌ **完全无埋点**（裸 `http.Client`，无 exit span/sw8） | ai `aiclient/{fer,sensevoice,xtts}.go`、`analyzer/http_analyzer.go`、`fusion/llm_fuser.go:80,270`；BFF `downstream/{ai,analytics,assessment}.go` |
| **Kafka DLQ 生产** | ❌ 无 span | `ai-svc/internal/consumer/dlq.go`、`analytics-svc/internal/kafka/dlq.go` |
| BFF tracer | ⚠️ **不调 `SetTracer`**，仅局部 `packageTracer` | `web-bff/main.go:188-202` |
| 异步 trace | ❌ relay 用 `context.Background()` + outbox 表无 trace 列 → 入站 trace 断 | `chat-svc/main.go:239`、outbox migration `c001:31-42` |

### 0.2 头条 bug：GORM 埋点挂错 processor（已亲自复核）

`emotion-echo-shared/pkg/skywalking/gorm_tracing.go`：

```go
27  for _, opType := range []string{"create","query","update","delete","row"} {
28      db.Callback().Raw().Before("gorm:"+opType).Register(...)   // ← 全挂到 Raw()
30      db.Callback().Raw().After("gorm:after_"+opType).Register(...)  // ← 全挂到 Raw()
31  }
```

- GORM 的 `Raw` processor **只由 `db.Raw()` / `db.Exec()` 触发**；ORM 的 `Create/Find/Updates/Delete` 各走 `Create()/Query()/Update()/Delete()` processor，**永不进入 Raw**。
- 正确写法应按 op 分派到 `db.Callback().Create()/Query()/Update()/Delete()/Row()`。
- **代码注释与实现自相矛盾**：`:12-17` 声称"在每个 op type 的最早期注册"、`:17` 声称"跳过 raw SQL"，而实现**恰恰只挂在 raw processor**。
- 后果：5 个服务（`InitGORM`：ai `main.go:651`、analytics `:356`、assessment `:189`、chat `:436`、user `:231`）埋点**全部形同虚设**；业务侧 `.WithContext(ctx)` 用得多（chat 20 处等），改对后会立刻出 span。
- **为何测试没抓到**：`gorm_tracing_test.go` 只测 `buildDBPeer/contextOrBg`，`init_test.go:17-36` 只断言 `InitGORM>=1`，**无任何断言 processor 位置**。

### 0.3 面板/采集现状（供 §7 划界）

| 项 | 现状 |
|---|---|
| Grafana 看板 | **2 个 JSON / 7 面板**：`emotion-echo-overview.json`（HTTP 4 面板）、`kafka-consumer-lag.json`（lag + DLQ depth）。只引用 `emotion_echo_http_requests_total`/`_duration_seconds_bucket`/`go_goroutines`/`kafka_consumergroup_lag` |
| Prometheus scrape | 7 svc `/metrics` + alertmanager + apisix + oap + kafka-exporter；**`metrics_path:/metrics`，不采 `/health`** |
| Alertmanager | 4 文件 7 规则（kafka-lag / kafka-dlq / outbox-dead / observability-self） |
| **缺失** | 无 postgres-exporter / redis-exporter / node-exporter / blackbox；无 recording rules；`/health` 未采集 |
| 已定义未埋点指标 | `emotion_fusion_calls_total`、`emotion_fusion_duration_seconds`（helper `RecordFusionCall/ObserveFusionDuration` **只在测试被调用**） |
| 已埋点未可视化 | 约 15 个（`fusion_*`、`tts_fallback`、`panic`、`skywalking/model init_failed`、`mv_refresh`、`outbox_*`） |

### 0.4 相关 ADR / 决策

- 无专覆盖"埋点正确性"的 ADR；`observability-sprint-b.md`（landed）确立"埋点 + 告警"范式。
- 约束：本项目**不用 k8s**，`charts/**` 是冻结资产；内存是硬约束（19 容器栈贴顶）→ **本计划只改代码 + 加配置，尽量不新增容器**（exporter 需评估内存，§8 D2）。

### 0.5 架构假设清单

| 假设 | 现状核对 |
|---|---|
| 埋点是"五层" | ❌ 名义五层、实际两层（gRPC + Kafka） |
| GORM 埋点生效 | ❌ 挂错 processor，全废（§0.2） |
| Redis 埋点生效 | ❌ 零调用 |
| 异步链路 trace 连续 | ❌ 断在 outbox（已有 `outbox-trace-continuity.md`） |

---

## §1 问题陈述

一句话：**埋点"看着全、其实漏"**——简历/文档里的"多模态全链路埋点"，实际 DB/Redis/HTTP 出站/异步四层都有硬断点，其中 **GORM 挂错 processor 是全废级 bug 且无测试覆盖**。**"可观测"号称是亮点，但底座的埋点本身没坐实。**

面试视角：这是把"我接入了 SkyWalking"升级为"我**验证**了埋点真的出 span"的关键——而现状恰恰是"接了但没验证，所以错了也没人发现"。

---

## §2 目标与非目标

### 目标（代码侧为主）

- **G1 修 GORM 埋点**：挂到正确的 `Create/Query/Update/Delete/Row` processor + **加回归钉**（断言注册位置/ORM 查询出 span）。
- **G2 接 Redis 埋点**：`InitRedis` 接到生产 redis client（web-bff `main.go:614` 的 authlock client）。
- **G3 补 HTTP 出站埋点**：统一 `http.RoundTripper`（sw8 注入 + exit span），覆盖 ai-svc aiclient 与 BFF downstream。
- **G4 补 Kafka DLQ 生产 span**（ai/analytics dlq.go）。
- **G5 补/删"已定义未埋点"指标**：`emotion_fusion_calls_total` / `_duration_seconds` 要么接线要么删定义。
- **G6 BFF tracer 对齐**：web-bff 也 `SetTracer`（或明确记录为何不设，消除"5 svc 守卫不覆盖 BFF"的盲区）。
- **G7 采集/面板增量（缩范围）**：新增 `/health` scrape 或 blackbox（B6/B7 代码侧）；其余面板**交接** `observability-business-metrics-wiring.md`。

### 非目标

- **不做面板/告警本身**（`observability-business-metrics-wiring.md` 已覆盖，**勿重复**）。
- **不改 19 个已有指标的定义**（那些已齐全，只是没人看）。
- **不重复设计异步 trace 方案**（`outbox-trace-continuity.md` 已有；本计划只列为依赖）。
- **不动 `charts/**`**（k8s 冻结）。
- 不登记 E2E-F、不挂阶段。

---

## §3 设计（要点）

### 3.1 GORM 修复（G1）

- `InstrumentGORM` 改为按 op 分派：
  ```go
  db.Callback().Create().Before("gorm:create").Register("skywalking:begin_create", ...)
  db.Callback().Query().Before("gorm:query").Register(...)
  // update / delete / row 同理
  ```
- 保留 `makeBeginCallback/makeEndCallback`（逻辑本身没问题）。
- **回归钉**：断言 `db.Callback().Query().Get("skywalking:begin_query") != nil`（或执行一次 ORM 查询断言产生 span）。

### 3.2 HTTP 出站（G3）

- 新增 `shared/pkg/skywalking/http_tracing.go`：`TracingRoundTripper`（建 exit span + 注入 `sw8` header + 结束 span）。
- 各处 `http.Client{Transport: ...}` 接入；或提供 helper `NewTracedClient(tgr)`。
- 覆盖范围：ai-svc aiclient（FER/SenseVoice/XTTS/HTTP analyzer/llm_fuser）、BFF downstream HTTP fallback。

### 3.3 其余

- Redis：`InitRedis(client)` 传入 web-bff 的 go-redis client。
- DLQ：在 dlq.go 发布前 `CreateExitSpan(ctx,"kafka-publish-dlq",topic)`。
- 指标：`RecordFusionCall/ObserveFusionDuration` 接到 fusion worker 路径，或删指标定义（§8 D3）。
- 采集：`/health` 加 blackbox 或加 job；评估是否加 pg/redis exporter（§8 D2）。

---

## §4 分阶段实施（TDD）

| Phase | 内容 | RED 断言 |
|---|---|---|
| 0 | 基线：起 obs 栈，`curl :9090 .../label/__name__/values` 核对指标真实存在 | — （人工） |
| 1 | **GORM 修复**：改 processor + 回归钉 | 先写 RED（断言 Query processor 有回调/ORM 出 span）→ 改 → GREEN |
| 2 | HTTP 出站埋点 | 单测：RoundTripper 产出 exit span + 注入 sw8 |
| 3 | Redis 埋点接线 | 单测/该 client hook 装配断言 |
| 4 | DLQ span | 单测：dlq 发布建 span |
| 5 | 指标接线或删除 | 单测：worker 路径递增指标 |
| 6 | BFF SetTracer 对齐 | 守卫：BFF 也纳入 tracer 断言 |
| 7 | 采集增量：`/health` scrape / blackbox | smoke：target up = 1 |
| 8 | 负向对照 + 回归钉：制造一次 ORM 查询/出站调用，**在 SkyWalking UI 看到 span** | 截图/查询作证据 |

---

## §5 验收与回归钉

- **回归钉（核心）**：GORM processor 断言 + 真实 ORM 查询在 SkyWalking 出 span（**这是"埋点真生效"的硬证据**，不同于"代码里调了 InitGORM"）。
- **单测**：HTTP RoundTripper / DLQ span / 指标接线。
- **负向对照**：修前复现"ORM 无 span"，修后有 —— 证明测试有效。
- **smoke**：`scripts/smoke_observability.py`（若有 target/面板断言）保持绿；`/health` target up。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **改埋点影响性能** | 每次 ORM/HTTP/Redis 都建 span，量级放大 | span 采样 + 只在 `.WithContext(ctx)` 有 ctx 时建（无 ctx 不建，避免根 span 泛滥） |
| **GORM 版本差异** | processor 名（`gorm:query` 等）随版本变 | 以当前 go.mod 版本核对（改前 grep GORM 版本） |
| **HTTP 出站改法分散** | 多处裸 client，易漏 | 统一 helper + 守卫扫描"裸 client 无 Transport" |
| **Span 重名/嵌套** | 出站 + gRPC 双重埋点重复 | 明确分层（HTTP 出站 vs gRPC 出站各一） |
| **内存约束** | 新增 exporter 容器可能击穿内存 | **本计划优先代码/配置，不新增容器**；exporter 需评估（D2） |
| **无 ctx 的 DB 调用** | 一部分 ORM 调用没 `.WithContext` → 仍无 span | 视为预期（无父 trace 不出 span），文档说明边界，不硬造根 span |
| **面板部分重复** | 与 `observability-business-metrics-wiring.md` 撞车 | §7 严格划界，本计划**不碰面板 JSON/告警规则** |

---

## §7 与既有计划的关系（严格划界）

| 计划 | 范围 | 与本文边界 |
|---|---|---|
| **`observability-business-metrics-wiring.md`（planned）** | **面板/告警**（新增 4 看板 + 多条规则），自述"**不改任何 Go 埋点**" | **本计划 = 它的另一半**（改埋点代码）。**本计划不写任何面板 JSON / 告警规则**，全部交接那份 |
| **`grafana-dashboard-gaps-2026-09-30.md`（planned）** | 缺口分析 B1~B9 | 本计划承接 **B5（APISIX 指标）/B6（pg/redis exporter）/B7（recording rules）的代码/采集侧**；B1~B4 面板归 business-metrics-wiring |
| **`outbox-trace-continuity.md`（planned）** | 异步 trace 断点（outbox trace 列 + relay 恢复） | 本计划 §0.1 列为**依赖**，**不重复设计** |
| **`observability-sprint-b.md`（landed）** | 现有 7 面板 + 4 规则 | 历史，不据此开工 |

> **一句话边界**：**埋点代码 = 本文；面板与告警 = `observability-business-metrics-wiring.md`**。两者合起来才是"可观测坐实"。

---

## §8 待拍板

| # | 事项 | 说明与建议 |
|---|---|---|
| D1 | span 采样策略 | 全采 vs 采样率；建议先全采（dev），生产评估 |
| D2 | 是否新增 pg/redis/node exporter | 内存硬约束下需评估；**建议仅 redis-exporter（web-bff 有 Redis）**，pg 用现有 metrics 替代或缓做 |
| D3 | `emotion_fusion_calls_total/duration`：接线 vs 删 | 若 fusion 有关注价值则接线，否则删（避免"定义了没数据"的假象） |
| D4 | web-bff 是否 `SetTracer` | 建议对齐（消除盲区），或文档明确记录原因 |
| D5 | `/health`：blackbox vs 直接 scrape | blackbox 更标准但要新容器；直接 scrape 更省（D2 相关） |

---

## §9 调研依据（本次实读，不凭记忆）

- **GORM**：`emotion-echo-shared/pkg/skywalking/gorm_tracing.go`（**亲自读过全文，§0.2 逐行复核**）、`init.go`、`gorm_tracing_test.go`、`init_test.go`
- Redis：`shared/pkg/skywalking/{init.go,redis_tracing.go}`；web-bff `main.go:614`
- HTTP 出站：ai `internal/aiclient/*.go`、`internal/analyzer/http_analyzer.go`、`internal/fusion/llm_fuser.go`；BFF `internal/downstream/{ai,analytics,assessment}.go`
- Kafka/DLQ：chat `internal/events/{kafka_publisher.go,dev_publisher.go}`、ai `internal/consumer/{consumer.go,dlq.go}`、analytics `internal/kafka/{consumer.go,dlq.go}`
- 指标：`shared/pkg/metrics/{metrics.go,fusion_metrics.go,tts_metrics.go}`、各 svc 埋点调用点
- 面板/采集：`deploy/grafana/**`、`deploy/prometheus/{prometheus.yml,rules/*.yml}`、`deploy/alertmanager/alertmanager.yml`
- 计划对账：`observability-business-metrics-wiring.md`、`grafana-dashboard-gaps-2026-09-30.md`、`outbox-trace-continuity.md`

**未能确认**（需实机）：当前运行态各指标是否已有 series（部分 counter 可能未触发）；GORM 版本对应的 processor 名；内存余量是否够 redis-exporter。
