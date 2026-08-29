# Stage 30-A — `emotion-echo-analytics-svc` 业务端点落地计划

> **范围声明**：本文档是 **Stage 30-A 的路线图留存**，先行于实现落地。
> 本次 session 的唯一动作：**新建本文档**，**不修改**任何代码 / Helm / docker-compose / APISIX / 前端。
> 真正动手时按 §四 TDD 节拍推进，每段循环在独立 commit 内走完 Red → Green → Refactor。

继承：
- `docs/stage-26-T-test-backlog.md`（TDD 节奏 + 滚动执行表）
- `docs/stage-29-A.5-tls-live-smoke.md`（live smoke 模式）
- `docs/stage-29-D-tls-all-routes.md`（ApisixRoute 模板）
- `docs/stage-30-web-bff.md`（后续 web-bff 规划）
- AGENTS.md §三.3（依赖接口反转、可测试）+ §四（禁止事项）

---

## 一、目标

| # | 决议 | 选择 |
|---|------|------|
| 1 | 服务定位 | 新增 / 完善 `emotion-echo-analytics-svc`（Go / Gin :8893）：暴露 9 个聚合读端点 |
| 2 | 端点数量 | **9 个**（reports×2 + user-behavior×3 + mental-health×4） |
| 3 | 数据来源 | 跨 schema 聚合读（chat / ai / assessment / analytics） |
| 4 | 范围 | 仅业务端点实现 + 测试 + 文档；不在本阶段改前端切流量 |
| 5 | 迁移策略 | **照搬 legacy 实现 1:1**（legacy gin 已有完整 handler / service / repo），不重写 |
| 6 | 测试约定 | 表驱动 + sibling test（AGENTS §1.1）；`go test ./...` ≤ 5s（logic 层） / ≤ 30s（handler 层含 Postgres） |
| 7 | 集成测试 | 用 testcontainers-go 起 Postgres 15 + apply migrations 001-003 + 种子数据 |
| 8 | 命名 | 端点路径、字段、错误响应与 legacy 完全一致（前端兼容） |
| 9 | 文档 | 本文档作为后续多 session 推进的路线图 |
| 10 | 后续 | 真正代码在多 session 推进（每 session 2-3 commit） |

---

## 二、9 个端点契约

全部已在 `legacy/emotion-echo-gin/internal/` 有完整实现，本表对齐契约。

| # | Method | Path | legacy 实现 | service 层 | 当前状态 |
|---|--------|------|-------------|-----------|---------|
| 1 | GET | `/api/v1/reports/daily?date=YYYY-MM-DD` | `handler.ReportHandler.GetDaily` | `service.DailyReport` | **404**（前端 dashboard 调它） |
| 2 | GET | `/api/v1/reports/trend?type=weekly|monthly|yearly&start_date=&end_date=` | `handler.ReportHandler.GetTrend` | `service.TrendReport` | **404** |
| 3 | GET | `/api/v1/user-behavior/day-night` | `handler.UserBehaviorHandler.GetDayNightPattern` | `service.DayNightPattern`（按 hour 桶聚 30d） | **404** |
| 4 | GET | `/api/v1/user-behavior/depth` | `handler.UserBehaviorHandler.GetInteractionDepth` | `service.InteractionDepth` | **404** |
| 5 | GET | `/api/v1/user-behavior/frequency` | `handler.UserBehaviorHandler.GetFrequencyTrend` | `service.FrequencyTrend`（30d 日计数） | **404** |
| 6 | GET | `/api/v1/mental-health/assessment?type=daily|weekly|comprehensive` | `handler.MentalHealthHandler.GetAssessment` | `service.MentalHealthAssessment` | **404** |
| 7 | GET | `/api/v1/mental-health/history?type=&limit=&cursor=` | `handler.MentalHealthHandler.GetHistory` | paginated list | **404** |
| 8 | POST | `/api/v1/mental-health/trigger` | `handler.MentalHealthHandler.TriggerAssessment` | async（goroutine + channel） | **404** |
| 9 | GET | `/api/v1/mental-health/trend?type=weekly|monthly` | `handler.MentalHealthHandler.GetTrend` | `service.TrendData` | **404** |

