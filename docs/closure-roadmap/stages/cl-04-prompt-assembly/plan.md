---
stage: cl-04
title: Prompt 组装层（统一组装 + 同会话历史滑动窗口 + 预算/参数 + 配置化 + 死路径清零）
type: transformation
status: in-progress
created: 2026-10-14
last-updated: 2026-10-14（A 轨：Phase 0~6 已落地并运行时验证（14/16 测试点 PASS）；余 T-14 IAB 人可见钉。新增 [M] T-17 = 基础人设去长度约束（用户裁定，实测见 report §3.12））
depends-on: []
blocks: [cl-06, cl-07]
gate: []            # 无开工前阻塞决策门；[M] 决策点见 §2 T-16
source: docs/legacy-plans/landed/prompt-pipeline-redesign.md（已落地）
related-plans:
  - docs/plans/user-custom-agent-persona.md（CL-06，注入槽位落在本阶段组装层）
  - docs/plans/chat-emotion-realtime-display.md（CL-07，情绪槽位生产/消费两侧）
  - docs/plans/fusion-result-consumption.md（§2.4 融合情绪注入 = 情绪槽位来源之一）
---

# CL-04 · Prompt 组装层 — 执行计划

> **核心口径（roadmap §4 CL-04）**：**"AI 记得你上一句说了什么"** —— 从无记忆单轮 → 有上下文的连续对话。
> **现状一句话**：对话 prompt **完全在 BFF 组装，且只有 `[system, 最后一条 user]` 两条消息** ——
> 多轮对话对 LLM 不可见；无 token 预算；system 文案硬编码分散 ≥8 处；意图风格注入是死路径；
> 前端还有一条死掉的 SSE 客户端。
>
> **本阶段是 T2 对话地基的第一块**，解锁 CL-06（自定义 Agent 人设槽位）与 CL-07（情绪回显注入）。
>
> **执行协议**：[RUNBOOK.md](../../RUNBOOK.md)（六步循环 / 证据有效性 / 收口契约）+ [parallel-contract.md](../../parallel-contract.md)（A 轨占位与文件面隔离）+ [anti-patterns.md](../../../e2e-roadmap/anti-patterns.md)。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件，本次实读）

