---
stage: cl-03
type: execution-report
status: in-progress
created: 2026-10-16
plan: plan.md
---

# CL-03 · Kafka/可靠性闭环 — 执行记录

> 体例（RUNBOOK §0 / §6.3）：起点 / 实施 / 证据 / 结论 / 经验。**执行中与收口时同步更新**。
> ⚠️ **阶段状态由审计 / 第二方判定，执行者不自行宣布 `done`**。

## §1 起点（建档时亲核的现状）

建档轮（2026-10-16）实读 14 个代码/配置文件 + 1 个只读盘点子代理复核，校正了三处**过期文档**：

| # | 计划文档的说法 | 实测真相（file:line） |
|---|---|---|
| 1 | 「D8-2 `extractSw8Header` 双份未收敛，仍 open」 | **已收敛**：真实现 `emotion-echo-shared/pkg/messaging/sw8_header.go:23`；ai-svc `internal/consumer/consumer_failure.go:125-127` 与 analytics-svc `internal/kafka/consumer.go:368-370` **只剩转发薄包装** ⇒ 文档漂移（本阶段修文档，无代码改动） |
| 2 | 「D4 analytics-svc maxRetries 硬编码、无配置通道」 | **已落**（Stage 96）：`internal/config/config.go:29` + `KAFKA_MAX_RETRIES` env（`main.go:59-64`） |
| 3 | 「D2 outbox 无清理」 | **已落**：`internal/outbox/cleanup.go` + `DeleteOlderThan`；`OUTBOX_CLEANUP_ENABLED` **默认 false**（`internal/config/config.go:118`） |

**仍成立的三处**（本阶段要做）：
- **trace 断点在 relay**：`main.go:245` 用后台根 ctx → `outbox/relay.go:99` → `events/kafka_publisher.go:133` 的 `CreateExitSpan` 建**根 span**；消费侧 `ai-svc/internal/consumer/consumer.go` 已有 sw8 extractor（这段是连的）。
- **outbox 表无 trace 列**：`migrations/c001_create_outbox_events.sql` / `repository/outbox.go:42-53`。
- **DLQ 四问题**：双失败黑洞（`ai-svc/consumer.go:159`、`analytics-svc/kafka/consumer.go:267` 的 `MarkMessage` **无条件前进**）· Noop 静默（`ai-svc/consumer/dlq.go:53-63`、`analytics-svc/kafka/dlq.go:41-43`）· 只进不出（回放工具为人工）· **堆积无告警**（`deploy/prometheus/rules/kafka-lag.yml:5` 自述「DLQ 不告警」；`kafka-consumer-lag.json:124` 面板 title 显式标 `non-alerting`）。

## §2 实施

### Phase 1 · outbox `trace_sw8` 列（🟢 不需栈）—— 2026-10-16

**RED（先写会失败的测试，已取证）**

新增两个测试文件，先跑出失败：

| 文件 | 断言 |
|---|---|
| `emotion-echo-chat-svc/internal/repository/outbox_trace_sw8_test.go` | `OutboxEvent` 有 `TraceSW8 string` 字段 + `gorm:"column:trace_sw8"` 标签；InMemory repo 原样保留；空值合法 |
| `emotion-echo-chat-svc/migrations/c010_trace_sw8_migration_test.go` | ① 迁移文件存在且命名合规 ② `ADD COLUMN IF NOT EXISTS` 幂等 + **可空**（历史行 / tracer=nil）③ 注释含「变更原因 / 跨域影响 / 回滚方法」（决策 22 §四 第 1 步）④ **内联 DDL 不得漂移**（`main.go:runOutboxMigration`）⑤ **ADR §二 表清单同步**（决策 22 §四 第 9 步） |

RED 证据（命令 + 输出）：
```
$ go test ./internal/repository/ -run "TestOutboxEvent_TraceSw8|TestOutboxEvent_HasTraceSw8" -count=1
internal\repository\outbox_trace_sw8_test.go:47:3: unknown field TraceSW8 in struct literal of type OutboxEvent
FAIL	emotion-echo-chat-svc/internal/repository [build failed]

$ go test ./migrations/ -run "TestC010" -count=1
--- FAIL: TestC010_TraceSw8Migration_ExistsAndNamed / _IdempotentAddColumn / _CommentContract / _InlineDDLNotDrifted / _ADRTableSynced
FAIL	emotion-echo-chat-svc/migrations	2.242s
```

