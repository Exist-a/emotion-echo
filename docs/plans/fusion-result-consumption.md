---
status: planned
priority: medium
owner: TBD
created: 2026-10-11
type: implementation-plan
source: 2026-10-11 会话（用户："融合结果消费这一块，调用方可以是统计、图表等，产出规划"）
depends-on: []
related-plans:
  - redis-two-layer-cache.md（融合统计的读缓存候选）
---

# Plan — 融合产物消费：让 `fused_emotions` 真正被统计 / 图表 / 消息用起来

> **只描述要做的事，不含代码改动。**
> 现状：多模态融合产物 `fused_emotions` **算了但没人用**——只有一个"按消息查"的端点，前端不消费；按模态的日聚合视图**明确排除**了它。本计划给它接上真正的消费方。

> **⚠︎ 与 E2E 路线图的关系**：本计划**独立于 E2E 路线图**——30 阶段已定稿，**不登记账本、不归属阶段**。

---

## §0 上下文（★ 功课）

### 0.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-ai-svc/internal/model/fused_emotion.go` | `FusedEmotion` 字段：`user_id` / `conversation_id` / `primary_emotion` / `sentiment_score` / `confidence` / `modality_contrib`(JSONB) / `reasoning` / **`fusion_method`（`llm` \| `late_fusion_weighted`）** / `available_modalities`(JSONB) / `created_at` |
| `emotion-echo-ai-svc/migrations/i004_create_fused_emotions.sql` | 表 + 索引 `uq(message_id)`、**`idx_fused_user_time(user_id, created_at DESC)`**、`idx_fused_conv_time` ⇒ **按 user+时间聚合现成高效** |
| `emotion-echo-ai-svc/migrations/i005_create_daily_emotion_by_modality_v.sql` | 按模态视图 UNION 三表（text/face/voice），**注释明确："fused_emotions 不在此 VIEW（fused 是收敛结果，不与单模态并列聚合）"** |
| `emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go:88-95` | 日报**已接** `ModalityReportRepo` → `report.EmotionDistributionByModality`（**按模态=text/face/voice，不含 fused**） |
| `emotion-echo-analytics-svc/internal/repository/modality_report_repository.go` | `GetDailyEmotionByModality` 读 `daily_emotion_by_modality_v` |
| `emotion-echo-web-bff/internal/handler/emotion_query_handler.go` | `GET /api/v1/emotion/message/:id/fused` → gRPC `GetFusedEmotion`（**唯一消费点，前端零调用**） |
| `emotion-echo-web-bff/internal/handler/analytics_view.go`（`toFrontendDailyReport`） | 前端日报字段：`summary / emotionDistribution / conversationCount / messageCount / intentDistribution` —— **无 fused** |
| `proto/metric.proto`（`ReportsDailyResponse`） | `summary / emotion_distribution / emotion_trend / message_count / conversation_count / date / intent_distribution` —— **无 fused 字段** |

### 0.2 结论（缺口）

`fused_emotions`（**收敛后的综合情绪**）**没有任何聚合 / 图表消费方**：
- 按模态的视图**主动排除**了它；
- 日报只有"单模态分布"和"文本情绪分布"，**没有"融合后综合情绪分布"**；
- 唯一的按消息端点，前端不调用。

⇒ 这条最有价值的产物（"综合三模态后的情绪"）**只写不读**。

---

## §1 目标与非目标

### 目标
- **G1** 新增**融合情绪日聚合视图** `daily_emotion_fused_v`（user × day × emotion，含 fusion_method 维度）。
- **G2** **报表消费**：`reports/daily` 输出"**融合情绪分布**"+"**融合方法占比**"（llm vs weighted）；`reports/trend` 输出融合趋势（可选）。
- **G3** **前端图表**：dashboard 增"多模态融合情绪"图（+ 融合方法占比）。
- **G4** **消息级消费**：前端消息气泡展示"融合情绪"标签（消费现有 `/fused` 端点）。
- **G5（可选）** **AI 闭环**：把"最近融合情绪"作为**情绪上下文**注入 prompt，让融合真正影响 AI 回复。

### 非目标
- 不改 FusionWorker 的融合算法（只做消费端）。
- 不改按模态视图（text/face/voice 那条保持不变）。
- 不登记 E2E-F、不挂阶段。

---

## §2 设计

### 2.1 聚合视图 `daily_emotion_fused_v`

```sql
-- 融合情绪按 user × day × 情绪（+ 融合方法）聚合
CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_fused_v AS
SELECT user_id,
       DATE_TRUNC('day', created_at)::date AS day,
       primary_emotion,
       fusion_method,
       COUNT(*)            AS cnt,
       AVG(sentiment_score) AS avg_sentiment,
       AVG(confidence)      AS avg_confidence,
       AVG(CASE WHEN available_modalities::jsonb ? 'face'  THEN 1 ELSE 0 END) AS face_ratio,
       AVG(CASE WHEN available_modalities::jsonb ? 'voice' THEN 1 ELSE 0 END) AS voice_ratio
FROM emotion_echo_ai.fused_emotions
WHERE deleted_at IS NULL
GROUP BY user_id, DATE_TRUNC('day', created_at), primary_emotion, fusion_method;
```
- 走现成索引 `idx_fused_user_time(user_id, created_at DESC)`。
- **`fusion_method` 维度**很关键：能算出"**多少比例是真 LLM 融合、多少是降级加权**"——这正是给面试看的硬指标。
- **`face_ratio/voice_ratio`**：反映"有多少消息真的带上了面部/语音模态"（回答"多模态到底用了多少"）。

