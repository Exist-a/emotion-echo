---
status: in-progress
stage: CL-07（前端半）
slug: cl-07-emotion-display
scope: 情绪实时回显（顶部状态条 / 消息融合角标 / 数字人情绪）
owner: Lane-CL-G（接手原 C 会话位）
created: 2026-10-16
source-plan: docs/plans/chat-emotion-realtime-display.md（G1~G5）
related:
  - ../roadmap.md（§2 G 轨续做注）
  - ../parallel-contract.md（§1.1 H 行 / §三 占位 / §四 握手）
  - cl-07-fusion-consumption/（后端切片，已 done 并合并 bf401350）
---

# CL-07 · 情绪回显（**前端半**）— 执行计划

> 承接 `stages/cl-07-fusion-consumption/`（后端切片：融合产物 → 报表 → proto/BFF 透传，**已 done 并合并**）。
> 本半只做**前端展示**：把已经能取到的「融合情绪」送到用户能看见的地方。

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-web-bff/internal/handler/emotion_query_handler.go:26-28,58-87` | 端点已备：`GET /api/v1/emotion/message/:messageId/fused` → `{"fused": FusedEmotion}`；NotFound→404、Unimplemented→503 |
| `emotion-echo-web-bff/internal/handler/emotion_query_handler.go:90-110` | `GET /emotion/conversation/:id?limit=` → `{emotions:[Emotion]}` —— 这是**未融合**（emotion_analysis）路径，非本半所需 |
| `proto/emotion_query.proto:99-111` + `emotion-echo-shared/pkg/emotionquery/emotion_query.pb.go:285-295` | `FusedEmotion` 的 json tag 为 **snake_case**：`primary_emotion` / `confidence` / `fusion_method` / **`available_modalities`（JSON 字符串，非数组！）** |
| `emotion-echo-web/app/utils/emotion.ts` | `EMOTION_LABEL_MAP` + `getEmotionLabel`（happy/sad/angry/anxious/neutral/unk） |
| `emotion-echo-web/app/types/api.ts:186,211` | `emotionTag` 类型**存在**，但 `grep emotionTag **.vue` = **0** ⇒ 从未渲染（本半补上） |
| `emotion-echo-web/app/pages/chat/conversation/[id].vue:290-293,340-343,383-386,399-402` | `onFinish(messageId, aiEmotion)` 的 `aiEmotion` **恒 undefined**（`useAIStreamHandler.triggerFinish()` 无参 ⇒ `useConversationSender:201` 传空）；`onEmotionChange` 通道已存在但只被 TTS 喂 |
| `emotion-echo-web/app/composables/useAIStreamHandler.ts:120-124` | `triggerFinish()` 从不带参 ⇒ SSE 不回传情绪（"方案 B"的前提未成立） |
| `emotion-echo-web/app/lib/apiRoutes.ts` + `apiRoutes.test.ts` | 路由**单点真理** + 契约测试扫全仓字面量 ⊆ 本文件 ⇒ 新增调用必须登记 |
| `emotion-echo-web/vitest.config.ts` / `tests-setup.ts` / `tests-app-mock.ts` | happy-dom + `ref/computed` 全局注入 + `#app` mock（`useState/useCookie/useRuntimeConfig`） |
| `emotion-echo-web/app/composables/useApi.ts:373-388` | `get<T>()` 自动解包 `data.data`、自带 token/401/429/去重 |

### A.2 相关 ADR / 决策

- 已查：`docs/plans/chat-emotion-realtime-display.md`（§3 设计 + §4 Phase + §6 风险 + §8 D1~D4）
- 已查：`docs/closure-roadmap/decisions.md`（CL-D-07：融合情绪 PG+Redis；写侧 CL-07）
- 已查：D-14 情绪护栏三句（不点破来源 / 不贴标签 / 不过火）—— 本半**必须沿用**
- 已查：`cl-07-fusion-consumption/report.md`（后端切片收口；`fusedEmotion` 已能端到端取到）