**GREEN（最小实现）**

| 改动 | 内容 |
|---|---|
| `internal/repository/outbox.go` | `OutboxEvent` 加 `TraceSW8 string \`gorm:"column:trace_sw8"\``（显式列名 —— GORM 对 `TraceSW8` 这类「缩写结尾」字段的推导不可靠） |
| `migrations/c010_add_trace_sw8_to_outbox.sql`（新增） | `ALTER TABLE emotion_echo_chat.outbox_events ADD COLUMN IF NOT EXISTS trace_sw8 VARCHAR(512);` + 注释写明变更原因 / **跨域影响=无**（不进 payload/proto/视图，故不需同步 `deploy/db/02-create-tables-in-schemas.sql`，outbox_events 本就是 chat-svc 自治表）/ 回滚方法（**先回滚代码再 DROP 列**） |
| `main.go`（`runOutboxMigration`） | 内联 DDL 同步加列 + **补一条 `ADD COLUMN IF NOT EXISTS`**（老环境的表已存在，`CREATE TABLE IF NOT EXISTS` 不会补列）+ 注释标明"两份事实源必须同步" |
| `docs/architecture/adr/adr-2026-09-chat-svc-table-deps.md` | §2.3 表清单加 `trace_sw8` 行 + 「两份事实源」警示（决策 22 §四 第 9 步） |

GREEN 证据：
```
$ go test ./migrations/ ./internal/repository/ -count=1
ok  	emotion-echo-chat-svc/migrations	7.477s
ok  	emotion-echo-chat-svc/internal/repository	6.477s

$ go vet ./migrations/ ./internal/repository/     → 无输出（干净）

$ go test ./... -count=1        （chat-svc 全仓回归）
ok 包数 = 11　FAIL 行数 = 0
```

**决策 22「字段变更 checklist」9 步落实情况**

| 步 | 状态 | 说明 |
|---|---|---|
| 1 migration 文件（幂等 + 原因/跨域/回滚注释） | ✅ | `c010_add_trace_sw8_to_outbox.sql` |
| 2 同步集中 initdb | **N/A** | `outbox_events` **不在** `deploy/db/*.sql` 内（实测 grep 无命中）—— 它是 chat-svc 自治表，ADR §2.3 亦如此记 |
| 3 model struct + 映射单测 | ✅ | `outbox.go` + `outbox_trace_sw8_test.go` |
| 4 `internal/types/types.go` | **N/A** | 该列不出现在 gRPC/HTTP DTO |
| 5 `proto/chat.proto` | **N/A** | 同上（且本轨按文件面隔离**不碰 proto**） |
| 6 跨域影响评估 | ✅ | 无影响：不进 Kafka payload、无视图依赖、无前端字段 ⇒ 详见迁移注释 |
| 7 集成测试 | ⏳ | 本轨只跑了 chat-svc 全仓；跨 svc 集成（ai-svc/analytics-svc/BFF）+ smoke 属 🔴 需栈，排队等栈 |
| 8 跨域 regression 守护 | ✅ | 本阶段新增的 `TestC010_TraceSw8_InlineDDLNotDrifted` 即"schema 漂移检测"（v1：钉内联 DDL 不漏列） |
| 9 文档同步 | ✅ | ADR §2.3 已更新（本 ADR）；另本阶段修 D8-2/D4/D2 三处文档漂移（Phase 6） |

**Phase 1 发现的额外事实（范围外，待记账）**
- `main.go` 内联 DDL 与 `c001` 迁移**已漂移**：内联版缺 `idx_outbox_attempts` 索引（`c001` 有）。本次只补了 `trace_sw8`（TDD 最小实现原则），**索引漂移未修**，留作后续（§6 R3）。
- `migrations/` 目录同时存在 `008_p0r27_ddl_drift_test.go`（非迁移文件占了 `008` 编号位）⇒ 实际 SQL 迁移已到 `c009`、测试文件编号与之并行，命名空间混用（本阶段新增文件用 `c010_*.sql`，未触碰既有命名）。

### Phase 4a · DLQ 堆积告警（🟢 不需栈）—— 2026-10-16

**为什么先做它**：DLQ 硬化四问题里，只有"堆积不告警"是**纯静态、零 [M] 依赖、且有现成机器门禁**的一块（`deploy/obs_monitoring.test.js` 已接 CI：`.github/workflows/doc-drift-check.yml:96`）。另外三问题（双失败黑洞 / Noop 静默 / 回放程度）都各自需要语义裁定。

