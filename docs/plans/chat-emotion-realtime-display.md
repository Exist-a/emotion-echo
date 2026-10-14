---
status: planned
priority: high
owner: TBD
created: 2026-10-13
type: implementation-plan
source: 2026-10-13 会话（用户：面试反馈"闭环程度不够"→ 挑最典型的半环补齐；用户原话："用户沟通的时候渲染一个用户当前状态，放在上面，算是一个回显，用户可以知道自己表现出的情绪"）
depends-on: []
related-plans:
  - fusion-result-consumption.md（融合产物的统计/图表/报表消费；本计划是它的**前端实时回显**补充，见 §7）
  - legacy-plans/landed/prompt-pipeline-redesign.md（已落地）（"实时情绪"也是 prompt 的情绪槽位来源，见 §7）
---

# Plan — 对话页「用户当前情绪」实时回显 + 消息级融合标签

> **只描述要做的事，不含代码改动。**
> 现状：多模态融合产物 `fused_emotions`（三模态收敛后的综合情绪）**算了但用户永远看不到**——BFF 有三个情绪查询端点，前端一处都不调；消息气泡不渲染任何情绪；SSE 流不带情绪，导致数字人拿到的情绪恒为 `undefined`。本计划把这条"情绪 → 回显给用户"的环补上。

> **⚠︎ 与 E2E 路线图的关系**：本计划**独立于 E2E 路线图**——30 阶段已定稿，**不登记 E2E-F 账本、不归属任何阶段**（沿用 `fusion-result-consumption.md` / `observability-business-metrics-wiring.md` 的既定口径）。

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-ai-svc/internal/model/fused_emotion.go:28-45` | `FusedEmotion`：`message_id(UNIQUE)` / `user_id` / `conversation_id` / `primary_emotion` / `sentiment_score` / `confidence` / `modality_contrib`(JSONB) / `reasoning` / `fusion_method`(`llm`\|`late_fusion_weighted`) / `available_modalities`(JSONB) / `created_at` |
| `emotion-echo-ai-svc/migrations/i004_create_fused_emotions.sql` | 表 + `uq(message_id)` + `idx_fused_user_time(user_id, created_at DESC)` + `idx_fused_conv_time`（**按会话取最新现成高效**） |
| `emotion-echo-ai-svc/internal/fusion/worker.go:92-191` | `FusionWorker` 每 **5s** 批次 `ListPending`（有 `emotion_analysis` 无 `fused_emotions` 的消息）→ 融合 → `Upsert`。**非实时**，延迟秒级 |
| `emotion-echo-web-bff/internal/handler/emotion_query_handler.go:34-38` | 端点：`GET /api/v1/emotion/message/:messageId`、`.../message/:messageId/fused`、`.../conversation/:conversationId?limit=`。**三个都活着，前端零调用** |
| `emotion-echo-web-bff/internal/downstream/emotion_query.go:28-80` | `EmotionQueryClient{ByMessage,ByConversation,ByFusedMessage}` → ai-svc gRPC（`GetEmotionByMessage/Conversation/GetFusedEmotion`） |
| `emotion-echo-web/app/pages/chat/conversation/new.vue:120-125`、`[id].vue:219-224` | **现有唯一情绪 UI**：`当前表情识别：<strong>{{currentEmotion.emotion}}</strong>（置信度…）`——只显示**实时人脸**，不是融合结果 |
| `emotion-echo-web/app/composables/useFaceEmotion.ts:199` | 摄像头走 `/multimodal/analyze` 且 `persist:'false'`（**不入库**）；实时 face 只在会被采集时才有值（3s 窗口，权限/设备关闭即空） |
| `emotion-echo-web/app/composables/useAIStreamHandler.ts:31,178,214` | `onFinish(messageId, emotion?)` 第二参**恒 undefined**（`triggerFinish({})`）；`[id].vue:339-340` 用它 `digitalHuman.setEmotion(aiEmotion)` → 数字人表情动画拿不到情绪 |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go`（SSE 输出段） | SSE 只发 `{"choices":[{"delta":{"content":...}}]}` + `[DONE]`，**无 emotion 字段** |
| `emotion-echo-web/app/types/api.ts:185,209`、`stores/message.ts:86`、`useVoiceRecorder.ts:190` | `emotionTag` 全链路类型 + 赋值**已存在**，但 `.vue` 模板 **零渲染**（`grep emotionTag` 于 `.vue` = 0） |
| `emotion-echo-web/app/utils/emotion.ts` | `getEmotionLabel` / `EMOTION_LABEL_MAP` 存在，**只被报表页**（`dashboard/dailyReport.vue`）用，消息渲染未用 |
| `emotion-echo-web/app/components/digital-human/DigitalHuman.client.vue:291-306` | `setEmotion` + `EMOTION_MAP` 已实现，`[id].vue:291,339-340,382-383,398-399` 已调用（但入参恒空） |