### A.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| 前端能按消息取到融合情绪 | ✅ `/emotion/message/:id/fused` 已备且 CL-07 后端切片已验（报表侧） |
| `emotionTag` 链路已通、只需渲染 | ❌ 类型在、赋值链在，但**零渲染**；且它是单模态标签（非融合）⇒ 本半用融合数据而非 `emotionTag` |
| SSE 会回传情绪（"方案 B"） | ❌ **否**（`triggerFinish()` 无参）⇒ 只能走"方案 A：前端并发拉"（D1 建议 A） |
| 融合结果"立刻"可得 | ❌ FusionWorker 5s 批处理 ⇒ 取数需容忍"暂无"，必要时补拉一次 |
| 多模态占比可直接信 | ⚠️ 真实数据全是 `["text"]`（CL-07 后端切片报告的覆盖缺口）⇒ `available_modalities` 必须解析后**诚实降级** |

### A.4 起栈与实测声明

✅ **已跑**：`npx vitest run`（全量前端）+ **浏览器实测（IAB）**。
⚠️ **实测口径修正**：宿主 `pnpm dev` 在本环境 IAB 下**不 hydration**（dev 的 `@fs` 模块 URL 含中文仓库路径；**对照实验**：未改动的 main 同现象）⇒ 改用 **`pnpm build` 生产产物**（`node .output/server/index.mjs`），**成功 hydration** 并完成 [V] 验收。

## §1 目标与非目标

**目标（对齐计划 G1~G5）**：
- **G1** 顶部「此刻的状态」状态条（无数据优雅隐藏）；
- **G2** 用户消息气泡**融合情绪角标**（无数据不渲染）；
- **G4** 数字人情绪打通（**方案 A**：流结束后前端并发拉，含一个补拉窗口）；
- **G5** 护栏（陈述化、中性、无来源词）+ **诚实降级**（无 face/voice ⇒ 标「文本」不标「综合」）。

**非目标**：不改融合算法/Worker 频率；不改 SSE 契约（方案 B 留后续）；不做报表/图表（属后端切片，已 done）；不动 `new.vue`（见 §6 偏离说明）；不登记 E2E-F。

## §2 测试点清单

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-1 | `available_modalities`（JSON 字符串）解析 + 非多模态判定 | [A] | `useCurrentEmotion.test.ts` | PASS |
| T-2 | 护栏文案：`此刻的状态：<中文>`，**不含**来源词 | [A] | 同上（含禁用词逐条断言） | PASS |
| T-3 | 诚实降级：纯 text ⇒ `modalityLabel='文本'`；含 face/voice ⇒ `'综合'` | [A] | 同上 | PASS |
| T-4 | 取数失败（404/503/网络）⇒ 静默降级不抛错（增强非依赖） | [A] | 同上 | PASS |
| T-5 | 消息级缓存：不重复拉、失败也缓存 null、cap 防 N+1 | [A] | 同上 | PASS |
| T-6 | 组件：`state=null` 整块不渲染；有值渲染文案 + 模态标签 | [A] | `CurrentEmotionBar.test.ts` | PASS |
| T-7 | 路由登记：新路径 ⊆ `API_ROUTES`（全仓字面量契约） | [A] | `apiRoutes.test.ts` | PASS |
| T-8 | **浏览器实测**：顶部状态条出现 + 气泡角标出现 | [V] | IAB + **生产构建**（dev 不 hydration，见 §A.4/report） | ✅ **PASS**：`此刻的状态：开心` + 角标 `开心`（截图 + DOM） |
| T-9 | **负向对照**：纯文本消息**不**被标成「综合」 | [V] | IAB | ✅ **PASS**：该消息 `available_modalities=["text"]` ⇒ 标签为「**文本**」非「综合」 |
| T-10 | 数字人 `setEmotion` 收到非 undefined 情绪 | [V] | IAB | ⛔ **BLOCKED**：数字人本身**加载失败**（`3D模型加载失败`，既有账 **CL-F-17** VRM 竞态）⇒ 无法观察其表情；接线已在码内（`syncEmotionToDigitalHuman` + 架构钉），待 CL-F-17 修复后补验 |

