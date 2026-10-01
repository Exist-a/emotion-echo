---
stage: e2e-24
title: 消息链路（outbox→Kafka→consumer→DLQ 全链 + 重试/死信/回放）
type: transformation
status: done
created: 2026-10-01
last-updated: 2026-10-02（收口：20/20 PASS，第二方核对两轮通过，用户批准收口）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策；回放工具形态为执行期内 [M] 决策点（见 §8）
related-findings: [E2E-F-149, E2E-F-12, E2E-F-150, E2E-F-96, E2E-F-160]
---

# E2E-24 消息链路 — 详档（任务书）

> **类型**：transformation —— 消息链路的**代码全部存在**（outbox relay / Kafka publisher / sarama consumer / DLQ publisher / 死信状态机 / Nacos 热更参数容器），但它**当前不能证明自己是对的**：
> ① **链路第一步就卡死**——账本 E2E-F-149：`user_behavior_events` 分区表化后唯一约束变为 `(event_id, occurred_at)`，而 consumer 写库仍是 `ON CONFLICT (event_id)` ⇒ **每条消息消费必失败**、重试 3 次进 DLQ，报表数据源实际写不进任何行；
> ② **死信堆积无处置**——账本 E2E-F-150：`chat-events-dlq` 已堆积 **24 条**死信，此前在 Grafana 上永不可见（E2E-22 已修面板可见性，**但清理/回放仍无机制**）；
> ③ **回放是纸面承诺**——账本 E2E-F-12：全仓 grep `replay` **零命中**，outbox dead 行注释写"保留供人工排查/回放"（`outbox.go:36-38`）却无任何回放路径；告警规则注释里的"回放"是手工 psql UPDATE。
> **本阶段把"消息链路有代码"变成"全链实测跑通 + 重试/死信/回放逐点可断言 + 历史死信有交代"。**

> **依据**：roadmap §第七批 E2E-24 行（"outbox→Kafka→consumer→DLQ 全链 + 重试/死信/回放"）+ 本阶段名下 3 条留账（E2E-F-12 / E2E-F-149 / E2E-F-150）。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：无依赖前置（roadmap 排期 ✅）。上一阶段 E2E-23 ✅ done（2026-09-30 用户批准收口，PASS 38 / FAIL 0 / BLOCKED 0 / N/A 2）。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件**，非引用历史结论（AGENTS §〇 文档功课）。执行期若发现与本节不符，**以实测为准并回填本节**（AP-02）。
>
> **已读实现文件**：`emotion-echo-chat-svc/internal/outbox/relay.go`、`outbox/ops.go`、`repository/outbox.go`、`emotion-echo-analytics-svc/internal/kafka/consumer.go`、`kafka/dlq.go`、`internal/repository/event_repository.go`、`emotion-echo-analytics-svc/migrations/a008_partition_user_behavior_events.sql`（配套 `a008_partition_pk_test.go`）、`deploy/docker-compose.infra.yml`（Kafka init）/`docker-compose.apps.yml`（三个 KAFKA 消费方）。
> **已查 ADR / 决策**：`docs/plans/kafka-pipeline-pending-decisions.md`（D1~D8）、账本 F-149/F-12/F-150/F-160、roadmap E2E-24 行。
> **smoke**：本轮未跑（未启动 dev 栈，避免占 `.devmode-session`）；契约 1/2/6 的 smoke 断言列为开工前置与收口门槛（§3 / §5）。

### 0.1 计划期实测事实表

