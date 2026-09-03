---
status: planned
superseded-by: 尚未实施；汇总 ADR-17 + Stage 36-FU §四 + docs/plans/ 中 8 项未落地业务/数据问题
date: 2026-09-03
branch: fix/stage-36-post-test-cleanup (bb7ebef)
---

# Stage 37 · 业务与数据缺口修复路线图（Fixes Roadmap）

> 状态：**待审批（Pending Approval）** · 日期：2026-09-03 · 目标分支：`fix/stage-36-post-test-cleanup`（沿用）
> ADR 关联：[ADR-17 chart-contract-alignment](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)、[ADR-18 incremental-rpc-adoption](/docs/architecture/adr/adr-2026-09-incremental-rpc-adoption.md)、[ADR-19 dev-publisher-user-behavior-events](/docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)
> 来源：盘点 ADR-17 §"stage-37 路线图" + Stage 36-FU §四 backlog + `docs/plans/` 5 个仍未落地计划

---

## 〇、第一性原则

按 [AGENTS.md](/AGENTS.md) §〇，**每一项修复都先 RED 后 GREEN**：
1. 先写失败测试描述"代码应该做什么"
2. 写最小实现让测试转绿
3. 重构保持绿

禁止提交"实现 + 后补测试"或"只测 happy path"。

---

## 一、目标

把以下 **8 项已盘点、未实施**的业务/数据问题一次性收口，**业务功能优先于架构扩展**：

### A 类（数据/权限 bug，影响 dev 与生产数据完整性）

| # | 位置 | 症状 | 严重度 |
|---|------|------|--------|
| **A1** | `emotion-echo-chat-svc/internal/kafka/...`（KAFKA_ENABLED=false 分支） | dev 模式 `user_behavior_events` 表空白，4 个 dashboard 没数据 | 🔴 高 |
| **A2** | chat-svc 写 `user_behavior_events.target` 的代码 | 写的是 `Event.UUID` 而非 `message_id`，下游 `WHERE target_id=?` 关联全部失效 | 🔴 高 |
| **A3** | chat-svc 写 `user_behavior_events.event_type` 的代码 | `conversation.created` / `closed` 都映成 `event_type='conversation'`，细分丢失 | 🟡 中 |
| **A4** | `deploy/db/04-create-views.sql` 或等价 GRANT 脚本 | `analytics_reader` role 缺 `daily_emotion_by_modality_v` SELECT 权限 | 🟡 中 |

### B 类（业务功能，前端用户可感知）

| # | 位置 | 症状 | 严重度 |
|---|------|------|--------|
| **B1** | `emotion-echo-user-svc/internal/handler/oauth_*.go` | QQ OAuth 完全没做，前端无 QQ 登录入口 | 🟡 中 |
| **B2** | `Emotion-Echo-Web/app/components/chat/ChatFile.vue` + 消息 store | `handleAttachment` 未实现、`MessageItem.contentType` 只支持 `text/audio/img`、附件按钮点了没反应 | 🔴 高 |
| **B3** | `Emotion-Echo-Web/app/pages/chat/...` AI 回复渲染 + intent_classifier | `marked` 已装但前端不会按 intent 类型用不同渲染模板 | 🟡 中 |
| **B4** | chat-svc intent classifier | 当前只产出 `emotional_support` / `other` 2 类，6 类扩展在 backlog | 🟢 低（依赖 B3） |

### C 类（环境/容量，非代码 bug，留 backlog）

| # | 描述 |
|---|------|
| **C1** | XTTS v0.1.7 镜像重 build（dev 环境 pypi CDN + 内存限制） |
| **C2** | APISIX Admin API 端口暴露 |
| **C3** | ai-svc 多副本 + Kafka consumer group 调优 |

**不在 Stage 37 范围**：ADR-19（go-zero/APISIX 状态合并）属于文档一致性，单独排期。

---

## 二、批次划分（基于"业务优先 > 数据次优 > 架构扩展最后"原则）

