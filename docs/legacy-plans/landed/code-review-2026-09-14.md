---
status: landed
priority: critical
owner: TBD
created: 2026-09-14
landed: 2026-09-14
landed-by: stage-94 (P0) + stage-96 (Round 1 P1/P2 残余)
source: external code review（会话 2026-09-14, 4 个并发子 agent 系统排查,非项目内部演进）
depends-on: []
related-stages:
  - stage-43-kafka-reliability-sprint-a.md
  - stage-44-observability-sprint-b.md
  - stage-50-e2e-validation.md
  - stage-86-outbox-dead-alert-2026-09-13.md
  - stage-92-kafka-sw8-propagation-2026-09-14.md
  - stage-93-analytics-svc-sw8-propagation-2026-09-14.md
  - stage-94-code-review-2026-09-14-p0-closure.md
  - stage-95-code-review-2026-09-14-round2-closure.md
  - stage-96-code-review-round1-p1p2-closure.md
  - stage-97-round2-p0-closure.md
related-adrs:
  - 决策 4 gRPC 化
  - 决策 6 JSON 日志 + trace_id 串联
  - 决策 10 Nacos 服务发现
  - 决策 12 APISIX 信任头
related-plans:
  - observability-edge-gaps-from-code-review.md (已 landed §A, 部分 §B/C/E)
  - code-review-2026-09-14-round-2.md (Stage 97 全 10 P0 + 部分 P1/P2 收口)
  - kafka-pipeline-pending-decisions.md (D1-D8 仍 planned)
landed-stages:
  - stage-94: Round 1 全 10 P0 (10 commits)
  - stage-95: Round 2 P0+P1+P2+P3+DOC (61 items, 43 files)
  - stage-96: Round 1 P1+P2 (22 P1 + 12 P2 应用, 6 P1 + 13 P2 deferred)
  - stage-97: Round 2 P0 全部 + 顺手 Round 1/2 P1+P2 (13 commits)
residuals:
  - Round 1 P1 6 项 deferred 到下一 sprint (P1-1/4/7/9/11/12/14/17/19/23/24/25/26)
  - Round 1 P2 13 项 deferred
  - 详见 stage-96-code-review-round1-p1p2-closure.md §1/§2 表
---

# Plan — 2026-09-14 外部代码审查综合漏洞清单

## 0. 来源

本计划不是项目内部演进识别出来的，是 **2026-09-14 会话** 中用户明确要求"分析代码找漏洞、写入待决策文档、不要修改代码、不要完全相信文档" 后由 4 个并发子 agent 分别覆盖：

| Agent | 覆盖技术域 | 报告项数 | P0 | P1 |
|------|------|------|----|----|
| A | 服务注册链路（Nacos / 服务发现 / 注册中心） | 20 | 5 | 5 |
| B | 可观测链路（SkyWalking / Prometheus / Loki / metrics） | 24 | 4 | 8 |
| C | Kafka 事件管道（producer / consumer / outbox / DLQ） | 21 | 2 | 8 |
| D | 中间件与基础设施（auth / DB / Redis / JWT / 配置 / 迁移） | 50+ | 6 | 9 |

本计划是 **A+B+C+D 的去重合并 + 优先级重排**，不重复列每条原文。

**已复核的关键事实**（避免 agent 误判）：
- ✅ `deploy/.env.local` 已正确 git ignored（`.gitignore:129`）—— 但 Agent D §D4 误判为 P0，应为 **P1**（key 仅在本机与 compose 容器内，不进 git，但本机明文仍是风险）。
- ✅ Agent C §14 / Agent B §2/§3 关于 `defer span.EndSpan` in for-loop 的判断与代码事实一致。
- ✅ Agent D §M3 / Agent B §7 关于 BFF gRPC 无 metadata 注入、无 interceptor 的判断与 `web-bff/main.go:52-54, 266-298` 一致。

---

## 1. 全文唯一索引（按严重度倒序）

### P0（必须立刻修，5 项中 1 项可降级）