| # | 事实 | 证据（`文件:行号`） |
|---|------|--------------------------|
| **F-a** | **F-149 未修，consumer 写库必失败** | `event_repository.go:234-242` `Create` 对非空 `EventID` 用 `OnConflict{Columns: event_id, DoNothing: true}`；而 `a008_partition_user_behavior_events.sql` 把表改为按 `occurred_at` 月分区，唯一约束为 **UNIQUE (event_id, occurred_at)**（`a008_partition_pk_test.go` 注释明确记录该 PG 限制）⇒ `SQLSTATE 42P10`（no unique constraint matching ON CONFLICT）。账本实测：发 1 条消息 → `handle 359 failed (will retry attempt=1/3)` → 3 次后进 DLQ |
| **F-b** | **outbox relay 死信状态机存在且可热更** | `relay.go:38-58` `MaxAttempts` 默认 100、`Ops.Snapshot()` 每轮 flush 重读（Nacos 热更生效）；`:111-126` MarkFailed → attempts 超限 → `MarkDead` + `IncDead()` Prometheus 计数（`metrics.go`，E2E-22 的 `outbox-dead.yml` 告警规则据此触发） |
| **F-c** | **outbox dead 行无回放路径** | `outbox.go:36-38` 注释："dead 行不再被 ListPending 扫描，**保留供人工排查/回放**"——但 `OutboxRepo` 接口只有 `ListPending/MarkSent/MarkFailed/MarkDead/DeleteOlderThan/Get`，**无按 id 重置回 pending 的方法**；全仓 `grep -rn "replay\|Replay"`（chat-svc + analytics-svc 内部）**零命中**（F-12 复核成立） |
| **F-d** | **consumer 重试计数是进程内存量** | `consumer.go:186` `attempts map[string]int`（attemptsMu 守卫）：不跨 rebalance、不跨进程重启（= 决策文档 **D3**，P2 未落地）。达上限（默认 3，F-160 已修配置通道）→ `handleFailure` 投 DLQ + `sess.MarkMessage`（`consumer.go:307-355`） |
| **F-e** | **DLQ 投递带诊断 headers，但 Noop 是默认** | `dlq.go:109-113` headers：`x-original-topic` / `x-error-reason` / `x-attempts`；`dlq.go:120-133` 投递失败自带 0/100ms/500ms 三次重试。**compose 已接线**：`docker-compose.apps.yml:203` `KAFKA_DLQ_TOPIC: chat-events-dlq`（analytics-svc）与 `:383`（ai-svc）——需执行期验证 `main.go` 实际调用了 `WithDLQ` |
| **F-f** | **DLQ 没有常驻消费者（设计如此）** | `chat-events-dlq` 仅 `kafka-exporter` 抓 offset 指标；E2E-22 实测堆积 24 条且原面板用 lag 恒 0 series。**回放 = 把死信重发布回原 topic**，目前无任何工具 |
| **F-g** | **链路有第二个消费组** | `docker-compose.apps.yml:376-383` ai-svc 也消费 `chat-events`（group `ai-svc`，同 DLQ topic）⇒ 全链验证须覆盖**双消费组**；`chat-events` 主 topic 6 partition（`docker-compose.infra.yml:103` kafka-init 显式建），DLQ topic 靠 `KAFKA_AUTO_CREATE_TOPICS_ENABLE=true` 自动建（执行期核实分区数） |
| **F-h** | **KAFKA_ENABLED=false 的 dev fallback 路径存在** | `chat-svc/internal/events/dev_publisher.go`（DevEventPublisher 直写路径，ADR-19 PR-A1.4 后与 consumer 落库值同构）——契约 6 要求该路径不空跑 |
| **F-i** | **消费写库不带 ctx** | `consumer.go:417` `h.repo.Create(nil, be)` —— ctx 传 nil。gorm 侧 `WithContext(nil)` 目前不炸但绕过了取消/超时语义；执行期评估是否顺手收口（范围外发现只记账，不强制修） |
| **F-j** | **历史决策 D1/D2/D4 已落地、D3/D5 未落地** | D1（producer init 失败 fallback InMemory 击穿 outbox 承诺）✅ Stage 94 PR-3；D2（outbox sent/dead 行清理）✅ `outbox/cleanup.go` + Ops 保留天数热更；D4（maxRetries 配置通道）✅ F-160 commit `4d4b1f9`；**D3（attempts 不跨 rebalance）🔴 未落地**；**D5（relay 多副本需分布式互斥）🔴 未落地**（dev 单副本，扩容前置条件） |

### 0.2 开工复核（2026-10-01 实测回填，防止任务书事实表过期）

> 环境基线：19 容器 healthy、Nacos count:6、BFF downstream 全 ok、db-migrate/apisix-seed Exited(0)。