| 文件 | 关键事实（file:line） |
|---|---|
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go` | 请求结构 `aiStreamReq` 在 `:343-358`；**两处等价组装点**：gRPC 路径 `:414-422`、HTTP 直连路径 `:477-485`，**两处都只组 `[system, user]` 两条**；`buildSystemPromptWithEmotion` `:108-125`（基础人设 `:61` + 人格段 + 情绪段）；`buildEmotionContext` `:156-168`；`collectFileAttachments` `:304-334`（`ListMessages(...,50,0)`）；`h.llm.StreamChat` 调用 `:431-441` **未设 Temperature/MaxTokens**（`LLMStreamRequest` 零值） |
| `emotion-echo-web-bff/internal/downstream/llm_grpc.go` | `LLMStreamRequest` 有 `Temperature/MaxTokens` 字段（`:38-45`）但调用方从不赋值；pb 组装 `:148-154` **未设 `WithIntent`**（proto 有这个字段）→ 意图注入死路径的 BFF 侧根因 |
| `emotion-echo-web-bff/internal/handler/personality_directive.go` | 人格适配指令文案硬编码（第二处 system 文案来源） |
| `emotion-echo-chat-svc/internal/repository/conversation_repository.go` | `ListMessagesPage` `:350-372`：`id DESC` 取 `limit+1` 探测 hasMore，**最后反转为时间升序**（最新在末尾）；内存版同语义 `:153-175` |
| `emotion-echo-chat-svc/internal/logic/listmessageslogic.go` | 会话归属鉴权 `:36-45`；`limit<=0 → 50` `:47-50`；响应视图含 `Role/Content/ContentType/Intent`（`Intent` 已在库，`:66`） |
| `emotion-echo-web-bff/internal/downstream/chat.go` | `MessageView` `:49-63`（含 `ContentType` / `Intent` / `FileName`）；`ListMessages` 接口注释 `:93-95`（**返回最新 limit 条 + hasMore**） |
| `emotion-llm-service/grpc_server.py` | `ChatCompletion` `:268`；messages 原样转 dict `:275`；`with_intent` 分支 `:287-293`（`inject_style`）；附件注入改在 **user 消息尾部** `:299-318`（Stage 90 实测 PDF 拒读后的位置改造） |
| `emotion-llm-service/intent.py` | 6 类意图关键词 `:24-50` + 风格指令 `:53-77`；`inject_style` `:113-124`（已有 system 就地扩写，否则前置） |
| `emotion-llm-service/file_context.py` | **缩进 bug 确认** `:189-196`：`for` 循环体只含 `name=` 与 `fetch_and_extract(...)` 两行，`if text is not None:` 及其 `sections.append` **在循环外**（缩进 4 空格）⇒ **多附件只输出最后一条**，且 `i/name` 泄漏为循环末值 |
| `emotion-echo-web/app/composables/useAIStreamHandler.ts` | 生产 SSE 客户端；参数接口 `:14-27`（含 `messageId/clientMsgId/shouldGenerateTitle/voiceConfidence`）；`body = JSON.stringify(params)` `:133` |
| `emotion-echo-web/app/composables/useConversationSender.ts` | 实发字段 `:140-152`：`message/emotion/conversationId/messageId/clientMsgId/shouldGenerateTitle/voiceEmotion/faceEmotion/faceConfidence` —— **`voiceConfidence` 确实没传** |
| `emotion-echo-web/app/composables/useAIStream.ts` | 死路径 SSE 客户端（OpenAI 兼容 body，`/ai/stream` 之外的另一条）；全仓生产零调用，**仅** `r09-ai-stream-auth-header.architecture.test.ts` 引其源码做断言 |
| `emotion-echo-web/app/composables/usePrompt.ts` | `buildRCTPrompt` `:1-36`（硬编码中文 role/context/task）；**仅 `usePrompt.test.ts` 引用** |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler_llmgrpc_test.go` | 现成 `fakeLLMStreamer`（`gotMsgs/gotModel/gotFiles`）+ 上游优先级契约测试范式 —— 本阶段 RED 直接复用扩展 |

**本次新发现（原计划未记）**：`ai_stream_handler.go` 的 `aiStreamReq` 里 **`Emotion` / `Model` / `Stream` 三个字段被解析但全文件从未读取**（`grep req.Emotion|req.Model|req.Stream` 零命中）—— 前端**发了 `emotion` 但 BFF 根本没用**。属缺陷 8（静默丢弃）的**同一族但更深**：不是"结构里没有"，而是"结构里有、值被丢"。一并进 T-13 处置。

### A.2 相关 ADR / 决策（已查）

- 已查 `docs/architecture/adr/adr-2026-09-personality-prompt-injection.md`：**注入落点选在 BFF 侧**（本阶段延续，不迁 llm-service）；其中"两条上游路径收敛为同一组装函数"**已部分落地仍有两处调用点**（缺陷 11）。
- 已查 `docs/architecture/adr/adr-2026-09-personality-prompt-semantics.md`：措辞纪律（相处说明书 ≠ AI 性格）—— 配置化重排文案时**不得改动措辞语义**。
- 已查 `docs/closure-roadmap/roadmap.md` §4 CL-04 已定参数（用户 2026-10-13，按推荐落地）：
  - 滑动窗口：**默认最近 10 条消息（≈5 轮），上限 20 条（≈10 轮）**，可配；**以 token 预算为硬顶**（条数只是粗筛）。
  - 死路径：`with_intent` **接回**（BFF 下发 `with_intent=true`）；`useAIStream` / `buildRCTPrompt` **删除**。
  - 配置载体：**Go 常量集中**（不引外置模板；将来需热更再局部替换）。
- 已查 `docs/closure-roadmap/decisions.md`：CL-D-01（同会话历史落 CL-04 / 跨会话 RAG 落 CL-08）、CL-D-06（排期顺序）均**已定**，本阶段无新增决策门。
- 已查 `docs/closure-roadmap/parallel-contract.md`：A 轨主文件面（BFF handler/downstream + llm-service + 前端 composables）；**无 spine 强制握手项**（本轮不碰 `proto/**`；`go.mod` 无新依赖；`package.json` 无新依赖 —— 删文件不减不增依赖）。

