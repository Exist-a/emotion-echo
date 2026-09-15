---
status: planned
priority: medium
owner: TBD
created: 2026-09-14
last-refresh: 2026-09-15（Round 2.1-2.4 收口 — D2/D8 已部分落，详见 §状态盘点）
type: pending-decision
source: external code review（2026-09-14 会话第二轮：Kafka 定位/健壮性/削峰解耦讲解前的装配链路核查；与 observability-edge-gaps-from-code-review.md 第一轮不重叠）
depends-on: []
related-plans:
  - observability-edge-gaps-from-code-review.md（第一轮 6 项，§A 已 landed Stage 92/93）
  - kafka-reliability-gaps.md（Sprint A 已落地，残余 §1.4 / §1.6）
  - multi-round-iteration-2026-09-15.md（Round 2.1-2.4 落地汇总）
related-stages:
  - stage-43-kafka-reliability-sprint-a.md
  - stage-73-kafka-protobuf-migration-2026-09-12.md
  - stage-86-outbox-dead-alert-2026-09-13.md
  - stage-92-kafka-sw8-propagation-2026-09-14.md
  - stage-93-analytics-svc-sw8-propagation.md（planned）
  - stage-99-round-2-closure.md（Round 2.1-2.4 收口：D2/D8 部分落地）
related-adrs:
  - adr-2026-09-dev-publisher-user-behavior-events.md（ADR-19）
---

# Plan — Kafka 管线待决策清单（第二轮代码审查发现）

## 〇、来源说明

本清单来自 **2026-09-14 会话第二轮**代码审查。第一轮（observability-edge-gaps-from-code-review.md）聚焦可观测链路；本轮应用户要求讲解 Kafka 的定位/健壮性/削峰/解耦，为保讲解基于代码事实，把 **生产→outbox→relay→Kafka→两个 consumer→落库** 全链路的装配代码重新核了一遍，发现 8 个需要 owner 拍板的点。

本轮已读文件：
- `emotion-echo-chat-svc/{main.go, internal/config/config.go, internal/events/{events.go, kafka_publisher.go, dev_publisher.go, proto_marshal.go}, internal/outbox/{relay.go, metrics.go}, internal/repository/outbox.go, internal/logic/{create,send,delete,update,pin}conversationlogic.go, internal/repository/conversation_repository.go, migrations/001_create_outbox_events.sql}`
- `emotion-echo-ai-svc/{main.go, internal/consumer/consumer.go, internal/repository/emotion_repository.go}`
- `emotion-echo-analytics-svc/{main.go, internal/kafka/consumer.go, internal/repository/event_repository.go}`
- `emotion-echo-shared/pkg/{eventrow/mapper.go, messaging/kafka_producer.go}`
- `docs/stages/stage-86-outbox-dead-alert-2026-09-13.md`、`docs/stages/stage-92-kafka-sw8-propagation-2026-09-14.md`

与第一轮 6 项（§A~F）**无重叠**；§A sw8 透传已由 Stage 92/93 落地。

---

## D1. Kafka producer init 失败时 fallback InMemoryEventPublisher，击穿 outbox 可靠性承诺（P1，建议优先拍板）

### 现象（代码事实）

`emotion-echo-chat-svc/main.go` §2.5：

```go
var pub events.EventPublisher = events.NewInMemoryEventPublisher()
if kafkaEnabled {
    kp, err := events.NewKafkaEventPublisher(kafkaBrokersList)
    if err != nil {
        log.Printf("[kafka] producer init failed: %v (fallback to in-memory)", err)
    } else {
        ...
        pub = kp
    }
}
```

随后 §4.1 relay 启动条件是 `db != nil && outboxRepo != nil && pub != nil`——**pub 永远非 nil**（最差也是 InMemory），relay 照常启动。

`InMemoryEventPublisher.Publish` 把事件 append 进内存 slice 就返回 nil（`events/events.go`），relay 收到 nil → `MarkSent`。

### 为什么这是问题

