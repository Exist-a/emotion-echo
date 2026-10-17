---
stage: cl-06
type: execution-report
status: in-progress
created: 2026-10-17
plan: plan.md
---

# CL-06 · 用户自定义 Agent（多角色）— 执行记录

> 体例（RUNBOOK §0 / §6.3）：起点 / 实施 / 证据 / 结论 / 经验。**执行中与收口时同步更新**。
> ⚠️ **阶段状态由审计 / 第二方判定，执行者不自行宣布 `done`**。
> 本报告写于 2026-10-17，阶段状态 **`partial`** —— 21 个测试点里 18 个有结论，
> **3 个待用户裁定**（T-18 音色听觉 / T-19 历史标注形态 / T-21 「小E」文案），
> 见 §4「本轮边界」。

## §1 起点（建档时亲核的现状）

建档轮亲核（plan §A，2026-10-17）推翻/纠正了三处**写计划时容易想当然**的前提：

1. **`proto/agent.proto` 不是用户角色契约**（它是 assessment-svc 的，`package emotion_assessment.v1`）
   ⇒ 角色契约落 `user.proto`，本阶段全程守住；
2. **CL-F-28（web 镜像构建失败）真因是两段**，不止账本原记的 `npm ERESOLVE`
   —— 第二段是"改用权威 pnpm 锁后依赖升到 nuxt@4.5.2，需要 Node 22"（详见 §3.1）；
3. **XTTS 已不是使用路径**（用户口径 + `.env.local` 实测 key 已配 ⇒ `auto` 生效为 cloud）
   ⇒ 音色验收**可行**，且必须按 cloud 口径做（旧计划书写的"dev 走 XTTS ⇒ 音色验不了"作废）。

开工前已登记：分支 + **draft PR #255** + 占位登记小 PR #256（已合 main `45ff19e1`）。

## §2 实施进度

| Phase | 状态（2026-10-17） | 测试点 | 备注 |
|---|---|---|---|
| **0** 修 `CL-F-28` + 守卫去假绿并接 CI | ✅ 完成 | — | §3.1（含**真构建**证据：BUILD_EXIT=0 / node v22 / curl 200） |
| **1** 修订 ADR（决策 44 用户角色身份层） | ✅ 完成 | — | §3.2 |
| **2a** user-svc 迁移 + 模型 + 仓储 | ✅ 完成 | T-1 | §3.3 |
| **2b** user-svc 服务层 + `user.proto` 3 RPC | ✅ 完成 | T-2/T-3/T-5 | §3.4 |
| **3a/3b** `chat.proto` + 迁移 + chat-svc 透传 | ✅ 完成 | T-1/T-6 | §3.5 |
| **4** BFF：角色端点 + 组装层 `agent`/`guard` 段 + 消息快照 | ✅ 完成 | T-7/T-8/T-9 | §3.6（5 个 commit） |
| **5** 安全护栏（shared 词表 + 服务端强制 + BFF 双保险） | ✅ 完成 | T-11 | §3.7 |
| **6** 音色（`/tts/phonemes` 每请求 `voice`） | ✅ 完成 | T-12/T-13 | §3.8 |
| **7** 前端（管理页 + 切换器 + 路由登记 + 音色接线） | ✅ 完成 | T-14/T-15/T-16/T-17 | §3.9（4 个 commit） |
| **8** 运行态验收（生产构建 + IAB + Playwright） | ✅ 完成（除 3 个待裁项） | T-15~T-20 | §3.10（**抓出 2 个真缺陷**） |
| **9** 收口 | 🟡 **进行中** | — | 本报告；待第二方核对回执 + 用户对 3 项裁定 |

## §3 证据

### §3.1 Phase 0 · `CL-F-28` 真修（含"不修则后续全是失真证据"的论证）