### A.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| 历史取数能力已具备、只差喂进 prompt | ✅ `ListMessages(convID, limit, cursor)` 端到端已有（proto `cursor` + `ListMessagesPage`），且**返回时间升序**，可直接按序拼历史 |
| 历史排序语义正确（不会喂反） | ✅ 已读 `conversation_repository.go:350-372` 确认升序；⚠️ 但 `ai_stream_handler.go:318` 的注释写"msgs 按时间倒序（最新在前）"**与实现不符**（陈旧注释），本阶段顺带纠正（不改行为） |
| llm-service 只做透传 | ✅ `grpc_server.py:275` 原样转；仅在 `with_intent` / `files` 时后置改写 |
| 人格/情绪段注入是"增强非依赖"（取不到回落基础人设） | ✅ `buildSystemPrompt` `:79-93` / `buildSystemPromptWithEmotion` `:116-124` 均已实现该纪律，重构必须保持 |
| 前端请求字段与 BFF 结构一致 | ❌ 不一致且**两个方向都有**：前端发的 `messageId/clientMsgId/shouldGenerateTitle` BFF 结构里没有；前端发的 `emotion/model/stream` BFF 有字段但不读；前端**没发** `voiceConfidence` |
| 删除死路径是纯删除 | ❌ `useAIStream.ts` 被 `r09-ai-stream-auth-header.architecture.test.ts` 引源码做断言 ⇒ **必须同步改该架构测试**，否则删完 vitest 红 |

### A.4 未跑 smoke / 未取运行时证据声明（诚实性，按 RUNBOOK §4）

**本次建档只做静态核实**：未启动 dev 栈（19 容器）、**未抓取一次真实 `/api/v1/ai/stream` 的 prompt**、未跑 §2.4 数据契约 smoke。
⇒ 因此 **Phase 0（基线取证）尚未执行**，本文件中"现状仅两条消息"的结论依据是**源码**（`:414-422` / `:477-485`）而非运行时观测。Phase 0 完成后须回填真实抓包证据；若与源码结论冲突，以运行时为准并回改本文。

---

## §1 目标与非目标

**目标**（对齐 roadmap §4 CL-04 验收 A~E）

- **G1 注入同会话历史**：按会话取最近 N 条进 prompt，修复"无记忆单轮"。
- **G2 统一组装层**：单一函数组装完整 messages，**消除两处等价调用点**（缺陷 11）与多处硬编码。
- **G3 预算与参数**：token 估算 + 历史从最近往回裁剪（至少保 1 轮）+ `userContent` 上界 + **真下发 Temperature/MaxTokens**。
- **G4 配置化**：各 system 段文案集中到**一处 Go 常量**，并加一致性守卫（人格文案三处同步债）。
- **G5 死路径清零**：删 `useAIStream.ts` / `usePrompt.ts`（含其单测 + 架构测试同步）；**接回** `with_intent`；修 `file_context.py` 缩进 bug。
- **G6 契约对齐**：统一前后端 `/api/v1/ai/stream` 字段集，静默丢弃字段**要么用起来要么别发**。

**非目标**

- 不把组装点迁出 BFF（ADR 已定落点）。
- 不做多模型路由 / 换模型。
- 不做向量检索式长期记忆（跨会话记忆 = CL-08）。
- 不改 llm-service 上游调用实现（`chat_completion.py` 内部不动，除上述死路径/bug）。
- 不引入新依赖（无外置模板引擎、无 tokenizer 库）。

---