1. 生产模式下 broker 短暂不可达（重启 Kafka、网络抖动、DNS 未就绪）→ producer init 失败 → 全部事件进入内存 slice → **relay 把 outbox 行标记 sent，但事件从未进入 Kafka**。
2. outbox 的核心承诺是「业务落库 ↔ 事件发布原子 + 发送失败保留重试」，被这个 fallback 静默击穿：行已 sent，不再重试，事件永久丢失。
3. 表面有 log 一行，但 sent 行数与 Kafka 实际消息数不会对不上账——没有任何指标能发现。
4. dev 模式（KAFKA_ENABLED=false）走 DevEventPublisher 不受影响；**问题只出现在「开关开了但 init 失败」的窗口**，正是最隐蔽的场景。

### 待决策选项

| 选项 | 内容 | 代价 |
|---|---|---|
| A | producer init 失败且 `STARTUP_STRICT` 时 fail-fast（拒绝启动） | 需要把 kafka 加入 required deps 判定；broker 未就绪会阻塞启动 |
| B | init 失败时**不启动 relay**（只有真 Kafka/Dev publisher 才启动），outbox 行保持 pending，Kafka 恢复后（需重启进程或加重连）继续发 | 行为变化最小；代价是事件延迟到进程重启 |
| C | 维持现状，但加指标：`pub` 非 Kafka 类型时 relay MarkSent 前递增 `outbox_sent_via_fallback_total` counter + warning 告警 | 保留可用性，把黑洞变成可见 |

### 建议

选 C（短期）+ B（长期）。C 是 30 分钟工作量；B 需要明确「relay 只为真 publisher 服务」的语义。**当前状态建议在 ADR-19 补一段登记**，避免后续读代码的人误判这是可靠路径。

### 已落地登记（2026-09-15 Stage 94 PR-3 · commit `44e9767`）

选项 C（短期 metric）已落地：

- `emotion-echo-chat-svc/internal/outbox/metrics.go` 定义 `OutboxSentViaFallbackTotal` counter：
  - Name: `emotion_echo_outbox_sent_via_fallback_total`
  - Inc 函数 `IncSentViaFallback()`，由 chat-svc main.go §2.5 Kafka init 失败 fallback 时调用一次
- `emotion-echo-chat-svc/main.go:159-164`：
  ```go
  if err != nil {
      log.Printf("[kafka] producer init failed: %v (fallback to in-memory)", err)
      outbox.IncSentViaFallback()
      // prometheus alert: emotion_echo_outbox_sent_via_fallback_total > 0 持续 1m → page
  }
  ```
- `emotion-echo-chat-svc/internal/outbox/metrics_test.go` 验证 counter 存在 + IncSentViaFallback() 能递增

**黑洞变成可见**：发送行数与 Kafka 实际消息数对账可监控；剩余事件级黑洞（fallback 后 InMemory slice 内的数据不可逐条观测）需要后续选项 B 解决（relay 只为真 publisher 服务 + Kafka 重连机制）。

选项 B（长期）待 kafka 扩副本 / 多副本迁移启动时再做。

### ADR-19 补登记

[adr-2026-09-dev-publisher-user-behavior-events.md](../architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)
末尾已加 "## D1 fallback 路径登记" 段（Stage 97 tail 闭环动作）。

### 工作量

- 选项 C：0.5h ✅ 已落地（Stage 94 PR-3 commit `44e9767`）
- 选项 B：1h（main.go 启动条件收紧 + 注释）— 待 kafka 扩副本触发时做

---

## D2. outbox_events 的 sent / dead 行无清理归档机制，表无限增长（P1）

### 现象（代码事实）

- `migrations/001_create_outbox_events.sql`：每条 message.created / conversation.created / conversation.closed 各一行；只有 `idx_outbox_pending`（partial, status='pending'）和 `idx_outbox_attempts` 两个索引。
- `outbox/relay.go` 只处理 pending；MarkSent 之后没有任何代码路径再触碰 sent 行；dead 行明确「保留供人工排查/回放」。
- 全仓 grep 无 vacuum/cleanup/retention/归档 相关逻辑（stage-86 文档亦无此章节）。

聊天是高频事件（每条消息一行），**sent 行随运行时间线性增长**。单用户 dev 无感；只要这个项目真的跑起来给人用（简历项目也有演示期），表会在数月内膨胀到百万行级，payload JSONB 让单行更大。

### 待决策选项