| # | 标题 | 来源 | 文件 | 工作量 | 影响 dev |
|---|------|------|------|--------|---------|
| **P0-1** | **BFF 5 个下游 gRPC conn 全部无任何拦截器、无 metadata 透传** | B §7 + D §M3 | `web-bff/main.go:52-54, 266-298` | 1d | 是 | ✅ Stage 94 landed (2026-09-14, web-bff:v0.1.12 + shared ClientDialOptions helper + 6 处接入含 EmotionQuery 第 6 处盲点)
| **P0-2** | **chat-svc → ai-svc gRPC client 完全无拦截器（无 trace / 无 retry / 无 timeout）** | B §6 | `chat-svc/internal/grpcclient/ai_client_grpc.go:33-50` | 0.5d | 否（仅 fallback 路径） | ✅ Stage 94 PR-2 landed (2026-09-14, chat-svc:v0.1.12 + shared ClientDialOptions helper 接入)
| **P0-3** | **`defer span.EndSpan(nil)` 在 for-loop 内 → consumer span 永远累积、OAP 上每条消息耗时 = 整 consumer goroutine 生命周期（Stage 92/93 核心收益被抵消 50%）** | B §2 + C §14 | `ai-svc/internal/consumer/consumer.go:135-142` + `analytics-svc/internal/kafka/consumer.go:212` | 1d | 是 | ✅ Stage 94 landed (2026-09-14, ai-svc:v0.1.7 + analytics-svc:v0.1.7 + 方案 A case 末尾显式 EndSpan)
| **P0-4** | **Kafka producer 关闭路径：chat-svc `os.Exit(0)` 让 `defer kp.Close()` 不执行 + relay ctx 取消无序 → in-flight 消息丢失** | C §2 + §18 | `chat-svc/main.go:161, 209-225, 282-292` | 1d | 是（compose 重启高频） | ✅ Stage 94 PR-4 landed (2026-09-14, chat-svc:v0.1.13 + http.Server.Shutdown + signal handler cancel rootCtx,main 自然 return 触发所有 defer)
| **P0-5** | **Kafka Producer InMemory fallback 静默击穿 outbox 承诺：producer init 失败 → 事件进内存 slice → relay MarkSent → 永久丢失** | C §1 | `chat-svc/main.go:148-173` | 0.5-1d | 是 | ✅ Stage 94 PR-4 landed (2026-09-14, chat-svc:v0.1.13 + outbox_sent_via_fallback_total counter 短期 C 修复)
| **P0-6** | **`CreateExitSpan` 返回值全 discard → chat-svc producer span 永远不 EndSpan** | B §1 + C §15 | `chat-svc/internal/events/kafka_publisher.go:78-92` | 0.5h | 是（OAP 上 producer span 失效） | ✅ Stage 94 landed (2026-09-14, chat-svc:v0.1.11 + SendMessage 后 span.EndSpan(sendErr))
| **P0-7** | **GinAuthMiddleware 信任未签名 X-User-Id header + 白名单过窄**（svc 端口若直连 → 任意用户身份） | D §A1 | `shared/pkg/middleware/gin_auth.go:20-39` | 1-2d | 是 | ✅ Stage 94 PR-6 landed (2026-09-14, shared middleware + web-bff:v0.1.14 + APISIXCIDRs 白名单 + RequireAPISIXIP=true)
| **P0-8** | **chat-svc `persistWithOutbox` 路径 2/3：DB 写完但 outbox 写失败 → 事件静默丢失** | D §B3 | `chat-svc/internal/logic/createconversationlogic.go:90-150` | 1-2d | 是（命中率高） | ✅ Stage 94 PR-7 landed (2026-09-14, chat-svc:v0.1.14 + persistWithOutbox DB 齐备路径走 CreateConversationTx/AppendMessageTx 同事务 + 退化路径禁止 CreateInTx(nil, ...))
| **P0-9** | **`analytics_reader` role 密码硬编码 `CHANGE_ME_AT_DEPLOY`（initdb 自动跑 SQL → 真实建出该 login）** | D §B6 | `analytics-svc/migrations/004_create_analytics_reader_role.sql:25` | 0.5d | 是 | ✅ Stage 94 PR-5 landed (2026-09-14, analytics-svc:v0.1.8 + role 改 NOLOGIN,仅作 schema/GRANT 定义)
| **P0-10** | **BFF `JWTSecret` 硬编码默认值 `dev-bff-secret`** | D §D1 | `web-bff/internal/config/config.go:156-158` + `etc/web-bff.yaml:70` | 0.5d | 是 | ✅ Stage 94 PR-5 landed (2026-09-14, web-bff:v0.1.13 + SetDefaults 删默认值 + auth.NewManager fail-fast)

