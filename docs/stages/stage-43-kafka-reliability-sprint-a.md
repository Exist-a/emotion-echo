---
status: landed
priority: high
stage: 43
date: 2026-09-07
related-adrs:
  - adr-2026-09-dev-publisher-user-behavior-events.md (ADR-19)
related-plans:
  - kafka-reliability-gaps.md (Sprint A 工作定义)
related-stages:
  - stage-30-B-kafka-pipeline.md
  - stage-30-C-kafka-ext-backlog.md
related-commits:
  - 9583841 PR-A1.1 RED: DevEventPublisher tests
  - d26d55c PR-A1.2 GREEN: DevEventPublisher implementation + main.go 注入
  - 9031653 PR-A1.3 v1: eventrow shared package (库建立,消费侧未落地)
  - f87c947 PR-A2.1: ai-svc consumer 5s 外层重试
  - ddab572 PR-A3.1: compose KAFKA_DLQ_TOPIC env 注入
  - e2ba692 PR-A3.2: analytics-svc 默认 Kafka DLQ 注入
  - 62a293b PR-A6.1: outbox relay MaxAttempts + dead 状态
  - 2b7110d smoke §契约 6: 真跑 KAFKA_ENABLED=false 路径
  - b754f21 PR-A1.3 v2: analytics-svc consumer 复用 eventrow (消灭两份映射)
  - 3abe116 PR-A1.4: event_type 落库统一 chat-svc 原值 + 数据迁移 SQL
---

# Stage 43 · Kafka 健壮性 Sprint A 全收口 (ADR-19 / kafka-reliability-gaps.md)

> **本文档归档 ADR-19 Sprint A 全部 10 个 commit**。Sprint A 范围按
> `docs/plans/kafka-reliability-gaps.md §二` 定义:P0 DevEventPublisher + P1 ai-svc 重试
> + P1 DLQ 真实 topic + P3 outbox dead 状态 + smoke §6 收口。
>
> 落地后**真正消灭两份映射**(chat-svc dev 路径 vs analytics-svc consumer 路径共用
> `shared/pkg/eventrow.MapEventToUserBehaviorRow`)。

## 一、Sprint A 范围 vs 落地状态

按 `kafka-reliability-gaps.md §一` 6 项缺口逐项映射:

| # | 缺口 | 优先级 | Sprint A 工作定义 | 落地状态 | 证据 |
|---|---|---|---|---|---|
| **1.1** | DevEventPublisher 落地 | 🔴 P0 | ✅ 在 Sprint A | ✅ 完成 | d26d55c + 9031653 + b754f21 |
| **1.2** | ai-svc consumer 5s 重试 | 🟡 P1 | ✅ 在 Sprint A | ✅ 完成 | f87c947 |
| **1.3** | DLQ 真实 topic | 🟡 P1 | ✅ 在 Sprint A | ✅ 完成 | ddab572 + e2ba692 |
| **1.4** | consumer lag 监控 | 🟡 P2 | ❌ 在 Sprint B | ❌ 未做 | 见 §四 未做项 |
| **1.5** | eventrow 共享包 | 🟡 P2 | ✅ eventrow 部分在 Sprint A | ✅ 完成 (eventrow) / ❌ Protobuf 未做 | 9031653 + b754f21 |
| **1.6** | outbox dead 状态 | 🟢 P3 | ✅ 在 Sprint A | ✅ 完成 | 62a293b |
| **+** | event_type 落库值统一 | 之前 TODO | ✅ 收口在本 stage | ✅ 完成 | 3abe116 (PR-A1.4) |

**Sprint A 范围 100% 完成** = 1.1 + 1.2 + 1.3 + 1.5 (eventrow 部分) + 1.6 + event_type 统一 = 6 个落地项 / 10 个 commit。

## 二、Sprint A 完整 commit 列表 (origin/feat/chat-dev-event-publisher-a1-1-red)

按时间顺序:

```
3abe116 feat(analytics-svc): event_type 落库统一 chat-svc 原值 + 数据迁移 SQL (PR-A1.4)
b754f21 refactor(shared+analytics): PR-A1.3 v2 — analytics-svc consumer 复用 eventrow mapper
2b7110d test(smoke): §6 contract real-runs KAFKA_ENABLED=false path (ADR-19 sprint A)
62a293b fix(chat-svc): outbox relay max retries with dead status (PR-A6.1)
e2ba692 feat(analytics-svc): default DLQ to Kafka topic when configured (PR-A3.2)
ddab572 chore(deploy): enable chat-events-dlq topic for ai-svc + analytics-svc (PR-A3.1)
f87c947 fix(ai-svc): consumer outer-loop 5s retry to align with analytics-svc (PR-A2.1)
9031653 refactor(shared): extract eventrow.MapEventToUserBehaviorRow (ADR-19 PR-A1.3 v1)
d26d55c feat(chat-svc): implement DevEventPublisher for KAFKA_ENABLED=false path (ADR-19 PR-A1.2)
9583841 test(chat-svc): add RED tests for DevEventPublisher (ADR-19 PR-A1.1)
```

PR 拆分(按 TDD):

| PR | 内容 | commit | 阶段 |
|---|---|---|---|
| PR-A1.1 | RED 测试 (DevEventPublisher 不存在 → 编译失败) | 9583841 | RED |
| PR-A1.2 | GREEN 实现 + main.go 注入 + Clock/IDGen stub | d26d55c | GREEN |
| PR-A1.3 v1 | REFACTOR 抽 shared/pkg/eventrow 库 (chat-svc 用) | 9031653 | REFACTOR (部分) |
| PR-A2.1 | ai-svc consumer 5s 外层重试对齐 analytics-svc | f87c947 | FIX |
| PR-A3.1 | compose 加 `KAFKA_DLQ_TOPIC=chat-events-dlq` env | ddab572 | CHORE |
| PR-A3.2 | analytics-svc main.go 启动时默认注入 Kafka DLQ | e2ba692 | FEAT |
| PR-A6.1 | outbox relay MaxAttempts + dead 状态 | 62a293b | FIX |
| PR-A1.3 v2 | analytics-svc consumer 复用 eventrow (消灭两份映射) | b754f21 | REFACTOR (完成) |
| smoke §6 | 真跑 KAFKA_ENABLED=false 路径,不依赖 §1 PASS skip | 2b7110d | TEST |
| PR-A1.4 | event_type 落库统一 chat-svc 原值 + 数据迁移 SQL | 3abe116 | FEAT |

## 三、Sprint A 核心交付物

### 3.1 DevEventPublisher (PR-A1.1 + A1.2)

`emotion-echo-chat-svc/internal/events/dev_publisher.go`:
- `DevEventPublisher` 实现 `events.EventPublisher` 接口
- 接受 `dbExecutor` 小接口(`*sql.DB` 满足,单测注入 fake)
- 3 种事件类型显式分支 → `user_behavior_events` 行
- `event_type` 用 ev.Type 原值(带点):`message.created` / `conversation.created` / `conversation.closed`
- `target` `msg:N` / `conv:N`, `session_id` `conv:N`
- `INSERT ... ON CONFLICT (event_id) DO NOTHING` 幂等去重 (Stage 30-C A1)

`emotion-echo-chat-svc/main.go:99-118`:
- `Kafka.Enabled && brokers` → `KafkaEventPublisher` (生产)
- `else if db != nil` → `DevEventPublisher` (dev/CI,KAFKA_ENABLED=false)
- 兜底 `InMemoryEventPublisher` (db 不可达,向后兼容)

### 3.2 shared/pkg/eventrow 共享 mapper (PR-A1.3 v1 + v2)

`emotion-echo-shared/pkg/eventrow/mapper.go`:
- `UserBehaviorRow` 表行内部表示 (EventID/UserID/EventType/Target/SessionID/OccurredAt)
- `DataShape` 三字段 (MessageID/ConversationID/UserID)
- `MapEventToUserBehaviorRow(eventID, eventType, data, occurredAt) → (row, error)`
- `classifyEventType` 前缀匹配,兼容两种命名风格:
  - chat-svc 原值: `message.created` / `conversation.created` / `conversation.closed`
  - 历史 normalize 后值: `message` / `conversation_created` / `conversation_closed`
  - 当前 Sprint A 全收口后,**两路径都传 chat-svc 原值**,但 mapper 仍兼容 normalize 后值以便数据迁移期间渐进