| 选项 | 内容 | 备注 |
|---|---|---|
| A | relay FlushOnce 里顺带清理：`DELETE WHERE status='sent' AND sent_at < now()-interval 'N days' LIMIT M`（每 tick 限量） | 简单；但把清理和发送耦合在一个 goroutine |
| B | 独立清理 goroutine / cron job，N 天 sent + dead 保留期配置化（OUTBOX_RETENTION_DAYS） | 干净；dead 行可配更长保留期（排障价值） |
| C | 按月分区表（pg_partman 或原生 partition） | 最彻底但迁移成本高，当前规模不匹配 |

### 建议

选 B。dead 行保留期 ≥ sent 行（排障/回放价值），并给 dead 清理加 log。在 `docs/plans/kafka-reliability-gaps.md` 残余段登记。

### 工作量

1-1.5h（含配置字段 + 单测）

---

## D3. consumer 的 attempts 重试计数不跨 rebalance / 进程重启（P2）

### 现象（代码事实）

ai-svc `consumer.go`：`attempts map[string]int` 注释明确「消费周期内有效」。analytics-svc `chatEventHandler.attempts` 同款。

sarama rebalance → 新 session → 同一个 handler 实例可能被重新调用，但**进程重启后 attempts 必然清零**。同一条毒消息：
- 进程稳定时：3 次失败 → DLQ，符合预期；
- 每次失败后进程恰好重启（OOM / 发布）：计数清零 → **再 3 次** → 永远到不了 DLQ，且每次都阻塞该 partition 后续消息消费。

### 待决策选项

| 选项 | 内容 |
|---|---|
| A | 接受现状 + 文档登记（单副本 dev 风险窗口小；DLQ 的主要敌人是稳定的毒消息而非「每次都重启」的巧合） |
| B | attempts 持久化：消费失败计数写 DB（如 `emotion_echo_ai.consume_attempts(event_id, attempts)`，或复用 outbox 表加 consume_attempts 列） |

### 建议

先 A（登记语义）——B 的复杂度（新表 + 清理 + 每消息一次 DB 写）与当前风险不成比例。但 B 可以作为「未来引入多 worker 消费」的前置项一并评估（与第一轮 §B 加锁是同一批前置）。

### 工作量

- A：15 分钟（文档）
- B：3-4h（表 + repo + 测试）

---

## D4. analytics-svc consumer 的 maxRetries 硬编码 3，无配置通道（P2，配置不对称）

### 现象（代码事实）

| 项 | ai-svc | analytics-svc |
|---|---|---|
| MaxRetries 来源 | `c.Kafka.MaxRetries`（yaml/env）+ 默认 3（main.go:318-321） | `NewConsumer` 硬编码 3（kafka/consumer.go:64），builder 有 `WithMaxRetries` 但 **main.go 从不调用** |
| DLQ 注入 | env `KAFKA_DLQ_TOPIC`（main.go:267-276） | 同（ADR-19 PR-A3.2）✅ |
| tracer 注入 | ✅（Stage 92 PR-2） | ✅（Stage 93 PR-2） |

analytics-svc 的 config 结构里没有 `Kafka.MaxRetries` 字段。运维调重试阈值只能改代码重编译；两服务的重试语义漂移没有机制约束。

### 待决策选项

| 选项 | 内容 |
|---|---|
| A | config 加 `Kafka.MaxRetries`（默认 3）+ main.go wire `WithMaxRetries` + applyEnvOverrides 读 `KAFKA_MAX_RETRIES`（对齐 ai-svc） |
| B | 维持硬编码，仅注释说明 |

### 建议

选 A。这是 Stage 93 顺手可做的 0.5h 工作（该 plan 尚未实施，可并入其 PR-2 wire 步骤），也符合项目「配置分层」决策 20 的方向。

### 工作量

0.5h

### 已落地登记（2026-09-15 Stage 96 PR-9a · commit `2cc05c8`）

选项 A 已落地：

- `emotion-echo-analytics-svc/internal/config/config.go:25-29` 加 `Kafka.MaxRetries int` 字段：
  ```go
  // MaxRetries P2-13: 消费失败最大重试次数，与 ai-svc 对齐（默认 3）。
  MaxRetries int
  ```