> **注**：原文 P0 共 ~12 项，本计划合并为 10 个独立 P0（P0-1/2/3/6 是 BFF/Kafka trace 链路 4 个共因 bug，建议合并 PR）。其余 D §D4（.env.local）经 git check-ignore 验证**已正确忽略**，从 P0 降为 P1。

### P1（1-2 sprint 内修）

#### 可观测链路（5 项）

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P1-1 | 全仓 `skywalking.InstrumentGORM` / `InstrumentRedis` 从未被调用 + 包级 `Init()` 未调 | B §4 + §5 | `shared/pkg/skywalking/{gorm,redis}_tracing.go` + 5 svc openPostgres | 1.5d |
| P1-2 | Kafka DLQ Publisher（chat-svc / ai-svc / analytics-svc）不传播 sw8 → 毒消息在 OAP 上是孤儿 | B §10 | `ai-svc/internal/consumer/dlq.go:141-158` 等 3 处 | 1d |
| P1-3 | APISIX file-logger 不带 trace_id + otel 死配置 → Loki↔OAP 联动断 | B §12 | `deploy/apisix/seed.sh:267-288` | 0.5d |
| P1-4 | Promtail 仅采 APISIX，未采 6 业务 svc stdout → Loki 这层对运维基本无效 | B §13 | `deploy/loki/promtail-config.yaml:22-30` | 1.5d |
| P1-5 | GinSkywalkingMiddleware 在 handler panic 时不 EndSpan（panic 路径泄漏） | B §8 | `shared/pkg/middleware/gin_skywalking.go:92-99` | 0.5d |

#### Nacos 服务发现（5 项）

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P1-6 | NacosConfig `TimeoutMs=5000ms` 太短 → ephemeral 实例 30s 内被踢 | A §1 | `shared/pkg/nacos/nacos_register.go` | 0.5d |
| P1-7 | Heartbeat 用 `UpdateInstance` 而非 `BeatInstance` → 30s 注册过期 | A §2 | 同上 | 1d |
| P1-8 | `WaitForNacos` TCP-only 不等 HTTP 200 → compose 启动顺序竞态 | A §3 + §10 | `shared/pkg/nacos/health.go` | 0.3d |
| P1-9 | web-bff / llm-service Nacos 失败被 `except Exception` / `log.Printf` 吞掉，无 fail-fast | A §7 + §14 | `web-bff/main.go:120` + `llm-service/main.py:88` | 0.5d |
| P1-10 | smoke `§契约 7` (Nacos `instance/list`) 未落地 + residuals 未更新（plan 声称验收，实际未跑） | A §19 | `scripts/smoke_data_layer.py` | 0.5d |

#### Kafka 事件管道（6 项）

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P1-11 | consumer `attempts` 不跨进程重启 → 毒消息永久卡死 partition | C §3 | `ai-svc/internal/consumer/consumer.go:63-65` + `analytics-svc/internal/kafka/consumer.go:146-147` | 0.25-4d |
| P1-12 | chat-events topic 默认 1 partition + auto-create（KRaft dev） → 水平扩展受限 | C §4 | `deploy/docker-compose.infra.yml:80` | 2d |
| P1-13 | 消息体大小无限制 → outbox 100 次后死信（业务事件丢失无告警） | C §6 | `chat-svc/internal/logic/sendmessagelogic.go:67-69` | 1-2d |
| P1-14 | DLQ 无监控 / 告警（投递不计数 / 无 DLQ lag 指标） | C §7 | `deploy/prometheus/rules/kafka-lag.yml:7-9` | 1d |
| P1-15 | DLQ 投递失败时仅 log 不重试 → 业务 + DLQ 双失败时永久丢消息 | C §12 | `ai-svc/internal/consumer/consumer.go:199-201` + `analytics-svc/internal/kafka/consumer.go:287-289` | 1-2d |
| P1-16 | chat-svc producer `Net.DialTimeout` 默认 30s（sarama）→ broker 抖动期间 Gin handler 卡 30s | C §20 | `chat-svc/internal/events/kafka_publisher.go:27-33` | 1d |