| # | 复核结果 | 证据 |
|---|---------|------|
| 1 | **F-149 仍在且实时复现**：echo/echo123 网关登录 → conv 360 发消息 → consumer `SQLSTATE 42P10`（`event_repository.go:238` 的 `ON CONFLICT ("event_id") DO NOTHING`），`user_behavior_events` 行数 163 不变 | consumer 日志 `handle 360 failed (will retry attempt=1/3 offset=82)` |
| 2 | **重试→DLQ 状态机工作**：attempt 1→4 → `failed after 4 retries → DLQ` → DLQ offset 24→25；死信 headers 完整（`x-original-topic=chat-events` / `x-error-reason=...42P10` / `x-attempts=4`） | kafka-get-offsets + kafka-console-consumer 读回 |
| 3 | **D3 实证**：重启 analytics-svc 后 attempt 计数清零、从 1/3 重新开始；outbox relay 半正常（行 status=sent） | 重启前后日志对照 |
| 4 | **DLQ 实时数 = 25**（24 旧 + 1 本轮 probe），与账本 F-150 的 24 一致（期间栈未跑） | `kafka-get-offsets.sh --topic chat-events-dlq` |
| 5 | **新发现 F-174（范围内）**：重试语义失真——sarama session 内不重投未 Mark 消息，attempt 计数靠后续同 key 消息推进；单条毒消息会永久阻塞分区（offset 82/83 卡住） | attempt 1/3 (offset 82) → 下一条直接是 83 |
| 6 | **新发现 F-175（范围内）**：a008 换表未推进 BIGSERIAL 序列 ⇒ F-149 修复后新行拿 id=1,2,3，与老行 id≤162 重叠（max(id)=162 < count=165）；序列 last_value=3 | psql 查询 2026-10-01 行 id=1,2,3 |
| 7 | **修复后运行时复验**：F-149 修复（conflict target 补 occurred_at，analytics-svc:v0.1.10）→ 新消息事件落库成功（0× 42P10，events 163→165）；F-175 修复（a010 setval）→ 新行 id=163 > 老 max 162 | psql 行 + 日志 |

### 0.3 本阶段三类工作定性1. **修真缺陷**（TDD 循环）：F-149 是本阶段的前置性修复——不修它，重试/死信测试点全部只能测出"每条消息都进 DLQ"这一种失败。
2. **补缺失能力**（TDD 循环）：DLQ 回放工具 + outbox dead 行回放能力（F-12/F-150）。
3. **验证既有实现**（测试点）：relay / consumer / DLQ / 热更 / 双消费组 / dev fallback 的正向与负向行为。

---

## 1. 阶段目标

| # | 目标 | 现状（有证据） | 目标态 |
|---|------|--------------|--------|
| 1 | **全链正向跑通** | F-149：每条消息消费必失败（F-a） | 发 1 条消息 → outbox pending→sent → `chat-events` → 双消费组落库 → §2.4 契约 1/2 断言通过 |
| 2 | **消费幂等可断言** | `ON CONFLICT DO NOTHING` 语义因 F-149 实际从未生效过 | 同一 event_id 重投/回放不产生重复行（真实 PG 实测，非只单测） |
| 3 | **重试语义实测** | 代码在（relay: attempts++ / consumer: attempts map）但从未在真实栈上逐点验证过 | 失败重投日志、attempts 递增、上限后转 DLQ/dead 三段各有运行时证据 |
| 4 | **死信有处置机制** | 24 条死信堆积、无回放工具（F-b/F-c/F-f） | 回放工具落地；**历史 24 条死信**逐条诊断 → 回放或归档，Grafana 面板归零或留明确账 |
| 5 | **运营参数热更在链路上生效** | Ops 容器/atomic 重试容器已建（E2E-23 E 组），但从未在消息链路上验证 | Nacos 推 `max_attempts` / `max_retries` → relay / consumer 下一轮即生效（日志级证据） |

---

## 2. 范围与边界

### 做

- **修复 E2E-F-149**：`event_repository.go` `Create` 的 conflict target 改为 `("event_id","occurred_at")`（或等价方案，见 §8 决策点 1），TDD：先在真实 PG（分区表 schema）写失败测试，再改实现；同步评估是否需要新 migration（a010）补索引——**倾向不需要**（a008 已建 `(event_id, occurred_at)` UNIQUE）。
- **全链 + 重试 + 死信 + 回放测试点**（§4，20 项）。
- **回放能力**（F-12/F-150）：DLQ topic 回放脚本 + outbox dead 行重置回放（形态见 §8 决策点 2），含回放幂等验证。
- **历史死信处置**：24 条存量死信逐条诊断（错误原因分布）→ 可回放的回放、不可回放的归档并留账。
- **账本动作**：F-149 owner 由 E2E-15/E2E-19 **转挂 E2E-24**（本阶段修复并闭环）；F-12/F-150 本阶段闭环。

### 不做（边界）