- `emotion-echo-analytics-svc/main.go:58-61,198` applyEnvOverrides 读 `KAFKA_MAX_RETRIES` env：
  ```go
  // P2-13 (Round 1): 从 env 读 MaxRetries 对齐 ai-svc（默认 3）
  // ...
  c.Kafka.MaxRetries = n
  ```
- `emotion-echo-analytics-svc/main.go` 注入 consumer：`c.consumer = &chatEventHandler{..., maxRetries: c.Kafka.MaxRetries}`
- ai-svc/analytics-svc 配置对称：两者都走 yaml `Kafka.MaxRetries` + env `KAFKA_MAX_RETRIES` + 默认 3

**测试**：[stage-96-code-review-round1-p1p2-closure.md §2 P2-13 已记录](../stages/stage-96-code-review-round1-p1p2-closure.md)，
analytics-svc `go test ./...` 全绿。运维可改 `KAFKA_MAX_RETRIES` 调重试阈值，无需重编译。

---

## D5. relay 多副本扩容前必须加分布式互斥（P2，前置条件型决策）

### 现象（代码事实）

`outbox/relay.go` `FlushOnce`：`ListPending`（普通 SELECT，无 `FOR UPDATE SKIP LOCKED`）→ 逐行 Publish → MarkSent。

chat-svc 单副本时无问题；**若未来 chat-svc 扩到多副本**（Stage 37-B「BFF 多副本」路线图隐含 svc 层也可能跟进），多个 relay 同时扫同一批 pending 行 → 同一事件被发送 N 次。消费端 event_id 幂等能兜住「不产生重复数据」，但：
- Kafka 里出现 N 倍重复消息（带宽/存储浪费）；
- `chat-events` 的 hash partitioner 靠 event_id 去重的初衷被稀释。

### 待决策选项

| 选项 | 内容 |
|---|---|
| A | SQL 层解决：`ListPending` 改 `SELECT ... FOR UPDATE SKIP LOCKED`（需把 MarkSent 改为同事务 UPDATE，行锁天然互斥） | 
| B | 进程租约：chat-svc 启动时向 Postgres/Nacos 注册 relay 租约，单活跃 relay |
| C | ADR 明确承诺「chat-svc 保持单副本」，多副本列入不支持矩阵 |

### 建议

短期 C（写进 ADR-19 或环境策略 ADR 的一段话），中期 A（SKIP LOCKED 是 Postgres 事实标准，改动集中在 repo 一层）。**触发条件**：chat-svc 决定扩副本的那个 stage 必须先做 A，建议现在把这条依赖登记进 roadmap。

### 工作量

- C：15 分钟（文档）
- A：2-3h（SQL + 事务边界调整 + 测试）

---

## D6. outbox payload JSONB ↔ Protobuf 双 schema 转换链的长期维护成本（P3）

### 现象（代码事实）

- 写入侧：logic 层 `json.Marshal(evt)` 存 outbox JSONB；
- 发送侧：relay `UnmarshalChatEventJSON`（按 Type switch 反序列化）→ `MarshalChatEvent`（switch 转 Protobuf envelope）→ Kafka。
- `events/proto_marshal.go` 两个 switch 各 3 个 case；**新增一个事件类型需要同步改：events.go 常量+Data 结构、proto 定义、Marshal switch、Unmarshal switch、eventrow mapper 分类**——漏掉 proto switch 的后果已被实证：Stage 73 e2e 中 map 反序列化 → outbox 行重试 100 次 → dead（relay.go publishOne 注释记录）。

### 待决策选项

| 选项 | 内容 |
|---|---|
| A | outbox payload 直接存 Protobuf bytes（新增 `payload_proto BYTEB`列或新表），JSONB 退役，relay 不再转换 | 
| B | 维持双 schema，加 CI 契约测试：反射枚举 EventType 常量，断言 Marshal/Unmarshal switch 全覆盖（漏 case 直接红） |

### 建议

选 B。A 是正确终态但涉及迁移脚本与双读窗口，收益在「事件类型频繁新增」时才兑现；当前 3 个事件类型，B 用 40 行测试锁住主要风险即可。与 eventrow 的 EventType 双处镜像（无编译期保障，见 D8）可合并为同一个「契约测试」PR。

### 工作量

B：1-1.5h

---