| 批次 | 范围 | 目标 | 预计工作量 | TDD 节奏 |
|------|------|------|------------|----------|
| **Stage 37-A** 数据 bug | A1 + A2 + A3 + A4 | dev 全链路数据完整 + analytics_reader 权限齐全 | 2-3 天 | 4 bug 各 RED→GREEN（共 ~10 commit） |
| **Stage 37-B** 业务功能 | B1 + B2 | QQ 登录 + 前端 ChatFile 组件收口 | 5-7 天 | B1（后端 3 commit）→ B2（前端 4-5 commit） |
| **Stage 37-C** AI 渲染 | B3 + B4 | intent 6 类扩展 + 前端分类渲染 | 3-5 天 | B4 先扩分类（2 commit）→ B3 前端渲染（3-4 commit） |

每批次单独 landing doc 收口。

---

## 三、Stage 37-A（数据 bug，预计 2-3 天）

### PR-A1：chat-svc DevEventPublisher 同步消费事件写入 user_behavior_events（A1）

> ⚠️ **roadmap 修订（2026-09-03）**：初版写"chat-svc 加 `devFallbackRepo *EventRepo` 同步写库"——经实际代码勘察（[events/events.go](/emotion-echo-chat-svc/internal/events/events.go)、[outbox/relay.go](/emotion-echo-chat-svc/internal/outbox/relay.go)），chat-svc 并**不直接写** `user_behavior_events`，它走 outbox + Kafka + analytics-svc consumer 链路。修复方案必须匹配这套架构，否则会引入跨服务职责并破坏事件溯源设计。

**架构现状（写代码前必读）**：

```
chat-svc handler
   ↓ InsertOutbox（status=pending）
outbox_events（PG 表）
   ↓ OutboxRelay.FlushOnce 每 1s 扫描
   ↓ EventPublisher.Publish
   ├─ KAFKA_ENABLED=true  → KafkaEventPublisher（sarama）→ Kafka topic=chat-events
   └─ KAFKA_ENABLED=false → publisher=nil，relay publishOne 永远返回 errors.New("publisher is nil")，outbox 行 status 永远 pending
                              ↓
                          analytics-svc Kafka Consumer
                              ↓ Insert user_behavior_events
                          analytics-svc consumer 单测齐全，main.go 在 Kafka.Enabled && evtRepo != nil 时启动
```

**真实问题**：`KAFKA_ENABLED=false` 时 outbox relay 启动但 publisher 为 nil，relay 每秒都失败一次（`relay.go:106-108`），outbox_events 行堆积，`user_behavior_events` 永远为空。dev 模式 docker compose 启动后 4 个 dashboard 看不到任何数据。

**修复方案**：在 `emotion-echo-chat-svc/internal/events/` 加 `DevEventPublisher`，实现 `EventPublisher` 接口，**同步**把事件写 `user_behavior_events`。`main.go` 在 `Kafka.Enabled == false` 时把 `KafkaEventPublisher` 替换为 `DevEventPublisher`。

**为什么这个方案不破坏单一职责**：
- `DevEventPublisher` 仍然实现 `EventPublisher` 接口，对 outbox relay 完全透明——relay 不知道下游是 Kafka 还是 DB
- `DevEventPublisher` 把"事件→user_behavior_events 行"的**映射**集中在一处，未来 A2（target 写 message_id）+ A3（event_type 细分）改这里就覆盖生产链路
- 写库逻辑复用 analytics-svc consumer 同一份 SQL（迁移到 shared 包或 chat-svc 内嵌 SQL 子集），保持数据契约一致

**代价（必须 ADR 化）**：
- chat-svc 知道 `user_behavior_events` 表结构 = 跨服务职责
- 但 `DevEventPublisher` 只在 `KAFKA_ENABLED=false` 时启用，prod 不会命中
- 类比 `InMemoryEventPublisher` 也是测试替身跨了"测试断言"的职责——`DevEventPublisher` 是"dev 模式"的同位概念