## §2 测试点清单（16 点，逐点独立编号）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|---|---|---|---|
| T-1 | **零回归**：无画像 / 无历史 / 无情绪时，组装层输出的 system 串与旧 `buildSystemPromptWithEmotion` **逐字相等** | [A] | Go 契约测试（同包内新旧对比） | 待执行 |
| T-2 | **段顺序固定**：system 段拼接顺序 = 基础人设 → 人格 → 情绪（护栏类段恒在用户可控段**之后**） | [A] | Go 单测断言段序 | 待执行 |
| T-3 | **历史注入生效**：多轮会话时 messages 含历史 user/assistant，且**时间正序**（最早在前、当前 user 在末） | [A] | Go 单测（fake 历史源）+ fakeLLMStreamer 断言 `gotMsgs` | 待执行 |
| T-4 | **单轮不多喂**：无 `conversationId` / 历史为空时 messages 长度仍为 2（不产生空 role 消息） | [A] | Go 单测 | 待执行 |
| T-5 | **预算裁剪**：超预算历史按"从最近往回"裁剪，且**至少保留最近 1 轮**（不断当前上下文） | [A] | Go 表驱动单测（超长历史 / 恰好边界 / 空） | 待执行 |
| T-6 | **userContent 上界**：超长输入被截断且不 panic；截断点不切裂 UTF-8 | [A] | Go 单测（rune 安全） | 待执行 |
| T-7 | **参数真下发**：`Temperature` / `MaxTokens` 出现在 gRPC 请求（非零值） | [A] | 扩展 `fakeLLMStreamer` 断言 `req.Temperature/MaxTokens` | 待执行 |
| T-8 | **两处调用点收敛**：gRPC 路径与 HTTP 直连路径**共用同一组装函数**（不再各自内联） | [A] | 源码断言（architecture 测试：`grep` 组装函数调用点 = 1）+ 两路径行为测试同绿 | 待执行 |
| T-9 | **配置化 + 一致性**：system 段文案集中于单一 Go 常量文件；人格文案三处（BFF 常量 / 前端 `configs/personality.ts` / assessment）一致性守卫 | [A] | 守卫测试（vitest 或 Go） | 待执行 |
| T-10 | **file_context 多附件全输出**：N 个附件 ⇒ N 个 section（修 `file_context.py:189-196` 缩进 bug） | [A] | pytest（llm-service），含负向对照 | 待执行 |
| T-11 | **意图注入接回**：BFF 下发 `with_intent=true`，llm-service 注入风格指令，首帧回带非空 `intent` | [A] | Go 断言 pb `WithIntent` + pytest 断言首帧 intent | 待执行 |
| T-12 | **死路径清零**：删 `useAIStream.ts` / `usePrompt.ts` 后全仓零引用（含同步修改的架构测试绿） | [A] | `grep` 零命中 + vitest 全绿 | 待执行 |
| T-13 | **契约对齐**：BFF 请求结构 ⊇ 前端实发字段；`messageId/clientMsgId/shouldGenerateTitle` 与 `emotion/model/stream`、`voiceConfidence` 三项**各有明确处置**（用 / 删 / 补发） | [A] | Go 结构断言 + 前端字段清单对比测试 | 待执行 |
| T-14 | **多轮"记得上文"（人可见）**：第 2 轮 AI 回复引用第 1 轮信息 | [V] | IAB 实测（浏览器多轮对话 + 截图） | 待执行 |
| T-15 | **负向对照**：故意关闭历史注入 ⇒ 复现"无记忆"（证明 T-3 的钉子真能红） | [A] | 单测开关版 + 一次运行输出 | 待执行 |
| T-16 | `MaxTokens` / `Temperature` **默认取值**（产品口径：疏导师回复长短与稳定度） | [M] ✅ | **用户裁定 2026-10-14** | **`MaxTokens=1024`、`Temperature=0.7`**（见 §6 R1 的长度张力跟踪） |
| T-17 | **基础人设的长度约束去留**（原"用中文简短回应（2-3 句话）"） | [M] ✅ | **用户裁定 2026-10-14** | **终态＝适中档**（两步裁定：先去掉硬约束看效果 → 实测失控为 657/766/629 字 → 再按"目前状态适中"定档）。**定档：全文 ≤300 字；只倾诉时约 100 字；需要建议才展开**，且 **base 与 llm-service 风格侧两侧同时给约束**（只改一侧必然被另一侧顶穿）。实测 243/269/256 字、e2e 2.7s。回归钉 `TestBasePersona_HasModerateLengthGuidance` + llm-service `test_intent_length_cap.py`；⚠️ 连带 T-1 语义变更为"逐字等于**本次裁定后**的基线"（report §3.12） |

> 关联账本：开工轮新登 **CL-F-06**（`check_git_layout.py` 在 worktree 恒红）、**CL-F-07**（worktree 缺 `.env.local` ⇒ 起栈缺 LLM key）。本阶段就地修项见 T-13。

---

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

