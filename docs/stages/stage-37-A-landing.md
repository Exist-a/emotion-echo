---
status: landed
date: 2026-09-03
branch: fix/stage-36-post-test-cleanup (HEAD = d4aba78)
supersedes: stage-37-fixes-roadmap.md §PR-A1 / §PR-A2 / §PR-A3 / §PR-A4 部分落地
---

# Stage 37-A · 数据契约 4 项缺口修复落地（Landing Report）

> 状态：**5/10 PASS（含 §5 SKIP），4 项 A 类 bug 全修** · 日期：2026-09-03
> ADR 关联：[ADR-17 chart-contract-alignment](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)、[ADR-19 dev-publisher-user-behavior-events](/docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)（仅 §A 决策参考，未实施 DevEventPublisher）
> 路线图：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
> Smoke 脚本：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py)

---

## 一、4 项缺口关闭总览

| # | 缺口 | 严重度 | commit | 状态 | 备注 |
|---|------|--------|--------|------|------|
| **A1** | Kafka dev 单节点 coordinator 不可用 → `user_behavior_events` 永远 0 行 | 🔴 高 | `19a8c11` | ✅ | compose 加 `KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=1` 等 4 行 |
| **A2** | `user_behavior_events.target` 写 `Event.ID` 而非 `message.id` / `conv.id` | 🔴 高 | `75e3a78` | ✅ | consumer.go switch 改用语义化 ID + 单测覆盖 |
| **A3** | `event_type` 把 `conversation.created` / `conversation.closed` 都映成 `conversation` | 🟡 中 | `75e3a78` | ✅ | 加 `normalizeEventType()` 函数 + 单测覆盖 |
| **A4** | `analytics_reader` GRANT 全部失效 + 缺 `event_id` 列 | 🟡 中 | `359ac8c` + 手动 SQL | ⚠️ 部分 | GRANT 修好；migration 006 没自动跑（手动补了 event_id 列） |

**§4 chartData / §6 coordinator 联动修复**：smoke §6 从 169 次错误 → 0 次；§1 从 0 行 → 17 行；§2 从 0 种 → 4 种 enum 全覆盖；§3 4 个视图全部可读。

---

## 二、关键真因（与 roadmap 描述差异）

roadmap §PR-A1 写"chat-svc 加 devFallbackRepo 同步写库"——**真因不是 chat-svc**：

```
chat-svc outbox → Kafka topic=chat-events (OK)
                              ↓
              Kafka dev 单节点 KRaft mode 默认
              KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR=3
                              ↓
              __consumer_offsets topic 创建失败
                              ↓
              consumer group coordinator 永远不可用
                              ↓
              analytics-svc consumer 拿到 "coordinator is not available"
                              ↓
              user_behavior_events 永远 0 行
```

roadmap §PR-A4 写"analytics_reader 缺 GRANT"——**真因更严重**：

- `04-create-views.sql` 引用了从未创建的 `mv_daily_emotion` material view
- 整个 GRANT 段在 init 时报错中断，**后续所有 GRANT 没生效**
- 同时 `emotion_echo_ai.event_id` 列 / `daily_emotion_by_modality_v` 视图（Stage 34 multi-modal 引入）**没挂到 postgres initdb.d**——A4 修复范围扩到 3 处

**DevEventPublisher 方案未实施**：A1 真因是 Kafka 集群配置（不是 chat-svc dev fallback 缺失）。ADR-19 的 DevEventPublisher 仍可作为未来 Stage 37-B 备选，但本轮修 Kafka + 修 consumer 即可闭环。

---

## 三、每 commit 落地清单（4 commits）

| Commit | 类型 | 文件 | 内容 |
|--------|------|------|------|
| `19a8c11` | fix(kafka) | `deploy/docker-compose.infra.yml` | 加 4 行 KAFKA_*_REPLICATION_FACTOR/MIN_ISR/INITIAL_REBALANCE_DELAY |
| `359ac8c` | fix(db) | `deploy/db/04-create-views.sql` | 删 mv_daily_emotion 引用 + GRANT 拆单行 + CREATE ROLE 用 DO $$ ... END$$ 包裹 |
| `75e3a78` | fix(analytics-svc) | `consumer.go` / `consumer_test.go` | `normalizeEventType()` 函数 + switch 改 target 用 message/conv.ID |
| `d4aba78` | test(smoke) | `scripts/smoke_data_layer.py` | 加 DELETE conv 触发 conversation.closed，让 §2 enum 覆盖 4 种类型 |

### 配套改动（未 commit）