**TDD 节奏**：
```
PR-A1.1: RED  DevEventPublisher 不存在
         （testcontainers Postgres + chat-svc binary 启 KAFKA_ENABLED=false，
          发 message.created → 断言 user_behavior_events 1 行 + outbox 行 status=sent）
PR-A1.2: GREEN chat-svc events.DevEventPublisher 同步写 user_behavior_events
         main.go 在 KAFKA_ENABLED=false 时构造 DevEventPublisher 替代 nil
PR-A1.3: REFACTOR 抽 EventConsumerBackport 接口
         提取 message.created/conversation.created/closed → EventRow 的映射函数
         后续 A2/A3 修复全部改这一处
```

**验收**：
- `KAFKA_ENABLED=false` 启 chat-svc → 发 10 条 message → `user_behavior_events` 增 10 行
- `outbox_events` 对应 10 行 status=sent（relay 走通）
- `KAFKA_ENABLED=true` 路径无回归（沿用 sarama）

---

### PR-A2：user_behavior_events.target 写 message_id（A2）

**问题**：当前写 `Event.UUID`（事件自身 ID）而非 `message_id`（消息 ID），下游所有 `WHERE target_id = message.id` 关联失效。

**修复方案**：
- chat-svc 写库前明确区分 `event_id`（unique constraint 列）和 `target_id`（语义化外键列）
- `message.created` → `target_id = message.id`、`target_type = 'message'`
- `conversation.created` → `target_id = conversation.id`、`target_type = 'conversation'`

**TDD 节奏**：
```
PR-A2.1: RED  target_id 单元测试（写一个 message.created event → 断言 target_id == msg.id）
PR-A2.2: GREEN 改 chat-svc 写库逻辑
PR-A2.3: REFACTOR 统一 EventRecord 结构体
```

**验收**：查询 `user_behavior_events ue JOIN messages m ON ue.target_id = m.id` 行数 > 0。

---

### PR-A3：conversation.created/closed event_type 细分（A3）

**问题**：当前两个事件都映成 `event_type='conversation'`，下游无法区分"用户开始对话" vs "用户结束对话"。

**修复方案**：按 [stage-30-A §303](/docs/stages/stage-30-A-analytics-business.md) 已定义的 event_type literal union：
- `conversation.created` → `event_type='conversation_created'`
- `conversation.closed` → `event_type='conversation_closed'`
- `message.created` → `event_type='message'`

**TDD 节奏**：
```
PR-A3.1: RED  enum literal union 测试（事件类型断言）
PR-A3.2: GREEN 改 chat-svc 映射逻辑
```

**验收**：`SELECT event_type, COUNT(*) FROM user_behavior_events GROUP BY 1` 出现 3 行（message/conversation_created/conversation_closed）。

---

### PR-A4：analytics_reader GRANT daily_emotion_by_modality_v（A4）

**问题**：`analytics_reader` role 没有 `daily_emotion_by_modality_v` 视图的 SELECT 权限，Stage 34 引入视图后 analytics-svc 实际从未成功读该视图（permission denied silently）。

**修复方案**：`deploy/db/04-create-views.sql` 末尾追加 `GRANT SELECT ON daily_emotion_by_modality_v TO analytics_reader;`，同时给前面已存在的 `daily_emotion_distribution_v` 等视图补齐 GRANT。

**TDD 节奏**：
```
PR-A4.1: RED  集成测试用 analytics_reader role 连库 → 查 daily_emotion_by_modality_v 报 permission denied
PR-A4.2: GREEN GRANT + 同步 .sql
PR-A4.3: REFACTOR 提取所有 view GRANT 到统一脚本
```

**验收**：`psql -U analytics_reader -c "SELECT * FROM daily_emotion_by_modality_v LIMIT 1"` 成功。

---

### Stage 37-A 收口

