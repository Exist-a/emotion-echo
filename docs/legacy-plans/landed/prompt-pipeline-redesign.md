---
status: landed
priority: high
owner: lane-cl-a（阶段 CL-04）
created: 2026-10-13
landed: 2026-10-14（阶段 CL-04 · PR #213；推进方式与逐点证据见 `docs/closure-roadmap/stages/cl-04-prompt-assembly/{plan,report}.md`）
last-updated: 2026-10-14（**落地归档**：CL-04 收口，本文件自 `docs/plans/` 迁入 `docs/legacy-plans/landed/`；引用旧路径的文档请改指本文件）
type: implementation-plan
source: 2026-10-13 会话（用户："其实这就引出一个最大的问题，目前 prompt 流程。我觉得目前的 prompt 构建流程是有问题的"）
depends-on: []
related-plans:
  - user-custom-agent-persona.md（用户自定义 Agent 是本计划组装层的一个槽位）
  - chat-emotion-realtime-display.md（情绪槽位的生产/消费两侧）
  - fusion-result-consumption.md（其 §2.4「AI 闭环：把融合情绪注入 prompt」= 本计划情绪槽位的一个来源）
---

# Plan — 对话 Prompt 构建流程重构

> **只描述要做的事，不含代码改动。**
> 现状：对话 prompt **完全在 BFF 组装**，只有 `[system, 最后一条 user]` **两条消息**——**多轮对话对 LLM 不可见**（每轮都是无记忆单轮）；无任何 token/长度预算；system prompt 硬编码中文分散在 BFF/llm-service/ai-svc **至少 8 处**；一条"按意图调整风格"的注入是**死路径**；前端另有一条**死掉的** SSE 客户端。本计划重构成**统一、可测、有预算**的组装层。

> **⚠︎ 与 E2E 路线图的关系**：本计划**独立于 E2E 路线图**——**不登记 E2E-F 账本、不归属任何阶段**。

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件）—— 生产调用链

```
[浏览器] chat/new.vue / chat/conversation/[id].vue
  └─ useConversationSender.sendToExistingConversation()/createNewConversation()   composables/useConversationSender.ts:78/:217
     ├─ messageStore.sendMessage()（先落库，写 chat-svc）                           :113
     └─ sendAIStream({message, emotion, conversationId, messageId, clientMsgId,
                      shouldGenerateTitle, voiceEmotion, faceEmotion, faceConfidence}, cb)   :140-152
        └─ useAIStreamHandler.sendAIStream()                                       composables/useAIStreamHandler.ts:88
           body = JSON.stringify(params) → POST {API_BASE_URL}/api/v1/ai/stream   :127-136

[BFF] POST /api/v1/ai/stream
  route 注册                    main.go:556
  └─ AIStreamHandler.ServeHTTP                               internal/handler/ai_stream_handler.go:361
     ├─ json.Decode(&aiStreamReq)（结构 :343-358）            :368
     ├─ userContent = req.Message 或 messages[] 最后一条 user :374-382
     ├─ buildSystemPromptWithEmotion(...)                    :415 / :478
     │    ├─ buildSystemPrompt → baseSystemPrompt(硬编码) + personality.LatestPersonalityProfile→buildPersonalityGuide  :79/:83
     │    └─ buildEmotionContext(face,voice)，空则 RecentEmotionPattern→buildEmotionHistoryContext  :115-119
     ├─ llmMessages = [{system,prompt},{user,userContent}]   :414-422 / HTTP 直连 :477-485   ← **仅两条**
     ├─ collectFileAttachments()（≤2 条 file 消息）           :304-334
     └─ llm.StreamChat(gRPC)                                 :431 / llm_grpc.go:133（组 pb :148-154）

[llm-service] gRPC ChatCompletion                            emotion-llm-service/grpc_server.py:268
  ├─ messages 原样转 dict（不重组）                            :275
  ├─ if request.with_intent: inject_style(messages,intent)   :287-293   ← **生产不触发（死路径）**
  ├─ build_file_context_text(files) → 追加到 user content     :299-318
  └─ iter_chat_chunks → OpenAI 兼容上游 / mock                chat_completion.py:94 / :118
```

### 0.2 核心缺陷清单（逐条 file:line）