## D7. 删除会话的级联生命周期不一致（P3，语义决策）

### 现象（代码事实）

`DeleteConversationTx`：硬删 `messages` + `conversations`；事件发 `conversation.closed`。

但 ai-svc 的 `emotion_analysis` / `fused_emotions` 行（按 conversation_id 关联）**不级联清理**，`GetEmotionByConversation` 查已删会话仍返回数据；analytics 的 `user_behavior_events` 收到的是 closed（关闭）而非 deleted（删除）。

三个不一致：
1. 消息没了，情绪数据还在（生命周期错位）；
2. 「closed」语义上应是「正常结束/归档」，被 delete 复用，未来若做会话归档功能会撞名；
3. 行为报表把删除当作普通关闭统计，产品语义存疑。

### 待决策选项

| 选项 | 内容 |
|---|---|
| A | 接受现状并文档化：「历史情绪保留」是特性（用户删会话但情绪趋势报表保留），closed 即终结语义 | 
| B | 删除时级联清理 ai schema 两张表 + 新增 `conversation.deleted` 事件类型 |
| C | 会话软删除（status=0），消息保留，closed 语义归位 |

### 建议

短期 A（成本最低且数据上自洽），把「若做会话归档/恢复功能则必须走 B/C」登记进 plans。这个决策应由 owner 从产品语义出发拍板，不是纯技术问题——所以放进待决策而非直接排期。

### 工作量

- A：15 分钟（文档）
- B：3h（跨 svc 清理 + 事件 + 测试）
- C：1 天级（软删除贯穿查询层）

---

## D8. 契约卫生小项打包（P3，可合并为一个小 PR）

三个互相独立的轻微失真，不值得各自开单，建议打包：

| # | 项 | 事实 | 影响 |
|---|---|---|---|
| 1 | producer ExitSpan 的 peer 用 topic 名 | `kafka_publisher.go`：`CreateExitSpan(ctx, "kafka-publish", topic, injector)`，peer=`chat-events` | SkyWalking 拓扑图的边显示 `chat-svc → chat-events`，把 topic 当对端「服务」；统一为 `kafka:9092` 之类 infra 标识更符合拓扑语义（或在文档注明「topic 即对端」的约定） |
| 2 | extractSw8Header 双份 | ai-svc 与 analytics-svc 各一份相同实现（Stage 93 plan 已登记「Stage 94+ 收敛到 shared/pkg/messaging」） | 上游已知，此处仅归档提醒 |
| 3 | eventrow.EventType 字符串双处镜像 | shared/eventrow 注释「任何变更必须同时改两处」，无编译期保障 | 加一个契约测试（与 D6-B 同 PR） |

### 工作量

合计 ~1h。

---

## 汇总与排期建议

| # | 项 | 严重度 | 工作量 | 建议动作 |
|---|---|---|---|---|
| D1 | InMemory 兜底击穿 outbox 承诺 | 🟡 P1 | 0.5-1.5h | **优先拍板**：短期 C（指标可见），长期 B（relay 收紧） |
| D2 | outbox sent/dead 无清理 | 🟡 P1 | 1-1.5h | 保留期配置化 + 清理 job |
| D3 | attempts 不跨重启 | 🟡 P2 | 0.25h（登记）/ 3-4h（持久化） | 先登记语义 |
| D4 | analytics maxRetries 无配置 | 🟡 P2 | 0.5h | 并入 Stage 93 PR-2 |
| D5 | relay 多副本互斥 | 🟡 P2 | 0.25h（承诺）/ 2-3h（SKIP LOCKED） | 先 ADR 承诺单副本，扩容前置 A |
| D6 | 双 schema 转换链 | 🟢 P3 | 1-1.5h | CI 契约测试（与 D8-3 合并） |
| D7 | 删除会话生命周期 | 🟢 P3 | 0.25h（登记）/ 3h+（改行为） | owner 产品语义拍板 |
| D8 | 契约卫生打包 | 🟢 P3 | ~1h | 一个小 PR |

**全部登记版 ≈ 1h；全部落地版 ≈ 1.5-2 人天。**

排期建议：D1+D2 组成「Kafka 管线可靠性补完」小 sprint（半天）；D4 搭 Stage 93 顺风车；D3/D5/D7 先做文档登记等触发条件；D6+D8 合并一个契约测试 PR。

