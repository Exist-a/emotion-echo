---
status: landed
date: 2026-09-03
branch: fix/stage-36-post-test-cleanup (HEAD = 704b51d + BFF/ai-svc v0.1.1 image)
supersedes: stage-37-fixes-roadmap.md §四 Stage 37-B QQ OAuth + ChatFile 部分（推到 Stage 38）；本轮实际是 Stage 37-B "§4 chartData 翻绿"
---

# Stage 37-B · /reports/daily 全链路数据贯通（Landing Report）

> 状态：**10/10 PASS · 0 FAIL** · 日期：2026-09-03
> ADR 关联：[ADR-17 chart-contract-alignment](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)
> 前置：[stage-37-A-landing.md](/docs/stages/stage-37-A-landing.md)
> Smoke：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py)

---

## 一、目标

**让 §4 /reports/daily 翻绿**——ADR-17 chart 契约修复后，BFF /reports/daily 返回扁平 `{summary, emotionDistribution, ...}` shape，summary 非空 + chartData 有真实情绪数据。dev compose up 后前端 dashboard 能真正显示数据。

---

## 二、§4 FAIL 真因（与 Stage 37-A landing 推测不同）

stage-37-A-landing.md §四 推测 §4 FAIL 是 2 个独立问题：
1. BFF 镜像 v0.1.0 没生效 ADR-17 chart 修复
2. ai-svc 缺 `fused_emotions` 表

实测发现**第 3 个隐藏问题**：

3. **ai-svc FusionWorker 的 `PostgresFusedEmotionRepo.ListPending` SQL 永远返 0 候选**——原实现只 `SELECT FROM fused_emotions`，而表刚建为空 → fusion worker 永远 candidates=0 → fused_emotions 永远不增 → /reports/daily 永远拿不到情绪数据

这是 Stage 34 PR-13/14 留下的 TODO（注释里写"Worker 在 PR-13/14 接入 emotion_analysis 反查找"有 text 但还没 fused"的更复杂逻辑"），**该 PR 实际未实施**。

---

## 三、修复清单

| # | 改动 | 文件 | 内容 |
|---|------|------|------|
| 1 | 手动 SQL | `emotion-echo-ai-svc/migrations/004_create_fused_emotions.sql` | docker exec 跑（migration 没挂 initdb.d，下次重建会丢——待 Stage 38 修 init script 挂载） |
| 2 | ListPending SQL | `emotion-echo-ai-svc/internal/repository/fused_emotion_repository.go` | LEFT JOIN emotion_analysis 返"有 text 但还没 fused"的候选 |
| 3 | ai-svc 镜像 | 重 build v0.1.1 + 重启容器 | 让 ListPending 修复生效 |
| 4 | BFF 镜像 | 重 build v0.1.1 + 重启容器 | 让 ADR-17 chart 修复（`summary` + `emotionDistribution` 字段）生效 |

### commit 落地

| Commit | 类型 | 内容 |
|--------|------|------|
| `704b51d` | fix(ai-svc) | ListPending 返 LEFT JOIN emotion_analysis 识别未 fused 候选 |

未 commit 改动（无源码 diff）：
- ai-svc / BFF 镜像重 build（代码无 diff，只是镜像 tag 升级）
- 手动 SQL（migration 没挂 initdb.d，dev compose up 不会自动跑）

---

## 四、最终 smoke 实证（2026-09-03）

```
Stage 37-A 数据契约 smoke (AGENTS.md §2.4)

[OK  ] §1 行数 ≥ 1: actual=25
[SKIP] §1 诊断 outbox→events 比: outbox_sent=58 events=25 (正常)
[OK  ] §2 event_type enum 细分: 4 种 ['conversation', 'conversation_closed', 'conversation_created', 'message']
[OK  ] §3 analytics_reader 读 msg_summary_v: OK
[OK  ] §3 analytics_reader 读 daily_emotion_v: OK
[OK  ] §3 analytics_reader 读 assessment_v: OK
[OK  ] §3 analytics_reader 读 user_behavior_events: OK
[OK  ] §4 /reports/daily 数据真有: summary='2026-09-03，你共有 5 段对话，24 条消息。主要情绪是 平静（11 次），整体心境 平稳。今天继续和 Echo 聊聊吧。' emotionDistribution.len=1
[SKIP] §5 schema 一致性
[SKIP] §6 dev 模式消费链路

汇总: 10/10 PASS, 0 FAIL
[OK] 全 PASS — Stage 37-A 数据契约全部满足
```

### 业务数据全链路打通

```
user_behavior_events |  28
emotion_analysis     |  12
fused_emotions       |  12
messages             |  24
outbox_events (sent) |  61
```

chat-svc outbox → Kafka → analytics-svc + ai-svc consumer → emotion_analysis → FusionWorker → fused_emotions → BFF → /reports/daily → 中文 summary + 情绪饼图，全部通路。

---

## 五、未在本轮关闭（明确留 Stage 37-C+ 或 38）

| # | 项目 | 留待原因 |
|---|------|--------|
| 1 | `emotion_echo_ai.fused_emotions` 表 migration 004 没挂 initdb.d | 重建 dev 环境会丢表，需写 init script 挂载 |
| 2 | 005 migration (`daily_emotion_by_modality_v`) 视图依赖 `face_emotion_results` / `voice_emotion_results`，但实际表是 `face_detections` / `voice_transcripts` | schema 改名不一致，需单独对齐 |
| 3 | 002/003 同上未挂 + 表名不一致 | 同 #2 |
| 4 | ADR-20（chat-svc 表依赖清单） | dev-only 跨服务职责积累到 3+ 时起 |
| 5 | `daily_emotion_by_modality_v` 视图当前 dev 模式不创建，§3 GRANT 没测 | 视图需要先建才能 GRANT |
| 6 | §5 schema 一致性 integration test | dev 环境无 testcontainers，单测覆盖够用 |

---

## 六、Stage 37-B 原计划（QQ OAuth + ChatFile）状态

[stage-37-fixes-roadmap.md §四 Stage 37-B](/docs/stages/stage-37-fixes-roadmap.md#四stage-37-bqq-登录--chatfile预计-5-7-天) 原计划是 QQ OAuth + 前端 ChatFile 组件。但本轮 §4 FAIL 更紧迫（用户目标：docker compose 跑通），所以本 landing doc **替代了原 Stage 37-B**——QQ OAuth + ChatFile 推到 Stage 38。

---

## 七、引用

- 前置：[stage-37-A-landing.md](/docs/stages/stage-37-A-landing.md)
- 路线图：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
- Smoke 脚本：[scripts/smoke_data_layer.py](/scripts/smoke_data_layer.py)
- AGENTS.md §2.4：[AGENTS.md §2.4](/AGENTS.md)
- ADR-17：[adr-2026-09-chart-contract-alignment.md](/docs/architecture/adr/adr-2026-09-chart-contract-alignment.md)
- ADR-19（DevEventPublisher，未实施）：[adr-2026-09-dev-publisher-user-behavior-events.md](/docs/architecture/adr/adr-2026-09-dev-publisher-user-behavior-events.md)