| # | 缺陷 | 证据 |
|---|---|---|
| 1 | **对话历史完全不进 prompt**——只组 `[system, 最后一条 user]`，多轮＝无记忆单轮 | `ai_stream_handler.go:414-422`、`:477-485`（`ListMessages` 仅用于挑文件附件 `:304-334`） |
| 2 | **无 token/长度预算**——无消息数/token 截断，`userContent` 无上界 | `ai_stream_handler.go:374-386`；`chat_completion.py:94-149` 不裁剪 messages |
| 3 | **不设 Temperature / MaxTokens**——用 `LLMStreamRequest` 零值，handler 从不赋值 → 上游无输出长度上限 | `ai_stream_handler.go:431-435`；`llm_grpc.go:151-152`；`chat_completion.py:114-117` |
| 4 | **system prompt 硬编码分散 ≥8 处** | BFF `ai_stream_handler.go:61`（base）、`:136/:167`（情绪句式）、`personality_directive.go:33-84`；llm-service `intent.py:53-77`、`file_context.py:34-46`、`main.py:192`、`intent_llm.py:31`；ai-svc `fusion/llm_fuser.go:151`（融合用，非对话） |
| 5 | **意图风格注入是死路径**——`with_intent=True` 才 `inject_style`，BFF 从不设置 | `grpc_server.py:287` vs `llm_grpc.go:148-154`（`chat_handler.sendMessage` 的 `ClassifyIntent` 只把意图存进消息，**不回灌 prompt** `chat_handler.go:158-162`） |
| 6 | **两条并存的前端 SSE 客户端**——`useAIStreamHandler`（生产）vs `useAIStream`（死路径，OpenAI 兼容 body） | `useAIStream.ts:27,50-54` 全仓无生产调用（仅 architecture 测试引用） |
| 7 | **`usePrompt.ts buildRCTPrompt` 是死代码**——硬编码中文 role/context/task，仅单测存活 | `usePrompt.ts:1-36`；`grep buildRCTPrompt` 仅 `usePrompt.test.ts` |
| 8 | **前端发但被静默丢弃的字段**——`messageId / clientMsgId / shouldGenerateTitle` 不在 BFF 结构里；`voiceConfidence` 前端根本没传 | `ai_stream_handler.go:343-358`；`useAIStreamHandler.ts:14-27` |
| 9 | **`file_context.py` 缩进 bug**——`for` 循环体只含 `name`，`append` 在**循环外** → 多附件只输出最后一条 | `file_context.py:189-196` |
| 10 | **人格文案三处人工同步** | `personality_directive.go:1-7` 自述；与前端 `configs/personality.ts`、assessment scoring |
| 11 | **两处等价组装点漂移**——`ServeHTTP` 内 llm-gRPC 与 HTTP 直连两个 `buildSystemPromptWithEmotion` 调用 | `ai_stream_handler.go:415`、`:478` |

### 0.3 相关 ADR / 决策

- `adr-2026-09-personality-prompt-injection.md`：**注入落点选在 BFF 侧**（本计划延续该落点，不迁到 llm-service）；已要求"两条上游路径收敛为同一组装函数"——**已部分落地**，但仍有两处调用点（缺陷 11）。
- `adr-2026-09-personality-prompt-semantics.md`：措辞纪律（相处说明书 ≠ AI 性格）。
- `docs/plans/fusion-result-consumption.md` §2.4：融合情绪注入 prompt 的既有计划（本计划情绪槽位的来源之一）。

### 0.4 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| prompt 组装有单一权威落点 | ⚠️ 落点在 BFF，但**无槽位抽象**、硬编码分散、两处调用点 → 需新建组装层 |
| 有现成的对话历史来源 | ✅ chat-svc `ListMessages`（BFF 已用其挑附件；**取数能力已有，只是没喂给 prompt**） |
| llm-service 只做透传 | ✅ 对话消息原样转（只在 `with_intent`/`files` 时后置改） |
| BFF 请求结构 = 前端实发字段 | ❌ 不一致（缺陷 8） |

---

## §1 问题陈述

一句话：**Prompt 构建散、薄、不可控**——散（硬编码 8 处 + 两处调用点），薄（只喂两条消息、无历史），不可控（无预算、无 Temperature/MaxTokens、死掉的意图注入）。

对"情绪疏导"这个品类，**无历史是最致命的**：情绪疏导天然是**多轮渐进**的（用户倾诉 → 被共情 → 继续深入）。每轮都对 LLM 无记忆，等于"每次都当陌生人重来"，直接伤害核心体验，也让"情绪历史/画像注入"的投入打了折扣。

面试视角：这是把"能聊"升级为"聊得像真的在陪伴"的地基，也是"Prompt 工程"最实的一块（分层 + 槽位 + 预算 + 配置化）。

---

## §2 目标与非目标

### 目标