### 0.2 相关 ADR / 决策

- `docs/architecture/adr/adr-2026-09-personality-prompt-injection.md`：画像注入落点在 BFF 侧（本计划复用其"增强非依赖、取不到不报错"的纪律）。
- D-14 情绪护栏（`ai_stream_handler.go:99-103`）：**不点破来源 / 不贴标签 / 不过火**——本计划的前端回显**必须沿用同一护栏**（回显给用户看也要"温和、不贴标签"）。
- `docs/e2e-roadmap/discovered-unresolved.md` E2E-F-122：情绪上下文"实时优先、历史回落"的既有语义（本计划数据来源分级与之对齐）。

### 0.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| 融合结果按消息唯一、按会话/用户可高效查最新 | ✅ `fused_emotions` UNIQUE(message_id) + `idx_fused_conv_time` |
| BFF 已有可复用的情绪查询端点 | ✅ 三个端点齐备且已 gRPC 化 |
| 融合是实时产生的 | ❌ **否**——5s 批处理，秒级~秒级+延迟；回显需容忍"暂无" |
| 前端已有情绪渲染的现成组件/样式 | 部分——`EMOTION_LABEL_MAP` 有，气泡标签**无**，顶部状态条**无** |
| 纯文本消息也有融合结果 | ✅ 但 `fusion_method` 可能 = `late_fusion_weighted`（仅文本时），需在 UI 上诚实区分"含多模态" vs "仅文本" |

### 0.4 未跑 smoke 声明

本文档为**规划期静态核实**（读代码 + 查 ADR），**未启动 dev 栈跑 §2.4 数据契约 smoke**（AGENTS §2.4 触发场景为"改契约的 PR"；本文件不含代码改动）。落地方案中已把"基线确认"列入 Phase 0，需实机执行。

---

## §1 问题陈述

一句话：**用户在整个对话过程中，看不到"系统识别到的自己此刻的情绪"**——融合情绪算了只写不读，实时人脸只在页面上以小字显示，且**数字人表情动画实际拿不到任何情绪**（`onFinish` 第二参恒 `undefined`）。

逐条（现状）：

1. **融合结果不可见**：`fused_emotions` 唯一消费者是 BFF `/fused` 端点，前端零调用（`grep "/emotion"` 于 `emotion-echo-web/app` = 0）。
2. **消息无情绪标记**：`emotionTag` 类型与赋值链路已存在，但没有任何模板渲染它。
3. **数字人表情空转**：`digitalHuman.setEmotion(aiEmotion)` 的 `aiEmotion` 恒 `undefined`（SSE 不回传、`onFinish` 不填）。
4. **回显只是"实时人脸小字"**：`new.vue`/`[id].vue` 的"当前表情识别"是**单模态**（face），不是融合；且设备关闭/权限拒绝就彻底消失。

⇒ 这条环断在"**处理结果 → 回到用户**"最后一跳。补上它，面试叙事从"融合算了没人用"变成"融合结果实时回显 + 反哺回复"。

---

## §2 目标与非目标

### 目标

- **G1 顶部「当前情绪」状态条**：对话页顶部常驻一块，显示**当前会话最新融合情绪**（`primary_emotion` + 温和中文文案 + 置信度可选）；无融合数据时优雅隐藏。
- **G2 消息级融合标签**：用户消息气泡角标显示其**融合情绪**（消费 `/fused`）；有 `fusion_method` 时以极轻量方式区分"综合（含多模态）"与"文本"。
- **G3 数据来源分级**：最新按钮优先级 = **本会话最新 fused** > **实时 face/voice（前端已有）** > **不显示**（与 E2E-F-122 的"实时优先、历史回落"精神一致，但这里主展示目标是**融合收敛结果**）。
- **G4 打通数字人情绪**：把真实情绪补进 `onFinish` 的第二参（SSE 回传融合情绪或 AI 回复情绪），使 `digitalHuman.setEmotion` 拿到真值。
- **G5 护栏一致**：顶部回显文案沿用 D-14 三句护栏（不点破来源 / 不贴标签 / 不过火），措辞中性。

### 非目标