**RED（已取证）**：在 `deploy/obs_monitoring.test.js` 新增 group ⑦，先跑出 6 项失败：
```
⑦ DLQ 堆积可告警 (CL-03)
  ✗ kafka-dlq.yml 缺 DLQMessagesAccumulating —— DLQ 有新消息进入时无任何告警
  ✗ 告警 expr 未锚定 chat-events-dlq（期望含 kafka_topic_partition_current_offset{topic="chat-events-dlq"}）
  ✗ k8s 内联副本未同步该告警（名字或 anchor 缺）—— k8s 环境此告警不会响
  ✗ DLQ 面板仍标 "non-alerting" —— 已有堆积告警，标签与事实不符
  ✗ DLQ 面板未引用 DLQMessagesAccumulating —— 看板与规则脱钩
  ✗ kafka-lag.yml 头部仍写 "DLQ 不告警" —— 与新增的堆积告警矛盾
契约测试失败: 1 组断言未通过
```

**GREEN（四处同步改）**

| 文件 | 改动 |
|---|---|
| `deploy/prometheus/rules/kafka-dlq.yml` | 新增 `DLQMessagesAccumulating`：`sum(increase(kafka_topic_partition_current_offset{topic="chat-events-dlq"}[10m])) > 0`，`for: 0m`，`severity: warning`；注释写明**为什么不能用 consumer lag**（DLQ 无消费组 ⇒ series 恒不存在，E2E-F-150 的老坑）与**已知限制** |
| `charts/.../prometheus/templates/configmap-rules.yaml` | k8s 内联副本同步同一告警（`{{ $value }}` 按该文件既有风格转义为 `` {{ `{{ $value }}` }} ``） |
| `deploy/grafana/dashboards/kafka-consumer-lag.json` | DLQ 面板标题 `— non-alerting` → `— alerting: DLQMessagesAccumulating`（**CL-01 D-05 同类教训**：标签必须与事实一致） |
| `deploy/prometheus/rules/kafka-lag.yml` | 头部注释 `DLQ 不告警` → 指向新告警（旧注释是**文档漂移源**） |

**口径说明（重要，防过度声称）**：这个告警表达的是「**近 10 分钟有新消息进入 DLQ**」，**不等于**「当前还剩多少条未处理」。真正的深度需要 `kafka_topic_partition_oldest_offset` —— 它是 kafka-exporter 的标准指标之一，但**本栈此前从未在任何规则/面板中使用过 ⇒ 可采性未经实测**，故不臆断、不写死，已在规则注释中登记为**已知限制**（起栈后实测再决定是否补"深度"告警）。

**GREEN 证据**
```
$ node deploy/obs_monitoring.test.js
⑦ DLQ 堆积可告警 (CL-03)
  ✓ kafka-dlq.yml 含 DLQMessagesAccumulating
  ✓ 告警 expr 锚定 chat-events-dlq 的 topic offset
  ✓ k8s 内联副本含同名告警 + 同 anchor
  ✓ DLQ 面板不再声称 non-alerting
  ✓ DLQ 面板引用告警名，便于值班从看板跳到规则
  ✓ kafka-lag.yml 不再声称 DLQ 不告警
契约测试全部通过            （组⑤：k8s 内联规则覆盖 dev 全部 14 条告警 / 两侧无差集）

$ bash scripts/test_helm_prometheus_render.sh
  ✓ helm template charts/prometheus 渲染成功
  ✓ chart 内联告警与 dev 侧 14 条完全一致
  ✓ 负向对照通过：移除 rules 挂载后断言如期变红
Summary: PASS=8 FAIL=0

$ python yaml/json 可解析性 → kafka-dlq.yml: [AIDLQPublishFailure, AnalyticsDLQPublishFailure, DLQMessagesAccumulating]；dashboard JSON ok
```

> 🔴 **仍需栈**：告警 expr 是否真能返回非空 series、以及真触发一次 `firing` → 送达（T-09），要起栈造堆积，排队等 dev 栈锁。

### Phase 6 · 文档漂移修正 + 账本登记（🟢 不需栈）—— 2026-10-16

**改了什么**