`docs/stage-37-A-landing.md`：A1+A2+A3+A4 全 ✅ + smoke（dev 模式下 4 dashboard 真实渲染数据 + analytics_reader 视图可读）。

---

## 四、Stage 37-B（QQ 登录 + ChatFile，预计 5-7 天）

### PR-B1：QQ OAuth 全链路（B1）

**问题**：微信 OAuth 已完成（代码 + 路由 + 数据模型 + config），QQ OAuth 完全没做。

**修复方案**：参考 `emotion-echo-user-svc/internal/service/oauth_wechat_api.go`，新增 `oauth_qq_api.go` + handler + 路由 + config。

**关键路由**：
```
GET  /api/v1/auth/oauth/qq/url
POST /api/v1/auth/oauth/qq/login
```

**TDD 节奏**：
```
PR-B1.1: RED  QQ OAuth URL 生成 + callback exchange 测试（mock HTTP）
PR-B1.2: GREEN QQ OAuth 完整实现
PR-B1.3: REFACTOR 抽 OAuthProvider 接口统一微信/QQ
```

**验收**：前端登录页有 QQ 按钮，点击跳到 QQ 授权页，回调后正确写入 `User.QQOpenID`。

---

### PR-B2：前端 ChatFile 组件收口（B2）

**问题**：[`file-upload-message-extension.md`](/docs/plans/file-upload-message-extension.md) 与 [`wechat-qq-login-and-upload.md`](/docs/plans/wechat-qq-login-and-upload.md) 都标记"ChatFile 组件在 backlog 未收口"。具体：
- `handleAttachment` 函数未实现
- `MessageItem.contentType` 只支持 `'text' | 'audio' | 'img'`
- 消息 store `sendMessage` 只支持纯文本

**修复方案**：
- `Emotion-Echo-Web/app/types/api.ts` 扩 `ContentType` 加 `'file' | 'video'`
- `ChatFile.vue` 实现 `handleAttachment` + 进度条 + 错误处理
- 消息 store 扩 `sendFile` action
- 上传复用现有 `/upload/file` 端点（已存在，Stage 19+）

**TDD 节奏**（前端 Vitest）：
```
PR-B2.1: RED  ChatFile.handleAttachment mock test（点击 → 调 /upload/file）
PR-B2.2: GREEN 完整 handleAttachment + 上传进度
PR-B2.3: RED  消息 store sendFile（file → message type='file'）
PR-B2.4: GREEN sendFile 完整
PR-B2.5: REFACTOR 抽 useUpload composable
```

**验收**：前端聊天页附件按钮可上传任意文件，消息列表显示文件名 + 下载链接。

---

### Stage 37-B 收口

`docs/stage-37-B-landing.md`：B1+B2 全 ✅ + 前端 Playwright e2e（QQ 登录 + 文件上传）。

---

## 五、Stage 37-C（AI 分类渲染，预计 3-5 天）

### PR-C1：intent 分类扩展到 6 类（B4）

**问题**：chat-svc 当前只产出 2 类（emotional_support + other），6 类扩展在 backlog。如果 PR-C2 要按类型渲染不同模板，必须先扩分类。

**修复方案**：参考 [`intent-classification-6-types.md`](/docs/plans/intent-classification-6-types.md)：
- 新分类：`emotional_support / study_help / tech_help / career_help / lifestyle / other`
- 改 prompt（prompt template 文件）+ few-shot examples
- emotion-echo-ai-svc gRPC `ClassifyIntent` 返回新 enum

**TDD 节奏**：
```
PR-C1.1: RED  prompt template 单元测试（输入样本 → 期望 6 类之一）
PR-C1.2: GREEN 改 prompt + few-shot
PR-C1.3: REFACTOR 抽出 intent classifier 独立包
```

**验收**：50 条样本消息测试集，6 类召回率 ≥ 85%。

---

### PR-C2：前端 AI 回复分类渲染（B3）

**问题**：[`ai-response-structured.md`](/docs/plans/ai-response-structured.md) 标记"前端 marked 已装但分类渲染未做"。当前前端所有 AI 回复都用同一渲染模板。