- **不改融合算法 / 不改 FusionWorker 频率**（只做消费端与展示）。
- **不做报表/聚合消费**（融合情绪的日报分布、趋势图、融合方法占比——那是 `fusion-result-consumption.md` §2.1/§2.2 的范围）。
- **不做 AI 回复的情绪分类**（AI 回复落库带不带 emotion 是后端契约问题，见 `fusion-result-consumption.md` 与 `../legacy-plans/landed/prompt-pipeline-redesign.md`；本计划只在 G4 里消费"已有/可算"的情绪）。
- 不登记 E2E-F、不挂阶段。

---

## §3 设计

### 3.1 数据来源与取数

| 展示位 | 数据来源 | 端点/调用 | 缺数据时 |
|---|---|---|---|
| 顶部情绪状态条 | 本会话**最新**融合情绪 | 新增 BFF 便捷读：`GET /api/v1/emotion/conversation/:id?limit=1`（复用现端点）或新增 `/conversation/:id/latest`（避免拉 50 条） | 隐藏整块 |
| 消息气泡角标 | 该消息的融合情绪 | `GET /api/v1/emotion/message/:messageId/fused` | 不显示角标 |
| 数字人表情 | 融合情绪（优先）/ 实时 face | 同顶部来源；或 SSE 增量回传 | 不调用 `setEmotion`（保持默认） |

> **取数时机**：发送用户消息后，AI 流式回复**开始前**先乐观渲染"识别中"，流式**结束后**（或 5s 融合窗口内轮询 1~2 次）拉一次融合结果回填。避免"每帧轮询"。

### 3.2 组件与落点

- **新组件** `components/emotion/CurrentEmotionBar.vue`（顶部状态条）：
  - props：`emotion` / `confidence` / `fusionMethod` / `availableModalities` / `loading`；
  - 文案：`EMOTION_LABEL_MAP`（复用 `utils/emotion.ts`）+ 中性句式（如"此刻的状态：低落"）；
  - `fusionMethod==='late_fusion_weighted'` 且无 face/voice → 退化为"文本情绪"，避免夸大"多模态"。
- **消息气泡角标**：在 `[id].vue` 的消息循环内对 `role==='user'` 渲染一个小 chip（复用 `emotionTag` 已有类型，避免新增类型）。
- **数字人**：把 `onFinish` 第二参接真（见 3.3）。
- 挂载点：`new.vue` 与 `[id].vue` 顶部（两页共用组件）。

### 3.3 数字人情绪打通（G4）

二选一（§8 待拍板）：

- **方案 A（轻）**：前端在 AI 流结束后，**并发**拉 `messageId` 的 fused → 调 `setEmotion`。零后端改动，但多一次请求、延迟融合窗口。
- **方案 B（重，但更"闭环"）**：SSE 结尾事件里带一条 `{"emotion": {...}}`（BFF 在流末尾把该轮融合/识别情绪附上）→ 前端 `onFinish(messageId, emotion)` 直接拿到。契约更干净，但改 SSE 契约 + 前端解析。

> 建议先 A（最小可用），把 B 列为后续（与 `fusion-result-consumption.md` §2.4 的"AI 闭环"合流讨论）。

### 3.4 护栏文案规范（G5）

顶部/标签文案一律**陈述化、中性、不贴情绪标签**：✅"此刻的状态：低落"；❌"你现在很抑郁/你被检测到 sad"。不得出现"识别/检测/摄像头/分析"等来源词（沿用 D-14 护栏）。

---

## §4 分阶段实施（TDD：先断言 RED，再实现 GREEN）

| Phase | 内容 | RED 断言 |
|---|---|---|
| 0 | **基线确认**（实机）：跑一条多模态 + 一条纯文本消息 → 查 `fused_emotions` 有行、`fusion_method` 取值、延迟量级 | — （人工，产证据） |
| 1 | BFF：确认/新增"会话最新融合情绪"读路径（优先复用 `/conversation/:id?limit=1`） | Go 契约测试：给定会话返回最新一条 fused；无则空 |
| 2 | 前端取数 composable（`useCurrentEmotion`：拉取 + 降级 + 护栏文案） | Vitest：有数据渲染情绪、无数据显示隐藏态、纯文本不标"多模态" |
| 3 | `CurrentEmotionBar.vue` 顶部状态条 | 组件契约测试（props → 文案） |
| 4 | 消息气泡角标（消费 `/fused`，复用 `emotionTag`） | Vitest：user 消息有 fused 显示 chip、无则不显示 |
| 5 | 数字人情绪打通（方案 A） | `useAIStreamHandler` 测试：`onFinish` 第二参被赋真值 → `setEmotion` 收到 |
| 6 | 回归钉（Playwright）：对话页顶部状态条 + 角标可见性 | spec：发消息→出现状态条→文本消息不误标多模态 |

---

## §5 验收与回归钉

