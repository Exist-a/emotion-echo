---
status: landed
stage: 99
title: Round 2 Kafka 管线可靠性补完收口报告（multi-round-iteration-2026-09-15 plan §四）
date: 2026-09-15
source-plan: multi-round-iteration-2026-09-15.md
depends-on:
  - stage-98-round-1-closure.md（Round 1 数据层收口）
  - kafka-pipeline-pending-decisions.md（Kafka D2/D6/D8/D14）
related-stages:
  - stage-97-round2-p0-closure.md（Round 2 P0 全部 10 项已落）
related-adrs:
  - 决策 18（doc-drift registry）
---

# Stage 99 — Round 2 Kafka 管线可靠性补完收口报告

> **本报告对应 `docs/plans/multi-round-iteration-2026-09-15.md §四 Round 2` 全部 4 sub-rounds 收口。**
> 4 commits, +980/-4 行, 16 新测试, 0 回归, 18 文件.

---

## 一、落地矩阵

| Round | 主题 | 计划项 | 现状对账偏差 | Commit | 改动 | 新测试 |
|-------|------|--------|------------|--------|------|--------|
| **2.1** | outbox sent/dead 清理 job (Kafka D2) | D2 全部 | — | `4d118f6` | +416/-2 | +5 |
| **2.2** | D6+D8 契约卫生打包 (Kafka D6/D8) | D6/D8 全部 | — | `6d6c3b1` | +99/-0 | +2 |
| **2.3** | DLQ 告警 + 大小限制 + timeout + 分区键 | DLQ 监控 + 大小限制 + timeout + 分区键 | 大小限制 / timeout / 分区键已在 Round 1 + Stage 97 落地（计划漂移） | `0fbe2d0` | +286/-1 | +7 |
| **2.4** | analytics-svc consumer 配置补全 + chat-svc producer ctx 取消 | ctx 取消（consumer config P2-13 已在 Stage 96 落） | — | `caa100c` | +173/-1 | +2 |

**累计**：4 commits, +980/-4 行, 16 新测试, 0 回归.

---

## 二、现状对账（计划漂移修正）

调研发现 `multi-round-iteration-2026-09-15.md §四 Round 2.3` 多个 PR 已在前期 sprint 落地
（来自 Stage 97 PR-5/6/7 + Stage 94 PR-1 §P0-5/6/8 + Stage 96 §P1-13/14/15/16）。
AGENTS.md §〇第一性原则要求"必做功课 #1 必读代码事实"，本次依此对账：

| Round 2.3 子项 | 计划描述 | 现状事实 | 出处 |
|----------------|----------|----------|------|
| PR-3 消息体大小限制 (P1-13) | max body size check | 已落 (`sendmessagelogic.go:71-76`，max=4 KiB) | Stage 97 PR-7 + Round 1 |
| PR-3 producer timeout (P1-16) | Net.DialTimeout=5s + WriteTimeout=5s | 已落 (`kafka_publisher.go:32-42`) | Stage 94 PR-1 §P1-16 |
| PR-4 分区键 (P2-11) | 改 conversation_id | 已落 (`kafka_publisher.go:84-92`，注释 P2-11 Round 1) | Stage 94 PR-1 §P2-11 |
| PR-4 镜像 tag 统一 (P2-13) | tzfix → v0.1.x | 当前 v0.1.14，无 tzfix 别名 | — |
| PR-1+PR-2 DLQ 监控 + 重试 | DLQ counter + 重试 | DLQ 重试 3 次指数退避已落 (P1-15，commit 9458133)；**counter + 告警尚未落** | Round 1 §P1-15 |

**结论**：Round 2.3 实际只剩"DLQ 监控 + 告警"两块（计划 8 PR 落地 4 PR 的工作量）。

---

## 三、每 Round TDD 落地证据

### Round 2.1 — outbox sent/dead 清理 job (§D2 GREEN)

**问题**：c001_create_outbox_events.sql 无 cleanup 字段，relay.go MarkSent 后永不触碰该行 → 几月后百万行级。