#### 中间件与基础设施（8 项）

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P1-17 | limiter in-memory 多实例失效（已被代码注释警示，prod ≥ 2 副本实际限流 = 配置 × pod 数） | D §A3 + B §22 | `shared/pkg/middleware/limiter.go` | 1d |
| P1-18 | limiter buckets map 无清理 → 长跑 OOM | D §A4 | 同上 | 0.5d |
| P1-19 | PG 连接池每个 svc 10 conn + 5 idle → 总连接预算未规划 | D §B1 | 5 svc main.go 同模式 | 1d |
| P1-20 | `gorm.DB.Transaction` 无 deadlock retry → PG `40P01` / `40001` 直接 5xx 给客户端 | D §B2 | 全部 chat-svc 事务 handler | 1-2d |
| P1-21 | `emotion_analysis` 表 DDL 在 `01-create-schemas.sql:111` 与 `02-create-tables-in-schemas.sql:97` 重复定义 | D §B5 | `deploy/db/{01,02}-*.sql` + `ai-svc/migrations/001` | 1d |
| P1-22 | migration `001_add_event_id_to_emotion_analysis` UNIQUE INDEX 无 `IF NOT EXISTS` 守卫 | D §B7 | `ai-svc/migrations/001_add_event_id_to_emotion_analysis.sql` | 0.5d |
| P1-23 | 全仓 0 处使用 Redis → 限流 + 缓存全缺位 | D §C1 | 全部 svc | 3-5d |
| P1-24 | `LLM_INTERNAL_API_KEY` 默认值 `dev-key-change-me-please-32chars-min` | D §D3 | `deploy/.env.local:19-20` | 0.5d |
| P1-25 | `LLM_API_KEY` 真实 DeepSeek key 写在 `.env.local`（已 git ignored，但本机明文仍是风险） | D §D4 | `deploy/.env.local:9,14` | 0.5d |
| P1-26 | ai-api.yaml 仍含 `${VAR:-default}` 字面值，靠 `applyEnvOverrides`+`applyDefaultFallbacks` 兜底 | D §F1 | `ai-svc/etc/ai-api.yaml:33-88` | 1-2d |
| P1-27 | 启动迁移失败仅 log 不 fail-fast → svc 带 missing 表启动，后续 500 | D §G3 | `chat-svc/main.go:114-120` | 0.5d |
| P1-28 | `grpcerr.Map` 默认 `codes.Internal` 返 `err.Error()` 全文 → SQL 错误泄露给前端 | D §I1 | `shared/pkg/grpcerr/grpcerr.go:155` | 0.5d |

### P2（下一 sprint 收口）

#### 可观测链路

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P2-1 | `SKY_FAILURE_MODE` env 文档承诺但无实现（dev 默认 warn → 静默退化） | B §11 | `shared/pkg/bootstrap/deps.go` | 1d |
| P2-2 | `ExpandShellEnvDefaults` helper 文档承诺但 6 svc 未接入 | B §D2 | `shared/pkg/config/expand.go` | 2d |
| P2-3 | HTTP duration buckets 上限 5s 太紧（AI 慢请求落 +Inf） | B §15 | `shared/pkg/metrics/metrics.go:46-53` | 0.5h |
| P2-4 | Prometheus scrape 缺 `service` label | B §14 | `deploy/prometheus/prometheus.yml:23-34` | 0.5h |
| P2-5 | 缺 panic 计数器 | B §9 | `shared/pkg/metrics/` | 0.5h |

#### Nacos

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P2-6 | ListenConfig callback 5 svc 仅打日志无 reload | A §6 | 5 svc `nacos_boot.go` | 1d |
| P2-7 | BFF Resolve 每次打 Nacos + 不缓存 | A §13 | `web-bff/main.go` | 1d |
| P2-8 | BFF resolve 失败 fallback env 违反 PR-2 验收 | A §16 | 同上 | 1d |
| P2-9 | Username/Password 字段定义有但未注入 SDK | A §11 | `shared/pkg/nacos/` | 0.5d |
| P2-10 | HotReloadLimiter 多副本不共享 | A §8 | `web-bff/handler/` | 0.5d |

#### Kafka

