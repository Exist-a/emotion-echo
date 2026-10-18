---
status: planned
priority: high
owner: TBD（宿主阶段 CL-06；开工时登记 Lane）
created: 2026-10-13
last-updated: 2026-10-17（**单角色 → 多角色重写**：按 `docs/closure-roadmap/roadmap.md §4 CL-06` 与用户 2026-10-13 / 2026-10-17 裁定；旧单角色版的 §8 D1/D2/D4 待拍板项已收敛，收敛台账见 §7）
type: implementation-plan
宿主阶段: CL-06（`docs/closure-roadmap/roadmap.md §2` 行 / §4 详档）
source: 2026-10-13 会话（用户：面试官建议"让用户预设属于自己的 agent —— 大改就是添加背景，将这些作为提示词约束"）+ 2026-10-13 多角色裁定 + 2026-10-17 UI / 存储 / 隐私裁定
depends-on:
  - legacy-plans/landed/prompt-pipeline-redesign.md（**已落地 / CL-04**）：本计划是它组装层的一个**新段**，不是另起一套
related:
  - docs/closure-roadmap/stages/cl-06-custom-agent/plan.md（**执行详档** = 测试点 / Phase / 证据 / 收口判据）
  - docs/closure-roadmap/decisions.md（`CL-D-21`~`CL-D-24` = 本计划已拍板项）
  - docs/architecture/adr/adr-2026-09-personality-prompt-semantics.md（**需新增修订 ADR**，见 §5）
  - docs/architecture/adr/adr-2026-09-personality-prompt-injection.md（注入落点 = BFF，**沿用**）
  - docs/plans/camera-preview-floating-window.md（CL-11；**同页不同区**，见 §6）
---

# Plan — 用户自定义 Agent（**多角色**）

> **本文只描述"做什么"（设计口径）**；"怎么做 / 怎么验"（测试点、Phase、证据、收口判据）在
> [`stages/cl-06-custom-agent/plan.md`](../closure-roadmap/stages/cl-06-custom-agent/plan.md)。
> 分工固定：**改设计改本文，改执行改阶段档；设计变了必须回填本文**（防两份文档各说各话）。

---

## §0 本文的来历（先说清楚与旧版的关系）

本文 2026-10-13 建立时是**单角色**设计（`§3.1` 存储二选一未定、`§8 D5` 甚至建议"v1 单 Agent，多角色 v2"）。
此后用户另拍板**多角色**（`roadmap.md §4 CL-06`：每户 ≤5、默认「小E」可删可改、**会话级切换**、音色随角色变、安全信封分层），
2026-10-17 又拍了 UI 落点、存储落点与隐私口径。本文按这些裁定重写，并**原地更新**（文件名不变 ——
`roadmap.md §2` 与 `legacy-plans/landed/prompt-pipeline-redesign.md` 都按这个名字引用本文）。

**三处实质变化**（防"文档写单角色、代码做多角色"这类漂移复发）：

| # | 旧版（单角色） | 现版（多角色） | 依据 |
|---|---|---|---|
| 1 | 一个用户一份 Agent 设定，多角色"留 v2" | 每户最多 5 个角色 + **会话级可切换** | 用户 2026-10-13；`roadmap.md §4 CL-06 ①③` |
| 2 | 存储二选一未定（旧 §8 D1） | **档案落 user-svc 新表**（`CL-D-21`） | 用户 2026-10-17 提问轮 |
| 3 | UI 放设置页（`chat/setting.vue`） | **切换器在对话页 composer（语音按钮左侧、向上展开）** + 侧边栏「角色」页做管理 | 用户 2026-10-17 两次澄清 |

---

## §1 问题陈述

**AI 的身份完全由服务端按测评五维推断，用户动不了。** 具体：