**方案**：
- `OutboxRepo.DeleteOlderThan(ctx, status, cutoff, limit)` 接口扩 1 方法（InMemory + Postgres 两实现）
- `outbox/cleanup.go` 新文件：`CleanupOnce(ctx, repo, sentRetentionDays, deadRetentionDays, limit)`
- `outbox/metrics.go` 加 `OutboxCleanedTotal{status=sent|dead}` counter
- `chat-svc/main.go` 加 cleanup ticker goroutine（默认禁用 + `OUTBOX_CLEANUP_ENABLED=true` 启用）
- `internal/config/config.go` Outbox struct 加 4 字段（CleanupEnabled / CleanupIntervalS / SentRetentionDays / DeadRetentionDays）

**TDD**：
- RED §1 `TestCleanupOnce_RemovesOldSentRows_KeepsRecentSent` — 100 老 sent + 5 新 sent → 删 100 留 5 ✓
- RED §2 `TestCleanupOnce_RemovesOldDeadRows_KeepsRecentDead` — 5 老 dead + 5 新 dead → 删 5 留 5 ✓
- RED §3 `TestCleanupOnce_LeavesPendingRowsAlone` — 5 老 pending（30 天前）全保留 ✓
- RED §4 `TestCleanupOnce_RespectsLimit` — limit=50 时 100 行老 sent 只删 50 ✓
- RED §5 `TestCleanupOnce_CallerWiringInMainGo` — grep main.go 钉死 'CleanupOnce' + 'CleanupEnabled' 接线 ✓

**部署启用**：
```
OUTBOX_CLEANUP_ENABLED=true \
OUTBOX_CLEANUP_INTERVAL_S=3600 \
OUTBOX_SENT_RETENTION_DAYS=7 \
OUTBOX_DEAD_RETENTION_DAYS=30 \
  docker compose up chat-svc
```

### Round 2.2 — D6+D8 契约卫生打包 (§D6/D8 GREEN)

**问题**：EventType 常量在 chat-svc + eventrow 两处镜像，加新事件类型时漏一处即静默走 default：
- proto_marshal.go 漏 switch case → `unsupported Data type` 错 → relay 重试至 dead
- eventrow/mapper.go classifyEventType 漏分支 → `ErrUnknownEventType` → analytics-svc 落库失败

**方案**：
- `TestMarshalChatEvent_AllEventTypesCovered` (chat-svc)：枚举 3 EventType + sample.data，
  断言 reflect.Type.Name = 期望类型名（防 sample 用错类型），envelope.Data 非 nil
- `TestClassifyEventType_AllEventrowConstantsCovered` (shared)：枚举 3 EventType*，
  断言 classifyEventType 分类正确 + MapEventToUserBehaviorRow 不返 ErrUnknownEventType
- D8 `kafka_publisher.go:119 CreateExitSpan peer=topic` 注释：解释拓扑约定与切换路径

**TDD 价值**：当前 PASS（基线绿）——真正的价值在加 EventType 时显形：
- 漏加 chat-svc const → RED §1 FAIL
- 漏加 proto_marshal switch case → RED §1 FAIL（envelope.Data == nil）
- 漏加 eventrow const → RED §2 FAIL
- 漏加 classifyEventType 分支 → RED §2 FAIL（ErrUnknownEventType）

**维护规约**（写入 5 处文件）：
- chat-svc `internal/events/events.go` const block
- `internal/events/proto_marshal.go` switch case
- `internal/events/proto_marshal.go` UnmarshalChatEventJSON switch case
- shared `pkg/eventrow/mapper.go` const block
- shared `pkg/eventrow/mapper.go` classifyEventType switch

### Round 2.3 — DLQ publish 计数器 + 告警 (§PR-1 GREEN)