- **失败两段**（原始命令 + 退出码）：① `npm install` → `ERESOLVE`；② 改 pnpm 后
  `pnpm run build` → `TypeError: trustedFunctions.difference is not a function`（Node 20 缺 `Set.prototype.difference`）。
- **修法**：`emotion-echo-web/Dockerfile` 改 pnpm + `--frozen-lockfile`，基础镜像 `node:20-alpine → node:22-alpine`。
- **真构建证据**：`BUILD_EXIT=0`（日志重定向到文件后 `echo $?`，**不用 `| tail`** —— 那会吃掉退出码，本阶段踩过）、
  镜像内 `node -v` = `v22.23.3`、容器起来后 `curl` 页面 **200**。
- **守卫去假绿**：`scripts/test_web_dockerfile_p0r28_r29.py` 原先**命中注释**假绿（断言 `npm ci` 命中的是注释），
  已加 `strip_comments()` + 6 条负向自检（含"满足条件写在注释里必须 FAIL"），并**接进 CI**（`doc-drift-check.yml` step 19）。
- 连带处理：删 `package-lock.json`（CL-F-42）、`packageManager` 修为完整 semver `pnpm@9.15.1`（CL-F-41，实测**推翻**了
  我原先"会与 CI version 冲突"的担心）、`pnpm build` 进 CI（CL-F-43）。

### §3.2 Phase 1 · ADR

`docs/architecture/adr/adr-2026-10-user-agent-identity-layer.md` + `architecture/decisions.md` 决策 44 登记。
理由：`adr-2026-09-personality-prompt-semantics` 决策 1 明令"基底人设一字不改、不做 AI 性格"，
本阶段让用户改 AI 的名字/身份 ⇒ **必须**有修订 ADR（不能只写在计划里）。

### §3.3~§3.5 后端数据面（T-1/T-2/T-3/T-5/T-6）

**T-1 迁移幂等 → PASS**
```
$ sh deploy/db/migrate.sh   # 第 1 次
[migrate]   OK  emotion-echo-user-svc/u004_create_user_agent_profile.sql（1000ms）
[migrate] 全部迁移应用完成，共 39 个文件（版本追踪已启用，幂等可重复执行）
$ sh deploy/db/migrate.sh   # 第 2 次（幂等复跑）
[migrate]   SKIP emotion-echo-chat-svc/c012_add_agent_id_to_conversations.sql（已应用，checksum 一致）
[migrate]   SKIP emotion-echo-chat-svc/c013_add_agent_id_to_messages.sql（已应用，checksum 一致）
[migrate]   SKIP emotion-echo-user-svc/u004_create_user_agent_profile.sql（已应用，checksum 一致）
```
证据：`evidence/phase8-db-evidence.txt`（`user_agent_profile` 行 + `is_default` + `deleted_at` 实测）。

**T-2 / T-5 → PASS**（经**真实网关** :19080 的 API 验收，`evidence/phase8-api-check-output.txt`）：
`T-5 首次读列表物化默认「小E」PASS`（id=1 / isDefault=true / voice=anna）；
`T-2b 人设逐字回读一致 PASS`（中文往返无损坏 —— 首版用 curl 内联中文把 payload 编码搞坏，
已改为 Python 构造 UTF-8 请求体，这条也在脚本头注明）；
`T-2e/f/g` 超长/空被拒（**拒绝**而非静默截断）。

**T-3 每户 ≤5 → PASS**（同上：`当前角色数 = 5（含默认）` + `第 6 个被拒`）。
⚠️ **并发（TOCTOU）未验证**，见 §4 边界。

**T-6 会话切角色 → PASS**：`T-6a` 200 且回显、`T-6b` 越权/不存在 400 且**不下达下游**、
`T-6c` 列表回读一致（**这条最初是 FAIL** —— 见 §3.10 真缺陷 1）。

### §3.6 Phase 4 · BFF（T-7/T-8/T-9）