| # | 标题 | 来源 | 文件 | 工作量 | 状态 |
|---|------|------|------|--------|------|
| P2-11 | 分区键用 `e.ID` 而非 `conversation_id` → 失去分区局部性 | C §5 | `chat-svc/internal/events/kafka_publisher.go:70` | 1d | ✅ Round 1 §P2-11 — `kafka_publisher.go:84-92` 改用 conversation_id（Stage 94 PR-1 期间落地）|
| P2-12 | proto + JSON 双 schema 嗅探边界（P3 章节） | C §13 | 3 svc `proto_decode.go` | 1d |
| P2-13 | analytics-svc `maxRetries` 硬编码 3（与 ai-svc 配置不对称） | C §8 | `analytics-svc/internal/kafka/consumer.go:64` | 0.5d |
| P2-14 | chat-svc producer `SendMessage` 同步阻塞 + 无 ctx 取消 | C §19 | `chat-svc/internal/events/kafka_publisher.go` | 1d | ✅ Round 2.4 commit `caa100c` — goroutine + select 包裹 SendMessage + 2 ctx cancel 测试 |
| P2-15 | prod topic 未显式配置（compose.prod.yml 空壳） | C §9 | `deploy/compose.prod.yml` | 3d |

#### 中间件

| # | 标题 | 来源 | 文件 | 工作量 |
|---|------|------|------|--------|
| P2-16 | BFF `TrustAPISIX=false` 分支挂同一个 middleware（注释承诺 Authorization JWT 解析未实现） | D §A2 | `web-bff/main.go:168-175` | 0.5d |
| P2-17 | ai-svc 无 IP 限流可被 anonymous DoS（auth 前需 IP 层） | D §E2 | `web-bff/main.go`（实际是 ai-svc） | 1d |
| P2-18 | gRPC ServerRecoveryInterceptor 把 panic value 写 status message → 内部错误信息外泄 | D §A8 | `shared/pkg/grpcinterceptor/server.go:75-95` | 0.5d |
| P2-19 | GinSkywalkingMiddleware panic 时不 EndSpan（与 P1-5 同源） | B §8 | `shared/pkg/middleware/gin_skywalking.go` | 0.5d |
| P2-20 | BFF gRPC dial 全部 `insecure`（依赖 K8s NetworkPolicy） | D §M4 | `web-bff/main.go:52-54` | 1-2d |
| P2-21 | migrate.sh 无版本表，新增 svc 必须改 SERVICE_ORDER 字符串 | D §B4 | `deploy/db/migrate.sh:36` | 2d |
| P2-22 | MV REFRESH 失败仅 log，无 metric | D §G4 | `analytics-svc/main.go:110-116` | 0.5d |
| P2-23 | dev mode 无 CORS 处理（直连 BFF 调试失败） | D §K1 | `web-bff/main.go:235-240` | 0.5d |
| P2-24 | 单一 `INTERNAL_API_KEY` 跨 svc 共享（违反最小权限） | D §M1 | `web-bff/main.go:190` + `ai-svc/main.go:294` | 1d |
| P2-25 | `applyDefaultFallbacks` 把 string 默认 localhost，prod 误配 silent fallback | D §F4 | `ai-svc/main.go:165-181` | 0.5d |

### P3（CI 收紧 / 长期重构）

- A §9-§12, §17, §18（Nacos Ephemeral 三态化 / ctx / 注释 / Heartbeat ctx）
- B §16, §17, §18, §19, §20, §21, §23, §24（metrics label 不对称 / 401 验证 / RecoveryWithWriter / 中间件顺序测试护栏 / Skywalking 跳过路径硬编码 / Shutdown 非幂等 / dashboard 缺 panel / gRPC health 无人调）
- C §10, §11, §16, §17（extractSw8Header 双份实现 / dlq.go itoa 重复 / proto+JSON switch 五处同步 / shared InMemoryProducer listener 满即丢）
- D §A5-§A7, §B8-§B9, §E1, §E3, §F2-§F3, §G1-§G2, §H1-§H4, §I2-§I3, §L1-§L2, §M2（XUserIDHeader 漂移 / gRPC metadata 长度未限 / panic stack 不上报 / nested savepoint / ConnMaxLifetime 与 idle_in_transaction_session_timeout 冲突 / metrics middleware 顺序 / gin.Recovery 默认 / lowercaseYAMLKeys / bool 零值问题 / migrate.sh 幂等 / runOutboxMigration 冗余 / TZ 漂移 / GORM column tag / 索引缺失 / 错误 envelope 三套 / 404 metrics 不可见 / LLM 上游 dial fail handler 反复试）

---

## 2. 文档与代码偏移（独立章节，不与代码 Bug 混算工作量）

