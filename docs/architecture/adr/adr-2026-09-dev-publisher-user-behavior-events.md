# ADR-19 · DevEventPublisher 同步写 user_behavior_events（2026-09-03）

> **状态**：已批准 · **类型**：dev-only 架构调整 · **目标分支**：`fix/stage-36-post-test-cleanup`（Stage 37-A 用）
> **来源**：Stage 37 路线图 PR-A1 实际代码勘察时发现初版描述与架构不符
> **前置**：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md) §PR-A1

---

## 上下文（Context）

盘点 chat-svc 事件发布链（[events/events.go](/emotion-echo-chat-svc/internal/events/events.go)、[outbox/relay.go](/emotion-echo-chat-svc/internal/outbox/relay.go)）后发现：

```
chat-svc handler
   ↓ InsertOutbox（status=pending）
outbox_events（PG 表）
   ↓ OutboxRelay.FlushOnce 每 1s 扫描
   ↓ EventPublisher.Publish
   ├─ KAFKA_ENABLED=true  → KafkaEventPublisher（sarama）→ Kafka topic=chat-events
   └─ KAFKA_ENABLED=false → publisher=nil，relay publishOne 永远返回 errors.New("publisher is nil")
                              ↓
                          analytics-svc Kafka Consumer → user_behavior_events
```

`user_behavior_events` 表是 **analytics-svc consumer 的唯一写入方**（除 ETL 外），它不在 chat-svc 的写权限里。

### 现状问题（dev 模式）

- `KAFKA_ENABLED=false` 时 outbox relay 启动但 publisher=nil，relay 每秒都失败一次（`relay.go:106-108`）
- `outbox_events` 行堆积 status=pending，`user_behavior_events` 永远为空
- 4 个 dashboard（daily/weekly/monthly/annual）在 dev docker compose 启动后看不到任何数据
- ADR-17 修复的 chart 渲染管道虽然契约对齐了，但**数据源头没数据**，所以即使契约对、契约渲染空

---

## 决策（Decisions）

### §A. 在 chat-svc 加 DevEventPublisher，实现 EventPublisher 接口

`emotion-echo-chat-svc/internal/events/dev_publisher.go`：

```go
type DevEventPublisher struct {
    db       *sql.DB
    clock    clock.Clock  // 时间可注入，便于测试固定
    idGen    idgen.IDGen  // 事件 ID 与 Event.ID 对齐
}

func (p *DevEventPublisher) Publish(ctx context.Context, topic string, e *Event) error {
    // 解析 e.Data 按 type 映射到 user_behavior_events 行
    return insertUserBehaviorEvent(ctx, p.db, e)
}
```

**main.go** 在 `Kafka.Enabled == false` 时把 nil 替换为 `DevEventPublisher`：

```go
var publisher events.EventPublisher
if cfg.Kafka.Enabled {
    publisher, err = events.NewKafkaEventPublisher(cfg.Kafka.Brokers)
    if err != nil { log.Fatal(...) }
} else {
    publisher = events.NewDevEventPublisher(db, clock, idGen)
    log.Printf("[events] using DevEventPublisher (KAFKA_ENABLED=false, dev-only)")
}
outboxRelay := outbox.NewRelay(outboxRepo, publisher, ...)
```

### §B. EventPublisher 接口契约不变，relay 无改动

`DevEventPublisher` 与 `KafkaEventPublisher` 实现同一个 `Publish(ctx, topic, *Event) error` 接口，对 outbox relay 完全透明。relay 不需要知道下游是 Kafka 还是 PG。

### §C. 同步语义：DevEventPublisher.Publish 必须同步落库后才返回

outbox relay 是**同步等待 Publish 返回**（`relay.go:109`），所以 Publish 落库成功 → relay MarkSent（status=sent）；落库失败 → relay MarkFailed。**与生产 Kafka 行为对齐**：Kafka 失败 → 重试；DB 失败 → 重试。两者失败语义一致。

### §D. 事件→行的映射集中到 chat-svc events 包

`DevEventPublisher` 内部维护 `message.created / conversation.created / conversation.closed` 三种事件类型到 `user_behavior_events` 行的映射函数。未来：
- **A2 修复**（target 写 message_id 而非 Event.UUID）：改 `mapMessageCreated`
- **A3 修复**（conversation.created/closed event_type 细分）：改 `mapConversationCreated/Closed`
- 一处映射，三处复用（dev publisher + analytics-svc consumer 后续 PR）

### §E. 跨服务职责的代价

**DevEventPublisher 让 chat-svc 知道了 `user_behavior_events` 表结构** = 跨服务职责。原本这表是 analytics-svc consumer 独占。

**接受这个代价的理由**：
1. DevEventPublisher **只在 `KAFKA_ENABLED=false` 启用**——prod 不命中，跨服务耦合不会影响生产路径
2. InMemoryEventPublisher 已经"跨了测试断言的职责"（events.go §InMemoryEventPublisher），DevEventPublisher 是同位的"dev 模式消费者"概念
3. 不写这个 publisher 就只能 `docker compose up kafka + zookeeper + analytics-svc consumer` 才能看到 dashboard 数据——dev 体验断裂