- **T-8 零回归（PASS，含突变）**：`prompt_assembly_agent_test.go` 三条 golden（无信号 / 有画像 / 画像+情绪）
  断言 `assembleSystemPrompt` **逐字等于** CL-04 的基线常量且**不含**护栏段。
  突变：把 `if hasSegment(segs, segmentKindAgent)` 改成 `if true`（guard 恒在）⇒ 立刻红，
  diff 明确显示无角色时多出 `\n\n【护栏】…`。
- **T-9 段序（PASS，含突变）**：位置断言（guard index > agent index 且 = 末位）。
  突变：把 guard 插到段首 ⇒ `"0" is not greater than "2"` 红。
- **T-7 消息快照（PASS）**：DB 实测三行并排（`evidence/phase8-db-evidence.txt` 与 UI 复验一致）：
  ⚠️ **精确口径（第二方核对指出我先前的写法偏松）**：消息上的 `agent_id` = **本轮请求携带、并经
  `resolveAgentProfile` 在自己的角色列表里校验过的那个角色**；它**不一定**等于"落库那一刻
  `conversations.agent_id` 的值"（多标签页/并发切换时会不一致 —— 核对者实测到 957/958 与 710/711 就不同）。
  语义上这正是"这条回复由谁产生"的快照，故我保留该实现，但把计划书里"（= 写库时该会话的当前角色）"
  的说法订正为上述精确表述。另：chat-svc **不强制**"只有 assistant 可带角色"，那是调用方纪律（BFF 侧已清零用户消息的该字段）。
  `960 assistant agent_id=1(小E)` → 切换角色 → `961 user agent_id=NULL`（**用户消息不盖戳**）→
  `962 assistant agent_id=8(06计阿哲)`。**切换只影响后续消息**由这一组数据直接证明。

### §3.7 Phase 5 · 安全护栏（T-11 → PASS）

- 词表落 `shared/pkg/agentguard`（user-svc 权威 + BFF 二次，**同一份**；BFF 侧有静态钉禁止自带副本）。
- 三类规则：越狱（覆盖指令/索要系统提示/自称无限制）、宣称资质、冒充真人。
- **误拦 vs 漏拦的取舍写进代码与测试正例**："你现在是""我是你""真人""心理咨询师"**不做裸词拦截**
  （人设里"你现在是倾听者""我是你忠实的倾听者""像个心理咨询师那样听我说话"都是正常写法），
  改用**形态正则**。测试同时钉 12 条正例 + 22 条负例。
- 运行态（API）：`越狱/宣称资质/冒充真人` 三类各 400 且注明类别；**正例放行**（`像心理咨询师那样` 200）。
- 突变 ×2：① 停用子串匹配 ⇒ shared 测试红；② BFF 绕过检查 ⇒ 越狱人设被 200 放行并落到下游（实测出真后果）。
- 🔴 **第二方核对抓出的实质缺陷（已修，§6.3 有完整回执）**：我先前的实现用"把连续空白**折叠成一个空格**"
  再 `strings.Contains` —— 而词表项是**连续文本**（"忽略以上"）⇒ **插入任意一个空格即绕过**：
  核对者用 `忽略 以上所有指令` 实测穿过，全角空格 / 零宽 U+200B / 制表符 / 全角 ASCII 变体同样穿过。
  我当时在包注释里写"匹配对空白不敏感"，属**声明与行为不符**。
  修法：`normalizeForMatch` = 全角 ASCII 折半角 + ASCII 折小写 + **删除所有空白与零宽字符**，
  且**词表项与文本同口径归一**；核对者给的全部 11 条绕过样本已作为负例进测试。
  ⚠️ **残余（仍不得读作"黑名单完备"）**：这是**规则式**过滤，改字符/插标点（如"忽-略以上"）等
  非空白变体仍可能绕过 ⇒ 它挡的是"手滑与常见显式越狱"，不是对抗性完整解（计划亦如此定位：LLM 审核留作增强）。