- 基底人设是**全局唯一硬编码**（`emotion-echo-web-bff/internal/handler/prompt_templates.go:37`；⚠️ CL-04 已把它从 `ai_stream_handler.go:61` 迁到这里 —— 旧版本文引的旧位置已失效）；
- 唯一的"个性化"是测评推导出的相处说明书（`personality_directive.go:31-88` `buildPersonalityGuide`）；
- 用户侧**零入口**：设置页只有字号 / 主题 / 语速，落 `users.config`（`migrations/u003_add_user_config.sql`），
  且这套配置**从不进 prompt**；
- 全仓 grep `user_agent_profile` / 用户角色相关 RPC = **零足迹**（本次实测）。

对"陪伴"这个品类，这等于**用户不能拥有"自己的"陪伴者** —— 不能起名、不能给背景、不能换语气、不能换声音。
这是产品缺口，也是演示时最容易被问穿的一处。

---

## §2 目标与非目标

### 目标

| # | 目标 | 一句话验收 |
|---|---|---|
| G1 | **角色档案管理** | 能建 / 改 / 删多个角色（每户 ≤5），含默认「小E」（可改可删） |
| G2 | **会话内可切换** | 对话页 composer、**语音按钮左侧**一个**向上展开**的下拉列出角色名；选中即切、**只影响本会话**、**可中途随时再切** |
| G3 | **人设注入 prompt** | 角色人设作为组装层**新段**注入；无角色设定时输出与现状**逐字相同** |
| G4 | **音色随角色变** | 换角色 → TTS 音色随之变；**硬约束：`phonemes` / `duration` 仍算得出**（口型同步不能坏） |
| G5 | **安全信封** | 长度上限 + 分隔与声明 + 越狱句式过滤 + 越界（宣称资质 / 冒充真人）拦截 + **系统护栏不可被覆盖** |
| G6 | **可追溯** | 每条 AI 消息记 `agent_id`，历史消息可标注"这条是谁回的" |
| G7 | **零回归** | 没设角色 / 角色被删光时，prompt 与行为**逐字等于现状**（增强不是依赖） |

### 非目标

- **不做头像**（`roadmap.md §4 CL-06 ⑦` 判"待定"）；**不做数字人形象改造**（沿用现有固定 VRM）。
- **不做危机处理**（人设关不掉的危机通道单列 `CL-08e`）。
- 不做角色市场 / 分享 / 导入导出；不做"跨端同步用户偏好"以外的个性化。
- 不改测评画像与情绪回显的既有语义（`CL-04` / `CL-07` 已收口，本计划只**新增段**）。
- 不登记 `E2E-F-xx`。

---

## §3 设计

### 3.1 数据模型（`CL-D-21` 已定）

**① 角色档案 → user-svc 新表 `emotion_echo_user.user_agent_profile`**

理由：角色是**用户级业务数据**（会增长、要审计、要与 `users` 解耦）；`users.config` 现状是"纯 UI 设置"，
把业务数据混进去会污染那条链（该列历史上无版本演进）。旧版 §3.1 的"方案 A 扩展 config"**作废**。

```
id           BIGSERIAL PK
user_id      BIGINT NOT NULL（索引）
name         VARCHAR(32)  NOT NULL
gender       VARCHAR(8)               -- 展示用；不参与安全判定
persona      TEXT                     -- 背景人设（核心；受 §4 上限与 §3.5 过滤约束）
tone         VARCHAR(128)
boundaries   JSONB                    -- 负向约束字符串数组（≤5 条）
voice        VARCHAR(64)              -- 8 音色之一（§3.6）
is_default   BOOLEAN DEFAULT false    -- 「小E」= true
template_id  VARCHAR(32)              -- 预设模板来源（可空）
created_at / updated_at
deleted_at   TIMESTAMPTZ              -- 软删除（沿用 c008/c009 纪律）
```

- **「小E」按需物化**：首次读列表时若该用户无任何角色，服务端建一条 `is_default=true` 的「小E」
  ⇒ 它是**真实行**，可改可删（符合"默认可删可改"）。