| # | 文档声明 | 代码现实 | 来源 |
|---|---------|---------|------|
| DOC-1 | `observability-edge-gaps-from-code-review.md §A` 标 "🟢 2026-09-14 已 landed" | chat-svc producer span 未 EndSpan（§P0-6）+ ai-svc/analytics-svc consumer `defer` in for-loop（§P0-3）→ OAP 上 producer/consumer span 数据全错 | B §D5 + C §21#9 |
| DOC-2 | `observability-sprint-b.md §2.3 修 3` 承诺 `SKY_FAILURE_MODE` env | 全仓 grep 零命中，仅测试注释提及 | B §D1 |
| DOC-3 | `observability-sprint-b.md §3.3 PR-OBS-3` 承诺 `ExpandShellEnvDefaults` helper | `shared/pkg/config/` 无 expand.go；6 svc main.go 仍是 `MustLoad → ApplyEnvOverrides` | B §D2 |
| DOC-4 | `observability-sprint-b.md 附录 A` 描述 "Stage 45 端到端 5 svc trace 通" | §P1-1 InstrumentGORM/Redis 全仓无调用 + §P0-1/2 BFF gRPC 无拦截器 + §P0-3/6 Kafka span bug → 端到端 50% 失效 | B §D4 |
| DOC-5 | `observability-compose-gap.md §1.5` "三层全失效" 描述 | 现状仍部分成立：OAP UI 看的数据全错（因 §P0-3/6 + §P1-1），但 Prometheus / Loki / Grafana 已起 | B §D8 |
| DOC-6 | 决策 6 "JSON 日志 + trace_id 串联" | 跨 APISIX 边界断（§P1-3 APISIX otel 死配置 + file-logger 无 trace_id） | B §D9 |
| DOC-7 | `nacos-enablement-dev.md §四 PR-3 验收` "smoke §契约 7 必须新增" | `scripts/smoke_data_layer.py` 全文 grep 无 `nacos` / `Nacos` / `契约 7` | A §19 |
| DOC-8 | `nacos-enablement-dev.md residuals` "SDK v2.3.5 ListenConfig 偶发不回调" | SDK 已升级 v2.4.3（import 路径证实），残留是否仍存未验证 | A DOC 表 |
| DOC-9 | `nacos-enablement-dev.md residuals` "🟢 Stage 88 落地 llm-service Nacos" | `llm-service/main.py:88` 注册失败被 `except Exception` 吞，仍"半启用"（§P1-9） | A DOC 表 + §P1-9 |
| DOC-10 | 6 份 `nacos_boot.go` 注释 "Stage 31 PR-12 会抽到 shared/pkg/nacosboot" | PR-12 从未落地；6 份仍复制粘贴 | A §20 |
| DOC-11 | `nacos_register.go:57-59` 注释 "0 值时使用包级默认（PR-1: false 持久实例）" | 包级默认是 `defaultRegisterEphemeral = true`（行 41，stage-52 已修复）；注释未同步 | A DOC 表 |
| DOC-12 | `web-bff/main.go:172-174` 注释 "dev fallback to Authorization JWT parsing" | 注释承诺的功能从未实现（§P2-16） | D §J3 |
| DOC-13 | `web-bff/main.go:71` 注释 "X-Trace-Id header 由 APISIX 路径注入" | `deploy/apisix/seed.sh` 未注入该 header（仅 cors expose_headers 列出）→ trace_id 永远空 | D §J4 |
| DOC-14 | `AGENTS.md §2.5` 强调"每轮结束必须 git push + 删 branch" | 未自动化，无法验证残留分支被删 | D §J1 |
| DOC-15 | `stage-92 §一 PR-1/2` "chat-svc producer 注入 sw8 + ai-svc consumer CreateEntrySpan 重建父 trace" | sw8 注入 ✅，但 span 数据失真（见 DOC-1） | C §21#1-2 |

---

## 3. 跨链路交叉问题（多处同因 → 合并修复）

### 3.1 "BFF → 下游 gRPC" 链路空载
- §P0-1：5 个下游 gRPC conn 无拦截器、无 metadata 注入
- §P2-20：5 个 conn 全部 `insecure`
- §P2-24：内部 API key 单一共享

**合并修复**：BFF 增加 `grpcDialer` 配置化（interceptor 链 + mTLS 可选 + per-svc API key），1 个 PR 解 3 项。工作量 ~2d。