### §3.8 Phase 6 · 音色（T-12/T-13 → PASS）

- `TTSPhonemesReq.Voice`（`omitempty`）+ `resolveVoice(配置默认, 请求值)`（空/纯空白回落）。
- 零回归：**不带 voice 时请求体不出现该键**（有测试断言 Marshal 结果不含 `voice`）。
- 运行态（`evidence/phase8-api-check-output.txt`）：带 voice 与不带 voice 两次都返回
  `phonemes=10 / duration>0`（口型链未回归）；同一文本换音色 ⇒ **音频 MD5 不同**
  （`anna=44d3bf0b…` / `alex=2359c1e0…`，`evidence/phase8-tts-voice-*.wav`）。
- 突变：`Voice: resolveVoice(...)` 改回 `p.opts.Voice` ⇒ 钉子红（`expected …:alex, actual …:anna`）。

### §3.9 Phase 7 · 前端（T-14~T-17）

- **接线契约测试**（`cl06-agent-wiring.test.ts`，15 例）逐条对应一条真实链路：
  `agentId` 从页面 → sender → `AIStreamParams` → 请求体；切换器**排在 voice-btn 之前**（index 断言）；
  音色逐层透传（manager → dhTTS → playStream → fetchPhonemes）且空值不发键；进入页面拉角色列表。
- **T-14 路由登记 → PASS**：`apiRoutes.test.ts`（扫全仓字面量 ⊆ API_ROUTES）+ BFF `wantRoutes`/`knownPathPrefixes`
  同步（新路由不进白名单会被两条防漂移断言拦下，实测过）。
- **T-15/T-16 运行态 → PASS**（§3.10）；**T-17 → PASS**（§3.10 人设 A/B）。

### §3.10 Phase 8 · 运行态验收（**抓出 2 个真缺陷**）

**环境**：按 §8.1 条款 10 取 `deploy/.devmode-session` 锁 + 在 parallel-contract §3.1 登记；
本轨镜像带后缀 `:cl06`（user-svc / chat-svc / web-bff / web），**不覆盖公共 tag**；4 个服务重建 + 迁移到位。
（过程中一个**既有**环境问题：analytics-svc 长跑 8h 后 `dbOk=false` ⇒ 其 `/health` degraded ⇒
`seed.sh` 因 BFF readiness 不健康而中止；`docker restart` 即愈 —— 已登账 **CL-F-47**。）

**缺陷 1（已修）：会话列表回读 `agentId` 恒 0** —— `evidence/phase8-api-check-output.txt` 首轮 `T-6c FAIL`。
库里写对（`agent_id=3`）、接口恒回 0 ⇒ 根因是**视图投影点漏赋值**（`types.ConversationView` 有字段、
proto 映射也带上了，但 `ListConversationsLogic` / `CreateConversationLogic` 手工构造视图时没赋）。
修复 commit `4750908c`（+ 同函数幂等重发路径的 4 个字段一起补齐）。**这是"各层单测分别绿、端到端才暴露"
的典型**，也是 Phase 8 存在的意义。

**缺陷 2（已修）：角色页 404** —— 生产构建实测 `/chat/agents` 返回 Nuxt 404。
根因：`app/router.options.ts` 用**手写路由表覆盖**了 Nuxt 自动路由（`routes: (_routes) => [...]` 忽略入参）
⇒ 本仓"文件系统即路由"**不成立**，新增页面文件静默 404 且无任何门禁能发现。
修复 commit `7a128029` + **新增门禁** `app/router.options.contract.test.ts`（扫 pages 全量与 router.options 比对，
反向自检 + 突变验证都做过）+ 账本 **CL-F-46**（如实定性"已缓解未根治"）。