### 2.2 报表链路（proto → analytics-svc → BFF → 前端）

| 层 | 变更 |
|---|---|
| proto | `ReportsDailyResponse` 加 `repeated EmotionDistribution fused_emotion_distribution` + `FusionMethodBreakdown fusion_method_breakdown` |
| analytics-svc | 新增 `FusedReportRepo.GetDailyFusedEmotion(...)` 读新视图；`reports_daily_logic` 装配（nil 时跳过，向后兼容） |
| BFF | `toFrontendDailyReport` 加 `fusedEmotionDistribution` / `fusionMethodBreakdown` |
| 前端 | dashboard 加"多模态融合情绪"饼图 + "融合方法占比" |

### 2.3 消息级消费（G4）

- 前端消息气泡：对用户消息调 `GET /api/v1/emotion/message/:id/fused`，有则在气泡角上显示"**综合情绪**"标签（融合结果，区别于单模态）。
- 拿不到（消息还没融合 / 纯文本）→ 不显示，优雅降级。

### 2.4 AI 闭环（G5，可选但价值最高）

- 现状：注入 prompt 的情绪来自 **前端实时 face/voice + 历史**（**不是**融合产物，见 `ai_stream_handler.go`）。
- 变更：`emotionSource` 增加"**最近融合情绪**"来源（读 `daily_emotion_fused_v` 或按会话查 `fused_emotions`），在 face/voice 为空时优先用它 → 让**融合产物真正影响 AI 回复**。
- 收益：补上"融合 → 影响回复"的闭环，面试叙事从"算了没人用"变成"融合结果反哺对话"。

---

## §3 分阶段实施（TDD）

| Phase | 内容 | RED 断言 |
|---|---|---|
| 0 | 确认融合数据基线（跑一条多模态消息 → `fused_emotions` 有行、`fusion_method` 取值） | — |
| 1 | migration：`daily_emotion_fused_v` | 视图存在 + 聚合结果正确（含 fusion_method 分组） |
| 2 | analytics-svc `FusedReportRepo` + 装配 | 日报含 `FusedEmotionDistribution` 非空；repo nil 时字段省略（兼容） |
| 3 | proto + BFF 字段透传 | 契约：daily 响应含 `fusedEmotionDistribution` + `fusionMethodBreakdown` |
| 4 | 前端图表 | 组件契约测试 + 有数据渲染 |
| 5 | 消息级标签（G4） | 前端调 `/fused`、有则显示 |
| 6 | AI 闭环（G5，可选） | prompt 在有融合数据时包含"综合情绪"段 |

---

## §4 验收与回归钉

- **视图单测**（testcontainers/pg 或 in-memory）：聚合正确、按 fusion_method 分组、含/不含 face/voice 比例正确。
- **报表契约钉**：daily 响应含 fused 字段；**无数据时不报错**（空分布）。
- **前端**：图表渲染 + 消息标签（有/无融合两种）。
- **回归**：纯文本消息的融合=文本，图表照常显示（不因缺 face/voice 而空）。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §5 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **跨 schema 读** | analytics-svc 读 ai schema 的视图（已有先例：`daily_emotion_by_modality_v` 同款） | 沿用现有 GRANT/视图模式 |
| **融合有延迟** | worker tick 5s，图表数据"稍晚才有" | 报表按天聚合，秒级延迟无影响；消息级标签"暂无则隐藏" |
| **纯文本时融合≈文本** | 图表意义打折 | 图中**标注"含多模态消息占比"**（用 face_ratio/voice_ratio），说清"多模态价值在有 face/voice 时体现" |
| **`available_modalities` 是 JSONB 字符串** | `?` 操作符需 ::jsonb 转换 | 视图里显式 `::jsonb` |
| **字段新增破坏旧前端** | 前端契约变 | 新字段 `omitempty`，旧前端忽略 |

---

## §6 待拍板

| # | 事项 | 说明 |
|---|---|---|
| D1 | 是否做 trend（融合趋势） | 建议先做 daily（简单、价值已够） |
| D2 | 是否做 G5（AI 闭环） | 建议做——它把"融合算了没人用"变成"融合反哺对话"，面试价值最高 |
| D3 | 消息级标签（G4）放气泡内还是悬浮 | UI 细节，建议气泡角标 |

---

## §7 调研依据（本次实读）

- `emotion-echo-ai-svc/internal/model/fused_emotion.go`、`migrations/i004_create_fused_emotions.sql`、`migrations/i005_create_daily_emotion_by_modality_v.sql`
- `emotion-echo-analytics-svc/internal/logic/reports_daily_logic.go`、`internal/repository/modality_report_repository.go`、`internal/svc/servicecontext.go`
- `emotion-echo-web-bff/internal/handler/{emotion_query_handler.go, analytics_view.go}`
- `proto/metric.proto`（`ReportsDailyResponse`）
- `emotion-echo-web-bff/internal/handler/ai_stream_handler.go`（prompt 情绪来源，确认**不含 fused**）

**未能确认**（需实机）：dev 库里 `fused_emotions` 现有行数与 `fusion_method` 分布；前端图表组件的现有契约（pieChart 入参）。