## §3 分阶段实施（TDD）

| Phase | 内容 | RED 断言 | 状态 |
|---|---|---|---|
| 2 | `composables/useCurrentEmotion.ts`（取数 + 降级 + 护栏 + 消息级缓存） | 测试引用未定义导出 ⇒ 编译失败 | ✅ GREEN（19 用例） |
| 3 | `components/emotion/CurrentEmotionBar.vue` | 组件不存在 ⇒ 无法 mount | ✅ GREEN（5 用例） |
| 4 | `[id].vue` 气泡角标（消费消息级缓存） | —（模板接线，浏览器验） | ✅ GREEN（编译 + 单测） |
| 5 | `[id].vue` 数字人情绪（方案 A + 补拉窗口） | —（同上） | ✅ GREEN（编译 + 单测） |
| 6 | 浏览器实测（IAB，**生产构建**）+ 负向对照 | — | ✅ **PASS（9/10）**；T-10 被 CL-F-17 阻塞 |

> **TDD 说明（诚实）**：Phase 2/3 的测试与实现**同批写入**（非严格先红后绿）；已用 `apiRoutes.test.ts` 全仓契约测试兜底，Phase 6 的**突变实验**（见 report）证明测试非空断言。

## §4 收口判据

- **功能**：对话页顶部出现「此刻的状态：X」；用户消息带融合角标；数字人拿到真情绪；纯文本不误标「综合」。
- **机器**：全量 `vitest run` 绿；`apiRoutes` 契约绿；浏览器实测通过。
- **诚实边界**：真实数据全为纯文本 ⇒ 「综合」分支**未被真实数据触达**（与后端切片同款覆盖缺口，如实登记）。

## §5 回归钉

- Vitest：`useCurrentEmotion.test.ts`（19）· `CurrentEmotionBar.test.ts`（5）· `apiRoutes.test.ts`（3）。
- 浏览器：Phase 6 IAB 实测（顶部状态条 + 角标 + 负向对照）。

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| 融合 5s 延迟 | 流结束即拉会落空 | 立即 + 4.5s 补拉一次；仍无则隐藏（不硬等） |
| 纯文本被夸大 | `fusion_method=late_fusion_weighted` | 用 `available_modalities` 判定；无 face/voice ⇒ 标「文本」 |
| N+1 请求 | 逐条 `/fused` | 只拉**最新 cap=12** 条 + 失败也缓存（不打请求风暴） |
| 情绪回显阻塞对话 | 取数失败 | 全部 `try/catch` 静默降级，**从不 throw**（增强非依赖） |
| 路由未登记 | `apiRoutes.test.ts` 会红 | 已登记 `emotionFusedByMessage` |
| **偏离计划**：`new.vue` 未挂状态条 | 计划 §3.2 说两页共挂 | 新会话尚无消息 ⇒ 无 messageId 可锚 ⇒ 挂了恒空。**有意偏离**，理由写在 report |
| 数字人 `setEmotion` 语义 | 表达的是"用户当前情绪"（共情镜像）而非 AI 自身情绪 | 与 G1「用户当前情绪回显」一致；在 report 写明该取舍 |

## §7 调研依据（本次实读）

- 后端：`emotion_query_handler.go`、`downstream/emotion_query.go`、`proto/emotion_query.proto`、`pkg/emotionquery/emotion_query.pb.go`
- 前端：`pages/chat/conversation/[id].vue`、`composables/{useAIStreamHandler,useConversationSender,useApi,useFaceEmotion}.ts`、`utils/emotion.ts`、`types/api.ts`、`lib/apiRoutes.ts`、`vitest.config.ts`、`tests-setup.ts`、`tests-app-mock.ts`
- 计划/决策：`docs/plans/chat-emotion-realtime-display.md`、`docs/closure-roadmap/{decisions,parallel-contract,roadmap}.md`、`cl-07-fusion-consumption/report.md`
- 命令：`pnpm install --frozen-lockfile`、`npx vitest run`