- **D3**（consumer attempts 跨 rebalance/重启持久化）：只如实记录风险（§6），不在本阶段实现。
- **D5**（relay 多副本分布式互斥）：dev 单副本跑不了多副本场景；记为扩容前置条件，不实现。
- **D6/D7/D8**（proto 双 schema 长期成本 / 删除会话级联语义 / 契约卫生小项）：P3 打包项，不属本阶段。
- **SkyWalking 全链追踪**：sw8 header 透传代码已在（Stage 92/93），端到端 trace 可视化归 **E2E-26**。
- **监控面板/告警规则新增**：E2E-22 已建 DLQ 面板与 outbox-dead 告警；本阶段只**消费**它们作证据，不改观测面。
- **性能基线**（吞吐/延迟压测）：归 **E2E-28**。
- **schema 变更**（除 F-149 最小修复外）：表结构/分区策略归 E2E-19 口径。

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | 无依赖前置（E2E-23 ✅ done） |
| dev 模式栈（Kafka/Nacos/Postgres/双消费组） | 开工时启动并登记 `.devmode-session`（AGENTS §八） |
| `.env.local` 红线 | 启动必须带 `--env-file .env.local`，严禁删除/覆盖该文件 |
| F-149 修复 | **本阶段第一个 TDD 循环**，完成前后续测试点不得开跑 |
| §2.4 契约 smoke | 契约 1/2/6 脚本就位（收口门槛） |