### 3.2 "Kafka trace" 链路失真
- §P0-3：ai-svc / analytics-svc consumer `defer` in for-loop
- §P0-6：chat-svc producer span 未 EndSpan
- §P1-2：DLQ publisher 未传播 sw8

**合并修复**：抽 `shared/pkg/trace/kafka.go` 提供 `CreateProducerSpan(ctx, topic) (ctx, span, err)` + `EndConsumerSpan(span, err)` helper，3 svc 共用，1 个 PR 解 3 项 + 消除 §C-§10 双份实现。工作量 ~1.5d。

### 3.3 "Kafka producer 关闭" 链路丢消息
- §P0-4：chat-svc `os.Exit(0)` + defer 不执行
- §P0-5：InMemory fallback 静默击穿 outbox
- §P1-16：chat-svc producer `Net.DialTimeout` 默认 30s

**合并修复**：chat-svc 重做 graceful shutdown（信号 → `relayCancel` → 等 relay 退出 → `kp.Close()` flush → `r.Shutdown` → exit），同时把 `kafka_publisher.go` 迁到 `shared/pkg/messaging/kafka_producer.go` 统一 timeout 配置 + 禁掉 InMemory fallback（dev 也走 stub publisher 显式 mock）。工作量 ~2d。

### 3.4 "全栈 trace 接通但 SQL 段黑盒"
- §P1-1：InstrumentGORM / InstrumentRedis 全仓未调
- §P1-2：DLQ 不传 sw8
- §P1-3：APISIX file-logger 无 trace_id
- §P1-4：Promtail 不采 svc stdout

**合并修复**：先修 §P1-1（最大价值），同时 Promtail 加 docker_sd_config 自动发现，APISIX 加 `serverless-post-function` 注入 trace_id 到 X-Trace-Id header。工作量 ~3d。

### 3.5 "auth 中间件无来源校验"
- §P0-7：GinAuthMiddleware 信任未签名 header
- §P0-10：BFF `JWTSecret` 硬编码默认
- §P2-16：BFF `TrustAPISIX=false` 分支死代码

**合并修复**：BFF `JWTSecret` 默认值删除 + 启动 fail-fast；GinAuthMiddleware 加 `AUTH_REQUIRE_APISIX_IP=true` 启动选项（k8s pod CIDR 白名单）；删除 BFF 死分支，注释改为"dev only"。工作量 ~2d。

---

## 4. 工作量估算（汇总，按 P0/P1 分层）

| 层 | 项数 | 工作量 | 备注 |
|---|------|--------|------|
| **P0 全部** | 10 | ~8-10 人天（紧凑 5-6 人天） | 建议分 3 个 sprint 串行：P0-1/2/6（trace 1 天）→ P0-3/4/5（Kafka 2 天）→ P0-7/8/9/10（auth + 数据完整性 2 天）|
| **P1 全部** | 28 | ~18-22 人天 | 建议分 2 个 sprint：observability sprint（Nacos 1 天 + 可观测 4 天 + Kafka 4 天）|
| **P2 全部** | 25 | ~12-15 人天 | 1 个 sprint |
| **P3 全部** | ~25 | 长期 | CI + 重构，季度计划 |

**总计**：约 **35-45 人天**（紧凑做 ~20-25 人天，~1.5-2 个月）。

---

## 5. 建议优先级

1. **本周**：修 §P0-3 + §P0-6 + §P0-1（合并 PR，1-1.5d）—— 这 3 项修完 Stage 92/93 做的 trace 跨进程工程才真正"端到端可见"。同步修 §DOC-1（更新 edge-gaps §A 标 "🟡 部分落地"）。
2. **本 sprint 剩余**：§P0-4 + §P0-5 + §P0-8 + §P0-10（数据完整性 + 鉴权默认值，2-3d）。
3. **下一 sprint**：§P0-7 + §P0-9 + §P0-2 + §P1-1 + §P1-3 + §P1-4（auth + OAP DB 段 + Loki 联动，5d）。
4. **之后**：按 P1 → P2 → P3 顺序 + 跨链路合并方案（§3.1-3.5）。

---

## 6. 调研依据（4 个 agent 的"已读文件"汇总）

### Agent A — 服务注册链路