**修复方案**：
- BFF 把 `intent_type` 加到 `/api/v1/ai/stream` SSE 事件
- 前端根据 `intent_type` 选模板：
  - `study_help` → 卡片 + 步骤列表
  - `tech_help` → 代码块高亮 + 折叠
  - `emotional_support` → 现有共情流（保持）
  - `lifestyle` → 列表 + emoji
  - `career_help` / `other` → 默认 Markdown
- 模板组件：`app/components/chat/IntentRenderer/{Study,Tech,Career,Lifestyle}.vue`

**TDD 节奏**（前端 Vitest）：
```
PR-C2.1: RED  IntentRenderer 路由测试（intent_type → 组件）
PR-C2.2: GREEN 6 个渲染组件
PR-C2.3: RED  BFF SSE 加 intent_type 字段契约测试
PR-C2.4: GREEN BFF 透传 intent_type
```

**验收**：6 类各 1 个样本消息，前端渲染样式符合 §B 设计。

---

### Stage 37-C 收口

`docs/stage-37-C-landing.md`：B3+B4 全 ✅ + 前端 Playwright e2e（6 类意图各 1 条对话验证渲染）。

---

## 六、Stage 37 整体验收

| 项 | 标准 |
|---|------|
| 8 项业务/数据问题 | 全部 ✅ |
| 所有 PR | RED→GREEN→REFACTOR 三段 commit |
| smoke 验证 | 每个批次完成后立即跑 |
| landing doc | 每个批次单独收口 + Stage 37 总结 |
| architecture-decisions.md | 不动（ADR-19 单独排期） |
| branch ahead of main | 累计 commit +15~25（视 PR 拆解粒度） |

---

## 七、不在 Stage 37 范围（明确 defer）

| 项目 | 理由 | 排期 |
|------|------|------|
| XTTS 镜像重 build（C1） | 环境问题，需要生产网络 | 后续运维阶段 |
| APISIX Admin API 端口暴露（C2） | 单点运维问题 | 后续运维阶段 |
| ai-svc 多副本 + Kafka consumer group 调优（C3） | 需 A1 完整跑通后评估 | Stage 38 |
| ADR-19（go-zero/APISIX 状态合并） | 文档一致性 | Stage 38 之前任何空闲时段 |
| docs/plans/ 中 `three-vrm-usage-reference.md` | 持续维护的工具手册，不是新功能 | N/A |
| K8s manifests 完善、Helm charts、CI/CD、DB migration tool | Stage 33 deferred 列表 | 评估期 |

---

## 八、参考资料

- ADR-17：[adr-2026-09-chart-contract-alignment.md](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md) §"stage-37 路线图"
- ADR-18：[adr-2026-09-incremental-rpc-adoption.md](/docs/architecture/adr/adr-2026-09-incremental-rpc-adoption.md) §"BFF→chat-svc 工作量最大排 stage-38"
- ADR-19：[adr-2026-09-dev-publisher-user-behavior-events.md](/docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)（roadmap PR-A1 修订）
- Stage 36-FU：[stage-36-followup-closure.md](/docs/stages/stage-36-followup-closure.md) §四
- docs/plans/[file-upload-message-extension.md](/docs/plans/file-upload-message-extension.md)
- docs/plans/[wechat-qq-login-and-upload.md](/docs/plans/wechat-qq-login-and-upload.md)
- docs/plans/[ai-response-structured.md](/docs/plans/ai-response-structured.md)
- docs/plans/[intent-classification-6-types.md](/docs/plans/intent-classification-6-types.md)
- AGENTS.md：[AGENTS.md](/AGENTS.md) §〇 TDD 第一性原则
- Stage 35 smoke：[stage-35-smoke-validation.md](/docs/stages/stage-35-smoke-validation.md)
- architecture-decisions：[architecture/decisions.md](/docs/architecture/decisions.md)
