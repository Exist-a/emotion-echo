---
stage: e2e-26
title: 链路追踪 SkyWalking（sw8 传播 + OAP 查询 + UI 可视化）
type: verification
status: pending
created: 2026-10-02
last-updated: 2026-10-02（建档：计划期调研完成，20 测试点，2 个 [M] 决策点）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；执行期 [M] 决策点见 §4
related-findings: []
---

# E2E-26 链路追踪 SkyWalking — 详档（任务书）

> **类型**：verification —— sw8 插桩**早已全量落地**（Stage 13 gRPC tracing / Stage 28 observability / Stage 46 gin entry span / Stage 49 rpc tags / Stage 92 Kafka sw8 / Stage 93 analytics sw8 / Stage 94 span EndSpan 修复 / PR-OBS-1/2/15/17/18/23 系列），但它**当前不能证明自己形成了可观测闭环**：
> ① **UI 宿主不可达**——`skywalking-ui` 8080 与 OAP 12800/11800 在 Stage 33 PR-20「端口收紧」后只有 `expose` 无宿主 `ports`，APISIX 也无对应 route ⇒ 从宿主浏览器根本打不开 Trace 页（§0.1 F-c）；
> ② **OAP 查询侧是空转的**——`scripts/smoke_bff_chat_grpc.sh` 契约 9 用宿主 `curl localhost:12800`（端口未映射，必失败）+ 一个**协议里不存在的 GraphQL 签名** `queryServices(serviceId:, startTimeBucket:, endTimeBucket:, topN:)`，且失败分支是 best-effort WARN 不计 FAIL ⇒ 这半条契约**从未真正跑通过**（§0.1 F-e）；
> ③ **历史「OAP 9.x queryDuration bug」归因存疑**——官方 query-protocol 明确 `SECOND` 步长格式是 `yyyy-MM-dd HHmmss`（**无冒号**），历史上报的 "malformed at :00:00" 与"客户端发了带冒号格式"完全吻合，且 apache/skywalking 上游 issues 搜不到该 bug ⇒ 更可能是当初查询/客户端格式错而非 OAP 缺陷（§0.1 F-f，执行期 M1 定性）；
> ④ **日志与 trace 是两套 ID**——HTTP 路径日志 `trace_id` = APISIX `request_id`（`gin_skywalking.go:75-77` 只挂 X-Trace-Id），只有 Kafka 消费侧经 `TraceIDFromSW8` 拿到的才是 sw8 traceId ⇒ E2E-21 移交的「OAP trace ↔ 日志 join」在 HTTP 路径上现状不可 join（§0.1 F-d）。
> **本阶段把「有插桩」变成「UI 查得到、查询非空转、join 分路径成立、降级有实测」。**