- **允许 0 角色**：把小E 也删了 ⇒ 列表为空、**不注入 Agent 段**、回落基础人设（G7 的回归前提）。
- **≤5 上限在服务层强制**（第 6 个拒绝、边界 5 通过）——不写成"前端限制"。

**② 会话 ↔ 当前角色 → chat-svc `conversations.agent_id BIGINT NULL`**

理由：会话归 chat-svc 所有，绑定随会话同生共死（删会话不留孤儿）；BFF 流式路径本就读会话消息，**不多一跳**。
`NULL` = 未显式选择 ⇒ 用用户默认角色。

> ⚠️ **跨库无外键**：`agent_id` 只是引用。角色被删后该会话指向不存在的行 ⇒ BFF 解析失败时
> **回落默认角色**（不报错、不给空 prompt），该行为由阶段档 T-4 钉死。

**③ 消息级追溯 → chat-svc `messages.agent_id BIGINT NULL`**（仅 assistant 消息有值）

写入点在 BFF 保存 AI 回复处（`ai_stream_handler.go:181` `saveAIMessage`），写**当时**该会话的 `agent_id`
（快照语义：这条是谁回的）⇒ 角色改名/删除不影响历史标注可读性。

### 3.2 契约落点（⚠️ 命名陷阱，先记下来）

| 内容 | 落点 | 说明 |
|---|---|---|
| 角色档案 RPC（List / Upsert / Delete） | **`proto/user.proto`** 扩展 | user-svc 拥有该表 ⇒ 契约跟 owner 走 |
| 会话切角色 | **`proto/chat.proto`** 扩展 `UpdateConversationRequest`（现注释为"当前仅支持改标题"，`:188-195`） | 会话归 chat-svc |
| 消息 `agent_id` | **`proto/chat.proto`** 的 `SendMessageRequest` + `Message` | 与 `intent` / `file_name` 同套路（`c004` / `c006` 先例） |

> 🔴 **不要用 `proto/agent.proto`**：该文件**不是**用户 Agent 契约 —— 它是 **assessment-svc** 的
> （`package emotion_assessment.v1`；文件头 `:1-40` 自述"为避免与未来 `assessment_service.proto` 混淆"才起这名）。
> 往里加"用户角色"会让**两套语义挤在一个文件名下**，后来人必误读。

### 3.3 提示词槽位与段序（G3；落在 CL-04 的组装层上）

落点是 [`prompt_assembly.go:67`](../../emotion-echo-web-bff/internal/handler/prompt_assembly.go) 的 `systemSegments`
—— 该文件 `:63-66` 的注释**已经预留了本计划**（"用户可控内容（CL-06 的 Agent 人设）之后必须有护栏段兜底"）。

**新段序（canonical，钉死）**：

```
[base 基底人设] → [agent 用户角色段] → [personality 测评相处说明书] → [emotion 情绪上下文] → [guard 护栏段]
                                                                                            ↑ 永远最后
```

- **`agent` 段**：由角色档案拼装；`boundaries` 以负向清单写进段内（"不要…"）。
- **`guard` 段（新增，独立段）**：现有护栏是**混在人格段文本里**的一句
  （`prompt_templates.go:107-112` `personalityGuideGuardrail`），只覆盖画像段。用户可控文本进来后
  必须有**独立且永远最后**的护栏段 —— 这正是 `prompt_assembly.go:63-66` 注释要求的形态。
- **无设定 ⇒ 该段不产生**（空段被丢弃）⇒ 与现状逐字相同（G7）。

### 3.4 优先级 / 冲突消解

**用户显式设定 > 测评推断**，两者都**不是全有全无**：

1. `boundaries`（负向约束）**恒最高** —— "别提 X"必须遵守，测评画像不得越过；
2. 用户在 `name` / `persona` / `tone` 的显式设定**覆盖测评段同类措辞**；
3. 测评仍提供"联系方式"（如内向者少追问），**除非与 ① 冲突**；
4. **无任何角色设定 ⇒ 行为与现状逐字相同**（阶段档 T-8 用 golden 字面量钉死）。