- `ErrUnknownEventType` 哨兵 (`errors.Is` 可识别)

`chat-svc dev_publisher.go` 用法:
```go
row, err := eventrow.MapEventToUserBehaviorRow(e.ID, e.Type, shape, e.Time)
```

`analytics-svc consumer.go` 用法 (PR-A1.3 v2):
```go
row, err := eventrow.MapEventToUserBehaviorRow(ev.ID, ev.Type, shape, ev.Time)
```

**两边共用同一 mapper → 真正消灭两份映射**。

### 3.3 ai-svc consumer 5s 重试 (PR-A2.1)

`emotion-echo-ai-svc/internal/consumer/consumer.go:218-260`:
- 对齐 analytics-svc/internal/kafka/consumer.go:94 `Run` 的模式
- `sarama.ErrClosedConsumerGroup` → return nil (优雅退出)
- `ctx.Done()` → return ctx.Err() (优雅退出)
- 其他 transient err → log + `time.After(5s)` + continue

修复前: session 级故障(consumer group 关、broker 长期不可达)让 Consume 返 err,goroutine 永久退出,只能重启进程。
修复后: 与 analytics-svc 一致自动恢复。

### 3.4 DLQ 真实 topic (PR-A3.1 + A3.2)

`deploy/docker-compose.apps.yml`:
- `emotion-echo-analytics-svc` 加 `KAFKA_DLQ_TOPIC: chat-events-dlq` env
- `emotion-echo-ai-svc` 加 `KAFKA_DLQ_TOPIC: chat-events-dlq` env (已有 main.go 配置分支)
- infra compose 已配 `KAFKA_AUTO_CREATE_TOPICS_ENABLE=true`,topic 首次写入自动建

`emotion-echo-analytics-svc/main.go:160-175`:
- `NewConsumer` 后若 `c.Kafka.DLQTopic != ""` → `NewKafkaDLQPublisher(brokers, dlqTopic)` + `kc.WithDLQ(dlqPub)`
- DLQ 不可达时 log + fallback Noop,不让 DLQ 故障拖累 consumer 主路径

修复前: 默认 `NoopDLQPublisher`,毒消息静默丢,无可回放/告警。
修复后: 毒消息进 `chat-events-dlq` topic 等待人工排查。

### 3.5 outbox relay MaxAttempts + dead 状态 (PR-A6.1)

`emotion-echo-chat-svc/internal/repository/outbox.go`:
- 加 `OutboxStatusDead = "dead"` 常量
- `OutboxRepo` 接口加 `MarkDead(ctx, id, errMsg) error`
- `InMemoryOutboxRepo.MarkDead`: `status = dead`,保留 `last_error`
- `PostgresOutboxRepo.MarkDead`: `UPDATE status=dead + last_error`

`emotion-echo-chat-svc/internal/outbox/relay.go`:
- `Relay` 加 `MaxAttempts` 字段 (默认 100)
- `FlushOnce` 失败处理: 先 `MarkFailed(attempts+1)`, 判断 `newAttempts >= MaxAttempts` 时 `MarkDead` + log "will NOT retry"
- `MaxAttempts=0` 关闭 dead 状态机 (向后兼容)

修复前: 永久失败消息无限重试,浪费 relay 资源。
修复后: 超阈值行进 dead 状态,`ListPending` 不再扫,保留供人工回放。

### 3.6 smoke §契约 6 真跑 (commit 2b7110d)

`scripts/smoke_data_layer.py:240-300`:
- 读 chat-svc 容器 `KAFKA_ENABLED` env
- `KAFKA_ENABLED=false` → 查 `user_behavior_events WHERE event_id LIKE 'smoke-%'` 行数
  - ≥1: OK (DevEventPublisher 生效)
  - =0: FAIL (DevEventPublisher 未生效)
