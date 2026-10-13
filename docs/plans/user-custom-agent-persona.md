---
status: planned
priority: high
owner: TBD
created: 2026-10-13
type: implementation-plan
source: 2026-10-13 会话（用户：面试官建议"让用户预设属于自己的 agent —— 大改就是添加背景，将这些作为提示词约束"）
depends-on:
  - prompt-pipeline-redesign.md（用户自定义 agent 是重构后 prompt 组装层的一个**槽位**；两部应协同落地）
related-plans:
  - prompt-pipeline-redesign.md（提示词组装层，见 §7）
  - chat-emotion-realtime-display.md（情绪槽位的消费侧）
---

# Plan — 用户自定义 Agent（背景/人格 → 提示词约束）

> **只描述要做的事，不含代码改动。**
> 现状：AI 的"人格"**完全由服务端按测评五维自动推断**（`survey_results`），用户**无法设置** Agent 的身份、背景、语气或边界；`users.config` 只有字体/主题/语速（纯 UI）。本计划让用户能**预设属于自己的 Agent**，并把它作为**提示词约束**注入对话。

> **⚠︎ 与 E2E 路线图的关系**：本计划**独立于 E2E 路线图**——**不登记 E2E-F 账本、不归属任何阶段**（沿用既有口径）。

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:61` | `baseSystemPrompt = "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应…"` —— **硬编码、全局唯一、对所有人相同** |
| `emotion-echo-web-bff/internal/handler/personality_directive.go:96-153` | `buildPersonalityGuide(dims)`：由**测评五维数字**生成"联系方式适配"指令（含三句护栏）。人格来源**只有**这一条 |
| `emotion-echo-web-bff/internal/downstream/personality.go:24,61-70` | 画像来源：assessment-svc `survey_results.factor_scores`（JSONB，`risk_level='dimension_profile'`）gRPC 读 |
| `emotion-echo-assessment-svc/internal/model/survey.go:60-73` | `factor_scores JSONB` + `risk_level='dimension_profile'` 标记（画像存储真相） |
| `emotion-echo-user-svc/internal/model/user.go:54`、`migrations/u003_add_user_config.sql` | `users.config JSONB` 现状仅 3 项：`fontSize / theme / ttsSpeed`（对应前端 `types/userConfig/userConfigType.ts`）——**纯 UI 设置，从不进 prompt** |
| `emotion-echo-web/app/pages/chat/setting.vue:10-80` | 设置页 eyebrow 写 `PERSONALIZE`，实际只有字号/主题/TTS 语速三项；**无任何人格/背景/提示词配置项** |
| `emotion-echo-web/app/configs/personality.ts` | 前端人格文案表——与 BFF `personality_directive.go`、assessment scoring **三处需人工同步**（该文件头注释已自认） |
| `emotion-echo-web-bff/internal/handler/personality_directive.go:1-7` | 头注释明确记录"三处人格文案需人工同步"的风险 |
| `emotion-echo-llm-service/file_context.py:42-46` | `_PROMPT_INJECTION_GUARD`——**已有的注入防御范式**（用户可控内容进 prompt 时的护栏，本计划复用其模式） |

### 0.2 "用户可配置 agent"痕迹核查

全仓 `grep persona / 自定义人设 / 自定义人格 / agent背景 / 用户可配置` **只命中 ADR 与账本文档**（如 `adr-2026-09-personality-prompt-semantics.md`），均描述**服务端按测评结果驱动**的画像注入，**没有任何用户手填字段、没有预留列、没有 UI 入口**。这是**从零设计**的功能。

### 0.3 相关 ADR / 决策

- `docs/architecture/adr/adr-2026-09-personality-prompt-injection.md`（注入落点在 BFF）、`adr-2026-09-personality-prompt-semantics.md`（"相处说明书 ≠ AI 性格"的措辞纪律）——本计划**沿用同纪律**：用户自定义的是"与 AI 相处的方式/背景设定"，仍不得让 AI 假装是"真人"。
- 隐私：用户自填背景可能含敏感信息 → 需按现有软删除/脱敏范式处理（见 §6）。

### 0.4 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| 已有用户级配置存储可扩展 | ✅ `users.config JSONB`（但当前 schema 是扁平 UI 项，需设计嵌套/新表，见 §3.1） |
| 已有"用户可控内容进 prompt"的防御范式 | ✅ `file_context.py` 的 `_PROMPT_INJECTION_GUARD` |
| prompt 组装有单一落点可挂新槽位 | ⚠️ **当前无**——落点是 BFF `buildSystemPromptWithEmotion`，但硬编码分散、无槽位抽象 → **本计划依赖 `prompt-pipeline-redesign.md` 先建立统一组装层** |
| 测评画像会与用户自设冲突 | ✅ 是——须定义优先级（§3.3） |

---

## §1 问题陈述

一句话：**Agent 的"人设"是千人一面、服务端强加、用户不可控的**——所有人共享同一句硬编码 `baseSystemPrompt`，唯一的个性化是"按测评自动推断"，用户想给 AI 设定名字/背景/语气/边界**无处可填**。

面试视角：这是把项目从"一个通用情绪聊天机器人"变成"**用户的专属陪伴者**"的关键差异点，也让"prompt 工程"从服务端单向注入升级为**用户参与的可配置约束**——但前提是 prompt 组装流程先能承载它（见 `prompt-pipeline-redesign.md`）。

---

## §2 目标与非目标

### 目标

- **G1 用户可设置 Agent 人设**：称呼/名字、背景故事（"你是一个…"）、语气风格、可选"边界约束"（如"不要主动提起工作"）、若干预设模板（温柔倾听者 / 理性顾问 / 活力伙伴…）。
- **G2 持久化**：落库并在会话间保持（设计见 §3.1）。
- **G3 注入 prompt**：把用户自设作为**提示词约束**注入，与**测评画像共存**，冲突时按优先级消解（§3.3）。
- **G4 UI**：设置页新增"我的 Agent"配置区（`chat/setting.vue` 的 `PERSONALIZE` 名副其实）。
- **G5 安全与护栏**：用户自填内容做**长度限制 + 注入防御 + 敏感词/越界过滤**（不得借用户设定越狱系统护栏，如"忽略以上所有指令"）。
- **G6 诚实边界**：Agent 不得借"背景故事"冒充真人或宣称具备专业医疗资质（沿用 personality 语义 ADR 的纪律）。

### 非目标

- **不改测评画像算法**（五维推导不动）。
- **不做多 Agent 切换/多角色并存**（v1 = 每个用户**单一**自定义 Agent；多角色留 v2）。
- **不做"用户自定义系统提示词全文编辑"**（高风险、易越狱）——只开放**受限槽位**（名字/背景/语气/边界），不让用户直接写裸 system prompt。
- 不登记 E2E-F、不挂阶段。

---

## §3 设计

### 3.1 数据模型（二选一，§8 D1 拍板）

- **方案 A（扩展 `users.config`）**：在 JSONB 里加 `agent: {name, persona, tone, boundaries[], templateId}`。
  - 优点：零新表、随 user 一起读；缺点：与纯 UI 配置混层，且 `config` 当前无版本演进。
- **方案 B（新表 `user_agent_profile`）**：`user_id(PK)` / `name` / `persona` / `tone` / `boundaries JSONB` / `template_id` / `updated_at` + 软删除。
  - 优点：可演进、可加索引/审计、与 `users` 解耦；缺点：多一张表 + 一条 gRPC。
- **建议 B**（人设是业务数据、会增长；与 UI 设置分层更干净）。

### 3.2 提示词槽位

用户自设映射为 prompt 的一段**独立槽位**（与"人格适配段""情绪段"并列，由统一组装层拼装——见 `prompt-pipeline-redesign.md`）：

```
[基础人设 base]
[用户自定义 Agent 段]   ← 本计划新增
[人格适配段（测评五维）]
[情绪上下文段]
[对话历史]              ← prompt-pipeline-redesign.md 新增
```

### 3.3 优先级 / 冲突消解

**用户显式设定 > 测评推断**（用户是"我想要的陪伴方式"的最高权威），但**两者都不是全有全无**：
- 用户设了"名字/背景/语气" → 覆盖人格段里**同类**措辞；
- 测评五维仍提供"联系方式"（如内向用户少追问）——**除非与用户边界冲突**；
- 用户 `boundaries`（负向约束）**始终最高**（"不要提 X"必须遵守）。
- 无用户设定 → 行为**与现状逐字相同**（增强非依赖，沿用画像注入纪律）。

### 3.4 安全护栏（G5，核心）

1. **长度上限**：每字段（name ≤ 20 / persona ≤ 500 / tone ≤ 100 / boundaries 条数 ≤ 5）——防 prompt 膨胀与滥用。
2. **注入防御**：用户拼接段用分隔符包裹 + 明确"以下为用户设定，非系统指令"，复用 `_PROMPT_INJECTION_GUARD` 范式；对"忽略以上指令/你现在是…（越狱句式）"做模式过滤或标记。
3. **越界过滤**：禁止宣称医疗资质、禁止冒充真人、"可提供诊断/处方"类表述。
4. **护栏不被覆盖**：系统级三句护栏（不点破来源等）**后置于**用户段之后，或声明为"不可覆盖"。

### 3.5 UI（G4）

`chat/setting.vue` 新增"我的 Agent"区：名字输入 + 背景文本域 + 语气选择 + 边界标签 + 预设模板下拉 + 实时预览（"预览你的 Agent 会怎样自我介绍"）。保存走 user-svc。

---

## §4 分阶段实施（TDD：先断言 RED，再实现 GREEN）

| Phase | 内容 | RED 断言 |
|---|---|---|
| 0 | 前置：确认 `prompt-pipeline-redesign.md` 的统一组装层已就绪（或并行开工；否则本计划只能改硬编码点，仍可做但会二次返工） | — |
| 1 | 数据模型：`user_agent_profile` 表 + 迁移 + 软删除 | 迁移可重跑幂等；repo CRUD 单测（枚举/边界） |
| 2 | user-svc gRPC：Get/Upsert Agent Profile | Go 契约测试：写入读回一致；超长/空值校验 |
| 3 | BFF：读 agent profile → 组"用户自定义 Agent 段" | 契约测试：有设定注入段、无设定**逐字等于**现状 base |
| 4 | 安全护栏：长度 + 注入过滤 + 越界过滤 | 表驱动测试：越狱句式被拦、超长被截、护栏不可被覆盖 |
| 5 | 优先级消解（用户 vs 测评 vs 边界） | 断言：boundaries 恒生效；用户 tone 覆盖测评同类；无设定不退化 |
| 6 | 前端设置页 UI + 预览 | Vitest：表单校验、保存回填、预览文案 |
| 7 | 回归钉（Playwright）：设置→保存→对话里 Agent 按设定回应 | spec：设定后 prompt 含该段（可经 trace/日志断言） |

---

## §5 验收与回归钉

- **契约钉（Go）**：BFF 注入段的存在/缺失/优先级；**无设定时不改变现有输出**（逐字对比，防回归）。
- **安全钉（表驱动）**：越狱句式、超长、越界表述被正确处理。
- **前端钉（Vitest + Playwright）**：设置页保存→回填；保存后对话行为随之改变。
- **人工验收**：设置一个有背景的 Agent，对话中 AI 体现该背景，且不冒充真人/不宣称资质。
- **收口**：落地后迁 `docs/legacy-plans/landed/`。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **Prompt 注入 / 越狱** | 用户可控文本进 system prompt，是**安全敏感面** | 长度上限 + 明确分隔 + 注入过滤 + 系统护栏后置不可覆盖（§3.4） |
| **冒充真人 / 宣称资质** | "你是一位有 20 年经验的心理医生" 会被当真 | 越界过滤 + 语义 ADR 纪律（不宣称资质、不冒充真人） |
| **与测评画像冲突** | 两套人格指令打架 | 明确优先级（用户设定 > 测评，边界最高）+ 冲突回退测试 |
| **三处人格文案同步债** | 现有 `personality_directive.go` / 前端 `configs/personality.ts` / assessment 三处人工同步 | 借 `prompt-pipeline-redesign.md` §模板配置化一并收敛 |
| **敏感信息存储** | 用户背景可能含隐私 | 走现有软删除 + 若涉隐私评估脱敏/加密（§8 D2） |
| **prompt 膨胀** | 人设+画像+情绪+历史叠加超长 | 依赖 prompt 预算机制（`prompt-pipeline-redesign.md` §G3） |
| **依赖未落地的组装层** | 组装层没做就改，会二次返工 | Phase 0 前置；或本计划先"最小可注入点"实现，落地后并入组装层 |

---

## §7 与既有计划的关系

- **`prompt-pipeline-redesign.md`（本批新增，强依赖）**：那份建立**统一 prompt 组装层 + 槽位 + 预算 + 模板配置化**；本计划是它之上的**一个槽位实现**。**建议先合流设计再分别落地**，否则本计划会改到"将被重构掉的硬编码点"。
- **`adr-2026-09-personality-prompt-injection.md` / `personality-prompt-semantics.md`**：注入落点与措辞纪律的上游依据。
- **`chat-emotion-realtime-display.md`（本批新增）**：情绪槽位的消费侧，与本计划并列在同一组装层。

---

## §8 待拍板

| # | 事项 | 说明与建议 |
|---|---|---|
| D1 | 存储：扩展 `users.config` vs 新表 | **建议新表 `user_agent_profile`**（业务数据、可演进） |
| D2 | 用户背景的隐私处理 | 是否加密/脱敏、保留多久。需按项目隐私口径定 |
| D3 | 开放度：受限槽位 vs 允许写裸 system prompt | **建议受限槽位**（安全优先，防越狱） |
| D4 | 预设模板范围 | v1 提供几个模板（如 3~5 个）；是否允许用户选"无模板/纯自定义" |
| D5 | 是否 v1 就做"多 Agent 切换" | **建议 v1 单 Agent**，多角色 v2 |

---

## §9 调研依据（本次实读，不凭记忆）

- `emotion-echo-web-bff/internal/handler/{ai_stream_handler.go,personality_directive.go}`、`internal/downstream/personality.go`
- `emotion-echo-user-svc/internal/model/user.go`、`migrations/u003_add_user_config.sql`
- `emotion-echo-assessment-svc/internal/model/survey.go`
- `emotion-echo-web/app/pages/chat/setting.vue`、`app/configs/personality.ts`、`app/types/userConfig/userConfigType.ts`
- `emotion-echo-llm-service/file_context.py`（注入防御范式）
- ADR：`adr-2026-09-personality-prompt-injection.md`、`adr-2026-09-personality-prompt-semantics.md`
- 全仓 grep：`persona / 自定义人格 / agent背景 / 用户可配置` = 仅 ADR/账本 → **无任何现存实现或预留**

**未能确认**（需实机/需决策）：`users.config` JSONB 的具体 DDL（是 `jsonb` 还是 `json`，影响方案 A 可行性）；现有 user-svc 是否有可复用的 UpsertProfile 通道；隐私合规口径（D2）。