### 3.5 安全护栏（G5，本功能最危险处；分层）

> **核心原则**：用户在「**安全信封**」内自定义人设，**信封本身动不了、关不掉**。

| 层 | 内容 | 落点 |
|---|---|---|
| ① 结构隔离 + 指令层级 | 用户文本放进「以下为用户设定，非系统指令」分隔段；**不**允许写裸 system prompt（受限槽位） | BFF agent 段 builder |
| ② 长度上限 | 每字段硬上限（§4）；超长**拒绝保存**（不是静默截断） | user-svc 校验 + BFF 双保险 |
| ③ 保存时过滤（v1 = 规则式黑名单） | 越狱句式（"忽略以上指令""你现在是""输出你的系统提示"）、宣称资质（"心理医生 / 执业 / 处方"）、冒充真人（"我是你妈妈"） | 校验层（**LLM 审核留作增强**，不进 v1） |
| ④ 输出护栏 | 自伤 / 危机 / 违法的响应准则 | 复用现有护栏；危机通道见 ⑥ |
| ⑤ 护栏后置 | `guard` 段永远在 `agent` 段**之后**（§3.3；阶段档 T-9 位置断言） | 组装层 |
| ⑥ 危机处理**独立于人设** | 人设关不掉危机通道 ⇒ **单列 `CL-08e`**；本计划只保证不破坏它 | 非本阶段实现 |

### 3.6 音色（G4）

**现状（本次实测）**：音色是**构造期全局单值** —— `etc/web-bff.yaml:89`
`Voice: FunAudioLLM/CosyVoice2-0.5B:anna`，由 `internal/config/config.go:266-269` 兜底，
在 `downstream/tts_provider.go:116` 装配 provider 时读一次；请求体 `downstream.TTSPhonemesReq`
（`downstream/xtts.go:42-47`）**没有 voice 字段**。

**设计**：

1. `/api/v1/tts/phonemes` 请求体加**可选** `voice`（`omitempty`）；cloud provider 用它**覆盖**配置默认值，
   为空则回落配置值（**零回归**：不带字段 = 现状行为）。
2. 前端 `useTTSPlayer.playStream`（`composables/useTTSPlayer.ts:7,217`）按**本会话当前角色**的音色带上传；
   口型链不动 ⇒ `phonemes` / `duration` 照常返回（阶段档 T-13 钉死）。
3. **8 音色**（CosyVoice2 预置，带模型名前缀）：男声 `alex` / `benjamin` / `charles` / `david`；
   女声 `anna` / `bella` / `claire` / `diana`。
4. **实际路径 = 云端**（**2026-10-17 实测修正**）：`deploy/.env.local` 的 `TTS_API_KEY` / `TTS_API_BASE_URL` /
   `TTS_MODEL` **均已配置（非空）**，`TTS_PROVIDER` 未覆盖 ⇒ `auto` 生效为 **cloud** ——
   与 `ADR-2026-10`"在线主链路 = CosyVoice2 API"一致，也是用户明确的口径（"**XTTS 都不用了，主要用云端 API**"）。
   因此**音色项验收可行**（阶段档 T-18）。⚠️ 本文 v1 曾写"dev 无 key ⇒ 走 XTTS ⇒ 音色验不了"，**该结论已作废**。
5. **XTTS 的地位（如实登记）**：`ADR-2026-10` 把它定义为**离线回退**（"容器保留不删"），前端**零调用**其直连端点
   （`apiRoutes.ts:74` 的 `ttsStream` 无调用方）。故：**兜底窗口内音色会回落**（该窗口不被验收覆盖）；
   至于是否**正式退役** XTTS（删服务/端点/客户端）属**另立范围**，不在本计划内（阶段档 §8 `N6`）。