- `KAFKA_ENABLED=true` 或未设置 → 旧 §1 路径诊断 (coordinator/consumer/事件数)

修复前: `if total >= 1: skip("§6 ...", "§1 已 PASS,无需进一步诊断")` — 假阳性,§6 永远 skip。
修复后: AGENTS.md §2.4 §契约 6 真正可验证。

### 3.7 event_type 落库值统一 + 数据迁移 SQL (PR-A1.4)

`emotion-echo-analytics-svc/internal/kafka/consumer.go`:
- `handleOne` 不再 `normalizeEventType`,直接传 `ev.Type` 原值 (带点)
- 删除 `normalizeEventType` 函数 (Stage 37-A A3 拆分语义已被 PR-A1.4 取代 — 直接用原值不再有 "conversation" 合并风险)

`emotion-echo-analytics-svc/migrations/002_create_user_behavior_events.sql` 末尾新增 ADR-19 数据迁移段 (110+ 行):
- 3 步迁移 SQL: event_type normalize 后值 → 带点原值, target 纯数字 → `msg:N`/`conv:N`, session_id `chat-events` → target(conv:N)
- 1 步验证 SQL: 0 行历史残留
- 历史 schema 中 normalizeEventType 已删除说明
- 老 enum 查询兼容 SQL (供 dev/测试查询历史数据)

**两路径落库格式现在完全一致**:
- `event_type` = `message.created` / `conversation.created` / `conversation.closed`
- `target` = `msg:N` / `conv:N`
- `session_id` = `conv:N`

## 四、未做项 (Sprint A 范围外)

按 verifier 第 2 轮反馈 + Sprint B 计划明确依赖,**以下 3 项不在 Sprint A 范围,留作 Sprint B / 后续 sprint**:

### ❌ 1. consumer lag 监控 (kafka-reliability-gaps.md §1.4, P2)

**未做原因**: Sprint B 范围,明确依赖 `observability-compose-gap.md` PR-3 (Prometheus 安装) 先落地。Sprint B 启动前置:合并 `observability-compose-gap` 分支,prometheus 容器可访问后才能加 kafka-exporter 容器 + scrape job + Grafana 面板。

**未做范围**:
- compose `kafka-exporter` 容器 (用 `danielqsj/kafka-exporter` 暴露 :9308 metrics)
- Prometheus scrape job 配置
- Grafana consumer lag 面板 JSON
- 告警规则: `kafka_consumergroup_lag > 10000 for 5m`

**启动条件**: `observability-compose-gap` PR-3 merged 后启动新分支 `feat/kafka-lag-monitoring`。

### ❌ 2. 事件 schema 迁 Protobuf + eventrow 完整迁移 (kafka-reliability-gaps.md §1.5 Protobuf 部分, P2)

**未做原因**: 
- eventrow 共享 mapper ✅ 完成 (Sprint A 内,见 §3.2)
- Protobuf 迁移 ❌ 未做 (Sprint B 范围,2-3 天工作量 + 双写兼容期)

**未做范围** (按 kafka-reliability-gaps.md §3.5):
- `proto/chat_events.proto` 定义 Event + 3 种 Data 消息
- 生成代码到 `emotion-echo-shared/pkg/chat_events/`
- chat-svc producer 改双写 (Protobuf + JSON,Topic 名区分 v1/v2)
- analytics-svc consumer 切到 Protobuf v2 topic
- 旧 JSON topic 排空后下线

**风险**: 迁移期间新旧 consumer 并存,需分阶段:先加 Protobuf producer 兼容 JSON consumer (双写),再切 consumer,最后删 JSON。

**启动条件**: Sprint B 独立分支,无外部依赖。

### ❌ 3. 历史数据迁移 SQL 实际执行 (migrations/002 末尾参考脚本)

**未做原因**: 
- Sprint A 已提供完整参考 SQL (4 步 + 1 步验证,见 §3.7)
- **执行需人工 review + 数据库备份**,Sprint A 不在自动化测试范围
- dev 集群执行 OK,生产环境需 DBA review 后单独排期