`emotion-echo-shared/pkg/config/*`、`shared/pkg/nacos/*`、6 svc `nacos_boot.go`、`web-bff/main.go`、`llm-service/main.py`、`docs/architecture/decisions.md`、`docs/plans/nacos-enablement-dev.md`、`deploy/docker-compose*.yml`、6 svc 启动配置 yaml。

### Agent B — 可观测链路

`shared/pkg/{skywalking,middleware,grpcinterceptor,metrics,logging}/*` 全量 + 6 svc main.go + `deploy/{prometheus,grafana,loki,promtail,apisix}/*` + 全部 docker-compose。

### Agent C — Kafka 事件管道

`shared/pkg/messaging/*`、`chat-svc/internal/{events,outbox}/*`、`ai-svc/internal/consumer/*`、`analytics-svc/internal/kafka/*`、3 svc main.go、`deploy/docker-compose.{infra,apps}.yml`、`proto/chat_events.proto`、`shared/pkg/{eventrow,chatevents}`、stages 43/73/86/92/93、`scripts/smoke_data_layer.py`。

### Agent D — 中间件与基础设施

`shared/pkg/{middleware,dbconnect,config,grpcinterceptor,grpcerr,password,bootstrap,logging,metrics,messaging,sentry}/*` + 6 svc main.go + 6 svc internal/config + 全部 migrations + `scripts/smoke_data_layer.py`。

---

## 7. 不在本计划范围（与已 landed / 已知文档一致）

- `kafka-pipeline-pending-decisions.md` D1-D8 已登记项 → 本计划 §P0-5, §P1-11, §P1-14, §P2-13, §P2-12 已交叉引用
- `observability-edge-gaps-from-code-review.md` 已 landed §A → 本计划 §DOC-1 已说明其"🟢"标签应改为"🟡"
- `nacos-enablement-dev.md` residuals 中 §P1-6/7/8/9/10（composes + smoke 契约 7）已在本计划登记为 P1
- Stage 92/93 的 sw8 注入功能本身（仅 span 数据正确性问题在 §P0-3/6）

---

## 8. 风险与缓解

| 风险 | 等级 | 缓解 |
|---|---|---|
| 修 §P0-1/2 时漏改一处 client | 中 | 共用 `sharedgrpc.NewClientWithDefaults()`，4-5 个 client 一并替换 |
| 修 §P0-3 时测试覆盖不全 | 中 | ai-svc `consumer_test.go` 已覆盖 ConsumeClaim 入口；§P0-3 修复模式可在 OAP UI 实测 e2e |
| 修 §P0-5 时 dev 模式被迫改 startup | 低 | 加 `STARTUP_REQUIRE_KAFKA=true` env，dev 默认 false |
| 修 §P0-7 时白名单冲突 | 低 | 默认白名单 + `AUTH_BYPASS_PATHS` env 注入 |
| 修 §P0-9 时影响业务读视图 | 中 | 先改 NOLOGIN + `SET LOCAL ROLE analytics_reader`，再评 |
| §3.4 trace 串联横跨 6 svc + deploy | 高 | 建议独立 sprint，专用 docker_sd_config + OAP 9.7+ 升级 配套 |

---

## 9. 文档更新建议（待本计划 PR 合并后）

- [ ] `docs/plans/README.md` 索引表加本计划条目
- [ ] `docs/legacy-plans/landed/observability-edge-gaps-from-code-review.md` §A 标 "🟡 部分落地"
- [ ] `docs/plans/observability-sprint-b.md` §2.3 修 3 / §3.3 PR-OBS-3 改为 "未落地" 或新建追踪 plan
- [ ] `docs/architecture/decisions.md` 决策 6 trace_id 串联标注 "跨 APISIX 边界待补"
- [ ] `docs/architecture/decisions.md` 决策 10 Nacos 加 "客户端定时拉取未实现" 备注
- [ ] `docs/architecture/decisions.md` 决策 12 APISIX 信任头加 "svc 端口若直连可伪造" 风险
- [ ] `deploy/.env.local.example`（gitignored 模板）加注释要求 prod 重新生成

---

> 本计划基于代码事实（4 个子 agent 各读完 27-107 个文件 + 部署配置 + 全部 stage/plan 文档），未修改任何代码。**唯一人工复核点**：§P0-10 D §D4（`.env.local` gitignore）已 verify 为真 ignored，从原文 P0 降为 P1。