## 状态盘点（2026-09-15 Stage 101 全量收口后）

| # | 严重度 | 工作量 | 状态 | 落地证据 / 触发条件 |
|---|---|---|---|---|
| **D1** | 🟡 P1 | 0.5h（已落）/ 1h（B 长期）| ✅ **短期 C 已落** | [emotion-echo-chat-svc/internal/outbox/metrics.go](../../emotion-echo-chat-svc/internal/outbox/metrics.go) `OutboxSentViaFallbackTotal` counter + main.go fallback 路径 `IncSentViaFallback()`；ADR-19 "D1 fallback 路径登记" 段已加。Stage 94 PR-3 commit `44e9767` |
| **D2** | 🟡 P1 | 1-1.5h | ✅ **已落**（Round 2.1）| [emotion-echo-chat-svc/internal/outbox/cleanup.go](../../emotion-echo-chat-svc/internal/outbox/cleanup.go) `CleanupOnce()` + [OutboxRepo.DeleteOlderThan()](../../emotion-echo-chat-svc/internal/repository/outbox.go) 接口（InMemory + Postgres 两实现）+ [main.go cleanup ticker](../../emotion-echo-chat-svc/main.go) + `OutboxCleanedTotal{status=sent\|dead}` counter。Stage 99 Round 2.1 commit `4d118f6`；默认禁用 `OUTBOX_CLEANUP_ENABLED=true` 启用（与 Round 2.4 ctx cancel + Round 2.3 DLQ 监控一并发布）|
| **D3** | 🟡 P2 | 0.25h（登记）/ 3-4h（持久化）| ⏳ **待触发** | attempts 不跨 rebalance/进程重启；触发条件 = ai-svc/analytics-svc 多副本部署。当前单副本 dev 模式无影响。Stage 101 收口后仍未触发 |
| **D4** | 🟡 P2 | 0.5h | ✅ **已落** | [emotion-echo-analytics-svc/internal/config/config.go](../../emotion-echo-analytics-svc/internal/config/config.go) `Kafka.MaxRetries` 字段 + main.go applyEnvOverrides 读 `KAFKA_MAX_RETRIES`。Stage 96 PR-9a commit `2cc05c8` |
| **D5** | 🟡 P2 | 0.25h（承诺）/ 2-3h（SKIP LOCKED）| ⏳ **待触发** | relay 多副本互斥前置；触发条件 = chat-svc 决定扩副本的那个 stage。当前单副本 + ADR-19 "单副本承诺" 登记已含 |
| **D6** | 🟢 P3 | 1-1.5h | ⏳ **未变** | outbox payload JSONB ↔ Protobuf 双 schema 转换链；Round 2.2 反射枚举护栏只覆盖"加 EventType 时漏 switch case"侧，**双 schema 转换链本身的 stage-73 备份策略仍 open**（下次动 schema 时一起收）|
| **D7** | 🟢 P3 | 0.25h（登记）/ 3h+（改行为）| ⏳ **owner 拍板** | 删除会话生命周期不一致（消息没了情绪数据还在）；owner 从产品语义出发拍板。Stage 101 仍 backlog（语义决策非纯技术）|
| **D8** | 🟢 P3 | ~1h | 🟡 **部分落地** | D8-1 peer=topic 拓扑约定注释 ✅ Round 2.2 commit `6d6c3b1`（kafka_publisher.go:119 注释解释 + 切换路径）；D8-3 EventType 双处镜像反射枚举护栏 ✅ Round 2.2 commit `6d6c3b1`（chat-svc proto_marshal_test.go + eventrow mapper_test.go）；D8-2 extractSw8Header 双份 ⏳ 仍 open（ai-svc/analytics-svc 各一份，未收敛到 shared/pkg/messaging）|

**Round 2 收口摘要**（[stage-99-round-2-closure.md](../stages/stage-99-round-2-closure.md)）：
- ✅ D2: outbox sent/dead 清理 — Round 2.1
- 🟡 D8: peer=topic 注释 + EventType 反射枚举 — Round 2.2（部分：D8-2 extractSw8Header 双份仍 open）