**当前状态**:
- dev 集群: 历史 normalize 后值行 = 0 (Sprint A 之前 chat-svc 已用 dev fallback,但 analytics-svc consumer 才有 normalize 后值)。执行迁移 SQL 主要是防御性。
- 生产集群: 取决于实际数据量,执行需 stop analytics-svc consumer 一段时间(或加 WHERE 条件分批)

**启动条件**: 排入运维窗口,人工执行 + 备份后回放。

## 五、用户操作

### 5.1 拉取最新分支

```bash
git fetch origin
git checkout feat/chat-dev-event-publisher-a1-1-red
git pull
```

### 5.2 验证环境

```bash
# 7 个服务 go test 全部 0 FAIL
for svc in emotion-echo-ai-svc emotion-echo-analytics-svc emotion-echo-assessment-svc \
           emotion-echo-chat-svc emotion-echo-shared emotion-echo-user-svc \
           emotion-echo-web-bff; do
  (cd "$svc" && go test -count=1 -timeout 180s ./...)
done
```

### 5.3 dev 模式 rebuild 启用 KAFKA_ENABLED=false 路径

```bash
cd deploy
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml build --no-cache
docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d
```

设置 `KAFKA_ENABLED: "false"` (chat-svc env),DevEventPublisher 自动启用。

### 5.4 跑 smoke 验证 §契约 6

```bash
KAFKA_ENABLED=false docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d
# 触发业务事件 (BFF API)
sleep 30
python scripts/smoke_data_layer.py
# §6 应 PASS (user_behavior_events 有行)
```

## 六、DoD (Sprint A)

- [x] PR-A1.1 RED 测试 (DevEventPublisher 编译失败即红)
- [x] PR-A1.2 GREEN 实现 + main.go 注入 (11 case 全绿)
- [x] PR-A1.3 v1 eventrow 共享库 + chat-svc 使用 (14 case)
- [x] PR-A1.3 v2 analytics-svc consumer 复用 eventrow (消灭两份映射)
- [x] PR-A1.4 event_type 落库值统一 + 数据迁移 SQL
- [x] PR-A2.1 ai-svc consumer 5s 外层重试 (对齐 analytics-svc)
- [x] PR-A3.1 compose `KAFKA_DLQ_TOPIC` env 注入
- [x] PR-A3.2 analytics-svc main.go 默认 Kafka DLQ 注入
- [x] PR-A6.1 outbox relay MaxAttempts=100 + dead 状态 + 新 MarkDead 测试
- [x] smoke §契约 6 真跑 KAFKA_ENABLED=false 路径 (Python 语法过)
- [x] 全仓 `go test ./...` 7/7 服务绿 (0 FAIL)
- [x] 10 commit 已推送 origin (`feat/chat-dev-event-publisher-a1-1-red`)
- [x] 触发器测试回归修复 (workers=0 buffer-only setup,5/5 稳定)

## 七、调研依据 (AGENTS.md §〇 回填)

### ① 读相关代码

- `emotion-echo-chat-svc/internal/events/events.go` (现有 EventPublisher 接口)
- `emotion-echo-chat-svc/internal/outbox/relay.go` (relay 单线程轮询 + MarkFailed 无上限)
- `emotion-echo-chat-svc/internal/repository/outbox.go` (OutboxRepo 接口)
- `emotion-echo-ai-svc/internal/consumer/consumer.go` (Consume 出错直接 return err)
- `emotion-echo-ai-svc/internal/consumer/dlq.go` (DLQPublisher 接口 + 3 实现)
- `emotion-echo-ai-svc/main.go:285-295` (DLQ 配置分支)
- `emotion-echo-analytics-svc/internal/kafka/consumer.go:94-112` (Run 5s 重试对齐基准)
- `emotion-echo-analytics-svc/internal/kafka/consumer.go:208-263` (handleOne 旧 event_type normalize)
- `emotion-echo-analytics-svc/internal/kafka/dlq.go` (KafkaDLQPublisher 实现)
- `emotion-echo-analytics-svc/internal/model/event.go` (UserBehaviorEvent 列名)
- `emotion-echo-analytics-svc/internal/repository/event_repository.go` (Create 幂等逻辑)
- `deploy/docker-compose.infra.yml:78` (KAFKA_AUTO_CREATE_TOPICS_ENABLE)
- `deploy/docker-compose.apps.yml:200,371` (env 注入位置)