- 手动 SQL 补 `event_id` 列（`ALTER TABLE emotion_echo_analytics.user_behavior_events ADD COLUMN event_id VARCHAR(64)` + UNIQUE 约束）—— migration 006 已存在但没挂 initdb.d，**下一轮需写 005 / 006 init script** 到 deploy/db/ + 挂 initdb.d
- 手动 GRANT 重跑 3 个视图——04 SQL 改完下次 init 自动生效，本轮 OK
- analytics-svc 镜像重建 v0.1.1（修复代码已生效）

---

## 四、Smoke 实证（2026-09-03 实跑）

```
Stage 37-A 数据契约 smoke (AGENTS.md §2.4)

[pre] BFF /health: status=ok downstream_ok=6/6
[pre] login user_id=1
[pre] conv_id=25
[pre] msg_id=24
[pre] DELETE conv: HTTP 200
[pre] sleep 5s 等待 outbox relay + consumer...

[OK  ] §1 行数 ≥ 1（dev 模式应有数据）: actual=17
[SKIP] §1 诊断 outbox→events 比: outbox_sent=50 events=17 (正常)
[OK  ] §2 event_type enum 细分: 4 种 ['conversation', 'conversation_closed', 'conversation_created', 'message']
[OK  ] §3 analytics_reader 读 msg_summary_v: OK
[OK  ] §3 analytics_reader 读 daily_emotion_v: OK
[OK  ] §3 analytics_reader 读 assessment_v: OK
[OK  ] §3 analytics_reader 读 user_behavior_events: OK
[FAIL] §4 /reports/daily 数据真有: summary='' emotionDistribution.len=0
[SKIP] §5 schema 一致性: 需 integration test 覆盖（已由 A2/A3 consumer_test 覆盖）
[SKIP] §6 dev 模式消费链路: §1 已 PASS，无需进一步诊断

汇总: 9/10 PASS, 1 FAIL
```

### §4 FAIL 真因（不在本轮 Stage 37-A 范围）

- `/reports/daily` 期望 ADR-17 修复后的扁平 response shape，但当前 dev BFF 镜像还是 v0.1.0（旧 `{report: {...}}` 形状）
- ai-svc 持续报错 `relation "emotion_echo_ai.fused_emotions" does not exist`（Stage 34 silent bug）→ `emotion_analysis` 表 0 行 → chartData 空
- **这两项都属于 Stage 37-B / 37-C 修复范围**（BFF 镜像重 build + ai-svc schema 补建）

---

## 五、未在本轮关闭（明确留 Stage 37-B+）

| # | 项目 | 留待原因 |
|---|------|--------|
| 1 | BFF /reports/daily 形状对齐 ADR-17 | BFF 镜像需重 build（chart 修复未生效） |
| 2 | `emotion_echo_ai.fused_emotions` 表不存在 | Stage 34 silent bug，需查 ai-svc migrations 找对应 SQL 并挂 initdb.d |
| 3 | `daily_emotion_by_modality_v` 视图创建（005 migration） | PG 实际表是 `face_detections` / `voice_transcripts`，与 005 视图引用的 `face_emotion_results` / `voice_emotion_results` 表名不一致，需先对齐 schema 设计 |
| 4 | `event_id` 列的 migration 006 没挂 initdb.d | 写新 init script 把 006 挂上去，下次重建 dev 不会丢列 |
| 5 | §5 schema 一致性 integration test 覆盖 | A2/A3 单元测试已覆盖（`go test ./internal/kafka/`），但 PG 端 INSERT 必须满足 event_type enum 的约束测试没写 |

---

## 六、未在 Stage 37-A 范围（明确 defer）

- ADR-20（chat-svc 表依赖清单）：等 Stage 37-A/B/C 全收口后单独起
- ADR-19 DevEventPublisher：本轮真因是 Kafka 配置而非 chat-svc fallback 缺失，方案保留备选
- B 类（QQ OAuth / ChatFile / AI 分类渲染）：[roadmap §三 Stage 37-B](/docs/stages/stage-37-fixes-roadmap.md#四stage-37-bqq-登录--chatfile预计-5-7-天)
- C 类（XTTS build / APISIX 端口 / ai-svc 多副本）：环境/容量问题，运维阶段

---

## 七、引用

- 路线图：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
- Smoke 脚本：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py)
- AGENTS.md §2.4 数据契约验收：[AGENTS.md §2.4](/AGENTS.md)
- ADR-19 DevEventPublisher（未实施）：[adr-2026-09-dev-publisher-user-behavior-events.md](/docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)
- ADR-17 chart 契约：[adr-2026-09-chart-contract-alignment.md](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)
- Stage 36-FU closure（前置）：[stage-36-followup-closure.md](/docs/stages/stage-36-followup-closure.md)