- **G1 注入对话历史**：按会话取最近 N 轮（user/assistant 成对）进 prompt，受**预算**约束；修复"无记忆单轮"。
- **G2 统一组装层**：**单一模块/函数**负责组装完整 messages（`system` 段拼装 + 历史 + 当前 user），消除多处硬编码与两处等价调用点。
- **G3 上下文预算**：token 估算 + 历史截断策略 + `userContent` 上界 + **设置 Temperature/MaxTokens**。
- **G4 模板/文案配置化**：system prompt 各段外置集中管理（常量/模板文件），收敛"三处人格文案同步"债。
- **G5 收敛死路径**：`useAIStream` / `buildRCTPrompt` 删除或明确标注；**意图风格注入**要么接回（`with_intent` 从 BFF 下发）要么删；修 `file_context.py` 缩进 bug。
- **G6 契约对齐**：统一前后端 `/api/v1/ai/stream` 字段集（丢弃字段要么用起来要么别发）。
- **G7 安全**：历史 + 用户内容进 prompt 时的**注入防御**统一化（复用 `file_context.py` 范式）。

### 非目标

- **不把组装点迁移出 BFF**（ADR 已定落点在 BFF）。
- **不做多模型路由 / 换模型**（只做 prompt 侧）。
- **不改 llm-service 的上游调用实现**（除 `file_context` bug 与意图路径处置）。
- **不做向量检索式长期记忆**（只需"最近 N 轮"；长期记忆是另一议题 `conversation-memory-pending-decision-2026-09-24.md`）。
- 不登记 E2E-F、不挂阶段。

---

## §3 设计

### 3.1 分层组装模型

```
buildMessages(ctx, req) → []LLMMessage
  ├─ system = join([ base人设, 用户自定义Agent段(见 user-custom-agent-persona.md),
  │                  人格适配段(测评), 情绪上下文段, 文件上下文 ])   ← 各段由独立 builder 产出，护栏后置
  ├─ history = 最近 N 轮（chat-svc ListMessages，按预算裁剪）
  └─ user = 当前 userContent（+ 上界校验）
```

- 每段一个纯函数 builder（好测、可单独断言"有/无/顺序"）。
- 组装顺序**固定且可测**：护栏段**永远在用户可控段之后**（防越狱）。

### 3.2 历史注入策略

- 取数：`chat-svc.ListMessages(conversationId, limit, cursor)`（**已有** cursor 能力，见 E2E-F-205/206 相关；注意当前 SQL 排序问题需一并确认）。
- 裁剪：**从最近往回**保留，直到触及预算；保证**至少保留最近 1 轮**（即使超预算也不断当前上下文）。
- 角色：user/assistant 成对；文件类消息附件单独走文件上下文（不塞历史原文）。

### 3.3 预算与参数

- token 估算：字数/字符近似（中文 1 字≈1 token 量级）或引入 token 估算函数；**不追求精确**，只需防爆。
- `MaxTokens`：设一个合理默认（如 512）并可配；`Temperature`：设业务默认（如 0.7）。
- 超预算时的降级链：截断更早历史 → 截断更长的单条 → 记日志。

### 3.4 配置化（G4）

- 所有 system 段文案集中到**一处**（如 BFF `prompt/templates.go` 或外置模板文件），前端 `configs/personality.ts` 与 assessment 的同步改为**单一来源派生**或加**一致性守卫测试**。

### 3.5 死路径处置（G5）

| 死路径 | 处置建议 |
|---|---|
| `useAIStream.ts`（前端死客户端） | **删**（保留 `useAIStreamHandler`） |
| `usePrompt.ts buildRCTPrompt` | **删**（含其 test）或明确 `@deprecated` + 注释 |
| 意图风格注入（`with_intent`） | **二选一**：BFF 下发 `with_intent=true` 接回，或删 llm-service 的 `inject_style` 死分支（§8 D2） |
| `file_context.py` 缩进 bug | **修**（多附件只出最后一条是真 bug） |

---

## §4 分阶段实施（TDD：先断言 RED，再实现 GREEN）

| Phase | 内容 | RED 断言 |
|---|---|---|
| 0 | 基线：抓一次真实 `/api/v1/ai/stream` 的 prompt（日志/抓包），确认"仅两条消息" | — （人工，产证据） |
| 1 | 抽出**统一组装层**（不改变行为：仍只 2 条消息，但走新 builder） | 契约测试：输出**逐字等于**现状（防回归） |
| 2 | 加**历史注入**（N 轮 + 预算） | 断言：多轮会话时 messages 含历史；单轮时不含多余 |
| 3 | 加**预算/参数**（MaxTokens/Temperature/截断） | 断言：超长输入被截；参数被下发到 gRPC pb |
| 4 | **配置化**：文案集中 + 一致性守卫 | 守卫测试：三处文案一致（或单一来源） |
| 5 | 死路径处置 + `file_context` bug | 断言：多附件全输出；删除项不再被引用 |
| 6 | 契约对齐：字段集统一 | 契约测试：BFF 结构 ⊇ 前端实发字段（无静默丢弃） |
| 7 | 回归钉（Playwright）：多轮对话中 AI 体现"记得上文" | spec：第 2 轮引用第 1 轮信息（可经 prompt 日志断言） |