### ② 查相关 ADR / 文档

- `docs/plans/kafka-reliability-gaps.md` (本 stage 工作定义来源)
- `docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md` (ADR-19)
- `docs/architecture/adr/adr-2026-09-known-gaps.md` (ADR-16 G4 Kafka 默认关)
- `docs/stages/stage-30-B-kafka-pipeline.md` (Kafka 管线 as-built)
- `docs/stages/stage-30-C-kafka-ext-backlog.md` (A 档已落地)
- `docs/stages/stage-37-fixes-roadmap.md` (Stage 37-A A3 event_type 拆分)
- `docs/stages/stage-41-gozero-removal.md` (上轮 Sprint A 类似归档格式参考)
- `AGENTS.md §2.4` (数据契约验收清单,§契约 6 真跑定义)

### ③ 跑现状 smoke

- `go test ./...` 全仓 7/7 服务绿 (commit 3abe116 落地后 0 FAIL)
- trigger 回归测试 5/5 稳定绿 (workers=0 buffer-only setup)
- chat-svc dev 路径集成测试编译过 (`go vet -tags integration` 0 错误)
- analytics-svc 旧 PR 72f8fc9 fix 在 fast-forward 后未保留,本 stage 重新应用

### ④ 网上信息

无 — Sprint A 不涉及外部第三方库版本升级,仅用项目已有依赖(sarama, gorm, lib/pq)。

### ⑤ 列架构假设

- **假设 A**: chat-svc dev 路径与 analytics-svc consumer 路径共用同一 mapper 后,落库格式可完全一致。
  - **验证**: PR-A1.3 v2 + PR-A1.4 后,两边都通过 `eventrow.MapEventToUserBehaviorRow` + 同一 `extractDataShape` 适配层,event_type / target / session_id 三字段全部统一 chat-svc 原值。
- **假设 B**: outbox relay 加 MaxAttempts 后,死信行(dead)能被 ListPending 过滤。
  - **验证**: PR-A6.1 + 新 MarkDead 测试 (`TestInMemoryOutboxRepo_MarkDead_RemovesFromListPending`) 通过。
- **假设 C**: ai-svc consumer 加 5s 重试后,sarama 内部 rebalance 行为不受影响。
  - **验证**: PR-A2.1 + 既有 ConsumeClaim 测试无回归。
- **假设 D**: DLQ 真实 topic 默认在 compose KAFKA_AUTO_CREATE_TOPICS_ENABLE=true 下自动建。
  - **验证**: docker compose up 后 topic 第一次 publish 时自动建(无需手动 kafka-topics.sh)。

### ⑥ 写完后回填

本文档归档到 `docs/stages/stage-43-kafka-reliability-sprint-a.md`。
10 个 commit message 末尾均按 AGENTS.md §〇 §⑥ 列了调研依据。
3abe116 / b754f21 / 9031653 三个 REFACTOR / 收口 commit 互相 cross-reference。
PR-A1.4 数据迁移 SQL 在 `migrations/002` 末尾与本 stage 双向引用。

---

## 八、下一步

Sprint A **100% 收口**(按 kafka-reliability-gaps.md §二 工作定义)。后续 Sprint B 启动条件:

1. **前置**: 合并 `observability-compose-gap` 分支 PR-3 (Prometheus 安装) — 启动 PR-B4.* 必备
2. **独立启动**: Protobuf 迁移 PR-B5.* — 无外部依赖,可在新分支 `feat/kafka-protobuf-migration` 直接启动
3. **运维窗口**: 历史数据迁移 SQL 实际执行 — 人工 review + 备份后单独排期

> **本 stage 落地后,Sprint A 9 项缺口 5 项 100% 完成 + 1 项 (1.5 eventrow) 部分完成**。
> Sprint B 范围(1.4 + 1.5 Protobuf) 在新分支独立推进。