**Stage 101 收口摘要**（[stage-101-multi-round-iteration-closure.md](../stages/stage-101-multi-round-iteration-closure.md)）：
- D3/D5/D7 仍 backlog（触发条件=多副本/上 prod/owner 拍板，单轮 TDD 不可独立完成）
- D8-2 extractSw8Header 双份：可作 D6/D8 收口的 sub-task，下次动 schema 时一起收

**Stage 97 调研结论**（代码事实）：
- D2 现状：`emotion-echo-chat-svc/migrations/c001_create_outbox_events.sql` 仅有 `idx_outbox_pending` (partial status=pending) + `idx_outbox_attempts`；`outbox/relay.go` 仅处理 pending；无 cleanup/retention/vacuum
- D3 现状：`ai-svc/internal/consumer/consumer.go` attempts 注释明写"消费周期内有效"；`analytics-svc/internal/kafka/consumer.go` 同款
- D5 现状：`outbox/relay.go FlushOnce` + `repository/outbox.go ListPending` 无 `FOR UPDATE SKIP LOCKED`；chat-svc 单副本运行
- D6 现状：`events/proto_marshal.go` + `outbox/relay.go publishOne` 注释承认双 schema 转换链；Stage 73 Protobuf 迁移后保留 JSONB 备份
- D7 现状：`conversation_repository.go DeleteConversationTx` 硬删 messages/conversations；事件复用 `conversation.closed`；ai schema `emotion_analysis`/`fused_emotions` 不级联清理
- D8 现状：`events/kafka_publisher.go Publish` CreateExitSpan peer 用 topic 名；`shared/pkg/eventrow/mapper.go` 包注释明写"EventType 字符串变更必须同步两处"

## 与既有文档的关系

- `observability-edge-gaps-from-code-review.md`（第一轮）：§A 已 landed（Stage 92/93），B~F 仍 open——本清单不含其内容。
- `kafka-reliability-gaps.md`：Sprint A 收口 6 项；本清单 D1/D2/D5 属于其「Sprint A 之后的新增面」，不是漏项。
- ADR-19（dev publisher）：D1 的 fallback 语义、D5 的单副本承诺建议在该 ADR 补充登记。

## 调研依据

| 文件 | 结论 |
|---|---|
| `chat-svc/main.go:138-158` | pub 默认 InMemory；Kafka init 失败仅 log 后继续（D1） |
| `chat-svc/main.go:191-222` | relay 启动条件 `pub != nil` 恒真（D1） |
| `chat-svc/internal/events/events.go` InMemoryEventPublisher | Publish 仅 append 内存（D1） |
| `chat-svc/migrations/001_create_outbox_events.sql` | 无清理/归档相关结构（D2） |
| `chat-svc/internal/outbox/relay.go` | 仅处理 pending；dead 行永久保留（D2） |
| `ai-svc/internal/consumer/consumer.go` attempts 注释 | 「消费周期内有效」（D3） |
| `analytics-svc/internal/kafka/consumer.go:64` | maxRetries 硬编码 3（D4） |
| `analytics-svc/main.go:186-208` | 只 wire WithDLQ/WithTracer，无 WithMaxRetries（D4） |
| `ai-svc/main.go:318-321` | ai-svc 有 Kafka.MaxRetries 配置（D4 对照） |
| `chat-svc/internal/outbox/relay.go FlushOnce` + `repository/outbox.go ListPending` | 无 FOR UPDATE SKIP LOCKED（D5） |
| `chat-svc/internal/events/proto_marshal.go` + `outbox/relay.go publishOne` 注释 | 双 schema 转换链 + Stage 73 dead 实证（D6） |
| `chat-svc/internal/repository/conversation_repository.go DeleteConversationTx` | 硬删 messages/conversations（D7） |
| `chat-svc/internal/logic/deleteconversationlogic.go` | 复用 conversation.closed 事件（D7） |
| `chat-svc/internal/events/kafka_publisher.go Publish` | peer 参数为 topic 名（D8-1） |
| `shared/pkg/eventrow/mapper.go` 包注释 | EventType 双处镜像（D8-3） |
| `docs/stages/stage-92-kafka-sw8-propagation-2026-09-14.md` §五 | OAP UI bug、analytics 残余、sw8 命名冲突（上游已知，未重复列入） |