| Phase | 内容 | RED 断言 |
|---|---|---|
| **0** | 基线取证：起 dev 栈 → 抓一次真实 `/api/v1/ai/stream` 的 prompt（日志/抓包）→ 产证据；同时确认 chat-svc `ListMessages` 运行时排序 | —（人工，产证据；**未做，见 §A.4**） |
| **1** | 抽统一组装层，**不改行为**（仍 2 条消息，但走新 builder）：`Assembler` + 段 builder 纯函数 | T-1 / T-2 / T-8（新函数不存在 ⇒ 编译 RED） |
| **2** | 注入同会话历史（N 条 + 预算裁剪 + rune 安全上界） | T-3 / T-4 / T-5 / T-6 |
| **3** | 预算与参数（`MaxTokens` / `Temperature` 下发 + 截断降级链 + 日志） | T-7 |
| **4** | 配置化：文案集中 + 一致性守卫 | T-9（守卫先 RED：故意改一处文案 ⇒ 守卫红） |
| **5** | 死路径处置：删 `useAIStream.ts` / `usePrompt.ts`(+单测+架构测试同步) / `with_intent` 接回 / 修 `file_context.py` | T-10 / T-11 / T-12 |
| **6** | 契约对齐：字段集统一（含 `emotion/model/stream` 处置） | T-13 |
| **7** | 人可见回归钉 + 负向对照 | T-14 / T-15 |

> **TDD 纪律**：每个 Phase 先提交 `test:` （RED）再 `feat:`/`fix:`（GREEN）；**不在同一 commit 混测试与实现**（CI 有 `tdd-gate`）。改 Go 后必须**重建镜像**再验收（RUNBOOK §2）；改 proto 不在本阶段范围。

---

## §4 收口判据（"做到什么程度"，对齐 roadmap §4 CL-04 验收）

- **A 零回归**：无画像 / 无历史时输出**逐字等于基线**（T-1 绿 + 全仓 Go 测试无回归）。⚠️ 该基线的**定义**在 Phase 6 被用户裁定变更过一次（去掉"简短"约束，T-17）——即 A 的语义现为"逐字等于**最近一次用户裁定后**的基线"，不再指改造前的旧输出。
- **B 记得上文**：多轮对话中体现记忆（T-3 单测 + T-14 IAB 实测截图）。
- **C 可控**：`MaxTokens` / `Temperature` 真下发（T-7）+ 超长截断不 panic（T-6）+ 默认值经用户裁定（T-16）。
- **D 配置化**：system 文案集中于一处（T-9）+ 三处人格文案一致性守卫。
- **E 死路径清零**：`useAIStream` / `buildRCTPrompt` 删除且零引用（T-12）+ `file_context` 多附件全输出（T-10）+ `with_intent` 接回（T-11）。
- **收口动作**：plan 每点四值 + 证据 → `report.md`（模板）→ 落地计划迁 `docs/legacy-plans/landed/` → roadmap 状态改 `done` → 删占位（契约 §六）→ AGENTS §2.5 三连自检。

---

## §5 回归钉

- **Go（主钉）**：`internal/handler/prompt_assembly_test.go`（段序 / 零回归逐字 / 历史条数 / 预算裁剪 / 参数下发）；`ai_stream_handler_llmgrpc_test.go` 扩展（历史进 `gotMsgs`）。
- **Go（架构钉）**：组装函数调用点唯一性（防缺陷 11 复发）。
- **pytest**：`emotion-llm-service/tests/unit/test_file_context_multi.py`（多附件）+ 既有 `test_grpc_intent_llm.py` 扩 `with_intent` 端到端。
- **vitest**：`usePrompt` 删除后同步改 `r09-ai-stream-auth-header.architecture.test.ts`（只保留 `useAIStreamHandler` 断言）。
- **Playwright（人可见钉）**：`docs/closure-roadmap` 要求每阶段收口必写——本阶段钉"多轮对话记得上文"（可与既有 chat spec 合并，具体 spec 名在 Phase 7 定）。

---

## §6 风险与坑