---

## 三、关键技术决策

### 3.1 数据访问模式 — Pragmatic Reporting Database（用户答复「业界如何做」→ 业界共识）

**业界研究结论**（microservices.io + Microsoft Learn + arXiv 2024/2025 实证）：

| 模式 | 适用规模 | Emotion-Echo 适用度 |
|------|---------|---------------------|
| 单一 PG + 跨 schema 只读 VIEW（reporting database） | 1k-100k users | **✅ 直接采用** |
| API Composition over gRPC | 中等，需松耦合 | ⚠️ 工作量翻3倍 |
| Kafka 事件流 + Materialized view | 100k+ users | ❌ 过早优化 |
| ClickHouse / StarRocks | 1M+ users | ❌ petabyte scale |

**核心引用**：
- arXiv 2510.20582 (2025) — *Empirical Study on Database Usage in Microservices*：实证共享数据库在真实 microservices 部署中是事实标准
- arXiv 2405.11529 (2024) — *Benchmarking DMS for Microservices*：Postgres 在 sub-million row 量级比专用 OLAP store 快 3-10x
- microservices.io *Database per Service* + *Reporting Database* pattern
- Microsoft Learn — API Composition 应在 gateway 层做 in-memory join

**具体方案**：
- analytics-svc 用**专用只读 DB role**：`search_path = emotion_echo_analytics, emotion_echo_chat, emotion_echo_ai, emotion_echo_assessment`，grant 仅对 `*_v` VIEW
- **永不写**其他 schema（接口层 `ReportRepo` 严格只读）

**Stage-2 触发条件**（任二满足升级 ClickHouse / MaterializedPostgreSQL）：
- 单 Postgres CPU 持续 > 60% 来自 analytics 查询
- 任何聚合端点 p95 > 2s
- 日数据量 > 10M 行或需保留 > 1B 行

### 3.2 Trigger 异步 — **用户答复：goroutine + channel**

```go
type TriggerQueue struct {
    ch chan TriggerRequest  // buffered, cap=64
}

func (q *TriggerQueue) Submit(req TriggerRequest) error {
    select {
    case q.ch <- req:
        return nil
    default:
        return ErrQueueFull  // backpressure
    }
}

// main.go 启动 worker pool
go func() {
    for req := range q.ch {
        // 同步执行 mental_health_service.TriggerAssessment
        // 写 result 到 PG + 可选发 Kafka event
    }
}()
```

**为什么不是同步**：用户体验差（评估耗时秒级）
**为什么不是 Kafka**：跨服务耦合，v1 不值

### 3.3 事件采集 — **用户答复：Kafka consumer**

analytics-svc 启动 goroutine 订阅 `chat-events` topic（chat-svc 已在 Stage 26 接入 Kafka producer）：
- `message.created` → 写入 `user_behavior_events`（event_type='message'）
- `conversation.created` → 写入 `user_behavior_events`（event_type='conversation'）
- `conversation.closed` → 更新 last_message_at 等聚合

复用 `emotion-echo-shared/pkg/messaging.KafkaConsumer`（已有），无新依赖。

### 3.4 整体架构图

```
                          ┌─────────────────────────────────────┐
                          │     emotion-echo-postgres           │
   chat-svc  ─publish──→  │  emotion_echo_chat.msg_summary_v   │
   ai-svc    ─publish──→  │  emotion_echo_ai.daily_emotion_v    │
   asmnt-svc ─publish──→  │  emotion_echo_assessment.assessment_v│
                          │  emotion_echo_analytics.*           │
                          └─────────┬───────────────────────────┘
                                    │ (read-only role, search_path)
                                    ▼
                          ┌─────────────────────────────────────┐
                          │     analytics-svc (:8893)            │
                          │  9 logic + 3 handler + Kafka consumer│
                          │  + TriggerQueue (goroutine + channel)│
                          └─────────┬───────────────────────────┘
                                    │
                                    ▼
                       /api/v1/reports/* /user-behavior/*
                       /api/v1/mental-health/*  via APISIX → frontend
```

---