- **前端契约钉（Vitest）**：`useCurrentEmotion` / 组件级别，覆盖"有融合/无融合/纯文本/接口 5xx"，断言**失败不阻塞主对话**（情绪回显是增强不是依赖，与画像注入同纪律）。
- **Playwright 回归钉**：对话页顶部状态条渲染（需 dev 栈：`--profile dev --env-file .env.local`）+ 纯文本消息不标"多模态"的负向对照。
- **数字人**：`setEmotion` 入参非 `undefined`（可断言函数被以有效情绪调用）。
- **人工验收**：发一条会触发多模态的消息，顶部/气泡能看到情绪且措辞温和、不贴标签。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **融合延迟** | FusionWorker 5s 批次，发消息后不会"立刻"有融合结果 | 乐观点"识别中" + 流结束后拉 1~2 次；拿不到就隐藏，不硬等 |
| **纯文本被夸大** | 只有文本时 `fusion_method=late_fusion_weighted`，展示成"多模态"是夸大 | 用 `available_modalities` 判断是否含 face/voice，文案降级为"文本情绪" |
| **5s 窗口过期** | 实时 face 是 3s 窗口（`useFaceEmotion`），错过即空 | 主展示走**融合结果**（不依赖实时窗口），实时仅作回退 |
| **端到端取数放大请求** | 消息多时逐条拉 `/fused` 会 N+1 | 只对**可见的 user 消息**按需拉，或做会话级批量端点（§8 D3） |
| **数字人方案 B 改契约** | 改 SSE 契约影响面大 | 先做方案 A；B 与 AI 闭环一起评估 |
| **护栏失守** | 回显可能让用户感觉"被监视" | D-14 三句护栏写进组件文案与测试断言 |
| **前端无 emotion 路由登记** | `apiRoutes.ts` 无 `/emotion` 条目（孤儿守卫不覆盖） | 新增调用时同步登记路由（对齐 `apiRoutes.test.ts`） |

---

## §7 与既有计划的关系

- **`fusion-result-consumption.md`（planned）**：那份管**后端聚合/报表/AI 闭环**（`daily_emotion_fused_v`、日报融合分布、注入 prompt）。本计划管**前端实时回显 UI**。**重叠点**：那份的 G4「消息级标签」= 本计划的 G2——**以本计划为准实现一次**，落地时同步该文件状态避免两处各做。**不重复**：报表/图表归那份。
- **`../legacy-plans/landed/prompt-pipeline-redesign.md`（本批新增）**：本计划产出的"当前融合情绪"正是那份重构后 prompt 的情绪槽位来源之一（情绪注入的消费者与生产者对齐）。
- **`frontend-message-cache-offline-resilience.md`（planned）**：消息渲染层若同时大改，需协调挂载点（都动 `[id].vue`）。

---

## §8 待拍板

| # | 事项 | 说明与建议 |
|---|---|---|
| D1 | 数字人情绪打通方案 | A（前端并发拉，最小）vs B（SSE 回传，更闭环）。**建议先 A** |
| D2 | 顶部状态条展示字段 | 是否显示置信度/模态来源。**建议只显示情绪文案**（最不"监视感"） |
| D3 | 消息级取数方式 | 逐条 `/fused`（简单）vs 会话批量端点（省请求）。**建议先逐条、可见才拉** |
| D4 | 与 `fusion-result-consumption.md` 的边界 | 确认"UI 回显归本计划、报表归那份"，避免重复实现消息标签 |

---

## §9 调研依据（本次实读，不凭记忆）

- `emotion-echo-ai-svc/internal/model/fused_emotion.go`、`internal/fusion/worker.go`、`migrations/i004_create_fused_emotions.sql`
- `emotion-echo-web-bff/internal/handler/emotion_query_handler.go`、`internal/downstream/emotion_query.go`
- `emotion-echo-web/app/pages/chat/conversation/{new.vue,[id].vue}`、`composables/{useFaceEmotion.ts,useAIStreamHandler.ts,useVoiceRecorder.ts}`、`utils/emotion.ts`、`types/api.ts`、`stores/message.ts`、`components/digital-human/DigitalHuman.client.vue`
- `docs/architecture/adr/adr-2026-09-personality-prompt-injection.md`、`docs/e2e-roadmap/discovered-unresolved.md`（E2E-F-122）
- 与 `docs/plans/fusion-result-consumption.md` 对账（避免重复）

**未能确认**（需实机）：dev 库 `fused_emotions` 现有行数与 `fusion_method` 分布；顶部状态条与现有页头布局的视觉冲突（需 IAB 实看）；`EMOTION_LABEL_MAP` 是否覆盖 fused 的 `primary_emotion` 全部取值。