环境启动命令（**必须带 `--env-file .env.local` 与 `--profile dev`**）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev up -d
```

执行约束（继承 E2E-23 F-k 教训）：5 个业务服务健康端口不映射宿主，探针一律走 ① `docker exec` 容器网络内 ② BFF 聚合 `/health` ③ Kafka console 命令（`kafka-topics.sh` / `kafka-console-consumer.sh`，在 `emotion-echo-kafka` 容器内执行）。

---

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定。每个测试点须有**代码验证 + 运行时证据**双重支撑；`[V]` 必须截图并被查看。

### A 组：前置修复（TDD）

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | **F-149 修复**：真实 PG（分区表 schema）上 `Create` 重复 event_id 幂等不报 42P10 | [A] | RED：先写会失败的 repository 测试（连真 PG 或最小分区表 fixture）→ GREEN：改 conflict target；负向对照旧代码必红 | `event_create_partition_integration_test.go` RED 42P10 → GREEN；负向对照（还原单列即红）；PR #133 | PASS |
| 2 | F-149 修复后端到端：发 1 条消息 → `user_behavior_events` 出现新行（此前必失败） | [A] | BFF 发消息 → psql COUNT 前后对照 + consumer 日志无 `SQLSTATE 42P10` | v0.1.10 部署后发消息 → 0×42P10、events 163→165（plan §0.2 #7） | PASS |

### B 组：全链正向 + 幂等

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 3 | outbox 行生命周期：发消息 → `outbox_events` pending → relay interval 内 → sent | [A] | psql 查 status/sent_at + relay 日志 | probe 消息后 `SELECT id,status FROM outbox_events` → 877/879/880 均 sent（plan §0.2 #3） | PASS |
| 4 | §2.4 契约 1：`user_behavior_events` 行数增量 = 业务事件数（message.created 等） | [A] | psql COUNT 对照 | 发 1 消息 → +2 行（conv.created+msg.created，§0.2 #7）；回放 26 → +25 行逐条对应 | PASS |
| 5 | §2.4 契约 2：event_type 细分（≥2 种带点原值，不全 'conversation'） | [A] | psql GROUP BY | GROUP BY：message.created 83 / conversation.created 74 / conversation.closed 22（+历史旧值 8） | PASS |
| 6 | **消费幂等**：同一 event_id 手工重发布 → 行数不增 | [A] | kafka-console-producer 重发同 payload → COUNT 不变 | replay 重放同 5 条 → 192 不变；(event_id,occurred_at) 重复 0 行 | PASS |
| 7 | **双消费组**：analytics-svc 与 ai-svc 各自独立消费（互不依赖 offset） | [A] | 两消费组 lag/offset + 各自业务侧落库证据 | kafka-consumer-groups --all-groups：analytics-svc 与 ai-svc 均独立 offset、LAG=0 | PASS |
| 8 | KAFKA_ENABLED=false dev fallback（契约 6）：dev_publisher 直写路径事件落库不空跑 | [A] | 关 Kafka env 重启 chat-svc → 触发事件 → DB 行存在（标注 dev 路径） | compose override KAFKA_ENABLED=false → 日志 `using DevEventPublisher (dev-only path)` → 发消息事件直写落库（outbox sent）；**途中抓到 F-149 同型第二处**（dev_publisher 裸 SQL 单列 conflict target）已 TDD 修复（chat-svc:v0.1.16） | PASS |

### C 组：重试与死信

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 9 | **consumer 失败重试**：注入非法 payload 毒消息 → 原地重试日志 | [A] | kafka-console-producer 发坏 payload → consumer 日志 | F-174 修复后：同 offset=89 attempt=1/3(backoff=2s)→2/3(4s)→3/3(8s)，原地重试实测 | PASS |
| 10 | 超限进 **DLQ**：`chat-events-dlq` 收到消息且 headers 含 `x-original-topic`/`x-error-reason`/`x-attempts` | [A] | kafka-console-consumer 读 DLQ + headers | 死信 headers 实测读取：`x-original-topic:chat-events,x-error-reason:...42P10,x-attempts:4` | PASS |
| 11 | 毒消息被 Mark 后**不卡死**：后续正常消息继续消费 | [A] | 毒消息后发正常消息 → 正常落库 | 毒消息 DLQ 后再发消息正常落库（167→...）、LAG=0；旧缺陷（单条毒消息阻塞分区）已随 F-174 修复消除 | PASS |
| 12 | **outbox relay 失败重试**：停 Kafka → outbox pending 积压 + attempts++ → 恢复 Kafka → 全部 sent | [A] | 停/起容器 + psql status 对照 + relay 日志 | 行 889 停机期间 attempts 1→16（含 `circuit breaker is open`）→ Kafka 恢复后下一轮 **sent**、事件落库 | PASS |
| 13 | outbox **dead 状态机**：MaxAttempts 调小（Nacos 热更或配置）→ 超限行 status=dead、ListPending 不再扫、`IncDead` 计数 | [A] | psql status + chat-svc /metrics + 日志 | 热更 max=2 → 日志 `row marked dead attempts=5 max=2 (will NOT retry)`、行 status=dead、`outbox_events_dead_total` 0→2；恢复期 dead 行不被重扫 | PASS |

### D 组：回放（F-12/F-150 闭环）

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 14 | **回放工具形态拍板**（§8 决策点 2：DLQ 回放脚本 vs ops 命令 vs admin 端点；outbox dead 重置方式） | [M] | 用户裁定，本阶段按裁定实现 | 用户 2026-10-01 裁定 (a) ops 脚本形态 → **D-33** 落地 `scripts/replay_dlq` | PASS |
| 15 | **DLQ 回放**：工具从 `chat-events-dlq` 读死信 → 重发布原 topic → 正常消费落库 | [A] | 工具运行输出 + DB 行 + DLQ offset 变化 | 真实回放 26/26 发布成功 → 25 条落库（events 167→192） | PASS |
| 16 | 回放**幂等**：回放已消费过的消息 → ON CONFLICT DO NOTHING → 行数不增 | [A] | 重复回放同一批 → COUNT 不变 | 重放同 5 条 → 192 不变；(event_id,occurred_at) 重复 0 行 | PASS |
| 17 | **outbox dead 行回放**：dead 行经重置后重新被 relay 扫描并成功发出（status dead→pending→sent） | [A] | 构造 dead 行 → 重置工具 → psql 状态迁移 | `scripts/replay_outbox_dead.sh --all`（守卫 4/4 GREEN）：dead 887/888 → pending → relay 重发 **sent**，丢失的 conversation.created 事件补落库（events 193→195） | PASS |
| 18 | **存量 24 条死信处置**：逐条诊断错误分布 → 可回放回放 / 不可回放归档留账 → 面板归零或死信数与账面一致 | [A/M] | 工具 + psql + Grafana 面板对照；处置策略按 §8 决策点 3 | 26 条全量回放：25 落库 / 1 毒探针再进 DLQ 留证；账实一致（DLQ offset 27 = 累计历史，未处置余额 1） | PASS |

### E 组：运营参数与观测

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 19 | **热更生效**：Nacos 推 ops 配置 → relay/consumer 运行时生效 | [A] | 推配置 → 下轮 flush/下条毒消息行为变化（日志级证据） | chat-svc 半：`ops applied via hot-reload: max_attempts=2`（**注：dataId 必须用全名 `emotion-echo-chat-svc.ops.yaml`**，短名发不中）+ max=2 生效 + 回推 100 生效；analytics 半：`ops applied via hot-reload: max_retries=5` → 毒消息 attempt=1/5→5/5（退避 2/4/8/16/30s）→ 6 次进 DLQ（已回推 3） | PASS |
| 20 | Grafana **DLQ 面板实数据**：面板值 = kafka console 实测死信数（含处置后归零） | [V] | 面板截图 + console 数值对照（双证据） | `screenshots/20-grafana-dlq-panel-27.png`：DLQ Depth 面板显示 **27** = kafka-get-offsets 实测 27（曲线含本轮死信事件形状） | PASS |

---

## 5. 验收标准（DoD）

- [ ] 全部测试点通过（或已分类：范围内修复 / 范围外记账本）
- [ ] F-149 修复走完 TDD（RED → GREEN → Refactor），负向对照旧代码必红
- [ ] 回放工具落地且走 TDD；**存量 24 条死信有逐条交代**（回放/归档清单）
- [ ] §2.4 契约 1/2/6 smoke 全绿（真实 dev 栈）
- [ ] 账本 F-12 / F-149 / F-150 闭环；roadmap 状态同步
- [ ] 回归钉：F-149 幂等契约 + 回放幂等固化为可重跑断言（spec 或脚本测试）
- [ ] §2.5 收口自检三连通过（push / 删分支 / `git status` 干净）

---

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| **event_id 唯一性语义变化**：分区表下唯一性 = `(event_id, occurred_at)`，单独 event_id 不再全局唯一 | 评估写入路径：同一 event_id 总伴随同一 occurred_at（同一条事件）⇒ 语义等价；在修复 PR 中写明论证，并用测试点 #6/#16 锁死 |
| **24 条存量死信可能是旧格式**（proto 双写窗口前的 JSON、或 F-149 之前的其他错误）→ 回放可能再次失败 | 处置策略必须允许"诊断 → 归档"出口，不强行回放；逐条记录 LastError 分布后再决定 |
| **D3：attempts 不跨 rebalance/重启** → 毒消息在重启后重置计数，理论可无限循环 | 本阶段如实记账（范围外），风险写入账本条目；DLQ + 告警是现有兜底 |
| **双写窗口**：payload JSONB ↔ Protobuf 双 schema（D6）→ 老消息 decode 失败归入死信 | 回放工具须透传原始 bytes（`DLQEntry.Value`），不做 schema 转换 |
| **dev 栈启动竞态**（F-96 代码半未修：svc 单次连库失败即永久降级） | 启动后先核对 6 服务 Nacos 注册 + BFF 聚合 /health，再开跑测试点；发现降级先 `docker restart` 并记录 |
| **`.devmode-session` 双轨占用** | 开工前检查锁登记；被占用则协商，不强占 |

---

## 7. 产出物

- 修复：F-149（TDD：repository 测试 + 必要时 migration a010）
- 工具：DLQ 回放 + outbox dead 重置（形态按 §8 决策点 2 裁定）
- 回归钉：幂等契约测试（消费幂等 / 回放幂等）
- 执行记录：`stages/e2e-24-message-pipeline/report.md`
- 截图：`stages/e2e-24-message-pipeline/screenshots/`
- 账本更新：F-12 / F-149 / F-150 闭环记录（含 24 条死信处置清单）

---

## 8. 决策点（执行期内裁定，非开工前阻塞）

| # | 决策 | 选项 | 建议 |
|---|------|------|------|
| 1 | **F-149 修法** | (a) conflict target 改 `("event_id","occurred_at")`；(b) 先查后插（无 ON CONFLICT） | **(a)**：最小改动、保留数据库层幂等原子性；(b) 引入竞态窗口 |
| 2 | **回放工具形态**（测试点 #14，[M]） | (a) ops 脚本（kafka console consumer/producer 编排，入 `scripts/`）；(b) chat-svc/analytics-svc 内 ops 命令/端点；(c) 独立 Go 小工具 | **(a)**：与现有 `scripts/` 守卫测试范式一致、无需改服务代码、可 TDD（脚本守卫测试）；缺点是依赖 kafka 容器内 console 命令。outbox dead 重置同理优先脚本化（psql UPDATE status='pending'），若 (b) 则复用 E2E-23 的 Ops 通道 |
| 3 | **存量死信处置策略** | (a) 全量回放；(b) 逐条诊断后回放可回放者、归档其余；(c) 全部归档（只清面板不追数据） | **(b)**：先诊断 LastError 分布——若主因是 F-149 则修复后回放即愈；旧格式消息归档留账 |