**缺陷 3/4（Phase 9 整改进程中、在真栈上又抓到 2 个，均已修）**：
- **咨询锁参数溢出**：`pg_advisory_xact_lock(7751467009, 1)` 报
  `unable to encode ... into binary format for int4`（双参版本两参数都须 int4）⇒ 角色接口**全部 500**。
  改单参 bigint 形式（`类<<32 | userID`）。
- **配额类错误落 500**：`MapGRPCError` 没有 `codes.ResourceExhausted` 分支 ⇒ 落到 default ⇒
  用户看到"内部错误"而事实是"已达上限"。改映射 **409**（刻意非 429：前端对 429 会自动重试，
  而配额是确定性结果）。
⇒ 两个都是"**InMemory 替身与 Postgres 语义不同**"因此单测永远测不到的形态 —— 与 §5.1 同一条教训。

**T-15 管理页 → PASS**：IAB 实读快照渲染出角色卡（含「默认」徽标 / 人设 / 语气 / 音色名）与「新建角色」入口；
Playwright 钉住"不是 404 页 + 列表存在"。

**T-16 切换器位置/形态 → PASS**（用户 2026-10-17 两次澄清的要求，**量化为 DOM 几何**）：
```
composer 子元素顺序: [附件, 摄像头, spacer, agent-switcher, voice-btn, send]
switcherRight(1091) ≤ voiceLeft(1127)   ⇒ 在语音按钮左侧 ✓
展开后 menuBottom(613) ≤ triggerTop(621) ⇒ 向上展开 ✓
options: [小E(默认), 06计阿哲, …]；含「管理角色」入口 ✓
```
截图：`evidence/phase8-iab-switcher-upward.png`（同图可见切换器紧邻语音按钮左侧 + 菜单向上展开）。
UI 切换后 DB 复验：`conversations.agent_id=8`（06计阿哲）✓。

**T-17 角色差异可辨 → PASS（口径见下）**：用一个"每条回复必须以『彩虹小马』开头"的角色，
同问题对照（`evidence/phase8-persona-ab.txt`）：
```
[A] 挂『彩虹小马』角色 ⇒ 「彩虹小马～我叫06计标记，你可以随便叫我06。…」
[B] 切回默认（agentId=0）⇒ 「我叫小E，就是那个随时在线的陪伴者～…」（不含该标记）
```
⇒ 证明**人设真进了 prompt**且**随角色进退**。⚠️ **口径如实**：这证明的是"注入生效 + 可区分"，
**不是**"两个自然风格角色对同一问题会给出风格明显不同的回答"（后者依赖模型行为，本阶段未做风格评分）。

**T-20 负向对照 → PASS**：上述 [B] 侧 + 单测（无档案/未装配 ⇒ `system` 逐字等于 `goldenBase` 且不盖戳）。

**Playwright 回归钉**：`e2e/cl-06-custom-agent.spec.ts` **2 passed**（`evidence/phase8-playwright-nail.txt`）。

**本阶段对演示账号做的测试数据已清理**（删除 3 个 `06计*` 测试角色，账号回到只有默认「小E」）。

## §4 结论与本轮边界（**不自行宣布 done**）

### 4.1 测试点结论（21 点）

| 判定 | 测试点 |
|---|---|
| **PASS（18）** | T-1, T-2, T-3, T-4, T-5, T-6, T-7, T-8, T-9, T-10, T-11, T-12, T-13, T-14, T-15, T-16, T-17, T-20 |
| **BLOCKED（2，待用户裁定）** | **T-18**（换音色**听起来**是否确实是不同的人 —— 机器已证音频不同，听觉只有耳朵能判）、**T-21**（「小E」初始文案与音色，属用户可感内容，执行侧不自拟） |
| **partial（1，形态待裁）** | **T-19**（历史消息"谁回的"标注 —— **数据链路已 PASS**：每条 AI 消息带 `agentId` 且实测正确；**标注形态**属 `N4` 待裁） |

### 4.2 必须写明的边界与未验证项（**不得读作"全覆盖"**）

