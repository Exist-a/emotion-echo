---
status: pending
stage: CL-06
slug: cl-06-custom-agent
scope: 用户自定义 Agent（多角色）—— 档案管理 + 会话内切换 + 人设注入 + 音色随角色变 + 安全信封 + 消息追溯
owner: TBD（开工时登记 Lane-CL-xx）
created: 2026-10-17
source-plan: docs/plans/user-custom-agent-persona.md（2026-10-17 由单角色重写为多角色）
related:
  - ../../roadmap.md（§2 CL-06 行 / §4 CL-06 范围）
  - ../../decisions.md（CL-D-21 ~ CL-D-24）
  - ../../parallel-contract.md（§三 占位登记 / §1.2 共享脊柱）
  - ../../RUNBOOK.md（执行六步 / 证据有效性 / 收口契约）
  - ../../../plans/user-custom-agent-persona.md（设计口径 = §3 数据模型 / 槽位 / 安全分层）
---

# CL-06 · 用户自定义 Agent（多角色）— 执行计划

> **设计口径在** [`docs/plans/user-custom-agent-persona.md`](../../../plans/user-custom-agent-persona.md)（数据模型 / 契约落点 / 槽位段序 /
> 优先级 / 安全分层 / 音色 / UI）。**本文只管执行**：上下文功课、测试点、Phase 切分、证据、收口判据。
> 两份文档分工固定：**改设计改源计划，改执行改本文**。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-web-bff/internal/handler/prompt_assembly.go:28-36,63-76` | CL-04 落地的**段式组装层**：段类型 `base/personality/emotion` + `segmentSeparator = "\n\n"`；`:66` 注释**已为本计划预留** —— "用户可控内容（CL-06 的 Agent 人设）之后必须有护栏段兜底" |
| `emotion-echo-web-bff/internal/handler/prompt_assembly.go:102-141` | `emotionSegment` 优先级 face/voice → textEmotion → 会话历史；`assembleSystemPrompt` 丢弃空段后 join |
| `emotion-echo-web-bff/internal/handler/prompt_assembly.go:143-157` | `assembleMessages` = 两条上游路径的**唯一**组装点（system + history + 当前 user） |
| `emotion-echo-web-bff/internal/handler/prompt_templates.go:24,37` | `baseSystemPrompt` 硬编码、全局唯一（G7 的"现状"基线）。⚠️ **CL-04 已把它从 `ai_stream_handler.go` 迁到这里** —— 旧计划书里写的 `ai_stream_handler.go:61` 已失效 |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:346-355` / `:416-425` | 两处 `assembleMessages(assembleInput{…})` 调用点（llm-gRPC 路径 / HTTP 直连路径）⇒ 新增字段必须**两处都传**，否则路径间漂移 |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:181-197` | `saveAIMessage` 流结束后写 AI 回复 —— **当前不带 agent_id**，是 §3.1 ③ 的写入点 |
| `emotion-echo-web-bff/internal/handler/prompt_templates.go:107-112` | 现有护栏是**混在人格段文本内**的一句（`personalityGuideGuardrail`），不是独立段 |
| `emotion-echo-web-bff/internal/handler/personality_directive.go:31-88` | `buildPersonalityGuide(dims)`：测评五维 → 相处说明书（本计划不改它，只定义与它的**优先级**）。⚠️ 旧计划书写的 `:96-153` **已失效**（该文件现在只有 88 行；文案表在 CL-04 迁到了 `prompt_templates.go`） |
| `emotion-echo-web-bff/etc/web-bff.yaml:82-92` + `internal/config/config.go:257-285` | TTS 音色 = **全局单值** `FunAudioLLM/CosyVoice2-0.5B:anna`，yaml 与默认值双向钉（`TestConfig_TTSDefaults`） |
| `emotion-echo-web-bff/internal/downstream/tts_provider.go:61-120,343-364` | cloud provider 在**构造期**绑定 `opts.Voice`；provider = `auto`（cloud 优先 / 失败降级 XTTS） |
| `deploy/.env.local`（TTS 三项，**实测 2026-10-17**）+ `deploy/docker-compose.apps.yml:698` | **云端就是实际使用路径**：`TTS_API_KEY` / `TTS_API_BASE_URL` / `TTS_MODEL` **均已配置（非空）**，`TTS_PROVIDER` 未覆盖 ⇒ `auto` 生效为 **cloud**。XTTS 只是 `ADR-2026-10` 写明的**离线回退**（保留容器不删），不是使用路径（见 §A.5） |
| `emotion-echo-web/app/lib/apiRoutes.ts:74` | XTTS 直连端点 `ttsStream`（`/tts/stream`）在本文件**登记了但全仓零调用**（前端播放只走 `/tts/phonemes`）⇒ 本地 TTS 已不在前端链路上 |
| `emotion-echo-web-bff/internal/downstream/xtts.go:40-47` | `TTSPhonemesReq{Text,Language,Speed,Volume}` —— **无 voice 字段**（G4 的缺口） |
| `emotion-echo-web-bff/internal/handler/tts_handler.go:39-46,88-107` | 三个端点；`/tts/phonemes` 是**口型同步通道**（前端 `useTTSPlayer.playStream` 消费 phonemes 驱动 BlendShape） |
| `proto/chat.proto:84-92` / `:113-127` | `Conversation` / `Message` **都无 agent 字段** |
| `proto/chat.proto:188-195` | `UpdateConversationRequest` 注释"当前仅支持改标题" ⇒ 会话切角色要扩这里 |
| `proto/chat.proto:95-127` | `SendMessageRequest` 已有 `intent` / `file_name` 的先例 ⇒ `agent_id` 同套路 |
| `proto/agent.proto:1-40` | ⚠️ **文件名陷阱**：名为 agent，实为 **assessment-svc** 契约（`package emotion_assessment.v1`）⇒ 用户角色契约**不得**落此文件 |
| `proto/user.proto:41-102` | user-svc 现有 RPC（GetMe / UpdateProfile / GetUserById / Login / Register / ResetPassword / 密保两条）——**无任何角色相关 RPC** |
| `emotion-echo-chat-svc/internal/model/conversation.go:15-26` | `Conversation` 结构体无 agent 列 |
| `emotion-echo-chat-svc/migrations/{c004,c006,c008,c009}*.sql` | 加列迁移的现成套路（intent / file_name / 软删除）⇒ 新迁移照此命名 `c012_*` |
| `emotion-echo-user-svc/migrations/u003_add_user_config.sql` + `internal/model/user.go:54` | `users.config JSONB`（NULL = 未设置）——现状仅 fontSize/theme/ttsSpeed，**纯 UI，从不进 prompt** |
| `emotion-echo-web/app/pages/chat/conversation/[id].vue:58-215` | composer 结构：`textarea`（`:73`）+ `composer-actions`（`:81`）= camera-btn（`:113`）→ voice-btn（`:153`）→ send-btn（`:180`）；`:214-215` 是**内联** `<video class="camera-preview">` |
| `emotion-echo-web/app/layouts/nav.vue:82-92` | 导航项数组（对话 / 心理测验 / 日报…设置）——新标签加在此处 |
| `emotion-echo-web/app/pages/chat/setting.vue:2-82` + `:118-120` | 设置页只有字号 / 主题 / 语速；`handleFontSizeChange` 无 debounce（**另一账 `CL-F-31`，归 CL-11**） |
| `emotion-echo-web/app/composables/useTTSPlayer.ts:7,217` | `playStream` 走 `POST /api/v1/tts/phonemes` —— 音色参数要在这里带上 |
| `emotion-echo-web/app/lib/apiRoutes.ts` + `apiRoutes.test.ts` | 路由**单点真理**；契约测试扫全仓字面量 ⊆ 本文件 ⇒ 新增端点必须登记，否则门禁红 |

### A.2 相关 ADR / 决策

- ✅ **沿用**：`adr-2026-09-personality-prompt-injection.md`（注入落点在 **BFF**，不迁 llm-service）。
- 🔴 **冲突（必须修）**：`adr-2026-09-personality-prompt-semantics.md` **决策 1** ——
  "只给相处说明书、**不做 AI 性格**、基底人设**一字不改**"。本阶段让用户改 AI 名字/身份/背景 ⇒ 需**新增修订 ADR**（源计划 §5）。
- ✅ 已查 `docs/closure-roadmap/decisions.md`：本阶段四项已拍板 `CL-D-21`~`CL-D-24`。
- ✅ 已查 `legacy-plans/landed/prompt-pipeline-redesign.md`：组装层 `§3.1` 分层图**已预留**"用户自定义 Agent 段"。
- ✅ 已查 `legacy-plans/landed/chat-emotion-realtime-display.md`（CL-07）：情绪段与本段并列，段序见源计划 §3.3。

### A.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| 有统一组装层可挂新段 | ✅ `prompt_assembly.go:67` 已落地，且注释已预留本阶段 |
| 已有角色档案表 / RPC / 预留列 | ❌ **零足迹**（全仓 grep `user_agent_profile` = 0；`proto/user.proto` 无角色 RPC） |
| 音色可按请求切 | ❌ 音色是**构造期全局单值**；请求体无 voice 字段 ⇒ 需新开每请求通道 |
| 会话可绑定角色 | ❌ `conversations` 无该列；`UpdateConversationRequest` 只支持改标题 |
| AI 回复可记角色 | ❌ `saveAIMessage` 不带该字段 |
| `proto/agent.proto` 是用户 Agent 契约 | ❌ 是 **assessment-svc**（历史命名，见 A.1 行） |
| 能构建 web 镜像做 UI 验收 | ❌ **当前直接失败**（`CL-F-28`：npm ERESOLVE）⇒ Phase 0 必做 |
| 云端 TTS 是实际使用路径 | ✅ **实测 2026-10-17**：`.env.local` 的 `TTS_API_KEY`（51 字符）/`TTS_API_BASE_URL`/`TTS_MODEL` 均非空、`TTS_PROVIDER` 未覆盖 ⇒ `auto` = **cloud 生效**；前端 XTTS 直连端点零调用 ⇒ **音色 [V] 验收可行** |

### A.5 XTTS 的现状（2026-10-17 修正，覆盖本文先前版本的错误结论）

本文 v1 曾写"dev 栈无 `TTS_API_KEY` ⇒ 走 XTTS ⇒ 音色验收做不了"。**该结论已作废**，实际情况：

| 项 | 事实 | 依据 |
|---|---|---|
| 使用路径 | **云端 `FunAudioLLM/CosyVoice2-0.5B`** | `.env.local` key 已配 + `TTS_PROVIDER` 未覆盖 ⇒ `auto` 走 cloud；`ADR-2026-10` 决策亦为"在线主链路 = cloud" |
| XTTS 的地位 | **仅 `ADR-2026-10` 写明的离线回退**（"XTTS 容器保留作离线回退（不删）"），当前**不触发**、**前端零调用**其直连端点 | `ADR-2026-10` 决策 + 回退链（决策 4）；`apiRoutes.ts:74` `ttsStream` 无调用方 |
| 用户口径（2026-10-17） | 用户明确"**XTTS 都不用了，主要用云端 API**" | 本会话用户陈述 |
| ⚠️ 措辞张力（**未裁**） | ADR 写"保留作兜底"，用户说"不用了"——两者**不冲突但需对齐**：是否**正式退役** XTTS（删 compose 服务 / BFF 兜底 provider / `/tts/stream` 端点 / ai-svc 客户端）属**另立范围**，非本阶段 | 见 §8 `N6` |

### A.4 起栈与实测声明（诚实边界）

⚠️ **本计划书撰写时未起栈、未跑 smoke** —— 结论来自**读码 + 文档核对**（无运行态证据）。
按 RUNBOOK §4「证据有效性」，本文件中的实现细节属**待验证假设**，Phase 0/8 必须用运行态证据补齐。
另：`CL-F-28` 未修前**无法构建 web 镜像**，任何前端改动都只能"拿旧镜像验错对象"（E2E-F-70 同型）⇒ 它是 Phase 0。

✅ **已核实的配置面**（2026-10-17，非运行态）：`deploy/.env.local` 的 TTS 三项非空 + `TTS_PROVIDER` 未覆盖
⇒ 主链路为 cloud（§A.5）；前端 XTTS 直连端点零调用。**这两条是"配置事实"，不等于"跑起来验过"** ——
音色是否真变、口型是否仍同步，仍须 Phase 8 的 IAB 运行态证据。

---

## §1 目标与非目标

**目标**（对齐源计划 G1~G7）：见源计划 §2 表。摘要：
① 角色档案 CRUD（≤5，默认「小E」可改可删）；② 会话内可切换（**composer、语音按钮左侧、向上展开**，可中途随时切）；
③ 人设注入组装层新段；④ 音色随角色变且**口型仍算得出**；⑤ 安全信封分层；⑥ 每条 AI 消息记 `agent_id`；
⑦ 无设定时逐字等于现状。

**非目标**：头像（待定）；数字人形象改造；危机处理（`CL-08e`）；角色市场/分享；跨会话长期记忆（`CL-08`）；
不改测评画像与情绪回显既有语义；不登记 `E2E-F`。

---

## §2 测试点清单

> 判定分级见 RUNBOOK §4：`[A]` 自动可判 / `[V]` 视觉（人看截图）/ `[M]` **仅用户可裁**。
> 结果四值 `PASS / FAIL / BLOCKED / N/A` + **执行输出**（命令 / 退出码 / 截图）。"已创建"不算证据。

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-1 | 三条迁移（`user_agent_profile` 建表 / `conversations.agent_id` / `messages.agent_id`）**幂等可重跑** | [A] | 迁移工具连跑两次全绿 + `*_ddl_parity_test.go` 同款守卫 | |
| T-2 | 角色档案 CRUD 契约：写入读回一致；`name`/`persona`/`tone` 超长与空值被**拒绝**（非静默截断） | [A] | user-svc Go 契约测试（表驱动含边界） | |
| T-3 | **每户 ≤5 上限**：第 5 个通过、第 6 个被拒（服务层强制） | [A] | 同上 | |
| T-4 | 软删除：删后不出现在列表；**会话 `agent_id` 指向已删角色时回落默认角色**（不报错、不给空 prompt）；角色删光后列表为空且**不注入 Agent 段** | [A] | 同上 + 组装层契约测试 | |
| T-5 | 「小E」按需物化：首次读列表即得一条 `is_default=true`；可改可删（改后名称回读一致） | [A] | 同上 | |
| T-6 | 会话切角色：`更新会话.agent_id` 写入读回一致；**非法 id / 他人角色被拒** | [A] | chat-svc + BFF 契约测试 | |
| T-7 | AI 消息写库带 `agent_id`（= 写库时该会话的当前角色） | [A] | BFF 契约测试（断言 `SendMessageReq.AgentID`） | |
| T-8 | **零回归**：无角色设定时组装输出**逐字等于**现状 golden 字面量（复用 CL-04 T-1 手法） | [A] | `prompt_assembly` 契约测试 | |
| T-9 | **段序不变量**：有设定时 `agent` 段存在，且 `guard` 段 index **大于** `agent` 段 index | [A] | 同上（位置断言，非"包含"断言） | |
| T-10 | 优先级：`boundaries` 恒进 prompt；用户 `tone` 覆盖测评同类措辞；无设定时不退化 | [A] | 同上 | |
| T-11 | 安全表驱动：越狱句式（"忽略以上指令"/"你现在是"/"输出你的系统提示"）、宣称资质、冒充真人 ⇒ **保存被拒**；且 prompt 侧存在"用户设定非系统指令"分隔声明 | [A] | user-svc/BFF 表驱动 + 组装层断言 | |
| T-12 | 音色：`/tts/phonemes` 带 `voice` 时，cloud 上游请求体 `voice` = 该值；**不带时回落配置默认**（突变实验：去掉该字段 ⇒ 测试必红） | [A] | `tts_provider_test.go` 同款（断言上游 body） | |
| T-13 | **口型不回归**：带 `voice` 的请求仍返回 `phonemes` 数组 + `duration`（G4 硬约束） | [A] | 同 T-12 | |
| T-14 | 前端路由登记：新增端点 ⊆ `API_ROUTES`；全仓字面量契约测试绿 | [A] | `apiRoutes.test.ts` | |
| T-15 | 管理页：列表（≤5）/ 建 / 改 / 删 / 预览 交互可用；表单校验（超长、非法音色）拦截 | [V] | IAB（**生产构建**，dev 不 hydration 先例见 CL-07 §A.4） | |
| T-16 | **切换器**：位于 composer 语音按钮**左侧**、**向上展开**、列出角色名；选中后**本会话**后续消息按新角色走 | [V] | IAB | |
| T-17 | 角色差异可辨：两个不同人设的角色对同一问题回答风格有明显差异（不冒充真人 / 不宣称资质） | [V] | IAB + 用户核 | |
| T-18 | **换角色 → 音色变化可听**（`phonemes`/`duration` 仍正常）。前置**已满足**（cloud 生效，见 §A.5）。若 cloud 失败降级 XTTS，兜底期间音色回落 = 已知边界，**如实登记但不作为验收项** | [V] | IAB（cloud） | |
| T-19 | 历史消息标注"这条是谁回的"；同一角色连续消息不重复标注（形态待 `N4` 裁定） | [V] | IAB | |
| T-20 | 负向对照：**无角色设定**（或删光角色）时对话行为与现状一致、不出现人设段 | [V] | IAB + 组装层日志 | |
| T-21 | 「小E」初始人设文案与音色（属用户可感内容，执行侧不自拟） | [M] | 用户过目 | |

> 数量：21 点（[A] 14 / [V] 6 / [M] 1）。`[M]` 点未裁不得判 `done`（可先 `BLOCKED`）。

---

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

| Phase | 内容 | RED 断言 | 备注 |
|---|---|---|---|
| **0** | **修 `CL-F-28`**：`emotion-echo-web/Dockerfile` 改用 **pnpm**（与仓库权威锁 `pnpm-lock.yaml` 一致），并验证整栈能构建 web 镜像 | 现在 `docker compose build emotion-echo-web` 直接 `npm error ERESOLVE`（命令 + 退出码为证）；改后构建成功 | **独立小 PR**（纯工程修复，**不含**本阶段功能）；不修则后面所有 UI 验收都是失真证据 |
| **1** | **文档/架构**：新增修订 ADR（源计划 §5）+ 源计划重写已就位（本文件同批） | —（`adr-gate` CI 守卫要求新 ADR 有索引登记） | 防"架构改动无 ADR" |
| **2** | **user-svc**：`u0NN_create_user_agent_profile.sql` + model + repo + RPC（List/Upsert/Delete）+ ≤5 上限 + 软删除 + 「小E」get-or-create | 引用不存在的类型/表 ⇒ 编译失败；契约测试全红 | 对应 T-1/T-2/T-3/T-4/T-5 |
| **3** | **proto + chat-svc**：`user.proto` 扩角色 RPC；`chat.proto` 扩 `Conversation.agent_id` / `SendMessageRequest.agent_id` / `Message.agent_id` / `UpdateConversationRequest.agent_id`；chat-svc 迁移 `c012_*` + 透传 | 同上（字段不存在） | 对应 T-1/T-6；**改 proto 必须按仓库既有方式重生成**（`.pb.go` 生成物同批提交） |
| **4** | **BFF**：角色端点透传 + 组装层新增 `agent` 段与 `guard` 段（含段序不变量）+ `saveAIMessage` 带 `agent_id` | T-8 零回归 + T-9 段序 + T-7 写库 | **两处 `assembleInput` 调用点都要传**（A.1 行） |
| **5** | **安全护栏**：长度上限 / 越狱句式黑名单 / 越界（资质、冒充真人）/ 分隔与声明 / 护栏后置 | T-11 表驱动全红 → 全绿 | 规则式黑名单（`CL-08e` 不做危机层） |
| **6** | **音色**：`TTSPhonemesReq.Voice` 可选字段 + cloud provider 用请求值覆盖 + 回退口径 | T-12/T-13（断言上游 body；突变实验证钉子非空） | 零回归：不带字段 = 现状 |
| **7** | **前端**：侧边栏「角色」管理页（增改删+预览）+ composer 切换器（语音按钮左侧、向上展开）+ `apiRoutes` 登记 + 情绪/数字人/口型链路不回归 | T-14/T-15/T-16（vitest：表单校验 / 保存回填 / 切换写库 / 预览文案） | `stores/agent.ts` 新增；`useTTSPlayer` 带 voice |
| **8** | **浏览器实测（IAB，生产构建）+ 回归钉（Playwright）** | T-15~T-20 | 音色项（T-18）依赖 cloud key |
| **9** | **收口**：report / 账本对账 / 源计划迁移（`legacy-plans/landed/`）/ roadmap 状态同步 / 第二方核对 | — | 见 §4 |

> **TDD 纪律**：每 Phase 的 RED 必须是**真会红**的钉子（先在实现前跑一次看到失败），
> 并在 report 里留下 RED→GREEN 两次输出 —— 不允许"测试与实现同批写完、从未见红"。

---

## §4 收口判据（对齐 roadmap §3 与源计划 G1~G7）

- **功能判据**：用户能建/改/删 ≤5 个角色；对话页能随时切换且只影响本会话；AI 回答体现该角色人设；
  换角色音色随之变且口型正常；每条 AI 消息可追到角色；无角色时行为与现状一致。
- **机器判据**：§2 的 14 个 `[A]` 点全 PASS（含 T-8 零回归、T-9 段序、T-12 突变实验）；
  全量 `vitest run` 与后端 `go test ./...` 绿；`apiRoutes` / `adr-gate` / `tdd-gate` 等既有门禁绿。
- **诚实边界（必须写进 report）**：① 音色验收在 **cloud** 上做（key 已配，§A.5）；cloud 降级到 XTTS 的窗口内音色会回落，该窗口**不被覆盖**；
  ② `[M]` 项（T-21）由用户裁定；③ 跨库无外键 ⇒ 角色删除后的历史标注依赖快照语义（T-4 已覆盖）。
- **收口契约 7 条**（RUNBOOK §6）：逐点四值 + 回归钉绿 + report 用模板 + **账本对账** + 源计划迁移 + roadmap 同步 + 第二方核对。

---

## §5 回归钉

- **Go / 后端**：`prompt_assembly` 契约（段序不变量 + 零回归 golden）· 角色 CRUD 契约（含 ≤5 / 超长 / 软删除 / 回收落）·
  `tts_provider` 上游 body 断言（含突变实验）· chat-svc 迁移 parity。
- **Vitest**：管理页表单校验 / 保存回填 / 预览文案 · 切换器交互 + 写库调用 · `apiRoutes` 字面量契约。
- **Playwright**：建角色 → 切到该角色 → 发消息 → 回答体现人设（可经 prompt 日志断言）→ 切回默认 → 音色/口型仍正常。
- **负向对照**：无角色设定 ⇒ 组装输出逐字等于现状（T-8/T-20 双保险：机器 + 浏览器）。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **Prompt 注入 / 越狱** | 用户可控文本进 system prompt（安全敏感面） | §3.5 五层：分隔声明 + 长度上限 + 句式黑名单 + 护栏段**后置** + 危机通道独立 |
| **冒充真人 / 宣称资质** | "你是有 20 年经验的心理医生"会被当真 | 保存时拦截 + 沿用 personality 语义 ADR 的措辞纪律 + 修订 ADR 写明边界 |
| **与测评画像打架** | 两套人格指令冲突 | 优先级写死：boundaries 恒最高 → 用户显式设定 > 测评；冲突回退有测试 |
| **改 proto 的连锁** | `Message`/`Conversation` 加字段要重生成 `.pb.go` 并**同批部署** | 按仓库既有生成方式；新增字段号、旧字段不动（无 wire-breaking） |
| **`proto/agent.proto` 误用** | 文件名像"用户 Agent"，实为 assessment 契约 | 源计划 §3.2 已写死：角色契约落 `user.proto`，**禁止**进 `agent.proto` |
| **两处组装点漂移** | `ai_stream_handler.go` 有 llm-gRPC / HTTP 直连两处 `assembleInput` | 新增字段两处都传 + 单一 `assembleMessages` 已收敛（CL-04）；T-8/T-9 覆盖 |
| **`CL-F-28` 不修就开工** | 只能拿旧镜像验收（E2E-F-70 同型失真） | Phase 0 强制前置；**未完成不得进入 Phase 7/8** |
| **cloud 降级窗口内音色回落** | 兜底 provider 是 XTTS（`ADR-2026-10` 决策 4），没有这 8 个音色 | 主链路已是 cloud（§A.5）⇒ 正常不触发；T-18 只验 cloud 路径，降级窗口如实登记、不冒充"全覆盖" |
| **口型链路被音色改动带坏** | `phonemes`/`duration` 是硬约束 | T-13 单独钉；Phase 8 实测口型 |
| **与 CL-11 同页** | 摄像头悬浮窗也改 `[id].vue` | 并轨时按 `parallel-contract §1.2` 握手；不同区（`:81-210` vs `:214-215`）但**同名文件串行** |
| **路由未登记** | `apiRoutes.test.ts` 会红 | Phase 7 同批登记新端点 |
| **「小E」文案自拟** | 属用户可感内容 | 列为 `[M]` T-21，用户过目后才定稿 |

---

## §7 调研依据（本次实读）

- 后端：`emotion-echo-web-bff/internal/handler/{prompt_assembly.go,prompt_templates.go,ai_stream_handler.go,tts_handler.go,personality_directive.go,chat_handler.go}`、
  `internal/downstream/{tts_provider.go,xtts.go,ai.go}`、`internal/config/config.go`、`etc/web-bff.yaml`
- 服务与数据：`emotion-echo-chat-svc/internal/model/conversation.go`、
  `emotion-echo-chat-svc/migrations/{c004,c006,c008,c009}*.sql`、`emotion-echo-user-svc/migrations/u003_add_user_config.sql`、
  `emotion-echo-user-svc/internal/model/user.go`、`proto/{chat.proto,user.proto,agent.proto}`
- 前端：`app/pages/chat/conversation/[id].vue`、`app/layouts/nav.vue`、`app/pages/chat/setting.vue`、
  `app/composables/useTTSPlayer.ts`、`app/lib/apiRoutes.ts`
- 文档：`docs/closure-roadmap/{roadmap.md,decisions.md,RUNBOOK.md,parallel-contract.md,discovered-unresolved.md}`、
  `docs/plans/user-custom-agent-persona.md`（本阶段源计划，同批重写）、
  `docs/legacy-plans/landed/{prompt-pipeline-redesign.md,chat-emotion-realtime-display.md}`、
  `docs/architecture/adr/adr-2026-09-personality-{prompt-injection,prompt-semantics}.md`
- 命令：`grep -rn "user_agent_profile\|agent_profile\|agent_id"`（零足迹核对）、`git ls-remote --heads origin`（无同名在飞分支）、
  `grep -rn "CL-D-2[0-9]" docs/`（决策编号占位核对）
- **未能确认**（需运行栈，Phase 0/8 补）：整栈 pnpm 构建 web 镜像能否一次过；
  composer 增加控件后窄视口换行行为；cloud 音色切换的真实可听差异；`[M]` 项的文案与口径

---

## §8 待拍板（just-in-time；已拍板项见 `decisions.md`）

| # | 事项 | 建议 | 归属 |
|---|---|---|---|
| N1 | 预设模板范围（旧 D4） | v1 给 3~5 个 + 允许"纯自定义" | 进阶段时提 |
| N2 | 「小E」初始人设文案与音色 | 音色沿用默认 `anna`；**文案需用户过目** | 进阶段时提（T-21） |
| N3 | ~~XTTS 回退下音色不变，UI 是否提示~~ | ✅ **已作废（2026-10-17）**：误判前提（"dev 走 XTTS"）—— 实际走 cloud（§A.5） | — |
| N6 | **是否正式退役 XTTS**（删 compose 服务 / BFF 兜底 provider / `/tts/stream` 端点 / ai-svc 客户端） | 属**另立范围**（非 CL-06）；与 `ADR-2026-10`「容器保留作离线回退」需对齐 ⇒ 建议单开一项（或登 `CL-F`/挂 `CL-09`）。**不确定前不动手** | 开工前问用户 |
| N4 | 历史消息"谁回的"标注形态 | 建议同一角色连续消息只标首条 | 进阶段时提（T-19） |
| N5 | 阶段是否与 CL-11 并轨（同页不同区） | 不并轨也可；若并轨按 §1.2 握手 | 开工前定 |