**缓解措施**：
- 把 `user_behavior_events` 表的 INSERT SQL 抽到 `emotion-echo-shared/internal/eventrow/` 包，analytics-svc consumer 和 DevEventPublisher 共用，未来 schema 变更只需改一处
- ADR-20（待写）记录 chat-svc 表依赖清单

### §F. 不做

- 不在 chat-svc 起 goroutine 模拟 consumer（会增加调度复杂度，事件顺序也难保证）
- 不引入第二份事件 schema（保持 events.Event 不变）
- 不写 backport 到 `outbox_events` 的旁路表

---

## 实施细节

### 新增文件

| 文件 | 用途 |
|---|---|
| `emotion-echo-chat-svc/internal/events/dev_publisher.go` | `DevEventPublisher` 实现 `EventPublisher` 接口 |
| `emotion-echo-chat-svc/internal/events/dev_publisher_test.go` | 单测覆盖 3 种事件类型 + DB 失败重试 + 顺序保留 |
| `emotion-echo-chat-svc/integration_test/dev_publisher_integration_test.go` | testcontainers PG + chat-svc binary 启 `KAFKA_ENABLED=false`，发 message → 断言 user_behavior_events 1 行 + outbox status=sent |
| `emotion-echo-shared/internal/eventrow/mapper.go` | `MapEventToUserBehaviorRow(*events.Event) (UserBehaviorRow, error)` 公共映射函数 |
| `emotion-echo-shared/internal/eventrow/mapper_test.go` | 3 种事件类型映射表驱动 |

### 修改文件

| 文件 | 改动 |
|---|---|
| `emotion-echo-chat-svc/main.go` | 在 `Kafka.Enabled == false` 分支构造 `DevEventPublisher`，替代原 nil publisher |
| `emotion-echo-chat-svc/internal/config/config.go` | 加 `Clock clock.Clock`、`IDGen idgen.IDGen` 字段注入（AGENTS.md §3.2 时钟/UUID 必须接口化） |

### TDD 节奏

```
PR-A1.1: RED  DevEventPublisher 不存在 → 编译失败 + integration test 报表空
PR-A1.2: GREEN events.DevEventPublisher + main.go 注入（最小实现，hard-code SQL）
PR-A1.3: REFACTOR 抽 eventrow.MapEventToUserBehaviorRow 到 shared 包
         改 DevEventPublisher + analytics-svc consumer 同时调用共享 mapper
```

---

## 后果（Consequences）

### ✅ 正向

- dev docker compose 启动后 4 个 dashboard 能看到 message/conversation_created/conversation_closed 真实数据
- outbox_events status=pending 不再无限堆积
- A2/A3 修复只需改一处 mapper，dev + 生产链路同步
- shared pkg `eventrow` 让 chat-svc + analytics-svc consumer 共用映射函数，杜绝契约漂移
- Clock/IDGen 接口化满足 AGENTS.md §3.2 可测试性硬规则

### ⚠️ 代价

- chat-svc 在 dev 模式跨服务写 `user_behavior_events`（prod 不命中）
- 新增 shared 包 `eventrow` 增加编译耦合（chat-svc + analytics-svc + future services 都依赖）
- DevEventPublisher 跟 Kafka publisher 在事务一致性上不对等（Kafka 是 at-least-once，DB 是直接 insert；DB 失败 relay 会 MarkFailed 重试，但中间状态可能短暂不一致）

### 🔁 替代方案（已否决）

- **起 dev consumer in chat-svc**：调度复杂、事件顺序难保
- **改 relay publisher=nil 时不报错**：把"未发布"事件丢到 dead_letter 表 → analytics-svc 启动时回放 → 跨服务状态同步复杂
- **用 mock kafka（miniredis 思路）**：minimock/saramamock 项目成熟度参差，不如 DevEventPublisher 直接

---

## 验收

| 维度 | 标准 |
|---|---|
| DevEventPublisher 单测 | 3 事件类型 × 3 错误路径 = ≥ 9 case 全绿 |
| shared eventrow 单测 | 3 事件类型 + 边界值 ≥ 12 case 全绿 |
| integration test | testcontainers PG + chat-svc binary，发 10 条 message → user_behavior_events 10 行 + outbox 10 行 status=sent |
| prod 路径无回归 | `KAFKA_ENABLED=true` 启动路径与 Stage 36 行为一致（sarama producer 不变） |
| `go vet ./...` | 无 warning |
| `go test ./internal/events/...` | 全绿 |

---

## 待办（Stage 37-A 内）

- [ ] PR-A1.1 + PR-A1.2 + PR-A1.3 落地
- [ ] shared eventrow 包从 analytics-svc consumer 同步引用（消除两份映射）
- [ ] stage-37-A-landing.md 收口

## 不在本 ADR 范围

- ADR-20（chat-svc 表依赖清单）：等 Stage 37-A 全收口后单独起
- ADR-21（dev 模式跨服务职责总清单）：积累到 3+ 个 dev-only 跨服务点时起