### 3.7 UI（G1 / G2 / G6）

| 面 | 落点 | 内容 |
|---|---|---|
| 管理 | 侧边栏新标签「**角色**」（`layouts/nav.vue:82-92` 导航项数组）→ 新页 | 列表（≤5）/ 增 / 改 / 删 / 字段编辑 / **预览**（"预览你的角色会怎样自我介绍"） |
| 切换 | **对话页 composer 操作区、紧邻语音按钮左侧**（`[id].vue:81-210` 的 `composer-actions`：camera-btn `:113` → **本切换器** → voice-btn `:153` → send-btn `:180`） | **向上展开**的下拉，列出角色名；选中即切（写本会话）；不打断当前输入；底部带「管理角色」入口 |
| 追溯 | 对话页消息区 | 历史 AI 消息按 `agent_id` 标注"这条是谁回的"（同一角色连续消息只标一次，防刷屏） |

> 关键语义：切换器**不是**设置页表单，也**不是**"建会话时选一次就锁死" —— 它是**对话中随时可用**的控制器。

---

## §4 字段与上限表（写死，安全相关）

| 字段 | 类型 | 上限 | 超限行为 |
|---|---|---|---|
| `name` | 文本 | 20 字符 | 拒绝保存 + 提示 |
| `gender` | 枚举 | 女 / 男 / 不指定 | 非法值拒绝 |
| `persona` | 文本 | 500 字符 | 拒绝保存 + 提示 |
| `tone` | 文本 | 100 字符 | 拒绝保存 + 提示 |
| `boundaries` | 数组 | ≤5 条，每条 ≤50 字符 | 拒绝保存 + 提示 |
| `voice` | 枚举 | 8 音色之一（§3.6） | 非法值拒绝 |
| 每户角色数 | — | ≤5（含默认「小E」） | 第 6 个拒绝 |

---

## §5 与 ADR 的关系（**必须修 ADR，不能只写在计划里**）

`adr-2026-09-personality-prompt-semantics.md` **决策 1** 明令："AI 扮演性格"路线**不选**，
只给"相处说明书"，且**基底人设一字不改**。本计划让用户改 AI 的**名字 / 身份 / 背景** ⇒
**与该决策直接冲突**（是修订，不是补充）。

处置（Phase 1 落地，防"架构改动无 ADR"）：

- **新增修订 ADR**（`docs/architecture/adr/adr-2026-10-user-agent-identity-layer.md`），
  并在 `docs/architecture/decisions.md` 索引登记，内容明确：
  ① 基底人设**仍保留**（用户角色是**叠加层**，不是替换）；② 用户显式设定 > 测评推断，`boundaries` 恒最高；
  ③ **安全信封不可覆盖**（护栏段后置、危机通道独立）；④ **无设定时逐字等于现状**；
  ⑤ 本决策**只适用于用户自定义角色这条链路**，不改测评画像与情绪回显的既有语义。
- 原 ADR **不删不改**，只加"已由 ADR-2026-10 修订 / 边界澄清"的指针。

---

## §6 与既有计划的关系

- **`legacy-plans/landed/prompt-pipeline-redesign.md`（已落地 / CL-04）**：本计划是它组装层的**一个段** ——
  它的 `§3.1` 分层图已预留"用户自定义 Agent 段"的位置，本文是其实现口径。
- **`legacy-plans/landed/chat-emotion-realtime-display.md`（已落地 / CL-07）**：情绪段与本段**并列**，段序见 §3.3。
- **`docs/plans/camera-preview-floating-window.md`（CL-11）**：**同页不同区** —— 它改对话页的**摄像头悬浮窗**
  （`[id].vue:214-215` 内联 `<video>`），本计划的切换器在 `composer-actions`（`:81-210`）。
  两者互不阻塞，但同属该页 ⇒ 并轨时须按 `parallel-contract §1.2`「同名文件串行」握手。