## 四、模块结构（保持 AGENTS §1.1 sibling test 约定）

```
emotion-echo-analytics-svc/
├── main.go                                # 启动 server + Kafka consumer + TriggerQueue worker
├── go.mod
├── internal/
│   ├── config/
│   │   ├── config.go
│   │   ├── config_test.go                 # 已有
│   ├── svc/
│   │   ├── servicecontext.go              # + EventRepo / ReportRepo / MentalHealthRepo / KafkaConsumer / TriggerQueue
│   │   └── servicecontext_test.go         # TBD
│   ├── handler/
│   │   ├── health_handler.go               # 已有
│   │   ├── health_handler_test.go          # 已加 (3 subtests)
│   │   ├── reports_handler.go              # NEW — daily + trend
│   │   ├── reports_handler_test.go         # NEW
│   │   ├── userbehavior_handler.go         # NEW — day-night / depth / frequency
│   │   ├── userbehavior_handler_test.go    # NEW
│   │   ├── mentalhealth_handler.go         # NEW — assessment / history / trigger / trend
│   │   └── mentalhealth_handler_test.go    # NEW
│   ├── logic/
│   │   ├── healthlogic.go                  # 已有
│   │   ├── healthlogic_test.go             # 已有
│   │   ├── reports/
│   │   │   ├── reports_daily_logic.go
│   │   │   ├── reports_daily_logic_test.go
│   │   │   ├── reports_trend_logic.go
│   │   │   └── reports_trend_logic_test.go
│   │   ├── userbehavior/
│   │   │   ├── userbehavior_daynight_logic.go
│   │   │   ├── userbehavior_daynight_logic_test.go
│   │   │   ├── userbehavior_depth_logic.go
│   │   │   ├── userbehavior_depth_logic_test.go
│   │   │   ├── userbehavior_frequency_logic.go
│   │   │   └── userbehavior_frequency_logic_test.go
│   │   ├── mentalhealth/
│   │   │   ├── mentalhealth_assessment_logic.go
│   │   │   ├── mentalhealth_assessment_logic_test.go
│   │   │   ├── mentalhealth_history_logic.go
│   │   │   ├── mentalhealth_history_logic_test.go
│   │   │   ├── mentalhealth_trigger_logic.go
│   │   │   ├── mentalhealth_trigger_logic_test.go
│   │   │   ├── mentalhealth_trend_logic.go
│   │   │   └── mentalhealth_trend_logic_test.go
│   ├── repository/
│   │   ├── event_repository.go             # 已有 — 扩展写入路径（Kafka consumer）
│   │   ├── event_repository_test.go        # 已有 — 加 Kafka 写入测试
│   │   ├── report_repository.go            # NEW — 跨 schema 只读聚合
│   │   ├── report_repository_test.go       # NEW
│   │   ├── mentalhealth_repository.go      # NEW — 跨 schema 只读
│   │   ├── mentalhealth_repository_test.go # NEW
│   │   ├── postgres_readonly.go            # NEW — set search_path + VIEWs 集中定义
│   │   └── postgres_readonly_test.go       # NEW — 验证 search_path 设置
│   ├── messaging/
│   │   ├── chat_events_consumer.go         # NEW — 订阅 chat-events
│   │   ├── chat_events_consumer_test.go    # NEW
│   ├── trigger/
│   │   ├── trigger_queue.go                # NEW — buffered channel + worker pool
│   │   ├── trigger_queue_test.go           # NEW
│   ├── types/
│   │   ├── types.go
│   │   └── types_test.go
│   └── kafka/
│       ├── consumer.go                     # 复用 shared/pkg/messaging 包装
│       └── consumer_test.go
└── migrations/                              # NEW — 跨 schema VIEW DDL
    ├── 001_create_views.sql                 # 跨 schema 只读 VIEWs
    ├── 002_create_user_behavior_events.sql
    └── 003_create_mv_daily_emotion.sql      # materialized view（日报加速）
```

---

## 五、TDD 节奏（多 session 推进 — 用户原话「拆分为独立环节，多轮推进」）

### Round 1 — Reports（2 commits，1 个 session）