> **依据**：roadmap 排期总表 E2E-26 行（"sw8 传播 + OAP 查询 + UI 可视化（OAP 9.x queryDuration bug）"）+ E2E-21 plan §边界表移交项「SkyWalking OAP trace ↔ 日志 join → 归 E2E-26」+ `docs/plans/known-issues-backlog-2026-09-16.md` Item 4（A5 queryDuration，DoD = OAP UI 能查 trace + service map 完整调用链）。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：无依赖前置。上一阶段 E2E-25 ✅ done（2026-10-02，20/20 PASS + 第二方核对通过）。
> **名下账本**：0 条（2026-10-02 计划期对账核实，账本无归属 E2E-26 的未闭环条目）。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的探针 / 亲自取回的官方协议原文**（AGENTS §〇 文档功课），非引用历史结论。执行期若发现与本节不符，**以实测为准并回填本节**（AP-02）。
>
> **已读实现文件**：`emotion-echo-shared/pkg/bootstrap/tracer.go`（全文，BootstrapSkyWalkingTracer：CheckTCP→GRPCReporter→go2sky.NewTracer 三段错误包装）、`emotion-echo-shared/pkg/middleware/gin_skywalking.go`（全文 195 行，EntrySpan + 4 tag + X-Trace-Id 挂 ctx + buildSpanError + SKIP_PATH_LIST）、`emotion-echo-chat-svc/internal/events/kafka_publisher.go`（全文 160 行，sw8 注入 Kafka header / EndSpan(sendErr) / peer=topic 拓扑约定 / nil tracer 降级）、`emotion-echo-chat-svc/main.go`（PR-OBS-2 bootstrap 段 :135-170 + InitGORM :432）、`emotion-echo-web-bff/main.go`（tracer bootstrap :188-216）。
> **已读测试文件**：`emotion-echo-chat-svc/internal/events/kafka_publisher_test.go`（sw8MockTracer / sw8 header 断言 / spanEndRecorder 锁 EndSpan / nil tracer 降级语义）、`emotion-echo-shared/pkg/bootstrap/tracer_test.go`（LiveAddr / DeadAddr / NilContext 三 case）、`emotion-echo-shared/pkg/grpcinterceptor/traceid_test.go`（TraceIDFromSW8 提取）。
> **已查 ADR / 决策 / stage**：`docs/architecture/adr/` **无 SkyWalking 专属 ADR**（全目录 grep skywalking 零命中）；`docs/architecture/decisions.md` 仅架构图提及（决策 29~38 无 tracing 条目）；历史 stage：stage-13-grpc-tracing / stage-28-observability / stage-44 obs-sprint-b / stage-46 gin-entry-span / stage-49 rpc-tags / stage-92 kafka-sw8 / stage-93 analytics-sw8 / stage-94 P0-6 span 修复；E2E-21 plan §5 边界表（join 移交本阶段）；[E2E-21 report](../e2e-21-logging-observability/report.md)（X-Trace-Id 全链已打通，但那是 APISIX request_id 不是 sw8 traceId）。
> **smoke（本轮可跑部分）**：环境全停（F-i），运行时探针留 §0.2；协议层核验见下。
> **外部依赖官方文档**（功课④，经 GitHub MCP 取原文）：
> - [apache/skywalking-query-protocol `common.graphqls`](https://github.com/apache/skywalking-query-protocol/blob/master/common.graphqls) —— **Duration 官方格式**：`SECOND: yyyy-MM-dd HHmmss`（无冒号）/ `MINUTE: yyyy-MM-dd HHmm` / `HOUR: yyyy-MM-dd HH` / `DAY: yyyy-MM-dd`。
> - 同仓 `metadata.graphqls`（getAllServices/searchServices，**均需 duration**）、`metadata-v2.graphqls`（`listServices(layer)` / `getService`）、`trace.graphqls`（queryBasicTraces / queryTrace）、`topology.graphqls`（getGlobalTopology / getServiceTopologyByName）、`metrics-v2/v3.graphqls` —— **五个协议文件逐一核对，均不存在** `queryServices(serviceId:, startTimeBucket:, endTimeBucket:, topN:)` 签名（F-e 证据）。
> - GitHub issue 检索（`queryDuration repo:apache/skywalking`，15 条）—— **无任何条目对应 "malformed at :00:00" 时间格式 bug** ⇒ "上游 bug" 归因无官方佐证（F-f 证据，M1 存在原因）。

### 0.1 计划期实测事实表

| # | 事实 | 证据（`文件:行号` / 探针输出） |
|---|------|--------------------------|
| **F-a** | **6 个服务全部默认启用 tracer**：user/chat/analytics/assessment/ai/web-bff 均注入 `SKYWALKING_ENABLED: "${SKYWALKING_ENABLED:-true}"` + `SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800` | `deploy/docker-compose.apps.yml:99,152,205,254,385,643`；ServiceName 形如 `emotion-echo-chat-svc`（`internal/config/config_test.go:29-32`） |
| **F-b** | **OAP 宕机降级是 warn 模式**：`STARTUP_STRICT` 默认 false（dev 兼容），tracer init 失败只 `log.Printf warn` + `IncSkyWalkingInitFailed` 指标，svc 照常起；fail-fast 需 `STARTUP_STRICT=true` + `STARTUP_STRICT_DEPS` 含 `skywalking` | `emotion-echo-shared/pkg/bootstrap/deps.go:16-40`、`chat-svc/main.go:142-151`、`web-bff/main.go:191-199` |
| **F-c** | **OAP/UI 宿主端口全封**：11800/12800（OAP）与 8080（UI）只有 `expose` 无 `ports`（Stage 33 PR-20 端口收紧），APISIX seed 无 sw-ui route ⇒ **宿主 curl :12800 必失败、宿主浏览器打不开 UI** | `deploy/docker-compose.infra.yml:165-168,192-195`（注释明写"UI 通过 APISIX 网关或容器内访问"，但网关实际无该 route：`seed.sh` grep sw-ui/8080 零命中） |
| **F-d** | **日志 trace_id 与 OAP traceId 是两套 ID**：HTTP 侧 = X-Trace-Id（APISIX `ctx.var.request_id`，seed `TRACE_ID_PLUGIN`）→ `gin_skywalking.go:75-77` 只挂它；gRPC 侧 = x-trace-id metadata；**只有 Kafka 消费侧**经 `TraceIDFromSW8` 拿到 sw8 traceId（F-146 修复）⇒ HTTP 路径日志**不可**与 OAP trace join | `gin_skywalking.go:73-77`（注释"2. skywalking span 上下文（**若未来** span 内携带 trace id）"= 现状未接）、`grpcinterceptor/sw8.go:21`、`grpcinterceptor/tracing.go:24-38` |
| **F-e** | **smoke 契约 9 双重空转**：① 宿主 `curl localhost:12800/graphql`（F-c 端口未映射 ⇒ 必 `OAP_UNREACHABLE`）；② 查询签名 `queryServices(serviceId:, startTimeBucket:, endTimeBucket:, topN:)` **在官方五个协议文件中均不存在** ⇒ 即便端口通也是 GraphQL 校验错误；③ 两层失败都落 best-effort WARN **不计 FAIL** ⇒ 这半条契约从未真正验过 | `scripts/smoke_bff_chat_grpc.sh:158-186`（`--max-time 10 || echo OAP_UNREACHABLE` + `log "[WARN] 探测失败（best-effort，不计入 FAIL）"`）+ §0 官方协议比对 |
| **F-f** | **queryDuration「bug」归因存疑**：官方 `SECOND=yyyy-MM-dd HHmmss` 无冒号；历史报错 "malformed at :00:00" 与带冒号输入吻合；上游 issues 零命中（§0 外部文档） | stage-92:91 / stage-93:106 的历史描述 vs query-protocol `common.graphqls` 原文 |
| **F-g** | **APISIX access log 上报链已配置**：`skywalking-logger` 挂 CATCHALL + AUTH_WHITELIST 全部业务路由（endpoint_addr 容器 DNS :12800），健康探针路由豁免 | `deploy/apisix/seed.sh:346-390,543`、`config.yaml:168-175`、`seed_test.js:201-209` |
| **F-h** | **OAP 存储 = H2 内存库**（`SW_STORAGE: h2`, `jdbc:h2:mem`…`DB_CLOSE_DELAY=-1`）⇒ OAP 容器一重启，历史 trace **全部丢失**；且无持久化、无多节点 | `deploy/docker-compose.infra.yml:154-155` |
| **F-i** | **环境基线（2026-10-02 计划期实测）**：全栈 Exited（4h 前），`sw-oap`/`sw-ui` Exited(143)，仅 xtts/sensevoice/fer 3 容器 Up；另见 `emotion-llm-service Exited(127)`、`kafka-exporter Exited(2)`（异常退出码，归 E2E-22/23 范围，记录不修）与陌生容器 `cool_carson`（非本项目，不动） | `docker ps -a` 输出 |
| **F-j** | **OAP 存活信号依赖 telemetry**：OAP 无 `/internal/liveness`（未开 SW_HEALTH_CHECKER），healthcheck = wget `:1234/metrics`（E2E-23 #26 落地）；prometheus 有 `skywalking-oap` scrape job | `docker-compose.infra.yml:168-181`、`deploy/prometheus/prometheus.yml:64-68` |

### 0.2 开工复核清单（第一天执行，防止任务书事实表过期）

> 环境基线 = F-i（全停）。开工第一步不是写测试，是把环境拉回 RUNBOOK §2.1 基线再复核运行时事实。

| # | 复核项 | 通过标准 |
|---|--------|----------|
| 1 | 登记 `.devmode-session` → 按 RUNBOOK §2.1 带 `--env-file .env.local --profile dev` 错峰拉起 → Nacos `count:6` → 全栈 healthy | `docker ps` 全 healthy；apisix-seed Exited(0) |
| 2 | F-a 复核：6 svc 日志 `tracer initialized`；容器网内 `listServices` 返回 ≥6 个本项目服务 | 日志 + GraphQL 输出 |
| 3 | F-c 复现：宿主 `curl localhost:12800/graphql` 失败 vs `docker exec` 容器网内成功（两视角对照） | 两向输出留档 |
| 4 | F-d 复现：一次登录 → Loki 侧 trace_id（APISIX request_id）与 OAP 侧 traceId 分别取到 → 断言二者**不相等** | 两值对照 |
| 5 | F-e 复现：容器网内实发契约 9 原查询 → 记录 GraphQL 校验错误原文（证明签名不存在是运行时事实而非只看文档） | 错误响应留档 |
| 6 | 内存余量：`docker stats --no-stream` 基线留证（.wslconfig 8GB、19 容器 ≈6G 贴顶前科） | 余量 ≥ 1G 再起 OAP |

---

## 1. 范围

**做**：
- 组 A（sw8 跨进程传播）：HTTP 入口 / gRPC / Kafka 三条路径运行时实证 + OAP 宕机降级两向对照 + span 错误语义（Stage 92/93 是单测与 header 级实证，本阶段补"OAP 上看得见"这半边）
- 组 B（OAP 查询）：查询工具 TDD（官方 Duration 格式封装）+ queryDuration 历史 bug **定性**（M1）+ smoke 契约 9 修复（F-e）
- 组 C（UI 可视化）：宿主入口落地（M2）+ IAB 实测 Trace 树 / 拓扑图 / 错误过滤（backlog Item 4 DoD）
- 组 D（join 与生命周期）：E2E-21 移交的 trace↔日志 join 分路径验证 + 双 ID 处置（M2）+ H2 内存存储语义 + healthcheck 回归

**不做（明确划出边界）**：
- **OAP 存储持久化改造**（H2 → SQLite/ES/BanyanDB 属架构变更，需 ADR）——本阶段只实证现状语义并记账
- **trace 采样策略 / 上报性能压测**——无 ADR 不动架构
- **OAP alarm / ebpf / profiling / 浏览器端 JS agent**——观测栈其他面归 E2E-22 / 后续
- **k8s `charts/skywalking` 与 compose 的对齐**——本阶段只跑 dev compose；charts 漂移若发现只记账
- **Loki 日志侧改造**（E2E-21 已收口）——本阶段只做 join 验证，日志管线问题记账
- **APISIX skywalking-logger 配置语义**（E2E-25 已验 seed 治理）——本阶段只验"上报到 OAP 可查"

---

## 2. 测试点清单（20 个）

> 判定分级：`[A]` 自动/脚本断言 · `[V]` 视觉/IAB 实测 · `[M]` 需裁定（执行期决策点，见 §4）。运行时类测试点要求**正负两向对照**（§4.1 证据有效性）：采集类断言必须配上"停 OAP/重启后"的反向对照，避免"单次绿 = 巧合"。

### 组 A：sw8 跨进程传播（F-a / F-b —— 单测已绿，补 OAP 侧闭环）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 1 | [A] | **tracer 初始化与服务注册**：全栈拉起后 6 svc 日志 `tracer initialized`；OAP `listServices` 返回 ≥6 个本项目服务 | 6/6 日志 + GraphQL 输出回读 |
| 2 | [A] | **HTTP 入口 span**：一次登录 → OAP `queryBasicTraces` 查到 entry span，`http.method` / `http.url` / `http.status_code` / `user_id` 四 tag 齐全（`gin_skywalking.go:88-103`） | trace 查询输出含 4 tag 及正确值 |
| 3 | [A] | **gRPC 跨进程**：一次聊天（APISIX→BFF→chat-svc→ai-svc）→ 同 traceId 下 ≥3 服务 segment 且 refId 互指 | `queryTrace` 全树 + ref 关系断言 |
| 4 | [A] | **Kafka 跨进程**：发消息产生 `message.created` → analytics-svc / ai-svc consumer 侧 segment 同 traceId（sw8 header 重建父 span，Stage 92/93 运行时补证） | consumer segment 的 ref 指向 producer |
| 5 | [A] | **APISIX access log 上报**（F-g）：业务请求后 OAP `queryLogs` 查到 APISIX 侧记录；负向对照：健康探针请求**不产生**记录（`seed.sh:543` 豁免） | 两向查询对照 |
| 6 | [A] | **OAP 宕机降级两向对照**（F-b）：停 sw-oap → svc 重启走 warn 继续服务（业务请求仍 200）；`STARTUP_STRICT=true` + `STARTUP_STRICT_DEPS=skywalking` → fail-fast 拒启 | 日志两向 + 业务请求状态码 |
| 7 | [A] | **span 错误语义**：触发 5xx → OAP span 带 error（`buildSpanError`，`gin_skywalking.go:134-149`）；200/4xx 无 error | error 与非 error 两向对照 |

### 组 B：OAP 查询（F-e / F-f —— 查询工具先红后绿 + 历史 bug 定性）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 8 | [A] | **查询工具契约（TDD）**：OAP 查询 helper 按官方 Duration 格式封装（SECOND=`yyyy-MM-dd HHmmss`），fixture 契约测试先红后绿 | 契约测试 RED→GREEN 两段 commit |
| 9 | [A] | **合规格式查询**：用 SECOND 格式查 `queryBasicTraces` 近 10 分钟 → 查到 #3 的 trace | 输出含目标 traceId |
| 10 | [A] | **反向对照（历史报错复现）**：同查询改用带冒号 `yyyy-MM-dd HH:mm:ss` → 记录 OAP 响应原文（预期格式校验错误） | 两向响应均留档 |
| 11 | [M] | **queryDuration「bug」定性裁决**（M1）：F-f 证据（协议无冒号 + 上游零 issue）→ 用户拍板处置：① 判"客户端格式错"，回填 stage-92/93/backlog 注记；② 判"真上游 bug"，升级/降级 OAP；③ 其他 | 决策记录 + 相应回填完成 |
| 12 | [A] | **smoke 契约 9 修复**（F-e）：宿主→容器网访问 + 合规查询签名 + WARN 降级改真断言；补 shell 守卫测试先红后绿 | 守卫 RED→GREEN；OAP 通时真验、OAP 停时契约 9 **真 FAIL**（负向对照） |

### 组 C：UI 可视化（F-c —— 入口先落地，IAB 实测）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 13 | [M] | **sw-ui 宿主入口方案**（M2）：现状宿主不可达（F-c）；备选 ① `127.0.0.1` 限定 ports 映射（dev only，需回填 Stage 33 端口收紧注记）② APISIX route 反代（Stage 33 注记允许的路径，需验 UI 子路径行为）③ 降级宿主端口转发临时命令（不动仓库）。用户拍板后落地 + 结构守卫先红后绿 | 决策记录 + 守卫 RED→GREEN + 宿主访问 200 |
| 14 | [V] | IAB 打开 Trace 查询页 → 查到 #3 聊天 trace，跨进程树形展开可见（backlog Item 4 DoD 前半） | 截图 `26-14-*.png` 且被查看 |
| 15 | [V] | 服务拓扑图显示完整调用链（backlog Item 4 DoD 后半 "service map 完整调用链"） | 截图 + 节点/边清单与 #3 服务集吻合 |
| 16 | [V] | 错误 trace 在 UI 过滤/标记可见（对应 #7 的 error span） | 截图 |

### 组 D：日志 join 与数据生命周期（E2E-21 移交项 + F-d / F-h / F-j）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 17 | [A] | **OAP trace ↔ 日志 join 分路径验证**（E2E-21 移交）：Kafka 消费侧日志 `trace_id`（`TraceIDFromSW8`）与 OAP traceId **可 join**；HTTP 路径日志 `trace_id`（APISIX request_id，F-d）与 OAP traceId **不相等**——两路径各一组对照 | 两路径各一组实测值对照 |
| 18 | [M] | **双 trace ID 处置裁决**（M2 之二）：HTTP 日志与 OAP 不可 join 是否统一——① sw8 traceId 注入 HTTP 日志 ctx（改 `gin_skywalking.go`，动 E2E-21 已验语义）② 保留双 ID + 文档化各自用途 + Loki/OAP 互查指引 | 决策记录 + 对应落地 |
| 19 | [A] | **存储生命周期**（F-h）：OAP 重启 → 重启前 trace 查询为空（数据不持久）；TTL/retention 现状记录 | 重启前后查询对照；持久化改造记账（范围外） |
| 20 | [A] | **OAP/UI healthcheck 语义**（F-j，E2E-23 #26 回归）：OAP healthy = telemetry `:1234/metrics` 可达；停 OAP → sw-ui healthcheck 翻 unhealthy；恢复 → healthy 且新 trace 恢复采集 | 三态对照时间戳 |

---

## 3. TDD 循环划分（RED→GREEN→REFACTOR）

> 运行时验收类测试点（#1-#7、#9-#10、#14-#17、#19-#20）是验收断言，不作 TDD 对象；但每个**守卫/工具/配置改动**必须先红后绿：

| 循环 | RED（先写失败的测试） | GREEN（最小实现） |
|------|----------------------|-------------------|
| C1 | OAP 查询 helper 契约测试：fixture 断言 SECOND 格式封装 + 响应解析 → FAIL | helper 脚本落地（`scripts/`，docker exec 容器网访问，不引新依赖） |
| C2 | `scripts/test_smoke_oap_contract.sh` 静态断言契约 9 用容器网地址 + 合规签名 + 非 best-effort → FAIL（现状 F-e 命中） | `smoke_bff_chat_grpc.sh` 契约 9 修复 |
| C3 | UI 入口结构守卫（按 M2 裁决的方案断言 ports 映射 / seed route 存在）→ FAIL | 入口落地 + Stage 33 注记回填 |
| C4 | （若 M2-ii 裁"统一 sw8 traceId 进 HTTP 日志"）`gin_skywalking_test.go` 断言 sw8 traceId 注入 ctx → FAIL | middleware 改动；裁"保留双 ID"则本循环改为文档化守卫（断言指引文档存在且含两套 ID 互查说明） |

---

## 4. 执行期 [M] 决策点

| # | 决策 | 背景 | 备选 |
|---|------|------|------|
| M1 | queryDuration「历史 bug」定性与处置（#11） | F-f：协议格式无冒号 + 上游 issues 零命中 + 历史报错与带冒号输入吻合 | ① 判客户端格式错→回填三处历史注记（stage-92/93、backlog Item 4、roadmap 行 122 措辞）；② 判上游 bug→列 OAP 版本升级/降级专项；③ 运行时复现与 F-f 不符→按实测重定性 |
| M2 | sw-ui 宿主入口方案（#13）+ 双 trace ID 处置（#18） | F-c 端口收紧是 Stage 33 既定动作；F-d 双 ID 牵动 E2E-21 已验语义 | #13：① 127.0.0.1 ports 映射 ② APISIX route ③ 临时端口转发；#18：① 统一 sw8 traceId ② 保留双 ID + 文档化 |

> 决议产生后登记 [decisions.md](../../decisions.md)（**D-37 起**；D-36 为 E2E-25 M2 已占用）+ 涉及架构语义的同步 `docs/architecture/decisions.md`（决策 39 起）。

---

## 5. 收口门槛

1. 20/20 测试点四值判定（PASS/FAIL/BLOCKED/N/A），**BLOCKED ≤ 1/3 不得判 done**；`N/A` 必须附"为什么不适用"证据
2. 名下账本 0 条现状保持——执行期新发现按 RUNBOOK §5 契约登记（连续编号），属本阶段未闭环条目存在时只能标 `partial`
3. `e2e_stage_audit.py --all` 0 FAIL（plan 已用「测试点清单」标题 + 整数编号首列，A3 可解析——吸取 E2E-25 F-180 教训）
4. Playwright 回归钉落地：`emotion-echo-web/e2e/skywalking-trace.spec.ts`（OAP 查询 + UI Trace 页可见，至少 1 条，收口时跑过且绿）
5. M1/M2 裁决登记 decisions.md + 三处 queryDuration 历史注记按裁决回填
6. RUNBOOK 涉及处更新（命令速查补 OAP 查询 helper / UI 入口方式）
7. report.md 按 §10 模板 + §7 收口 11 项全过 + 第二方核对（§13.3）后才可判 done

---

## 6. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 观测栈同刻 Exited(255) 复发（E2E-22/E2E-25 两轮前科） | 开工复核 #1 先恢复基线；故障注入（#6/#19/#20）测完立即恢复；收工三查（AGENTS §八.6） |
| OAP H2 内存库：重启即丢 trace + 采集→可查有窗口延迟 | 测试点顺序 = 采集后立即查询；#19 的重启对照放最后；M1 查询窗口留 ≥2 个数据点 |
| 19 容器 + XTTS 内存贴顶（8GB 上限） | 开工复核 #6 余量 ≥1G 再起 OAP；错峰启动（RUNBOOK §2.1） |
| M2 入口方案被裁定阻塞 UI 组 | 备选 ③（临时端口转发）保证 #14-#16 不 BLOCKED；BLOCKED >1/3 停止收口升级 |
| go2sky ↔ OAP 9.7 gRPC 协议兼容性若不成立（#1 即红） | 属真缺陷进修复队列；若需升 go2sky 版本，先查 go2sky 对 OAP 9.x 支持矩阵（官方文档）再动 |
| 契约 9 修改影响 §2.4 smoke 门槛 | 契约 9 属 `smoke_bff_chat_grpc.sh` 独立脚本非 `smoke_data_layer.py`；修改后本地全跑一遍确认退出码语义 |

---

## 7. 引用

- 账本：[discovered-unresolved.md](../../discovered-unresolved.md)（2026-10-02 对账：归属 E2E-26 条目 = 0）
- 移交项：[E2E-21 plan §5 边界表](../e2e-21-logging-observability/plan.md)（OAP trace ↔ 日志 join）
- 旧账：[known-issues-backlog-2026-09-16.md](../../../plans/known-issues-backlog-2026-09-16.md) Item 4（A5 queryDuration DoD）
- 决策：[decisions.md](../../decisions.md)（编号续 D-37 起）；[docs/architecture/decisions.md](../../../architecture/decisions.md)（决策 39 起）
- 历史阶段：[stage-92-kafka-sw8-propagation](../../../stages/stage-92-kafka-sw8-propagation-2026-09-14.md)、[stage-93-analytics-svc-sw8-propagation](../../../stages/stage-93-analytics-svc-sw8-propagation-2026-09-14.md)、stage-46 gin-entry-span、stage-49 rpc-tags、[E2E-21 report](../e2e-21-logging-observability/report.md)
- 官方文档：[query-protocol common.graphqls（Duration 格式）](https://github.com/apache/skywalking-query-protocol/blob/master/common.graphqls)、[metadata-v2.graphqls](https://github.com/apache/skywalking-query-protocol/blob/master/metadata-v2.graphqls)、[trace.graphqls](https://github.com/apache/skywalking-query-protocol/blob/master/trace.graphqls)、[topology.graphqls](https://github.com/apache/skywalking-query-protocol/blob/master/topology.graphqls)