---

## §5 验收与回归钉

- **契约钉（Go）**：组装层输出结构（段顺序、历史条数、护栏位置）；**无历史/无画像时逐字等于旧输出**（防回归）。
- **预算钉**：超长输入被截且不 panic；`MaxTokens/Temperature` 真下发。
- **集成钉**：真实 LLM 下多轮对话体现记忆（人工 + 可脚本化）。
- **负向对照**：故意关闭历史 → 复现"无记忆"（证明测试有效）。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **改动面大** | 触碰对话主链路（评测链路核心） | **Phase 1 先"换实现不改行为"**，后续逐步加能力，每步可回滚 |
| **历史注入放大 token/成本** | 多轮全塞成本陡增、可能超模型窗口 | 预算优先、从最近往回、硬上限 |
| **历史排序** | chat-svc 消息 SQL 排序历史上有问题（取最早 N 条） | 落地前确认排序语义（见 E2E-F-205/206 相关） |
| **护栏被历史/用户内容稀释** | 越狱句式经历史进入 prompt | 护栏后置 + 注入过滤（缺陷 7） |
| **两处调用点漂移复发** | llm-gRPC 与 HTTP 直连 | 组装层统一后被两者共用（缺陷 11 收敛） |
| **删除死路径误伤** | `useAIStream` 可能被测试/未来引用 | 先 grep 全仓确认零生产调用再删；保留历史于 git |
| **前端契约变更** | 统一字段集可能需前端配合 | 新字段 `omitempty`、旧前端可忽略 |

---

## §7 与既有计划的关系

- **`user-custom-agent-persona.md`（本批新增）**：本计划 §3.1 的"用户自定义 Agent 段"由那份定义；**先合流设计**再落地（否则那份会改到被重构的硬编码点）。
- **`chat-emotion-realtime-display.md`（本批新增）**：情绪槽位的消费侧。
- **`fusion-result-consumption.md`（planned）§2.4**：融合情绪注入 = 情绪槽位来源之一；落地时并入本计划组装层。
- **`conversation-memory-pending-decision-2026-09-24.md`（planned）**：**长期记忆**（跨会话/向量），与本计划的"最近 N 轮"不同层；本计划是长期记忆的地基。
- **`frontend-message-cache-offline-resilience.md`（planned）**：历史取数若改用 cursor 分页，与本计划协同。

---

## §8 待拍板

| # | 事项 | 说明与建议 |
|---|---|---|
| D1 | 历史注入轮数 N 与预算口径 | 建议 N=最近 10 轮、字符预算可配 |
| D2 | 意图风格注入：接回 vs 删 | **建议先删 llm-service 死分支**（低风险）；接回作为后续 |
| D3 | `MaxTokens/Temperature` 默认值 | 需产品口径（疏导师回复长短） |
| D4 | 配置化载体 | Go 常量集中 vs 外置模板文件（可热更） |
| D5 | 死路径删除范围 | `useAIStream` / `buildRCTPrompt` 是否本轮一并删 |

---

## §9 调研依据（本次实读，不凭记忆）

- 前端：`composables/{useConversationSender.ts,useAIStreamHandler.ts,useAIStream.ts,usePrompt.ts}`
- BFF：`internal/handler/{ai_stream_handler.go,personality_directive.go,chat_handler.go}`、`internal/downstream/{personality.go,emotion_history.go,llm_grpc.go}`
- llm-service：`grpc_server.py`、`chat_completion.py`、`intent.py`、`file_context.py`
- ai-svc（融合 prompt，非对话）：`internal/fusion/llm_fuser.go`
- ADR：`adr-2026-09-personality-prompt-injection.md`、`adr-2026-09-personality-prompt-semantics.md`
- 测试清单：前端 `usePrompt.test.ts`/`personality.test.ts`/`d14-ai-stream-emotion-fields.architecture.test.ts`；BFF `ai_stream_handler*_test.go`、`personality_directive_test.go`、`d14_emotion_context_test.go`、`f122_emotion_history_fallback_test.go`；llm-service `tests/unit/test_*.py`

**未能确认**（需实机）：当前 dev 运行态 prompt 的真实内容（建议 Phase 0 抓一次）；chat-svc `ListMessages` 的**排序语义**（影响"最近 N 轮"取数正确性）；上游 LLM 的上下文窗口与 `max_tokens` 真实约束。