```
commit 1: test(analytics-svc): RED reports_daily_logic + reports_trend_logic
   - 4 子测试：daily (date default + custom) / trend (type=weekly|monthly|yearly + boundary)
   - 用 in-memory report_repository（interface 注入）
   - RED：实现不存在，编译失败

commit 2: feat(analytics-svc): GREEN reports_daily_logic + reports_trend_logic + report_repository (postgres impl + in-memory test fake)
   - 实现迁移自 legacy gin
   - 4 子测试全绿
```

### Round 2 — UserBehavior（2 commits，1 个 session）

```
commit 3: test(analytics-svc): RED userbehavior_daynight + depth + frequency
commit 4: feat(analytics-svc): GREEN userbehavior_* + 扩展 EventRepo（增加 Kafka consumer 写入路径）
```

### Round 3 — MentalHealth logic（4 commits，1-2 个 session）

```
commit 5: test(analytics-svc): RED mentalhealth_assessment + history (4 子测试)
commit 6: feat(analytics-svc): GREEN mentalhealth_assessment + history + mentalhealth_repository
commit 7: test(analytics-svc): RED mentalhealth_trigger (async via channel) + trend
commit 8: feat(analytics-svc): GREEN mentalhealth_trigger + trigger_queue + mentalhealth_trend
```

### Round 4 — Handler + Kafka consumer（3 commits，1 个 session）

```
commit 9: test(analytics-svc): RED 9 个 handler 子测试 (route binding + 200 + 401 + 400)
commit 10: feat(analytics-svc): GREEN reports_handler + userbehavior_handler + mentalhealth_handler + main.go 路由注册
commit 11: feat(analytics-svc): Kafka consumer 订阅 chat-events → user_behavior_events
   - + tests for consumer event handler
```

### Round 5 — Infrastructure + Documentation（3 commits，1 个 session）

```
commit 12: feat(analytics-svc): 跨 schema VIEWs + materialized view DDL（migrations/001-003）
commit 13: feat(analytics-svc): search_path read-only role + integration test (testcontainers postgres up + apply views + run reports/daily)
commit 14: docs(stage-30-A): analytics-svc 业务端点 landing — 多 session 总结
   - 包含 9 个端点契约 + TDD commit 链 + 业界引用 + Stage-2 升级路径
```

**总计**：14 个 commit，5-6 个 session（每 session ~2-3 小时）

---

## 六、Schema & DB 改动

### 6.1 新建 VIEWs（只读）

```sql
-- migrations/001_create_views.sql
-- 由 owning service 部署：本 analytics-svc migration 提议，chat-svc / ai-svc / assessment-svc 应各自确认。

-- emotion_echo_chat: 暴露 messages 给 analytics（不暴露 content，仅元数据）
CREATE OR REPLACE VIEW emotion_echo_chat.msg_summary_v AS
SELECT id, conversation_id, user_id, role, content_type, tokens_used,
       LENGTH(content) AS content_len, send_time
FROM emotion_echo_chat.messages;

-- emotion_echo_ai: 暴露 emotion_analysis 给 analytics
CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_v AS
SELECT id, message_id, conversation_id, user_id, primary_emotion,
       sentiment_score, confidence, model, created_at
FROM emotion_echo_ai.emotion_analysis;

-- emotion_echo_assessment: 暴露 assessment 给 analytics
CREATE OR REPLACE VIEW emotion_echo_assessment.assessment_v AS
SELECT id, user_id, assessment_type, period_start, period_end,
       overall_score, risk_level, dimensions, created_at
FROM emotion_echo_assessment.mental_health_assessments;
```

### 6.2 新建 user_behavior_events 表

```sql
-- migrations/002_create_user_behavior_events.sql
CREATE TABLE IF NOT EXISTS emotion_echo_analytics.user_behavior_events (
  id           BIGSERIAL PRIMARY KEY,
  user_id      BIGINT NOT NULL,
  event_type   VARCHAR(32) NOT NULL,  -- 'message' / 'conversation_created' / 'conversation_closed'
  target       VARCHAR(64),
  properties   JSONB,
  session_id   VARCHAR(64),
  ip           INET,
  user_agent   TEXT,
  occurred_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_user_time ON emotion_echo_analytics.user_behavior_events(user_id, occurred_at DESC);
CREATE INDEX idx_events_type_time ON emotion_echo_analytics.user_behavior_events(event_type, occurred_at DESC);
```