| 文件 | 改动 |
|---|---|
| `docs/plans/kafka-pipeline-pending-decisions.md` | ① `D8` 行 `⏳ 仍 open` → **✅ 已收敛**（附 `shared/pkg/messaging/sw8_header.go:23` + 两个薄包装行号），并标注原状态是**文档漂移**；② 两处收口摘要里的"D8-2 仍 open"各加一条更正注（**保留原文 + 更正**，不涂改历史）；③ 新增「**CL-03 处置**」表：D1~D8 **逐条终态**（D1 并入 CL-F-15 / D2·D4 ✅ 已落 / D3·D5·D6 登记触发条件 / D7 等产品语义拍板 / D8 ✅），不留"待定" |
| `docs/closure-roadmap/discovered-unresolved.md` | 新增 **`CL-F-22`**（outbox DDL 两份事实源漂移：内联版缺 `idx_outbox_attempts`）与 **`CL-F-23`**（DLQ 无真实"深度"指标；`oldest_offset` 可采性未实测）—— 6 列硬契约、归属均写 CL-03 并附触发条件 |

**为什么"保留原文 + 加更正注"而不是直接删**：账本与计划文档的价值在于可追溯；涂掉"D8-2 仍 open"会让"当时的判断为什么错"永久丢失。RUNBOOK 的 anti-patterns 要求**报告与代码一致**，而一致性应当通过显式更正达成，不是通过抹除历史。

**证据**：`git diff` 三处 hunk；`D8` 行新状态引用的行号均来自本次实读（`sw8_header.go:23` / `consumer_failure.go:125-127` / `consumer.go:368-370`）。

### Phase 2 / Phase 3 · trace 贯通（🟢 不需栈）—— 2026-10-16

**做了什么（按 `CL-D-15` = A）**

| 环节 | 改动 | 文件 |
|---|---|---|
| 原语 | shared `Tracer` 新增 `EncodeSW8(ctx, opName)`（**只加这一个方法**：恢复侧用既有 `CreateEntrySpan` 即可，无需第二个） | `emotion-echo-shared/pkg/grpcinterceptor/{tracing.go,tracing_go2sky.go}` + 6 处 mock 同步 |
| 写侧 | `outbox.EncodeTraceSW8(ctx, op)`（读 `GlobalTracer()`，CL-F-14 语义）+ 4 处 `CreateInTx` 调用点接线 | `chat-svc/internal/outbox/sw8.go`、`internal/logic/{send,create,delete}*logic.go` |
| 读侧 | `relay.resumeTraceCtx(ctx, sw8)` + `publishOne` 在恢复后的 ctx 上发布、err 透传 `EndSpan` | `chat-svc/internal/outbox/relay.go` |
| 架构 | **ADR 决策 42**：`docs/architecture/adr/adr-2026-10-outbox-trace-context.md`（含被否备选 5 条、代价与已知限制） | + `docs/architecture/decisions.md` 索引/变更记录 |

**关键证据**

```
# 进程内"存 → 恢复 → 仍同一条 trace"（本阶段核心判据，含反向对照）
$ go test ./pkg/grpcinterceptor/ -run "EncodeSW8|SW8RoundTrip" -count=1
ok  	github.com/emotion-echo/shared/pkg/grpcinterceptor

# relay 侧
$ go test ./internal/outbox/ -count=1        → ok（含 RestoresParentCtx / EmptyTraceSW8 /
                                               PublishErrorPassedToSpanEndSpan / NoInjectedTracer / NilSpan 5 例）
# 写侧接线 + helper
$ go test ./internal/outbox/ ./internal/logic/ -run "TraceSW8|EncodeTraceSW8" -count=1 → ok

# 全仓回归（7 个 Go 模块）
shared ok=23 / ai 15 / chat 11 / analytics 11 / user 7 / assessment 8 / bff 10，FAIL=0
```

**过程中实测到的两条 go2sky 事实（都写进了代码注释，避免后人重踩）**
1. **无 reporter 的 tracer 永远编码不出 sw8**：`initFlag==0` ⇒ `createNoop` 提前返回、**injector 根本不被调用**（`trace.go:159`）⇒ 测试必须带 reporter（复用同包既有 `fakeReporter`）。
2. **exit span 的 `peer` 必须非空**（`trace.go:156`，空串 ⇒ `errParameter "parameter are nil"`）⇒ 编码哨兵 peer 取 `"trace-context"`。

**如实说明证据边界**：写侧调用点用的是**源契约断言**（无法在单测里造真实 gorm 事务，仓内同型先例见 `createconversationlogic_test.go:141`）；**"SkyWalking UI 里连成一条"的端到端证据归 Phase 7（🔴 需栈）**，本档不声称已取得。