1. **`≤5` 的并发（TOCTOU）—— 核对确认存在，已修并取得运行态证据**：
   原实现 `CountActive()` → 判上限 → `Create()` 是两次独立往返、无事务无行锁、DDL 无 CHECK/UNIQUE
   ⇒ 并发可双双读到 `active=4` 各插一条。修法（双层）：仓储新增**原子**方法
   `CreateWithActiveLimit` / `CreateIfNoHistory`（Postgres 侧同事务内先取**每用户咨询锁**再复核+插入；
   InMemory 侧单锁等价），服务层改走它们；DB 层再补迁移 `u005` 的唯一部分索引
   （`(user_id) WHERE deleted_at IS NULL AND is_default`）⇒ 默认角色每户至多一条，任何未来写入路径都挡得住。
   **运行态证据**：`evidence/phase9-concurrency-live.txt` —— 并发 10 个新建 ⇒ `200=5, 其余=[409]`、活跃数 = 5。
   **方法学教训**：并发**行为**测试在 InMemory 替身上**不构成证明**（实测：把实现改回旧写法它仍 PASS）
   ⇒ 另加**确定性接线钉**（断言走的是原子方法），突变后精确红。
2. **音色在 XTTS 降级窗口内回落**（local provider 无这 8 个音色）：主链路是 cloud（计划 §A.5），
   该窗口**不被本阶段音色验收覆盖**。
3. **T-17 的口径**见 §3.10（注入生效 ≠ 风格评分）。
4. **T-10 的"覆盖"是 prompt 指令级**，不是代码级强制：测评段仍在 prompt 里，优先级靠一句显式指令消解。
5. **`agentguard` 是规则式黑名单**（v1 按计划不含 LLM 审核）：能挡保存期的显式越狱，
   **挡不住**用户在**日常聊天输入**里写同类文本（那属危机通道 CL-08e 的另一面）。
6. **CL-F-45 残余风险**：`/tts/phonemes` 的 `voice` **未做白名单**（客户端可传任意 `model:voice`），
   已登账归 CL-12（本阶段计划只授权"请求值覆盖 + 回落口径"）。

### 4.3 本轮状态与下一步

- **阶段状态 = `partial`**（3 个待裁项 + 第二方核对回执待回）：不改 roadmap 为 `done`。
- **PR #255 保持 draft**：收口纪律是"收口时转 ready → 串行合并"，而收口需用户对 T-18/T-19/T-21 裁定。
- 待用户：① 听两个样音（`evidence/phase8-tts-voice-*.wav`）判 T-18；② 过目「小E」文案/音色定 T-21；
  ③ 定 T-19 的标注形态（建议：同一角色连续消息只标首条）。

## §5 经验（可复用）

1. **端到端验收不可被单测替代**：本轮 2 个真缺陷（列表回读恒 0、页面 404）**全部**是"各层单测绿、
   端到端才暴露"的形态。前者是"投影点漏字段"（字段加了、构造视图时没 copy），
   后者是"手写路由表覆盖框架约定"。二者的共同点：**没有任何一层为"它整体是否连通"负责**。
2. **定位手法（值得留档）**：现象是前端 404 时，① 先在页面里 `fetch(location.href)` 拿 **SSR 真实响应**，
   一下把范围从"客户端路由"收窄到"服务端路由表"；② 把镜像里的产物拷出来**比对两张路由表**；
   ③ **本地跑同一 build** 复现 ⇒ 排除 Docker 缓存方向。若跳过 ③，很可能一路往"Docker 构建缓存"查错。
3. **守卫会写错，必须反向自检**：本轮新写的路由守卫第一版把比对路径写成 `~/pages/${rel}`（应为 `~/${rel}`）
   ⇒ **恒不匹配**；是"修好问题后守卫仍红"这个反常信号暴露了它。⇒ 新守卫必须做一次"故意破坏"演练。