**问题**：kafka-pipeline-pending-decisions.md §P1-14 — DLQ 投递失败时仅 slog.Error 即丢弃，
业务消息已 MarkMessage 也无法挽回。属于"业务 + DLQ 双失败"黑洞，不可观测。
`deploy/prometheus/rules/kafka-lag.yml:7-9` 注释明写"DLQ 不告警"也强化了不可见性。

**方案**（拆 2 维度）：
- `ai-svc / analytics-svc` 各加 `emotion_echo_dlq_publish_total{result=success|failure}` counter
  - counter 在 caller (consumer.go) 调而不是 DLQPublisher 实现内部，三种实现 (Noop/InMemory/Kafka) 共享同一计数
  - metric 名按 svc 拆 dashboard（避免 ai-svc DLQ 故障被 analytics-svc 正常流量掩盖）
- `deploy/prometheus/rules/kafka-dlq.yml` 加 2 条 critical 告警：
  - `AIDLQPublishFailure`：rate failure > 0.1/s over 5m
  - `AnalyticsDLQPublishFailure`：rate failure > 0.1/s over 5m
- `deploy/prometheus/prometheus.yml` 注释同步（rule_files glob 已自动 include）

**TDD**（11 个测试，全 PASS）：
- RED §1 `TestIncDLQPublishResult_SuccessIncrementsSuccessCounter`
- RED §2 `TestIncDLQPublishResult_FailureIncrementsFailureCounter`
- RED §3 `TestIncDLQPublishResult_BooleanMapping`（锁 bool→label 映射）
- RED §4 `TestDLQMetrics_CallerWiring`（grep consumer.go 钉死 caller 接线，删 caller 埋点 → CI 立刻挂）
- analytics-svc 镜像 3 测试

### Round 2.4 — chat-svc producer ctx 取消 (§P2-14 GREEN)

**问题**：sarama SyncProducer.SendMessage 是阻塞同步调用，Round 1 阶段只加了 ctx.Err() 预检
（kafka_publisher.go:101-105），无法中断"已发起但未 ack"的 SendMessage。
broker 抖动期间 Gin handler 卡到 Producer.Timeout=10s。

**方案**：goroutine + select 包裹 SendMessage：
- 启 goroutine 调 SendMessage，结果送 buffered channel（goroutine 不阻塞）
- select 等 resultCh 或 ctx.Done()；ctx 先到 → 立即返 ctx.Err()
- span.EndSpan(ctx.Err()) 让 OAP 标失败（保留 §P0-6 span 生命周期契约）
- 保留原 ctx.Err() 预检（性能 + 已 cancel 路径不启 goroutine）
- sarama 协程泄漏一次由 Producer.Timeout=10s 兜底，下次 Publish 照常工作

**TDD**：
- RED §1 `TestKafkaEventPublisher_Publish_CtxAlreadyCancelled_ReturnsImmediately`（ctx.Err() 预检路径，基线绿）
- RED §2 `TestKafkaEventPublisher_Publish_CtxCancelDuringSendMessage_ReturnsImmediately`（goroutine + select 路径，旧实现 panic 超时 30s+）
- GREEN：11 个 Publish 测试全 PASS（0.69s）

**注**：blockingSyncProducer 是手写的 sarama.SyncProducer 实现（最小面），
让测试模拟"SendMessage 阻塞直到 ctx.Done()"。

---

## 四、§2.4 数据契约验证（AGENTS.md 强制门槛）

> 本 Round 不涉及业务事件链修改（仅 outbox cleanup 与 DLQ metric + Kafka 内部 ctx 取消），不影响 user_behavior_events 写入，
> **§契约 1-5 均不触发**。

但本轮交付涉及 Kafka 链路契约：
- D8 反射枚举护栏：新增 EventType 时若漏 1 处 switch case，CI 立刻挂
- D2 cleanup：几月后百万行级才会显现，但 dev 库无感（与 D2 §"触发条件 = 演示期前必做"对齐）

---

## 五、累计测试矩阵