| # | 风险 | 说明 | 规避 |
|---|---|---|---|
| R1 | **~~取值需产品口径~~ → 已裁定（T-16）** | 计划 §8 D3 原未决；取值直接影响"疏导师回复长短" | **用户 2026-10-14 裁定：`MaxTokens=1024`、`Temperature=0.7`**。⚠️ 比原计划建议的 512 更长，**与"2-3 句话"基础人设有张力** ⇒ Phase 3 落地后须在 T-14 的 IAB 实测里**核对回复是否真的变啰嗦**（对照 Phase 0 基线长度）；若明显超长，回报用户复议（不自作主张改回）。**Phase 3 实测结论：512→1024 本身未使回复变长**（模型按人设自我约束，上限未触及；3 轮 53~86 字）。<br>**Phase 5 更新（2026-10-14，实测）**：真正把回复拉长的是**意图风格注入接回**后的 `STYLE_INSTRUCTIONS`（career/study/tech 类明确要求"分点 1.2.3.、加粗"）——同一次 A/B 里回复由 53~86 字变为 **192/278 字（≈6/13 句）**，与基础人设"2-3 句话"直接矛盾。**已作为产品口径问题上报用户**（选项：调 `STYLE_INSTRUCTIONS` 对齐陪伴人设 / `llmWithIntent` 翻回 false / 按意图分流），未自行改动 |
| R2 | **改动面大、触碰主链路** | 对话链路是评测/演示核心 | Phase 1 先"换实现不改行为"，逐 Phase 加能力，每步可回滚 |
| R3 | **历史注入放大 token / 成本** | 多轮全塞成本陡增、可能超窗口 | 预算优先、从最近往回、条数粗筛 + 硬上限 |
| R4 | **护栏被历史稀释**（越狱句式经历史进 prompt） | 历史内容同样不可信 | 护栏段恒**后置**于用户可控段（T-2 钉死）；历史文本按不可信数据处理 |
| R5 | **陈旧注释误导后续改动** | `ai_stream_handler.go:318` 注释写"倒序"与实际升序不符（A.1） | 顺手纠正注释（不改行为），并在 §A.1 留档 |
| R6 | **删死路径误伤** | `useAIStream.ts` 被架构测试引源码 | 先 `grep` 全仓确认零生产调用（**已确认**：仅架构测试）；同步改测试；git 留有历史 |
| R7 | **改完 Go 未重建镜像 ⇒ 验收假绿** | 历史踩坑 | RUNBOOK §2 铁律；验收前重建镜像 |
| R8 | **与 C 轨（CL-05）同碰前端** | 两轨都动 `emotion-echo-web/app/**` 与 `package.json`（追加式） | 契约 §1.2：同名文件串行；本阶段前端改动集中在 `composables/` 与架构测试，CL-05 集中在 stores/pages —— **不同文件可并行，同文件先握手** |
| R9 | **T-16 未裁定而擅自写死默认值** | 反模式"擅自决议" | 按 R1 建议值落地并**显式标注"待追认"**，写入 report 与 PR 描述 |

---

## §7 调研依据（本次实读）

- **代码**：`ai_stream_handler.go`、`downstream/{llm_grpc.go,chat.go,chat_grpc.go}`、`personality_directive.go`、`chat-svc/internal/{repository/conversation_repository.go,logic/listmessageslogic.go}`、`llm-service/{grpc_server.py,intent.py,file_context.py}`、前端 `composables/{useAIStreamHandler,useConversationSender,useAIStream,usePrompt}.ts`、测试 `ai_stream_handler_llmgrpc_test.go` —— 共 14 个文件（见 §A.1）。
- **文档**：`docs/legacy-plans/landed/prompt-pipeline-redesign.md`（来源计划，已落地）、`docs/closure-roadmap/{roadmap.md,parallel-contract.md,RUNBOOK.md,decisions.md,stages/_TEMPLATE.md}`、`adr-2026-09-personality-prompt-injection.md`、`adr-2026-09-personality-prompt-semantics.md`。
- **`grep` 断言**：`useAIStream` 生产零调用（仅架构测试）；`buildRCTPrompt` 仅单测引用；`req.Emotion|req.Model|req.Stream` 在 handler 内零命中；`with_intent` 仅 proto/py 侧存在、BFF 侧未设置。
- **未执行**：dev 栈未起、真实 prompt 未抓、§2.4 smoke 未跑（§A.4 声明）。
