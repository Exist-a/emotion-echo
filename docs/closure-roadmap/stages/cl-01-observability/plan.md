---
stage: cl-01
title: 可观测闭环（埋点真生效 + 采集 → 面板 → 告警真消费）
type: transformation
status: in-progress
created: 2026-10-14
last-updated: 2026-10-14（建档 B 轨并行开工 → Phase 1 GORM 修复落地 → [M] 裁定回填；**仍未起 dev 栈**，"埋点真生效"待 Phase 0/9 取证，见 §A.4）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；本阶段 6 个 [M] 决策点见 §7（均不阻塞"代码+单测"部分）
source:
  - docs/plans/observability-instrumentation-and-dashboards.md（埋点代码侧，主）
  - docs/plans/observability-business-metrics-wiring.md（面板/告警实施版）
  - docs/plans/grafana-dashboard-gaps-2026-09-30.md（缺口 B1~B9）
related-plans:
  - docs/plans/redis-two-layer-cache.md（roadmap §2 挂在本阶段，但与 §4 范围存在张力 → §7 D-06）
  - docs/plans/outbox-trace-continuity.md（异步 trace 断点 = CL-03，本阶段只做"埋点本身真生效"）
---

# CL-01 · 可观测闭环 — 执行计划

> **核心口径（roadmap §4 CL-01）**：验证 **"那些指标/埋点是否都真的接入了面板并投入使用"** ——
> 端到端跑通「**埋点 → 暴露 → 采集 → 面板 → 被看/告警**」，**杜绝"埋了点没人看"**。
> 别的不用多说，就盯一条：**每个自定义指标都要在面板/告警上被真正消费到**（不全则不算闭环）。
>
> **现状一句话**：埋点是"**名义五层、实际两层**"——gRPC 双向 + Kafka 双向真通；
> **DB 层挂错 GORM processor（等于全废）**、**Redis 层零生产调用**、**HTTP 出站零埋点**；
> 面板侧：19 个自定义指标只画了 2 个。**"可观测"是简历亮点，但底座埋点本身没坐实。**
>
> **执行协议**：[RUNBOOK.md](../../RUNBOOK.md)（六步循环 / 证据有效性 / 收口契约）
> + [parallel-contract.md](../../parallel-contract.md)（**B 轨**占位与文件面隔离）
> + [anti-patterns.md](../../../e2e-roadmap/anti-patterns.md)（14 类失真反例）。
>
> **并行**：本阶段为 **B 轨**，与 **A 轨（CL-04）** 并行。文件面冲突经亲核为**零**（§A.2 末尾）；
> 唯一共享资源是 dev 栈 —— A 轨持 `deploy/.devmode-session` 到 **2026-10-14 12:00**，本阶段"需栈"的 Phase 排在锁释放后（§3 Phase 0/9）。
>
> **阶段 PR**：[#211](https://github.com/Exist-a/emotion-echo/pull/211)（draft）—— 按「每阶段开工先建 PR」约束（AGENTS §七 / RUNBOOK §1 前置检查 ④）创建。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件，本次实读亲核）

| 文件 | 关键事实（file:line，已亲自 Read/Grep） |
|---|---|
| `emotion-echo-shared/pkg/skywalking/gorm_tracing.go` | `InstrumentGORM` 在 `:27-32` **把 5 个 op 的 begin/end 回调全注册到 `db.Callback().Raw()`** —— GORM 的 `Raw` processor 只由 `db.Raw()/Exec()` 触发，ORM 的 `Create/Find/Updates/Delete` 各走 `Create()/Query()/Update()/Delete()`，**永不进 Raw** ⇒ ORM 查询零 span。文件头注释 `:12-19` 自述"在每个 op type 最早期注册 / 跳过 raw SQL"，**与实现自相矛盾**（实现恰恰只挂 raw）。 |
| 同上（回调体） | `makeBeginCallback` `:36-58` / `makeEndCallback` `:61-78` 逻辑本身可用（`InstanceSet/Get` 存 end 闭包）；`buildDBPeer` `:84-100` 用 dialect+Schema.Name 拼 peer。**修复只需改注册位置，回调体不动**（`gorm.query` 名字不变，端侧 `makeBeginCallback("query",…)` 已按 op 传参）。 |
| `emotion-echo-shared/pkg/skywalking/init.go` | `InitGORM` `:25-33`（返回 1 作"调用发生过"信号，5 svc 已接线）；`InitRedis` `:46-52`（nil-safe，返 bool）。 |
| 同上 —— **Redis 零调用实证** | `grep -rn "InitRedis\|InstrumentRedis" --include=*.go` 生产代码命中 **0**（仅 `init.go` 定义 + `init_test.go` 测试）。 |
| `emotion-echo-shared/pkg/skywalking/redis_tracing.go` | `InstrumentRedis` `:16-22` + `redisTracingHook`（`ProcessHook` `:35-52` / `ProcessPipelineHook` `:55-77`）**实现完整可用**，只是没人挂。 |
| `emotion-echo-shared/pkg/skywalking/tracing.go` | `createExitSpan` `:23-41` 用 `tgr.CreateExitSpanWithContext(ctx,…)`；**ctx 里无 span 时 go2sky 的返回行为待验证**（T-08）——现实现经 `contextOrBg` 在 nil ctx 时回落 `Background`（`:41/102-107`），**可能不建 span**（依赖 go2sky 语义），与 CL-01 约束③"不硬造根 span"方向一致但需钉死。 |
| `emotion-echo-shared/pkg/skywalking/svc_wiring_test.go` | 守卫 `:20-50` **只覆盖 5 个 svc 的 `SetTracer`**（user/chat/ai/assessment/analytics）；**不含 web-bff** —— 与 G6 盲区一致。 |
| `emotion-echo-shared/pkg/skywalking/gorm_tracing_test.go` | 现测试**只测 `buildDBPeer` / `contextOrBg` / nil-tracer 不 panic**（`:15-150`），**无任何 processor 注册位置断言** ⇒ 挂错 processor 无人抓。`init_test.go:19-37` 只断言 `InitGORM>=1`。 |
| `emotion-echo-web-bff/main.go` | `packageTracer` 在 `:86` 声明、`:201` 赋值，**全文件无 `SetTracer`**（`grep` 确认）⇒ BFF 的 gRPC/HTTP 出站走各自拦截器能拿 tracer，但**没有对齐到 shared `Tracer()`**（G6）。`buildRedisClient()` `:605-615` 建 client（**受 `LOGIN_LOCK_BACKEND=redis` 门控，否则返 nil**），main `:282` 持有、与 authlock/health 共用。 |
| `emotion-echo-ai-svc/internal/aiclient/{fer,sensevoice,xtts}.go` | 三处裸 `&http.Client{Timeout: …}`（`fer.go:58` / `sensevoice.go:53` / `xtts.go:76`），**无 Transport** ⇒ HTTP 出站零埋点、无 sw8 注入。 |
| `emotion-echo-ai-svc/internal/analyzer/http_analyzer.go` | `:62` 裸 `http.Client{}`（无 Transport）。 |
| `emotion-echo-ai-svc/internal/fusion/llm_fuser.go` | `:80` 裸 `http.Client{Timeout: timeout}`（注释 `:62-63` 说明刻意复用连接池）；**fusion LLM 出站无 span**。 |
| `emotion-echo-ai-svc/internal/fusion/fusion_metrics.go` | `RecordFusionCall` `:49-51` / `ObserveFusionDuration` `:54-56` —— `grep` 全仓**生产 0 caller**（仅 `fusion_metrics_test.go:71-86` 调用）⇒ 两指标"定义了没数据"（G5）。 |
| `emotion-echo-web-bff/internal/downstream/` | HTTP 降级路径（`ai.go`/`analytics.go`/`assessment.go`/`chat.go` 等）走裸 client ⇒ 出站零埋点（需与 gRPC 路径分层，避免双埋点）。 |
| `scripts/smoke_observability.py` | **已具备**：① `EXPECTED_TARGETS` 清单 `:43-53` **+ 直接从 `prometheus.yml` 解析比对**的断言（`:26-40` 注释记述 llm-service 曾"双方都通过"的缺口教训）；② 面板 expr 断言机制 `_panel_exprs`（把每个面板 expr 真打到 Prometheus、要求非空 series）；③ Loki 新鲜度窗口 `FRESH_WINDOW=300`。**这是本阶段机器回归钉的落点。** |
| `deploy/prometheus/rules/` | 现 **4 文件 7 规则**（`kafka-lag` / `kafka-dlq` / `outbox-dead` / `observability-self`）。 |
| `deploy/grafana/dashboards/` | 现 **2 看板**（`emotion-echo-overview.json` / `kafka-consumer-lag.json`），**零引用 15+ 业务指标**。 |
| `deploy/docker-compose.infra.yml` | obs 服务：`prom/prometheus:v2.51.2` `:435`、`prom/alertmanager:v0.27.0` `:505`、`danielqsj/kafka-exporter@sha256:…` `:536`（**已钉 digest**）、`grafana/grafana:10.4.2` `:566`、`grafana/loki:3.2.0` `:600`、`grafana/promtail:3.2.0` `:622`。**无 pg/redis/node exporter、无 blackbox**（G7/D2）。 |

### A.2 相关 ADR / 决策（已查）

- **已查** `docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md`（Loki docker_sd 采集决策，`/query`→`/query_range` 迁移关联 E2E-F-147）：本阶段做日志看板须沿用 `/query_range`。
- **无** 专覆盖"埋点正确性 / 指标→面板映射"的 ADR（`grep adr-2026-09-…observ|trace|metric|grafana` 仅命中 loki 一篇）⇒ **若本阶段引入架构级改动（如统一 RoundTripper 落点、是否引入 exporter）须补 ADR**（anti-patterns §架构改动无 ADR）。
- **已查** `docs/closure-roadmap/decisions.md`：CL-D-01/02/03/04/05/06/07 均**已定**；**无门阻塞 CL-01**。本阶段新决策续 **CL-D-xx**（与 E2E 的 `D-NN`、`architecture/decisions.md` 的「决策 N」三套编号隔离）。
- **已查** `docs/closure-roadmap/roadmap.md` §4 CL-01 约束（写死 5 条）：① 不改指标语义；② 内存红线（**默认不新增容器**）；③ **span 只在有 ctx 时建、不硬造根 span**；④ 面板以既有计划为准（不重复造）；⑤ **诚实性——"埋点接了但没验证"不算完成，必须见 span 真出现在 SkyWalking UI**。
- **已查** `docs/plans/observability-instrumentation-and-dashboards.md` §7 划界：**埋点代码 = 该计划；面板与告警 = `observability-business-metrics-wiring.md`** —— 本阶段**两者都做**（roadmap §2 已把两份都挂 CL-01），但**严格按此分工落文件**（改 Go 埋点 vs 改面板 JSON/告警 YAML）。
- **并行文件面隔离核对（亲核）**：本阶段主文件面 = `emotion-echo-shared/pkg/skywalking/**` + 各 svc `main.go` + BFF `downstream/{ai,analytics,assessment}.go` + `emotion-echo-ai-svc/internal/{aiclient,analyzer,fusion}/**` + `deploy/{prometheus,grafana,alertmanager,loki}/**` + `scripts/smoke_observability.py`。**A 轨（CL-04）文件面** = BFF `handler/ai_stream_handler.go` + `downstream/llm_grpc.go` + `emotion-llm-service/*.py` + 前端 `composables/{useAIStreamHandler,useAIStream,usePrompt}.ts`。**交集 = ∅**（本阶段碰 `downstream/{ai,analytics,assessment}.go`，A 碰 `downstream/llm_grpc.go` —— **不同文件**，符合 parallel-contract §1.2 "同名文件串行、不同文件可并"）。

### A.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| 埋点是"五层全通" | ❌ **名义五层、实际两层**：只有 gRPC 双向 + Kafka 双向真通；DB/Redis/HTTP 出站三层有硬断点（A.1）。 |
| GORM 埋点生效 | ❌ **挂错 processor，全废**（A.1 `gorm_tracing.go:27-32`，已亲核）。5 svc（ai `main.go:651`/analytics `:356`/assessment `:189`/chat `:436`/user `:231`）全调了 `InitGORM`，但底层注册到 Raw ⇒ 5 svc ORM 查询全无 span。 |
| Redis 埋点生效 | ❌ `InitRedis` 零生产调用（A.1 grep 实证）。 |
| 指标埋点后就能被看 | ❌ 19 个自定义指标只 2 个有面板；且 2 个 fusion 指标**定义了从未被生产调用** ⇒ "有指标名、无数据"。 |
| HTTP 出站有埋点 | ❌ 全部裸 `http.Client`（无 Transport），无 exit span、无 sw8 传播。 |
| BFF 与 5 svc 同名装配纪律 | ❌ 5 svc 有 `SetTracer` 守卫，**BFF 不在 `svc_wiring_test.go` 覆盖内**且确实没调（G6 盲区）。 |
| 面板改动可随手在 UI 调 | ❌ provider `allowUiUpdates:true` + `updateIntervalSeconds:30` + 持久卷 ⇒ **UI 改 30s 后被磁盘 JSON 覆盖**，一切改动必须落 `deploy/grafana/dashboards/*.json`。 |
| 埋点仍是"接了就算完" | ❌ CL-01 约束⑤：**必须见 span 真出现在 SkyWalking UI**，否则不算完成。 |
| **无 ctx 调用会"硬造根 span"** | ⚠️ go2sky 默认采样率=1.0（`NewDynamicSampler(1)`），ctx 无父 span 时会**新建根 trace** ⇒ 必须显式守卫。本次已在 `createExitSpan` 加 `go2sky.ActiveSpan(ctx)==nil → noop`（约束③/T-08 落地），否则 Phase 1 的 GORM 修复会为无 ctx 的 ORM 调用刷出大量孤立 trace。 |

### A.4 未跑 smoke / 未取运行时证据声明（诚实性，按 RUNBOOK §4）

**本次建档只做静态核实**：**未启动 dev 栈（19 容器）**、**未在 SkyWalking UI 看到一个 span**、**未跑 `scripts/smoke_observability.py`**、**未 `curl :9090/api/v1/label/__name__/values` 核对指标真实存在**。
⇒ 因此本文所有"现状无 span / 零调用"结论**依据是源码静态分析**（file:line 见 A.1），非运行时观测。
**Phase 0（基线取证，需栈）完成后须回填真实证据**；若运行结论与源码冲突，**以运行时为准并回改本文**（RUNBOOK §反模式底线「根因臆断」）。

---

## §1 目标与非目标

### 目标（对齐 roadmap §4 DoD）
- **G1 修 GORM 埋点真生效**：把回调从 `Raw()` 改挂正确的 `Create/Query/Update/Delete/Row()` processor + **补回归钉**（断言注册位置 + ORM 查询真出 span）。
- **G2 Redis 埋点接线**：`sharedskywalking.InitRedis(redisClient)` 接到 BFF 持有的 redis client（`main.go:282` 处）。
- **G3 补 HTTP 出站埋点**：新增统一 `TracingRoundTripper`（建 exit span + 注入 `sw8`），覆盖 ai-svc aiclient（FER/SenseVoice/XTTS）+ `http_analyzer` + `llm_fuser` + BFF downstream 的 **HTTP 降级路径**。
- **G4 补 Kafka DLQ 生产端 span**（ai `internal/consumer/dlq.go` / analytics `internal/kafka/dlq.go`）。
- **G5 treatment 两个"已定义未埋点"指标**：`emotion_fusion_calls_total` / `emotion_fusion_duration_seconds` **接线或删定义**（§7 D-03）。
- **G6 BFF 装配对齐**：web-bff 也 `SetTracer`（并把 BFF 纳入装配守卫），或**明确记录为何不设**，消除盲区（§7 D-04）。
- **G7 面板/告警/采集（"被看"侧，承接 business-metrics-wiring）**：① 建 business/reliability/observability/logs 看板；② 补告警规则（降级/panic/init 失败/黑洞/5xx）；③ 采集增量（`/health` scrape 或 blackbox，§7 D-02/D-05）。

### 非目标（范围 Out）
- **不改 19 个已定义指标的语义**（只补齐"被消费"）。
- **不动 `charts/**`**（k8s 冻结资产）。
- **默认不新增容器**（内存红线 —— 19 容器栈贴顶，见 E2E-16 `.wslconfig` 记录）。exporter 需按 D-02 评估。
- **不重复造面板**：面板以 `observability-business-metrics-wiring.md` 为准落地，本计划不另起炉灶。
- **异步 trace 连续性（outbox 断点）= CL-03**，本阶段只做"埋点本身真生效"（含 DLQ span），**不重复设计** outbox trace 列。
- **gRPC 死链清理 = CL-02**，不在本阶段。

---

## §2 测试点清单（每点标 `[A]` / `[V]` / `[M]`）

> 判定分级见 RUNBOOK §4；`[M]` = 仅用户可裁（汇总见 §7）。

### A 组 · 埋点真生效（代码侧）

| # | 测试点 | 判定 | 验证方式 | 证据（Phase 完成后回填） |
|---|--------|------|---------|------|
| T-01 | GORM 回调注册在**正确的 processor**（Query/Create/Update/Delete/Row） | [A] | Go 单测：`db.Callback().Query().Get("skywalking:begin_query") != nil`（**修前 nil=RED**），5 op 全覆盖 | ✅ 2026-10-14：`go test ./pkg/skywalking/ -count=1` → **ok**；修前 RED 已留痕（负向对照）；另修 raw 单列（`begin_raw`，analytics 22 处 `db.Raw/Exec` 用） |
| T-02 | **真实 ORM 查询在 SkyWalking UI 出 span**（修前 0 → 修后 >0） | [A] | 起栈触发一次 `.WithContext(ctx)` 的 ORM 查询 → SkyWalking UI/GraphQL 查询 trace 含 `gorm.query` | UI 查询结果/截图（**约束⑤硬证据**） |
| T-03 | Redis 埋点接线：BFF 装配 `InitRedis` + 真 Redis 命令出 span | [A] | 装配断言（BFF main 调用点存在）+ 起栈后 Redis 命令出 `redis.GET` span | ✅ 2026-10-14：实现迁 `pkg/skywalking/redistrace`（**无 gorm**）；BFF `main.go` 在 `buildRedisClient()` 后调 `redistrace.InitRedis(redisClient)`；hook/joinCmds/nil-safe 单测随包迁移全绿。**待起栈验 `redis.*` span**（受锁） |
| T-04 | HTTP 出站 `TracingRoundTripper`：建 exit span + 注入 `sw8` | [A] | 单测（httptest server 收 `sw8` 头非空 + span 产生）；**覆盖清单核对**（ai aiclient×3 / http_analyzer / llm_fuser / BFF downstream） | ✅ 2026-10-14：组件+单测（`trace` 包 6 用例）+ **接线 8 处**（ai-svc：fer/sensevoice/xtts/http_analyzer/llm_fuser；BFF：downstream/{ai,analytics,assessment}，即契约 §1.2 划给 B 轨的三处）。**未纳入**：BFF `chat.go`（C 轨）、`llm.go`/`xtts.go`/`tts_provider.go`/`user.go`/`handler/health_handler.go`（未在 B 轨文件面，留待后续） |
| T-05 | Kafka DLQ 生产建 span（ai / analytics） | [A] | 单测：dlq 发布路径建 `CreateExitSpan` | 单测输出 |
| T-06 | fusion 两指标：**接线**（生产路径递增）**或删除定义** | [M] | 按 D-03 结论：接线 → 单测断言 worker 路径递增；删 → 断言定义已移除且无悬空引用 | D-03 结论 + 单测 |
| T-07 | web-bff `SetTracer` **对齐**（含纳入守卫）**或记录不设原因** | [M] | 按 D-04：对齐 → 扩展 `svc_wiring_test.go` 含 BFF；不设 → 文档记录 | ✅ 2026-10-14（D-04=对齐）：BFF `main.go` 成功分支调 `sharedtrace.SetTracer(t)`；`svc_wiring_test.go` 扩为表驱动、**含 web-bff**（断言 `sharedtrace.SetTracer(`）；测试全绿 |
| T-08 | 无 ctx 边界：ORM/HTTP 调用无 ctx 时**不硬造根 span** | [A] | 验证 go2sky `CreateExitSpanWithContext` 无父 span 时语义（不建 span/noop）；守卫断言之 | ✅ 2026-10-14：查实 go2sky 默认采样=1.0 ⇒ 无父 span 会**新建根 trace**；已在 `createExitSpan` 加 `ActiveSpan(ctx)==nil` 守卫；单测 `NoSw8_WhenNoParentSpan` 覆盖（HTTP 侧），GORM/Redis 同受该守卫保护 |

### B 组 · 面板/告警**被真正消费**

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-09 | 新看板 JSON 落地（business / reliability / observability / logs） | [A] | 文件存在 + Grafana 加载（`GET :13000/api/search` 含新 UID） | ⚙️ 2026-10-14：**observability 看板已落**（`deploy/grafana/dashboards/emotion-echo-observability.json`，3 面板：Target 健康矩阵/Down 计数/Firing 计数）；business/reliability/logs 待 Phase 7（需栈核指标名） |
| T-10 | **每个面板 expr 返回非空 series** | [A] | 扩展 `scripts/smoke_observability.py`（复用 `_panel_exprs`） | ⚙️ 2026-10-14：smoke 的看板断言列表已加入 `emotion-echo-observability`（3 面板 expr 均用 `or vector(0)` 保证零值时仍非空）；**待起栈实跑验证** |
| T-11 | 打开 business 看板能答"**是否在降级 / 是否在跟 mock 对话**" | [V] | 人看截图：`fusion_fallback_total` 曲线可见 | 截图（人核）——待 Phase 7 |
| T-12 | 新告警规则**可加载** | [A] | `promtool check rules`（有则用）或 Prometheus 启动日志无加载错 | 命令输出 |
| T-13 | **一条告警真触发并送达**（负向对照） | [A] | 故意制造条件（如停 target / 触发 fallback）→ 断言 `:9093` Alertmanager 收到 firing | Alertmanager API/截图（**对抗"面板有数据≠告警链路通"**） |
| T-14 | Loki trace_id 面板实机跑通 | [A] | `{job="services"} \|= "<trace_id>"` 走 **`/query_range`** 返回行 | 查询输出 |

### C 组 · 采集与守卫

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-15 | 采集增量：`/health` 进监控（scrape 或 blackbox）→ target `up=1` | [M] | 按 D-05：加 job/blackbox → 断言 `up{job=…}=1` | D-05 结论 + smoke 输出 |
| T-16 | 是否新增 pg/redis exporter（内存红线评估） | [M] | 按 D-02 结论；若加 → 内存实测不击穿 + target up | D-02 结论 |
| T-17 | `scripts/smoke_observability.py` **全绿**（含 targets 与 prometheus.yml 解析比对） | [A] | 跑脚本、退出码 0 | 脚本输出 |
| T-18 | 装配守卫仍绿 + BFF 纳入（若 T-07 定对齐） | [A] | `go test ./emotion-echo-shared/pkg/skywalking/...` | 单测输出 |

### D 组 · 范围

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-19 | `redis-two-layer-cache` 是否纳入本阶段 | [M] | 按 D-06 结论（roadmap §2 挂了、§4 只提"埋点接线"） | D-06 结论 |

---

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

> 微序对齐 roadmap §1「T1 内部微序：① 埋点修复 → ② gRPC∥Kafka → ③ 面板/告警收尾」——
> **本阶段 = ① + ③**（② 的 gRPC 归 CL-02、Kafka 归 CL-03）。
> **栈依赖标注**：🟢=不需栈（worktree 内可做） / 🔴=需 dev 栈（受 `.devmode-session` 约束）。

| Phase | 内容 | 栈 | RED 断言 |
|---|---|---|---|
| **0** | **基线取证**：起 obs 栈；`curl :9090/api/v1/label/__name__/values \| grep -E "emotion\|fusion"` 逐条核对指标真实存在（**不凭记忆写指标名**）；**负向对照**记录"修前"：一次 ORM 查询在 UI **无** span、`InitRedis` 无调用、`/health` 无 target；面板现状 2 看板 | 🔴 | —（人工取证，回填 A.4） |
| **1** | **GORM 修复（G1）** | 🟢 | RED：`TestInstrumentGORM_RegistersOnOpProcessors` 断言 `Callback().Query().Get("skywalking:begin_query") != nil` —— **修前 RED**（现挂 Raw）→ 改 `gorm_tracing.go:27-32` 按 op 分派 → GREEN |
| **2** | **HTTP 出站（G3）**：新增 `shared/pkg/skywalking/http_tracing.go` `TracingRoundTripper` + helper `NewTracedClient(tgr)` | 🟢 | RED：`httptest` server 断言收到非空 `sw8` 头 + `createExitSpan` 被调用 → GREEN → 逐处接线（ai aiclient×3 / http_analyzer / llm_fuser / BFF downstream） |
| **3** | **Redis 埋点接线（G2）** | 🟢（装配）/ 🔴（出 span 验证） | RED：装配断言 —— BFF main 在 `redisClient` 非 nil 时调 `InitRedis`；单测：`InitRedis(rdb)` 挂 hook（沿用 `init_test.go:52` 契约） → GREEN |
| **4** | **DLQ span（G4）** | 🟢 | RED：单测断言 dlq 发布路径调用 `CreateExitSpan` → GREEN |
| **5** | **fusion 指标处置（G5）**（依赖 D-03） | 🟢 | 接线：RED 断言 worker 路径 `Inc()`；删：RED 断言定义移除 → GREEN |
| **6** | **BFF SetTracer（G6）**（依赖 D-04） | 🟢 | RED：扩展 `svc_wiring_test.go` 覆盖 `emotion-echo-web-bff/main.go` → 对齐则 GREEN；不设则文档化 |
| **7** | **面板/告警/日志看板（G7）**（承接 business-metrics-wiring §3） | 🟢（写 JSON/YAML）/ 🔴（expr 非空验证） | RED：扩展 `smoke_observability.py` 断新看板 UID 可读 + 每 expr 非空 → 此时必 RED → 加 4 看板 JSON + 告警规则 → GREEN |
| **8** | **采集增量**（依赖 D-02/D-05） | 🔴 | RED：smoke 断 `/health` target `up=1` → 加 job → GREEN |
| **9** | **负向对照 + 收口**：真触发一条告警 → firing → 送达（T-13）；**在 SkyWalking UI 看到 gorm/redis/http/dlq span**（T-02 硬证据）；跑全 smoke | 🔴 | 观测记录 + 截图 |

> **关键洞察（评审须知）**：`TracingRoundTripper` 是**新增共享组件**（多处复用），落点选择（shared pkg vs 各 svc）若被认为是架构决策，须补 ADR（本计划倾向放 `shared/pkg/skywalking/`，与 `createExitSpan` 同包）。
> **负向对照是硬要求**：T-02（修前无 span / 修后有）与 T-13（告警真送达）都是"负向对照"——对齐 `observability-business-metrics-wiring.md` §4 Phase 4 与 E2E-22 做法，**防假绿**。

---

## §4 收口判据（"做到什么程度"，对齐 roadmap §4 DoD）

- **功能判据**：
  1. ORM 查询在 SkyWalking **出 span**（修前无 / 修后有）；
  2. HTTP 出站**出 exit span**（sw8 可传播）；
  3. 面板能答"**是否在降级 / 是否在跟 mock 对话**"；
  4. **一条告警真触发并送达**（T-13）。
- **机器判据**：GORM processor 回归钉 + ORM 出 span；RoundTripper 单测；DLQ span 单测；`SetTracer` 装配断言；看板**每个 expr 返回非空 series**（`smoke_observability.py`）；**负向对照**（故意触发 → firing → 送达）。
- **诚实性判据（约束⑤）**：**必须见 span 真出现在 SkyWalking UI** —— "代码里调了 InitGORM"**不算**。

---

## §5 回归钉

- **Go 单测**：`gorm_tracing_test.go`（processor 位置）、`http_tracing_test.go`（新）、`dlq` span、`init_test.go`（Redis 装配）、`svc_wiring_test.go`（BFF 纳入）。
- **Python smoke**：`scripts/smoke_observability.py` 扩展（新看板 UID + 每面板 expr 非空 + `/health` target up）。
- **配置校验**：`promtool check rules`（如环境有）或 Prometheus 加载日志。
- **视觉/人工**：Grafana business 看板截图（T-11）；Alertmanager firing 截图（T-13）。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **BFF 被拖入 gorm 依赖** | `shared/pkg/skywalking` 含 GORM/Redis 埋点 ⇒ BFF（网关）import 它会把 gorm 拖进模块图（实测 build 报 missing go.sum entry；BFF go.sum 里 gorm 条目数原为 0） | ✅ 已解：拆出**无 gorm 依赖**的 `shared/pkg/skywalking/trace` 子包（注册表 + span 助手 + HTTP RoundTripper）；BFF 只引该子包，`go.sum` gorm 条目仍为 0。**该子包同时解掉 Phase 3（Redis 接线）与 Phase 6（SetTracer）的同一堵墙** |
| **改埋点放大 span 量** | 每次 ORM/HTTP/Redis 都建 span | 遵守约束③：仅在 `.WithContext(ctx)` 有 ctx 时建；无 ctx 不硬造根 span（T-08 钉死） |
| **GORM 版本差异** | processor 名（`gorm:query` 等）随版本变 | 改前 `grep gorm.io/gorm` 版本核对；回归钉以 `Callback().Get(name)` 断言而非依赖内部名 |
| **HTTP 出站改法分散易漏** | 多处裸 client | 统一 helper + **守卫扫描"裸 client 无 Transport"**；T-04 覆盖清单逐处核对 |
| **Span 重名/嵌套** | 出站 HTTP 与 gRPC 双埋点重复 | 明确分层（HTTP 出站 vs gRPC 出站各一）；BFF downstream 的 gRPC 路径已有拦截器，只补 HTTP 降级路径 |
| **内存红线** | 新增 exporter 可能击穿 19 容器栈 | **默认只改代码/配置不新增容器**；exporter 按 D-02 评估（T-16） |
| **UI 改面板被覆盖** | provider 30s 覆盖 UI 改动 | 一切改动落 `deploy/grafana/dashboards/*.json` |
| **凭记忆写指标名/LogQL** | expr 引用不存在指标 → 空 series 像"没数据" | Phase 0 先 `label/__name__/values` 核对；LogQL 用 `/query_range` 实机跑通（T-14） |
| **告警阈值拍脑袋** | 太低误报刷屏 / 太高形同虚设 | 沿用既有决策"零容忍 + `for:` 时长过滤"（business-metrics-wiring §3.5）；DLQ 深度保持 non-alerting |
| **dev 栈被 A 轨占用** | `.devmode-session` 由 `lane-cl-a` 持至 12:00 | 🟢 的 Phase 1–6/7(写文件) 先做；🔴 的 Phase 0/8/9 排在锁释放后（或协商让渡） |
| **共享文件冲突（roadmap/parallel-contract）** | A 轨同改这两份 | 按 parallel-contract §四 握手区**串行合并**；本次已在 §四 登记握手行 |

---

## §7 [M] 裁定记录（2026-10-14 用户）

| # | 事项 | 关联 | 结论 |
|---|------|------|------|
| D-01 | span 采样策略 | 全局 | ✅ **dev 全采**（生产采样率后议） |
| D-02 | 是否新增 exporter / 容器监控 | T-16 / G7 | ✅ **先做"免费那一半"**（**不加任何容器**）：用现成 `up{job}` + `ALERTS` 建「观测面自身」看板；**cAdvisor / redis / pg exporter 记 deferred**（见下方补注） |
| D-03 | `emotion_fusion_calls_total/duration` 接线 vs 删 | T-06 / G5 | ✅ **接线**（接进 Prometheus→Grafana 看板；属运维视角指标，非终端用户界面） |
| D-04 | web-bff 是否 `SetTracer` | T-07 / G6 | ✅ **对齐**（BFF 也 `SetTracer`，并把 BFF 纳入 `svc_wiring_test.go`） |
| D-05 | `/health`：blackbox vs 直接 scrape | T-15 | ✅ **直接 scrape**（不加 blackbox；与 D-02 容器监控方案一并复核） |
| D-06 | `redis-two-layer-cache` 是否纳入本阶段 | T-19 | ✅ **本阶段只做 Redis 埋点接线**，双层缓存（cache-aside 特性）另议 |

> **D-02 补注（2026-10-14 用户追问"各容器状态"）**：用户要求把**各容器状态**接进 Prometheus 观测。查证结论：
> ① **免费部分**：Prometheus 现有 `up{job}` 已能给出 9 个 target 的通断（6 应用服务 + APISIX + OAP + llm-service），做「观测面自身」看板即可（= 本计划 T-09/T-11）；
> ② **全容器部分**：要看全部 19 容器（含 postgres/redis/kafka/minio/nacos/etcd/loki/promtail…）的 CPU/内存/运行状态，标准做法是加 **cAdvisor**（+1 小容器 ≈50–150MB，需挂 `docker.sock`/`sys`/`var_lib_docker`；WSL2 下需实测 cgroup 挂载）；
> ③ **不可行路线**：Docker 自带 `:9323` metrics **仅引擎级、无 per-container**（Docker 官方文档明示 "you can't currently monitor your application using the Docker target"）⇒ 满足不了本需求。
> → **用户 2026-10-14 裁定**：**先做"免费那一半"**（`up`/`ALERTS` 看板，零新增容器）；**cAdvisor + redis/pg exporter 记 deferred**（不进本阶段，留待后续/backlog）。**本阶段 G7 的采集增量因此收敛为"不加容器的部分"**（`/health` scrape 等）。

---

## §8 调研依据（本次实读，不凭记忆）

- **埋点代码**：`emotion-echo-shared/pkg/skywalking/{gorm_tracing.go,init.go,redis_tracing.go,tracing.go,svc_wiring_test.go,gorm_tracing_test.go,init_test.go}`（**逐行亲核**）
- **BFF**：`emotion-echo-web-bff/main.go`（`:86/:201` tracer、`:282` redisClient、`:375` ClientDialOptions、`:605-615` buildRedisClient）、`internal/downstream/`（目录 + HTTP 降级路径）
- **ai-svc 出站**：`internal/aiclient/{fer,sensevoice,xtts}.go`、`internal/analyzer/http_analyzer.go`、`internal/fusion/{llm_fuser.go,fusion_metrics.go}`
- **采集/面板**：`deploy/prometheus/rules/*.yml`、`deploy/grafana/dashboards/*.json`、`deploy/docker-compose.infra.yml`（obs 服务段）、`scripts/smoke_observability.py`
- **计划对账**：`docs/plans/{observability-instrumentation-and-dashboards,observability-business-metrics-wiring,grafana-dashboard-gaps-2026-09-30,redis-two-layer-cache}.md`
- **治理**：`docs/closure-roadmap/{roadmap,RUNBOOK,parallel-contract,decisions,discovered-unresolved}.md`、`docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md`
- **grep 实证**：`InitRedis|InstrumentRedis` 生产 0 命中；`RecordFusionCall|ObserveFusionDuration` 生产 0 命中；`SetTracer(` 命中 5 svc、**不含 web-bff**。

**未能确认**（需实机，本文未据此下结论）：
- 当前运行态各指标的**实际 label 取值**、哪些 counter **已有数据**（部分可能从未触发）；
- go2sky `CreateExitSpanWithContext` 在**无父 span 的 ctx** 下的确切行为（T-08 验证）；
- 内存余量是否够任一 exporter（D-02）；
- `sw8` 在 BFF downstream HTTP 降级路径上是否与 gRPC 路径的 trace 头冲突（Phase 2 核）。

---

## §9 进度快照（2026-10-14，交接 / 恢复用）

> 本表是"打点"：任何会话（含 12:00 锁释放后回来的自己）据此恢复。

| Phase | 内容 | 状态 |
|---|---|---|
| 0 | 基线取证（**需栈**） | ⬜ 未做 —— 等 devmode 锁（A 轨持至 2026-10-14 12:00） |
| 1 | GORM 埋点修复 | ✅ 完成（RED→GREEN，已 push） |
| 2 | HTTP 出站埋点（G3） | ✅ **组件 + 接线 8 处完成**（ai-svc 5 + BFF 3）；**含拆出无 gorm 依赖的 `skywalking/trace` 子包**（否则 BFF import skywalking 会拖入 gorm、build 直接失败）。BFF 其余 http client 未纳入（`chat.go` 归 C 轨，余见 §2 T-04） |
| — | 「免费那一半」观测面看板 + smoke 钉 | ⚙️ 已落地，**待起栈实跑**（T-09/T-10 = ⚙️ 非 ✅） |
| 3 | Redis 埋点接线（G2） | ✅ 实现迁 `redistrace` 子包 + BFF 装配（**待起栈验 `redis.*` span**） |
| 4 | DLQ span（G4） | ⬜ |
| 5 | fusion 指标接线（G5；D-03 已定=接线） | ⬜ |
| 6 | BFF SetTracer（G6；D-04 已定=对齐） | ✅ 落地 + 守卫扩含 BFF |
| 7 | 看板 business/reliability/logs + 告警（**需栈**核指标名） | ⬜（observability 看板已提前落） |
| 8 | 采集增量 `/health` scrape（**需栈**；D-05 已定=直接 scrape） | ⬜ |
| 9 | 负向对照 + 收口（**需栈**） | ⬜ |

- **恢复点**：Phase 2 接线 → 3 → 4 → 5 → 6（**均不需栈**，可连续做）；Phase 0/7/8/9 待锁释放。
- **已落地 commit**：见 **PR #211**（建档 / GORM 修复 / HTTP 组件+守卫 / 免费一半看板 / 决策回填）。
- **门禁现状**：`go test ./pkg/skywalking/` 绿；`obs_monitoring.test.js` 绿；`check_tdd_gate.sh` 绿；`check_orphan_outputs.sh` 绿。