- **`conversation-memory-pending-decision-2026-09-24.md`（CL-08）**：跨会话记忆是另一层，本计划不做。

---

## §7 待拍板项台账

### 已收敛（旧版 §8 的 D1~D5）

| 旧编号 | 事项 | 现状 |
|---|---|---|
| D1 | 存储：扩展 `users.config` vs 新表 | ✅ **已定 `CL-D-21`**：新表 `user_agent_profile`（+ 会话绑定归 chat-svc） |
| D2 | 用户背景的隐私处理 | ✅ **已定 `CL-D-24`**：明文存 + 软删除 + 管理页可删（与消息正文同口径） |
| D3 | 开放度：受限槽位 vs 裸 system prompt | ✅ **已定**：受限槽位（写入 §3.5 ①） |
| D4 | 预设模板范围 | 🟡 **剩余**（不影响建模：`template_id` 可空） |
| D5 | 是否 v1 就做多角色 | ✅ **已定**：**做多角色**（用户 2026-10-13）；旧版"v1 单 Agent"作废 |

### 剩余（just-in-time，进阶段时逐条提出；已给建议）

| # | 事项 | 建议 |
|---|---|---|
| N1 | 预设模板范围（旧 D4） | v1 给 3~5 个（如"倾听者 / 温和鼓励 / 轻松闲聊"）+ 允许"纯自定义" |
| N2 | 「小E」的初始人设文案与音色 | 音色沿用现有默认 `anna`（女声）；**文案需用户过目**（属用户可感内容，执行侧不自拟） |
| N3 | ~~XTTS 回退下音色不变，UI 是否提示~~ | ✅ **已作废（2026-10-17）**：误判前提 —— 实际走云端（§3.6 第 4 条）；无"回退单音色"要提示 |
| N5 | **是否正式退役 XTTS**（compose 服务 / BFF 兜底 provider / `/tts/stream` / ai-svc 客户端） | 属**另立范围**；需与 `ADR-2026-10`"容器保留作离线回退"对齐 ⇒ 由用户裁 |
| N4 | 历史消息"谁回的"标注形态 | 建议轻量：同一角色连续消息只在**首条**标一次（参考 CL-07 用户裁定"消息角标不需要"的取向） |

---

## §8 调研依据（本次实读，不凭记忆）

- 后端：`emotion-echo-web-bff/internal/handler/{prompt_assembly.go,prompt_templates.go,ai_stream_handler.go,tts_handler.go,personality_directive.go,chat_handler.go}`、
  `internal/downstream/{tts_provider.go,xtts.go,ai.go}`、`internal/config/config.go`、`etc/web-bff.yaml`
- 服务与数据：`emotion-echo-chat-svc/internal/model/conversation.go`、
  `emotion-echo-chat-svc/migrations/{c004_add_intent_to_messages.sql,c006_add_file_name_to_messages.sql,c008_soft_delete_conversations.sql}`、
  `emotion-echo-user-svc/migrations/u003_add_user_config.sql`、`proto/{chat.proto,user.proto,agent.proto}`
- 前端：`app/pages/chat/conversation/[id].vue`、`app/pages/chat/setting.vue`、`app/layouts/nav.vue`、
  `app/composables/useTTSPlayer.ts`、`app/lib/apiRoutes.ts`
- 文档：`docs/closure-roadmap/{roadmap.md,decisions.md,RUNBOOK.md,parallel-contract.md}`、
  `docs/legacy-plans/landed/prompt-pipeline-redesign.md`、
  `docs/architecture/adr/adr-2026-09-personality-{prompt-injection,prompt-semantics}.md`
- 命令：`grep -rn` 全仓核对零足迹；`git ls-remote --heads origin` 核对无同名在飞分支
- **未能确认**（需运行栈）：「小E」首次物化时延、cloud 音色切换的真实可听差异、
  composer 多一个控件后在窄视口的换行行为（Phase 8 用 IAB 看）