4. **管道会吃掉退出码**：`docker build ... | tail` 曾让我把一个**失败**的构建当成成功
   （容器还用旧镜像被重建）。凡影响结论的命令，退出码必须单独 `echo`。
5. **含中文的请求体不要用 curl 内联**（Git Bash 下编码损坏 ⇒ 会把"输入坏了"误读成服务问题）——
   改 Python/Node 构造 UTF-8 body；这条在本阶段被再次验证（首轮 API 验收造出一条乱码角色）。
6. **"加了字段"≠"通了"**：本阶段同一类缺陷出现 3 次（BFF `ConversationView`/`MessageView` 投影、
   chat-svc 两处视图构造、幂等重发路径）。⇒ 凡给已有结构加字段，**必须端到端读回一次**。

## §6 收口记录

- 2026-10-17：Phase 0~8 完成；本报告落盘；`deploy/.devmode-session` 释放（见 §6.2）。
### §6.1 dev 栈释放

- 2026-10-17：取锁窗口结束，`deploy/.devmode-session` 已删（栈仍开着，跑的是本轨 `:cl06` 镜像的
  user-svc / chat-svc / web-bff / web；analytics 等其余服务为公共镜像 —— 已在 parallel-contract §3.1 交代）。

### §6.2 本轮自检（AGENTS §2.5 三连）

见 §6.4。

### §6.3 第二方独立核对回执（2026-10-17）

核对者按 A~K 十一项逐条独立验证（自读代码 + 自跑命令 + **自己做突变**），结论摘要：

| 项 | 结论 |
|---|---|
| A 零回归 T-8 | **成立**（并自行做了 2 次突变：guard 恒在 / 改段文案 ⇒ 均如期变红，恢复后 md5 一致） |
| B 段序 T-9 | **成立**（并验证 guard 恒为末位） |
| C 内容黑名单 T-11 | **部分成立 → 已修**（空白折叠实现可被**一个空格**绕过；`gender` 未校验却声称白名单） |
| D 角色归属 IDOR | **部分成立 → 已修**（非流式 `sendMessage` 把客户端 `agentId` 原样透传且无归属校验） |
| E 每户 ≤5 并发 | **部分成立 → 已修**（TOCTOU 真实存在；无 CHECK/UNIQUE/事务/行锁） |
| F 迁移幂等 T-1 | **成立**（三条全是可重跑语句） |
| G 角色快照 T-7 | **成立**（附口径订正，见 §3.6） |
| H 音色 T-12/13 | **成立**（自行突变验证；另确认无路径会把 voice 变空串） |
| I 投影漏字段残留 | **部分成立 → 已修/已记账**（发现 BFF HTTP 传输层的 `agentId` ↔ chat-svc `agent_id` tag 不一致 ⇒ `CHAT_TRANSPORT=http` 时静默丢字段；生产默认 gRPC ⇒ 归 **CL-F-48**） |
| J 前端路由 | **成立**（自行摘掉注册演练，确认门禁变红） |
| K 证据有效性 | **部分成立 → 已订正**（api-check 的 T-7 只断言 `[DONE]`；一份早期证据文件中文字节损坏 —— 已删除该文件并在 §5 记录教训） |

**该核对的价值（如实记）**：它把"我声称的能力"与"代码的真实行为"分开了 —— C/E/D/I 四处都是我
**以为做到了**而实际没做到/没做全的。若无这一轮，阶段报告会以"含一条夸大的安全声明"的状态收口。

**核对者明确指出、我**未**修改的项（不是缺陷，是口径）**：
① 消息 `agent_id` 的语义（见 §3.6 订正）；② `agentguard` 是规则式过滤、非对抗性完整解（§3.7 已写明）。

### §6.4 本轮状态

- 阶段 = **`partial`**（3 项待用户裁定，见 §4.3）；PR #255 保持 **draft**。
- 测试数据已清理（演示账号恢复为"只有默认「小E」"）。