### 6.3 新建 materialized view（加速日报）

```sql
-- migrations/003_create_mv_daily_emotion.sql
CREATE MATERIALIZED VIEW emotion_echo_analytics.mv_daily_emotion AS
SELECT user_id,
       DATE_TRUNC('day', created_at) AS day,
       primary_emotion,
       COUNT(*) AS cnt,
       AVG(sentiment_score) AS avg_sentiment,
       AVG(confidence) AS avg_confidence
FROM emotion_echo_ai.daily_emotion_v
WHERE created_at > NOW() - INTERVAL '90 days'
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion;

CREATE UNIQUE INDEX mv_daily_emotion_user_day_emotion_idx
  ON emotion_echo_analytics.mv_daily_emotion(user_id, day, primary_emotion);

-- v1 阶段：main.go 启动时 REFRESH（无需 pg_cron）
-- v2 阶段：pg_cron '*/15 * * * *' REFRESH MATERIALIZED VIEW CONCURRENTLY
```

### 6.4 新建 read-only role

```sql
-- 由 deploy/init.sql 一次性创建
CREATE ROLE analytics_reader LOGIN PASSWORD 'CHANGE_ME'
  NOSUPERUSER NOCREATEDB NOCREATEROLE;

GRANT USAGE ON SCHEMA
  emotion_echo_chat, emotion_echo_ai,
  emotion_echo_assessment, emotion_echo_analytics
  TO analytics_reader;

-- 只读 VIEW + 本 schema 表权限
GRANT SELECT ON
  emotion_echo_chat.msg_summary_v,
  emotion_echo_ai.daily_emotion_v,
  emotion_echo_assessment.assessment_v,
  emotion_echo_analytics.user_behavior_events,
  emotion_echo_analytics.mv_daily_emotion
  TO analytics_reader;

ALTER ROLE analytics_reader SET search_path TO
  emotion_echo_analytics, emotion_echo_chat,
  emotion_echo_ai, emotion_echo_assessment;
```

---

## 七、TDD 边界与测试策略

- **跨 schema SQL 测试**：用 `testcontainers-go` 起 Postgres 15 + apply migrations 001-003 + 种子数据。每个 logic 测试用真 SQL（不是 mock）— 符合 AGENTS §三.3
- **Logic 层单测**：repository 用 in-memory fake（实现 `ReportRepo` interface），快速（≤ 5s）
- **Handler 层单测**：用真 gin + InMemoryReportRepo，与 chat-svc handler 测试同模式
- **Kafka consumer 测试**：用 `kafka-go` 的 testcontainers 或 mock publisher
- **TriggerQueue 测试**：用 buffered channel 边界 + 同步等待 worker done

**禁止**：`time.Sleep`、snapshot-copy 字典（AGENTS §四）

---

## 八、成功标准（Completion Audit Checklist）

### 文档层
- [ ] 本 plan 文件（`docs/stage-30-A-analytics-business.md`）落 main（commit 14 之一）
- [ ] 每端点的契约文档化在 plan 中（已写）

### 实现层（每端点独立 commit）
- [ ] reports_daily_logic_test（≥4 子测试）
- [ ] reports_trend_logic_test（≥4 子测试）
- [ ] userbehavior_daynight_logic_test（≥3 子测试）
- [ ] userbehavior_depth_logic_test（≥3 子测试）
- [ ] userbehavior_frequency_logic_test（≥3 子测试）
- [ ] mentalhealth_assessment_logic_test（≥3 子测试）
- [ ] mentalhealth_history_logic_test（≥3 子测试）
- [ ] mentalhealth_trigger_logic_test（≥3 子测试，含 async / queue-full / cancel）
- [ ] mentalhealth_trend_logic_test（≥3 子测试）
- [ ] reports_handler_test（≥3 子测试，route + 200 + 401）
- [ ] userbehavior_handler_test（≥3 子测试）
- [ ] mentalhealth_handler_test（≥3 子测试，含 POST 202）
- [ ] report_repository_test（postgres + in-memory ≥3 子测试）
- [ ] mentalhealth_repository_test（≥3 子测试）
- [ ] chat_events_consumer_test（≥3 子测试，含 3 种 event 类型）
- [ ] trigger_queue_test（≥3 子测试，含 queue-full / worker drain / ctx cancel）