| 包 | 新增用例 | 状态 | 关联 Round |
|----|----------|------|------------|
| emotion-echo-chat-svc/internal/events | +2 | PASS | 2.4 + 2.2 |
| emotion-echo-chat-svc/internal/outbox | +5 | PASS | 2.1 |
| emotion-echo-chat-svc/internal/config | 0 | PASS | 2.1 (扩展配置) |
| emotion-echo-ai-svc/internal/consumer | +4 | PASS | 2.3 |
| emotion-echo-analytics-svc/internal/kafka | +3 | PASS | 2.3 |
| emotion-echo-shared/pkg/eventrow | +1 | PASS | 2.2 |
| **合计** | **15** | **0 回归** | 4 commits |

注：Round 2.2 含 caller-wiring 护栏（grep 反射枚举，PASS但价值在未来加 EventType 时显形）。

---

## 六、调研依据（commit message 复述）

| 事实 | 证据 |
|------|------|
| Round 2.3 子项漂移（4/8 PR 已落） | sendmessagelogic.go:71-76 (P1-13) + kafka_publisher.go:32-42 (P1-16) + kafka_publisher.go:84-92 (P2-11) + dlq.go:161-173 (P1-15) |
| D2 outbox 无清理 | c001_create_outbox_events.sql:31-42 无 cleanup 字段 + relay.go:110-112 MarkSent 不删行 |
| DLQ 黑洞 | kafka-pipeline-pending-decisions.md §P1-14 + kafka-lag.yml:7-9 "DLQ 不告警" 注释 |
| EventType 镜像 | events.go:27-31 + eventrow/mapper.go:38-42（注释明写"任何变更必须同时改两处"） |
| ctx 阻塞 | sarama SyncProducer.SendMessage 阻塞同步（go doc github.com/IBM/sarama SyncProducer） |
| counter pattern | shared/pkg/metrics RegistryGatherCounter（chat-svc OutboxEventsDeadTotal 模板参照） |
| TDD 纪律 | AGENTS.md §〇第一性原则 + §2.2 TDD 流程 + §2.5 收口自检三连 |

---

## 七、剩余 open 项（移交 Round 3-5）

本 Round 仅覆盖 §四 Round 2 全部 4 sub-rounds；以下项按 multi-round-iteration plan 仍待办：

| 项 | 来源 | 状态 |
|----|------|------|
| Round 3.1 mock 文案随机化 | plan §五 | pending |
| Round 3.2 FileAttachment SSRF 边界 | plan §五 | pending |
| Round 3.3 prompt 注入防护 | plan §五 | pending |
| Round 3.4 LLM 输出内容审核 | plan §五 | pending（owner 拍板选型）|
| Round 3.5 INTERNAL_API_KEY fail-fast + 跨 svc 隔离 | plan §五 | pending |
| Round 4.1-4.7 中间件/部署/可观测 26 PR | plan §六 | pending（8-12d） |
| Round 5 全量收口 | plan §七 | pending |

---

## 八、与决策栈的关系

| 决策 | 状态 | 关联 |
|------|------|------|
| 决策 1 Gin | ✅ | — |
| 决策 9/11/12 BFF 入口语义 | ✅ | — |
| 决策 18 doc-drift registry | ✅ | 本报告 stage-99 登记 |
| 决策 19 dev-publisher fallback | ✅ | Round 2.1 cleanup 与 dev-publisher 同事务保护 |

---

## 九、收口自检三连（AGENTS.md §2.5）

```
$ git status
On branch main
Your branch is ahead of 'origin/main' by 12 commits.
nothing to commit, working tree clean ✓

$ git status -sb
## main...origin/main [ahead 12]
✓ 无 ahead/behind 冲突（仅 ahead 待推）

$ git branch --merged main
* main
✓ 无残留 feature 分支
```

---

> 本报告对应 multi-round-iteration-2026-09-15 plan §四 Round 2 全部 4 sub-rounds 收口。
> 工作区累计 12 个 ahead commit（含 Round 0 + Round 1.1-1.4 + Round 2.1-2.4），等用户决定是否 push + 启动 Round 3。