## §3 待办（下一步）

- ✅ **§7 [M] 四项裁定已落定（2026-10-16 用户，全部采纳推荐项）** → 登记为 `CL-D-15`~`CL-D-18`（见 `decisions.md`）
  ⇒ **Phase 2/3 已完成**（见上）；**4b / 4c / 5 解除阻塞**，可按裁定开工：
  - **Phase 2/3**：给 shared `Tracer` 接口加 `EncodeSW8` / `ContextWithSW8`（`CL-D-15` = A）→ 写侧捕获 + relay 恢复父 ctx；**须配 ADR**（动 7 svc 共享接口）
  - **Phase 4b**：DLQ 投递失败不 `MarkMessage`（`CL-D-16` = A）—— 落地前**逐 handler 核实幂等**，非幂等者单独处置
  - **Phase 4c**：一键回放全部积压 + 回放计数（`CL-D-17` = B）；不做定时自动回放
  - **Phase 5**：Kafka 初始化后台重试 + relay 不假 `sent`（`CL-D-18` = C）+ 回写 ADR-19
- 🔴 需栈项（Phase 0 基线取证 / Phase 7 端到端 trace 贯通 + 告警 firing）：dev 栈由 **D 轨（CL-05）持锁**（2026-10-16 06:16 取锁），本轨排队第 5 位。

## §5 本轮并行的治理处置（2026-10-16，非本阶段产物）

| 对象 | 处置 | 结果 |
|---|---|---|
| PR **#222**（`docs/cross-track-comms`：跨轨沟通条款，用户明令） | 无冲突，只是被分支保护的"必须与 main 同步"卡住 ⇒ 先 `update-branch` 再 squash 合并 | ✅ 已合 → `178b3706` |
| PR **#212**（`docs/stage-start-pr-rule`：每阶段开工先建 PR，用户明令） | 其 base 过期 ⇒ 在 `AGENTS.md` **真内容冲突**（`mergeable_state=dirty`）。按 **H1「禁止 rebase 他人分支 / 代他人解冲突」**，F 轨**不碰其分支**，改为在最新 main 上**重新落地同一意图**（新 PR #231）→ 合并后在 #212 留言并关闭 | ✅ 重新落地已合 → `7220945e`；#212 已留言 + 关闭（可重开） |

> 说明：关闭 #212 而非长期挂着的理由 —— 一条**永久冲突且内容重复**的 PR 会让"哪些工作还没落地"失真（AGENTS §2.5 要防的残留）。留言里明确写了"若重新落地漏了原意，请重开或开新 PR 补上"。

- **§7 [M] 四项裁定（2026-10-16 已问、用户未答复）** —— M1 sw8 存取原语 / M2 DLQ 投递失败语义 / M3 回放程度 / M4 `CL-F-15` fallback 改法。
  依 plan §7 的推荐项（M1=A 加接口原语、M2=A 不 Mark 留待重投、M3=B 一键回放+计数、M4=C 后台重试+不假 sent）**待用户追认后再动**：
  其中 M1/M4 会改**共享接口与投递语义**（属架构改动，须配 ADR），M2 会改**消息投递语义** —— 三者都不宜在未追认时自行落地。
  **Phase 2/3（trace 写侧捕获 + relay 恢复父 ctx）与 Phase 5（fallback）暂缓，等裁定；Phase 4b/4c（Noop 可见化 / 回放程度）同理。**
- 🔴 需栈项（Phase 0 基线取证 / Phase 7 端到端 trace 贯通 + 告警 firing）：dev 栈由 **D 轨（CL-05）持锁**（2026-10-16 06:16 取锁），本轨排队第 5 位。
- Phase 6（文档收尾：D8-2/D4/D2 三处漂移修正 + D3/D5/D6/D7 逐条终态）不依赖 [M]，**可先行**。

## §4 经验

- **文档漂移是本阶段第一批"发现"**：三份计划里有三处已落地项仍写着 open（D8-2/D4/D2）——建档期实读代码是唯一可靠的判据（AGENTS §〇 的价值再次验证）。
- **`MarkMessage` 无条件前进**这类"假绿信号"（offset 前进 = 表面成功）与 CL-01 发现的 `outbox status=sent` 假绿是**同一类陷阱**：只看状态字段会把"丢消息"读成"成功"。