### 集成层
- [ ] `go test ./...` 全绿（不含 pre-existing config 失败）
- [ ] helm template 渲染：analytics-svc Deployment + APISIX 9 个新 ApisixRoute（Stage 30-A 路由组）
- [ ] migrations 001-003 在真 Postgres 上成功 apply
- [ ] Kafka consumer 在 dev kafka 上消费至少 1 条 message.created 事件成功

### 业务层（手动验证）
- [ ] `curl GET /api/v1/reports/daily` → 200 + JSON `DailyReport`
- [ ] `curl GET /api/v1/reports/trend?type=weekly` → 200 + JSON `TrendReport`
- [ ] `curl GET /api/v1/user-behavior/day-night` → 200 + JSON `DayNightPattern`
- [ ] `curl POST /api/v1/mental-health/trigger` → 202 + `{status: "accepted", task_id}`
- [ ] `curl GET /api/v1/mental-health/trend` → 200 + JSON `TrendData`

### 关键边界
- [ ] analytics-svc **不写**其它 schema（testcontainers 测 + search_path 检查）
- [ ] TriggerQueue worker pool ctx cancel 时正常 drain
- [ ] Kafka consumer 在 topic 不存在时不 crash（启动 error 但不影响 HTTP）

---

## 九、不在本次范围（显式 defer）

| 项目 | 原因 | 后续阶段 |
|------|------|---------|
| ClickHouse / StarRocks 替换 Postgres | v1 规模不需要 | Stage-2 触发条件满足时 |
| JWT 鉴权 / rate limit APISIX 插件 | 与 Stage 29-D 同源，独立 PR | Stage 29-E 或独立 |
| Stage 30 web-bff 实现 | 用户目标未包含 | Stage 30 后续 |
| CI 接入（`.github/workflows/`） | AGENTS 既有约束 | 独立 PR |
| materialized view 自动化刷新（pg_cron） | v1 用应用启动时 refresh 即可 | Stage-2 |
| 实时事件聚合（5 秒延迟） | v1 用 batch query 即可 | Stage-2 |
| `frontend` 切换 `API_BASE_URL` 到 analytics-svc | 前端工作量 | 独立 PR |

---

## 十、风险与缓解

| 风险 | 严重度 | 缓解 |
|------|-------|------|
| 跨 schema SQL 误读 | 中 | search_path + 只读 role + testcontainers 验证 |
| Postgres 读压力 | 中 | materialized view + 后续加 Redis 缓存层 |
| TriggerQueue 堆积 | 低 | buffered channel + backpressure（ErrQueueFull） |
| Kafka consumer lag | 低 | v1 容忍秒级滞后；监控 `consumer_lag` |
| VIEW schema drift | 中 | chat-svc / ai-svc 改字段时同步通知 analytics-svc |

### 业界引用

- microservices.io *Database per Service* — <https://microservices.io/patterns/data/database-per-service.html>
- arXiv 2510.20582 (2025) — *Empirical Study on Database Usage in Microservices* — <https://arxiv.org/abs/2510.20582>
- arXiv 2405.11529 (2024) — *Benchmarking DMS for Microservices* — <https://arxiv.org/abs/2405.11529>
- Microsoft Learn — *API gateway vs direct client-to-microservice communication* — <https://learn.microsoft.com/en-us/dotnet/architecture/microservices/architect-microservice-container-applications/direct-client-to-microservice-communication-versus-the-api-gateway-pattern>

---

> 最后更新：2026-08-29 by 当前协作 Agent
> 适用版本：Stage 29-D closure → **Stage 30-A 本 plan 实施** → Stage 30 后续（web-bff）