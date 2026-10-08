---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-18 (新增 D-05~D-08：补救期技术决策，参考业界常见做法定案)
type: e2e-transformation-decisions
---

# 改造项决策记录（transformation decisions）

> 建档预探查发现 3 个"设计意图与实现不符"的改造项。此类必须**实测前先对齐方向**，否则会修错方向。
> 每项记录：现状 → 候选方案 → 决议 → 影响面。
>
> **D-05~D-08（2026-09-18 新增）**：补救期技术决策，由用户授权"参考常见做法自行定案"。每项写明**依据的通用实践**，便于日后复核该依据是否仍成立。

---

## D-01 找回密码方式（归属 E2E-04，字段归 E2E-03）

### 现状

- 项目已从"手机号登录"改为"用户名登录"（Sprint 112 已把 UI 文案改成 username-only，有架构测试锁死不能再出现"手机号/邮箱"字样：`web/app/pages/login/forget/username-only-copy.architecture.test.ts`）
- 但流程本身仍是手机号短信时代的三步向导（`verify.vue` → `modify.vue` → `success.vue`）
- 验证码**无真实投递渠道**：`bff/internal/handler/auth_handler.go` 的 `verificationCode()` 只把码存进 in-memory map；dev 模式靠 `BFF_DEV_RETURN_CODE=1` 直接把码回显在响应里
- **基础设施可用性**：`emotion_echo_user.users` 表**已经有 `phone VARCHAR(20) UNIQUE` 和 `email VARCHAR(128) UNIQUE` 两列**（`deploy/db/02-create-tables-in-schemas.sql:10-11`），只是从未作为凭据使用

### 同类项目做法（检索结论）

| 做法 | 说明 | 适用场景 |
|------|------|---------|
| 手机验证码 | 行业主流（高校毕设系统、邮箱服务均如此） | 有短信服务商接入 |
| 备用邮箱重置链接 | 次主流 | 有邮件服务 |
| **密保问题（安全提示问题）** | 老式邮件系统常用：注册时设定问题+答案，找回时回答 | 无通讯基础设施 |
| **联系管理员/运维重置** | 无自助渠道时的兜底 | 内部系统、教学/演示项目 |

### 候选方案（**均不扩充基础设施**，即不引入短信/邮件服务商）

| 方案 | 实现方式 | 优点 | 缺点 | 是否扩表 |
|------|---------|------|------|---------|
| **A. 运维离线重置脚本（推荐）** | 加 `scripts/reset_password.sh`（psql + bcrypt），运维执行；登录页"忘记密码"改为提示联系管理员 | 零 schema 变更、零服务变更、最贴近同类项目做法 | 无自助、需人工介入 | ❌ 否 |
| **B. 已绑定字段核验** | 用已有的 `phone`/`email` 列做"身份核验"而非"投递"：输入用户名 + 注册时登记的手机号后 4 位/邮箱，匹配即允许改密 | 零 schema 变更、可自助、复用现有列 | 手机号/邮箱非机密，安全性弱；且现工程注册流程不采集 phone/email，需先补采集 | ❌ 否（但需补注册采集） |
| **C. 密保问题** | 注册时设 1-2 个安全问题 + 答案哈希存库，找回时校验 | 可自助、业界成熟模式 | 需新增列/表（严格说是小扩充）；答案可猜 | ⚠️ 是（小） |
| **D. 保持现状（dev 回显）** | 验证码继续只在 dev 回显，UI 明确标注"演示环境" | 零成本 | 生产不可用；用户会误以为能用 | ❌ 否 |

### 决议（用户 2026-09-17 确认）

**选 C 密保问题**。用户判定理由：

- **A 运维离线重置脚本** → 体验不好（纯人工介入）
- **B 已绑定字段核验** → 要接入额外 API（注册流程需补采集 phone/email）
- **C 密保问题** → **只需改页面 + 数据库**，成本最低且体验可接受 ✅

### 细化决议（用户 2026-09-17 第二轮确认）

| 决策点 | 结论 |
|--------|------|
| 密保可否跳过 | **不可跳过**——密保是找回密码的**唯一门禁** |
| 注册是否必设 | **必须带上**——注册时必须设定密保 |
| 注册 UI 形态 | **弹框**（1~2 个问题 + 答案）。原因：注册卡片空间不足（`.login-card` 为固定布局且 `overflow: hidden`，现已有 3 字段），塞不进密保字段 |
| 用户提示 | 弹框内必须提示「此密保用于找回密码」 |
| 注册验证码步骤 | **删除**——无投递渠道，已用不到 |

### 落地要点

| 层 | 改动 | 状态 |
|----|------|------|
| 数据库 | `users` 表新增密保字段：`security_question` + `security_answer_hash`（答案同 password_hash 用 bcrypt）。**支持 1~2 个问题**（建议独立 `user_security_answers` 表存多问题，或 2 组列）。**归属 E2E-06** | ✅ **已落地**（2026-09-18，E2E-06：`user_security_answers` 表 + model/repository/logic/gRPC 全链路） |
| 注册流程 | **删除验证码步骤**（含 `getVerificationCode` 按钮、`code-field`、`code-hint`、`registerInfo.verificationCode`）；新增**密保设定弹框**（不可跳过，关闭即中止注册）。**归属 E2E-09** |
| 找回流程 | `verify.vue` 从"输入验证码"改为"回答密保问题"（1~2 题全对才放行）；`modify.vue` 保留改密；BFF 的 `verification-code` 端点改为 `verify-security-answer`。**归属 E2E-07** |
| 后端 | `user-svc` model/repository/logic 加密保字段读写 + bcrypt 校验；`register` 接口要求密保字段必填 |
| 测试 | 架构测试需同步更新（`username-only-copy.architecture.test.ts` 锁死"不得出现手机号/邮箱"字样，密保问题文案需另立断言）；`verificationCodeCountDown` composable 若注册流程不再用，评估是否仅保留给其他流程 |

### ⚠️ 连带后果：存量用户的处理（已决议）

**删掉 `phone`/`email`（E2E-06）+ 密保不可跳过** ⇒ 存量未设密保的用户将失去找回密码能力。

**决议（用户 2026-09-17）→ 选 C：不处理存量用户。**

用户判定依据：**数据库里现有数据全是无用信息，项目尚未上线、处于测试阶段**——因此无需为存量用户设计补设/迁移路径。

**附带要求**：

| 要求 | 说明 |
|------|------|
| 更新 DB 字段后**写一个演示账号** | 新演示账号需自带密保问题（否则演示不了找回流程） |
| **演示账号后续必须能删** | 它是临时测试资产，不是永久种子。落地方式：可重跑的 seed 脚本 + 对应的清理路径（而非硬编码进 `initdb.d` 不可逆的种子） |
| 注意既有依赖 | 现有 Playwright spec（`login-flow` / `chat-flow` / `dashboard-flow`）都依赖 `echo`/`echo123` 演示账号。**删除演示账号时必须同步处理这些 spec 的依赖**（改为环境变量注入或 fixture 动态创建账号） |---

## D-02 心理测验定位与人格画像（归属 E2E-10 / E2E-11）

### 现状

现量表是**症状自评**（PHQ-9/GAD-7/PSQI），评分只产出 `总分 + 风险等级 + 逐题原始分`，无维度/人格标签；AI 侧 system prompt 是写死的静态字符串，测评结果从未进入任何 AI 请求。详见 [findings/2026-09-17-pretest-panorama.md](findings/2026-09-17-pretest-panorama.md) §一。

### 决议（用户 2026-09-17 确认）

**两种量表并存**：

1. **人格量表**（新增）：用于产出心理画像/人格维度，目标是驱动 AI 回复针对性（改造 system prompt 注入）
2. **症状量表**（保留）：PHQ-9/GAD-7/PSQI 继续用于风险预警

**页面区分**：`/question` 页需能区分两类测评（分类展示/筛选）。

**user 界面新增相关图表**：用户提到"user 界面记得有记录对话频率等图表"——**核实结论：确有 3 个图表**（详见 findings 补充节），需在此基础上**增加测评结果相关图表**（如人格维度雷达图、测评历史趋势）。

### 影响面

| 层 | 改动 |
|----|------|
| 数据库 | `surveys` 表已有 `category` 列可承载分类；人格量表需新种子数据（**现工程量表种子数据完全不存在**，见 E2E-F-03） |
| 评分器 | `assessment-svc/internal/scoring/scorer.go` 需新增人格维度评分器（现只有 PHQ9/GAD7/PSQI/Generic） |
| 结果模型 | `scoring.Result` 需扩展维度/画像承载字段（现仅 TotalScore/RiskLevel/Factors） |
| AI 链路 | proto `ChatCompletionRequest` 需加画像字段 或 BFF 组装注入 system prompt（现 prompt 写死在 `ai_stream_handler.go:226,284`） |
| 前端 | `/question` 分类展示 + user 页新增图表 + 结果弹窗字段对齐（现三层契约错位，见 E2E-F-02） |

---

## D-03 数字人口型同步与 TTS 断点（归属 E2E-14）

### 现状

口型是**随机轮播假动画**（`useTTSPlayer.ts:91-103` 每 150ms 切 5 个口型，与音频零对齐）；phoneme→口型映射表是死代码（L38-75，全文件无引用）。XTTS 侧 `/tts_with_phonemes`（`XTTS/server.py:283-341`）**已返回字符级时间戳，但前端从未调用**。段间断点根因明确（500ms debounce 聚合 + 每段 `stop()` 重连 + XTTS 每段冷启动推理）。

### 决议（用户 2026-09-17 确认）

**做真口型同步 + 排查断点**：

1. **真口型同步**：接 `/tts_with_phonemes` 的时间戳，前端音频播放改为按时间戳驱动 BlendShape，把死代码映射表接上线
2. **排查断点**：解决"500ms debounce 分段 → 每段 stop 重连 → 每段冷启动"造成的播放间隙

### 影响面

| 层 | 改动 |
|----|------|
| XTTS | 确认 `/tts_with_phonemes` 的流式能力与时间戳粒度（当前是非流式？需核实能否流式返回时间戳） |
| BFF | 需新增/改造转发端点（现 `/tts/stream` 走裸 PCM 流，无时间戳通道） |
| 前端播放 | `useTTSPlayer.ts` 播放层需从"按 chunk 喂 PCM"改为"按时间戳对齐口型"；`useConversationSender` 的 500ms debounce 与文本分段策略需重审 |
| 数字人 | `DigitalHuman.vue` 的 `setLipShape`/BlendShape 驱动逻辑复用，映射表激活 |

---

## D-05 注册链路断裂的修法（归属 R-01）

### 现状

后端在 E2E-06 中**强制要求 1~2 个密保**（`auth_handler.go:172-180`、`authlogic.go:96-102`），而前端注册表单仍只发 `{username, password, verificationCode}`（`login/index.vue:147`）⇒ **产品唯一的注册入口 100% 返回 400**。

### 候选方案

| 方案 | 做法 | 评价 |
|------|------|------|
| A. R-01 内补前端密保录入（弹框） | 按已决议设计做弹框 | 把"修 bug"扩成"做功能"，R-01 范围膨胀；但一次到位 |
| **B. 后端先回退为"密保可选"（推荐）** | 临时去掉强制校验（字段与写入能力保留），注册恢复可用；前端录入 UI 归 E2E-09，E2E-09 收口时**重新打开强制** | 最小风险、符合"先向后兼容再迁移" |
| C. 双端同时改 | 前后端同批改 | 单批改动面大，不好定位问题 |

### 决议：**选 B**

**依据的通用实践**：**Expand → Migrate → Contract（扩展-迁移-收缩）**，也叫向后兼容式契约变更。当服务端开始强制一个客户端还无法提供的字段时，标准顺序是——先让服务端**接受并记录**新字段（可选），等客户端具备发送能力后，再打开强制。反过来（先强制、后补客户端）等于制造一次自伤式中断。

**落地要点**：

| 项 | 内容 |
|----|------|
| R-01 | 后端强制校验临时降级为"可选"（保留 `security_questions` 字段解析与持久化）；补**回归测试**断言"不带密保也能注册成功" |
| E2E-09 | 补前端密保录入 UI（弹框，按 D-01）+ **重新打开强制校验** + 补"不带密保注册必失败"的负向测试 |
| 追踪 | 本临时状态登记为待复位的显式任务，**不得**只留代码注释——E2E-09 的 plan 必须包含"复位强制校验"一项 |
| 风险 | 降级期间注册的账号无密保 ⇒ 无法走找回密码。**当前库内全是测试数据、未上线（D-01 连带后果已按 C 处理）**，故风险可接受 |

---

## D-06 `-race` 的最终处置（归属 R-01 调查 / R-02 结论）

### 现状

E2E-03 plan 的 A3 要求加 `-race`；`0e29444` 把它删了，理由是"**疑似** Go 工具链 race detector 不可用"，report 进一步断言"CI Go 1.26.1 中不可用（本地 Windows 同样 0xc0000139）"。这是**用 Windows DLL 加载错误解释 Linux CI 的 exit 1**，证据链不成立；7 个模块**全部** exit 1 更符合"真实数据竞争"或"构建环境缺 C 工具链"的形态。账本 E2E-F-40 已被改判为「🟡 降级并记录」。

### 决议：**先查明再定，禁止直接降级收口**

**依据的通用实践**：① Go 官方文档明确 `-race` 需要 **CGO_ENABLED=1** 与可用的 C 工具链——若 CI 环境不满足，表现正是**全模块统一失败**；② 而当存在真实数据竞争时，`-race` 会让**特定包**失败并打印 `DATA RACE` 报告，**不会是 7 个模块整齐 exit 1**；③ 工程通则是"**先把失败原因定性，再决定手段**"，不得以"疑似"为由移除质量门禁（对应 AP-06 根因臆断、AP-05 删除需求）。

**R-01 的具体调查动作**（可复现，非推测）：

| 步 | 动作 | 判据 |
|----|------|------|
| 1 | 在 Linux 容器复现：`docker run --rm -v $PWD:/w -w /w/<svc> golang:1.26 go test -race -count=1 ./...` | 若能跑且通过 ⇒ CI 环境问题；若打印 `DATA RACE` ⇒ 真实竞争 |
| 2 | 单独用空测试验证 `-race` 可用性：`go test -race -run TestNothing ./...` 或不存在的包 | 若空跑也 exit 1 ⇒ 环境/工具链问题 |
| 3 | 检查 CI 日志中 `-race` 失败时的**首条错误行**（是 `DATA RACE` 还是 `gcc: not found` / 下载失败） | 二分定性 |

**结论分支**：① 环境问题 ⇒ 修 CI（装工具链/设 `CGO_ENABLED=1`）后**加回** `-race`；② 真实竞争 ⇒ 修竞争后加回；③ 确实不可用且有铁证 ⇒ 写经批准的降级记录（含残留风险），**不得**标"已解决"。

---

## D-07 `doc-drift-check` 当前红态的处置（归属 R-01 / R-03）

### 现状

main 上 `doc-drift-check` 连续红（`2cb9e58`、`0644988`），3 个 job 失败：① env 变量 lint（8 个未文档化）② Dockerfile digest（6 个占位）③ migration 服务顺序（Fail 13）。

### 决议：**分类处置——可修的先修，不可修的用"带理由的已知缺口"表达，不使用 `continue-on-error`**

**依据的通用实践**：主分支 CI 红 = broken build，行业标准是 **fix-forward 或 revert**，而不是抑制（suppress）。但本项目存在**客观上无法在本地修复**的一项（digest 需访问 docker.io 回填，实测网络不可达）。对此的标准做法是 **known-debt allowlist**：显式列出已知缺口 + 原因 + 复检条件，**而非**全局 `continue-on-error`（那会让该 workflow 永久失去信号）。

**2026-09-18 实测的 3 个失败 job 与逐项处置**（完整证据见 [remediation.md](remediation.md) §R-01「CI 红态精确诊断」）：

| # | job | 实测结论 | 处置 |
|---|-----|---------|------|
| 1 | Dockerfile digest pin（6 个占位） | **确认不可本地修复**：`curl registry-1.docker.io` → `HTTP 000`（21s 超时）、`docker manifest inspect` 亦失败 ⇒ `sync_docker_digests.sh` 跑不通 | 改为**显式已知缺口声明**：打印醒目警告（原因 + 复检条件），退出码 0。**非静默通过**，**不加 `continue-on-error`** |
| 2 | 环境变量 lint（8 项未文档化：`APISIX_ADMIN_KEY`/`BFF_DEV_RETURN_CODE`/`BFF_JWT_SECRET`/`CORS_ALLOW_ORIGINS`/`NACOS_GROUP`/`NUXT_PUBLIC_API_BASE_URL`/`SKYWALKING_ENABLED`/`XTTS_MODEL_PATH`） | 可直接修 | **补进 `.env.local.example`（两处副本）+ 只写占位与说明，不写真实值** |
| 3 | Migration 服务顺序独立性（Fail 13，全部在 `analytics-svc`） | **规则与其自述目的不符**：脚本自述「不依赖**启动顺序**」，实现却是绝对规则「migration 不得引用其他服务 schema」且**无豁免机制**。而 `analytics-svc` 架构上就是**跨域聚合器**（视图必须读 ai/chat/assessment schema），顺序由 `migrate.sh` 的 `PRIORITY_ORDER`（chat→ai→analytics）保证；ADR-18 §8.2 亦记为「⚠️ WARN」= 一直已知且可接受 | **修规则而非加豁免**：把规则收敛回"是否依赖启动顺序"，加声明式 `CROSS_SCHEMA_ALLOWED` 表登记 `analytics-svc`（附理由），其余服务仍严格禁止。若认为该架构本身要改，须先出 ADR，不由检查项驱动 |

**关键判断**：第 3 项是一个**"按设计必然失败"的检查项**——它不会发现真问题，只会让 CI 永久变红，进而**训练所有人忽略 CI**（比没有检查更糟）。这类"只会喊狼来了"的检查必须修规则或删除，不能靠抑制。

**统一禁止**：不得用 `continue-on-error`、不得把 FAIL 改判 WARN 而不修规则、不得删除检查项——这些都是 AP-11（门禁只报不拦）的变体。

---

## D-08 `enforce_admins` 与 `required_status_checks` 的取舍（归属 R-03）

### 现状

分支保护目前 `enforce_admins: true` + 防强推/防删除，但**无 `required_status_checks`** ⇒ CI 红了不拦（AP-11）。若直接加 required checks，在 `enforce_admins: true` 下**一旦 check 因故不上报，连管理员也会被锁死无法合并**。

### 决议：**保持 `enforce_admins: true`；只把"每次必跑"的 job 设为 required**

**依据的通用实践**：required status checks 的**锁死风险只来自"被设为 required 却不会在每次 push 时上报的 check"**。GitHub 官方对 path 过滤 workflow 的行为说明即指出：带 `paths` 过滤的 workflow **在路径不匹配时不会运行**，从而不产生 check run —— 若被设为 required，会一直停在 "Expected — Waiting for status to be reported"。因此标准做法是：

| 规则 | 落地 |
|------|------|
| 只有**无 `paths` 过滤、每次 push 必跑**的 job 才可设为 required | `go-test.yml`（无 paths 过滤）→ **可设 required** |
| 带 `paths` 过滤的 workflow **不设为 required** | `web-test.yml`、`llm-test.yml`（均有 paths 过滤）→ 仅报告 |
| 新 workflow 若也要 required，先去 `paths` 过滤 | `doc-drift-check.yml`（无 paths 过滤）→ 修好红态后可设 required |
| `strict`（要求分支 up-to-date）保持 `false` | 单人开发模式无需强制 rebase，减少无谓摩擦 |
| 过渡期 | 先用 `go-test` 一个 required check 试跑，确认不会卡住后再扩 |
| 保险丝 | 保留"紧急时可在 Settings 临时移除 required"的操作记录习惯（写进 report） |

**收益**：门禁对管理员**真实生效**（AP-11 解决），同时通过"只 required 必跑 job"消除锁死风险。

---

## D-09 用户配置（字号/主题）的持久化载体（归属 E2E-12）

### 现状（2026-09-19 建档实测）

设置页 `/chat/setting` 的字号与主题切换**在当前架构上不可能持久化**——四层契约各缺一环，任一缺失都会让 `PATCH /api/v1/users/me {"config":{…}}` 被静默丢弃（Go `json.Unmarshal` 忽略未知字段 ⇒ 前端拿到 200 并提示成功，服务端什么都没存）：

| 层 | 现状 | 证据 |
|----|------|------|
| 数据库 | `emotion_echo_user.users` 无 config 列 | `deploy/db/02-create-tables-in-schemas.sql:7-18`；`grep -rn config deploy/db/*.sql` 零命中 |
| proto | `UpdateProfileRequest` 无 config | `proto/user.proto:112-118` |
| BFF 入参 | `UpdateProfileReq` 无 Config | `emotion-echo-web-bff/internal/downstream/user.go:36-41` |
| BFF 出参 | `toProfileVM` 硬编码 `Config: map[string]any{}` | `emotion-echo-web-bff/internal/handler/viewmodel.go:106` |

账本条目：[E2E-F-82](discovered-unresolved.md)（与 E2E-F-80「age 同样被丢弃」同源）。

### 候选方案

| 方案 | 做法 | 优点 | 缺点 | 是否改契约 |
|------|------|------|------|-----------|
| **A. 服务端持久化** | `users` 加 `config JSONB` + proto 字段 + BFF 透传 | 跨会话/跨设备；与页面**既有设计意图一致**（`app/stores/user.ts:59-97` 早已在调 `updateProfile({config})` 写服务端）；关闭 E2E-F-82 | schema + proto 变更（4 层）、需迁移与 ADR | ✅ 是 |
| B. 仅本地存储 | localStorage/cookie 镜像，删除前端那段服务端写入 | 零后端改动 | 换浏览器/清缓存即丢；等于把 E2E-F-82「已实现却失效」改为「主动降级」，须按 AP-05 明确记录降级 | ❌ 否 |

### 决议（用户 2026-09-19 确认）

**选 A 服务端持久化。**

**理由**：前端写入路径已按服务端持久化写好（`setFontSize` / `setTheme` 均先调 API 成功再更新本地），A 属于"补齐契约让已有实现真正生效"，而非新增设计；B 需反过来删除既有代码，且不满足本阶段标题中的「持久化」。

### 影响面

- E2E-12 按 [plan §2](stages/e2e-12-settings/plan.md) 的契约扩展范围执行（DB 列 + 迁移 `u003_add_user_config.sql` + proto + BFF 透传 + VM 映射）
- 需配套 ADR：`docs/architecture/adr/adr-2026-09-user-config-persistence.md`（AP-08：架构/存储变更须有 ADR）
- 若日后改选 B：plan §2 契约扩展 4 行删除、测试点 #4/#7 转 `N/A`、#3/#6 降级为本地持久化验证

---

## 决策索引

| 编号 | 主题 | 归属 | 状态 |
|------|------|------|------|
| D-01 | 找回密码方式 = 密保问题 | E2E-07 | ✅ 已决议（细化见 D-05） |
| D-02 | 心理测验 = 两种量表并存 + user 页测评图表 | E2E-13 / E2E-14 | ✅ 已决议 |
| D-03 | 数字人 = 真口型同步 + 排查断点 | E2E-17 | ✅ 已决议 |
| D-04 | i18n 是否立项 | 不阻塞阶段 | 🟡 候选未决 |
| **D-05** | 注册链路断裂修法 = 后端先回退为可选 | **R-01** | ✅ 已定案 |
| **D-06** | `-race` = 先查明再定，禁止直接降级收口 | **R-01 / R-02** | ✅ 已定案 |
| **D-07** | `doc-drift-check` 红态 = 分类处置，不用 `continue-on-error` | **R-01 / R-03** | ✅ 已定案 |
| **D-08** | 分支保护 = 保持 `enforce_admins`，只 required 必跑 job | **R-03** | ✅ 已定案 |
| **D-09** | 用户配置（字号/主题）持久化 = 服务端 `users.config JSONB` | **E2E-12** | ✅ 已决议（用户 2026-09-19；备选方案 B 见上） |
| **D-10 ~ D-24** | _（编号保留，按需填入；当前无新决策）_ | | |
| **D-25** | XTTS 镜像源 = 仓内 `emotion-echo/xtts:v2.0.0`（build 自 `emotion-echo-models/XTTS/Dockerfile`）替代 vendor `ai4all/coqui:latest` | **E2E-17** | ✅ 已决议（2026-09-23 计划期调研；详见 ADR `docs/architecture/adr/adr-2026-09-xtts-v3-repo-image.md`） |
| **D-26** | 端侧化混合推理主方案 = 按 v0.2 选型（WebLLM 主力 + MindChat 双轨验证）/ v0.3 阶段切分实施「端侧优先 + 云端兜底」；分项 D-26.1~5 待 §十二 5 项决策拍板 | **端侧化（Lane O 阶段一）** | 🟢 **accepted**（2026-09-28 §十二 5 项决策全部用户拍板；D-26.1~5 全部立 ADR accepted；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-hybrid-main.md` + 计划 `docs/plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md`）|
| **D-26.1** | 端侧化隐私定位 = **(a) 推理本地 + 消息照常上传**（**两阶段拍板** = 前期 (a) + 未来 (b) 等排期）；用户 2026-09-28 拍板「前期先用 a，b 等排期」| **端侧化（Lane O T2#6 拍板）** | 🟢 **accepted**（2026-09-28 用户拍板；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-privacy-position.md`）|
| **D-26.2** | 端侧主力模型 = **WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC（Apache 2.0）**—— 用户 2026-09-24 会话口头授权选 B 方案，2026-09-28 正式拍板转 accepted；不选 A MindChat 核心理由 = GPL-3.0 copyleft 锁定商用 + 自编译成本高 + 无 head-to-head 优势证据 + 与项目 Apache-2.0 默认 license 冲突 + MindChat 2024-02-06 后未更新（19 个月）；分级加载（0.6B / 1.7B / 4B）| **端侧化（Lane O T2#3）** | 🟢 **accepted**（2026-09-28 正式拍板；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-model-selection-qwen3.md` + 决策材料 `docs/plans/on-device-model-selection-decision-material-2026-09-24.md`。Apache 2.0 NOTICE 致谢 + WebLLM Dynamic Import + §六握手（`package.json` 改动登记）|
| **D-26.3** | 端侧化来源告知 = **(a) 「本地完成」角标**；chat-svc 入库加 `reply_source` 字段（`local`/`cloud`/`hotline` 三态）| **端侧化（Lane O T2#6 拍板）** | 🟢 **accepted**（2026-09-28 用户拍板；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-source-disclosure.md`）|
| **D-26.4** | 端侧化离线范围 = **L0 + L1**（L2 二期；强联动 D-26.1=(b) 未来切）；L0+L1 必做 OC-01~OC-05，L2 二期 | **端侧化（Lane O T2#6 拍板）** | 🟢 **accepted**（2026-09-28 用户拍板；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-offline-scope.md`）|

| **D-28** | **E2E-20 多实例共享状态方案收尾裁定**：① 登录锁定跨实例 = 保留 Redis 化（`authlock.LoginLockStore`，D-27 首个业务接入点）；② 验证码防枚举 = **回退 Redis 化**——`VerificationCodeStore` 独立接口仅 in-memory + 契约测试锁死禁止 Redis 化，因 `/api/v1/auth/verification-code` 是 **D-01 决议下的遗留物**（E2E-07 只增未删，端点待删 = 新账本 E2E-F-144）；③ APISIX `policy: redis` 保留（跨节点防放大运行时实测 dev 不可行 → E2E-F-145 归 E2E-25）| **E2E-20（收尾）** | 🟢 **accepted**（2026-09-28 用户裁定："手机号、邮箱验证码早就不用了，之前决策说过了，所以目前不应该有验证码"；详见 ADR `docs/architecture/adr/adr-2026-09-e2e-20-multi-instance-state-sharing.md`；落实 commits 496a810 / 96d02a9 / f3a9a57）|
| **D-26.5** | 端侧化记忆/摘要存储 = **随 D-26.1 联动（a）服务端生成下发**；D-26.1=(a) ⇒ 服务端；D-26.1=(b) ⇒ 本地（联动 D-26.1 当前拍板 = (a)）| **端侧化（Lane O T2#6 拍板）** | 🟢 **accepted**（2026-09-28 用户拍板；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-memory-storage.md`）|
| **D-27** | **Redis = 保留并接入业务（不下线）**：① E2E-20 多实例修复的 `RedisLimiterBackend`（登录锁定/验证码防枚举/APISIX 跨节点限流）；② **记忆系统存储**（与会话记忆待决策联动，见 `docs/plans/conversation-memory-pending-decision-2026-09-24.md` §D-c）；③ token 获取/缓存等后续业务接入。盘点事实（2026-09-24 E2E-18 #6/#7）：当前**零业务引用**（6 svc `redis.NewClient`/`InitRedis` 零命中、`connected_clients:1`、`LimiterBackend`/`InitRedis` 0 caller）⇒ 现状是「**保留待接入**」而非「已接入」——各接入点落地时按所在阶段 TDD + ADR | **E2E-18** | ✅ 已决议（2026-09-24 用户拍板：「redis 正常需要接入业务，所以要保留，比如可以和记忆系统操作、token 获取」；备选=下线（被否，E2E-20 要重拉纯往返）/ 立即接入（超出本阶段边界）） |
| **D-29** | **E2E-23 `/health` 依赖挂掉时的契约 = liveness/readiness 分离**：`/health` 恒 200（保兼容 `seed.sh` 自带探活 + 现有 smoke + 前端引用）但 `status` 字段按依赖真实计算；**新增 `/health/ready`** 承载 200/503，compose 6 个 Go 服务的 `healthcheck:` 改指 ready。**否决"直接让 /health 返 503"**——`apisix-seed` 对 6 服务有 `condition: service_healthy`（`deploy/docker-compose.apps.yml:722-735`），下游一降级则 seed 永不运行 ⇒ 把"某下游降级"放大成"整站无路由"。**运行时验证**：停 Postgres ⇒ `/health` 仍 200 但 `status:"degraded"`、`/health/ready` 503（wget exit=8）；恢复 ⇒ 双双回 ok | **E2E-23** | 🟢 **accepted**（2026-09-29 用户拍板采纳方案 A；落实 commits 3034f52 / 52d0a4e / 10e494c / c230095） |
| **D-30** | **RUNBOOK §2.4「重建服务后必须重跑 `apisix-seed`」判为误导性文档，已更正**：2026-09-29 测试点 #23 实测——`docker restart emotion-echo-nacos` → 6 服务重新注册（`count: 6`）→ **未重跑 seed、未碰 Admin API** → 网关 login 400（路由通）。机制吻合：节点由 APISIX 内置 nacos discovery 插件按 `fetch_interval: 30` 拉取（`deploy/apisix/config.yaml:202-212`），`seed.sh` 只写 upstream **定义**、不含 `nodes`（`seed.sh:230-246`）。**边界**：实例摘除后 APISIX 摘除有滞后（停 user-svc 75s 后其路由仍 401 而非 503），主动健康检查归 F-154/E2E-25 | **E2E-23** | 🟢 **accepted**（2026-09-29 实测落定，非用户决策项；RUNBOOK §2.4 同轮更正） |
| **D-31** | **`emotion-llm-service` 的 `NACOS_REQUIRED` = 编排层显式声明，不改代码默认值**：`main.py:87-94` 保持"默认继续"（dev 不受影响），但 dev compose **显式写** `NACOS_REQUIRED: "0"`（注册失败继续跑是明示选择而非隐式默认）、prod 待办清单标注**必须** `"1"`（`compose.prod.yml` 是 ADR-20 空壳占位，故意不填值但"哪几项要改"必须列全）。依据：计划期实测**全仓该变量 0 命中** ⇒ 从未被任何编排声明过；prod 若沿用 "0" 会在注册失败时静默降级（HTTP 可达但 Nacos 无实例 ⇒ BFF Resolve 502，与 E2E-F-137 同型） | **E2E-23** | 🟢 **accepted**（2026-09-29 用户拍板；落实 commit 1579038；回归钉 `scripts/test_nacos_required_declared.sh` 3/3） |
| **D-32** | **Nacos 配置热更接线范围 = 按实测盘点接 14 个参数**：`chat-svc` 4（Outbox 死信阈值/保留天数/清理间隔）、`ai-svc` 9（LLM/FER/SenseVoice/XTTS 超时、语种、语速、Kafka 重试、熔断双阈值）、`analytics-svc` 1（Kafka 重试）。**全部从各自 yaml 已有配置项取，不新编业务概念**。`user-svc` / `assessment-svc` 实测**零候选**（`internal/config/config.go` 全文回读：仅连接类 + 监听地址类）⇒ **不硬造参数**，保留代码现状 + 账本记账。并同步修三个前置缺陷：P1 敏感字段白名单（`configcenter` 的 `sensitivePrefixes` 只拦 dataId、不拦 YAML 内 key）、P2 `ai-svc` `LLM.Timeout` 死配置（`main.go:487-491` 未传 Timeout）、P3 `analytics-svc` `SetDefaults` 漏 `Kafka.MaxRetries`（账本 F-158/159/160） | **E2E-23** | 🟢 **accepted**（2026-09-29 用户表述"我觉得还是加上比较好吧，这样服务是完整的"+"我并不知道选什么比较好"⇒ 授权执行者按盘点事实定范围；**明确否决**：为 user/assessment 硬造参数——假能力比死代码更危险。实施属 E 组，进行中） |
| **D-33** | DLQ 回放工具形态 = **`scripts/replay_dlq`（Go 小工具，`go run` 运行，复用仓库 sarama 依赖）**。理由：DLQ payload 是 Protobuf 二进制（Stage 73 起），bash + kafka console 文本管道会损坏字节流，console 管道方案不可行；工具零服务代码改动、独立 module（同 `scripts/grpc_smoke` 先例）、转换逻辑可单测 + 守卫脚本 `test_replay_dlq.sh` 接 CI。用户 2026-10-01 拍板"按执行者建议 (a) ops 脚本形态"，(a) 的实质 = scripts/ 下独立运维工具、不动服务代码 | **E2E-24** | 🟢 accepted（2026-10-01 用户裁定） |
| **D-34** | consumer 重试语义修法 = **原地重试**（deliverWithRetry：同一消息预算 = MaxRetries+1 次尝试、指数退避 2s/4s/8s 封顶 30s；耗尽 → DLQ + Mark 前进），废除旧"跨消息 attempt 计数 map"。analytics-svc 与 ai-svc 双双修复（同型同批）。连带语义更新：DLQ=nil 时预算耗尽也 Mark（旧"无限重投"实测=单条毒消息永久阻塞分区）；D3（计数跨重启清零）随之消解——每条消息在单次遍历内确定结局 | **E2E-24** | 🟢 accepted（2026-10-01 用户裁定"修法按执行者建议"；F-174 随此闭环） |
| **D-35** | **E2E-25 M1 裁决依据：APISIX upstream `checks.active` 对 nacos-discovery 动态节点有效**——官方 health-check 文档无明文承诺（计划期核实），开工后 control API `/v1/healthcheck` 实证：动态节点在首次请求后被健康检查管理器登记，停 user-svc ~7s 翻 unhealthy（对照 D-30 的 75s 滞后）/恢复 ~10s 翻回；无需 passive/fallback 方案。连带记录：echo 插件不终止请求（body=ok 仍 503），无 upstream 路由须用 serverless `core.response.exit`；连接拒绝 502 不计入 api-breaker（仅上游返回的 5xx 计数） | **E2E-25** | 🟢 accepted（2026-10-02 运行时实证，非用户偏好项） |
| **D-36** | **E2E-25 M2 裁决：seed 漂移处置 = B+（覆盖+报告+dev-up 自动检查）**：① seed 重跑照常覆盖 admin 手工改动（seed 是唯一真理源，手工改 admin 属违纪律行为，覆盖即恢复规范）；② 漂移不阻断启动（拒绝 fail-closed：compose up 卡半启动比漂移更难排查）；③ `check_apisix_drift.sh extras` **接入 dev-up.sh 尾部**，每次启动自动报告 seed 白名单外的野生路由（route 116/299 类），只报不拦 + 告警留痕。备选 A（fail-closed 拦截）与纯 B（仅工具）被否 | **E2E-25** | 🟢 **accepted**（2026-10-02 用户裁定"可以"采纳 B+ 建议） |
| **D-37** | **E2E-26 M1 裁定：历史「OAP 9.x queryDuration bug」= 客户端时间格式错，非上游缺陷**。依据：官方 query-protocol `common.graphqls` 明文 SECOND 步长格式 `yyyy-MM-dd HHmmss`（无冒号）；容器网实测带冒号报 `is malformed at ":30:00"` 与 stage-92/93 记录同型（OAP 按协议正确拒绝）；apache/skywalking issues 零命中；官方格式查询正常（`scripts/query_oap.sh` 封装）。**落地**：stage-92/93 顶部 + known-issues-backlog Item 4 已回填定性注记并关闭 | **E2E-26** | 🟢 **accepted**（2026-10-03 用户拍板「判客户端格式错」） |
| **D-38** | **E2E-26 M2a 裁定：SkyWalking UI dev 宿主入口 = compose 加 `127.0.0.1:18080:8080` 限定映射**。评判依据：APM UI 走 localhost 绑定是业界惯例（Jaeger/Grafana 同型），127.0.0.1 限定不扩大暴露面（不违反 Stage 33 PR-20 防局域网暴露初衷，注记已回填）；APISIX 反代 SPA 子路径脆且无必要；**部署解耦**——生产 k8s 走 charts/ingress 独立入口，dev compose 映射不进生产。OAP GraphQL 12800 保持不映射（`query_oap.sh` 容器网封装） | **E2E-26** | 🟢 **accepted**（2026-10-03 用户授权执行者按部署考量评判，判映射方案） |
| **D-39** | **E2E-26 M2b 裁定：HTTP 日志 trace_id（=APISIX request_id）与 OAP traceId（=sw8）两套 ID 保留 + 文档化，不强行统一**。评判依据：业界「日志 trace_id = APM id」常态的前提是网关也进 trace 树；本项目 APISIX 仅 skywalking-logger 上报 access log、不传播 sw8 ⇒ 单改应用侧只是把不一致从「对不上 OAP」挪成「对不上网关日志」，假收益；真一致需 APISIX 接入 trace 传播（中大型改 + 动 E2E-21 已验 13 测试点语义）。**文档化口径**：HTTP 入口段日志用 `trace_id`（=网关 request_id，APISIX/Loki 互查）；Kafka 消费侧 `trace_id` 已 = sw8（可直查 OAP）；OAP UI 用 traceId（sw8）。跨查路径：网关日志↔应用日志靠 request_id，应用日志(Kafka 段)↔OAP 靠 sw8。**候选账本**：APISIX 接入 trace 传播后统一（网关专项） | **E2E-26** | 🟢 **accepted**（2026-10-03 用户授权执行者评判，判「保留+文档化」——改一致工程量中等且净收益为假） |
| **D-40** | **E2E-27 M1 裁定：F-116 修复 = avatar+uploads 新数据网关相对 + 各自 GET/HEAD 反代端点 + 存量惰性兼容（不回填）**。落地：avatar 响应/落库 `/api/v1/user/avatar/image/<fk>`、uploads `data.url=/api/v1/uploads/file/<fk>`（ADR-2026-09 决策 1/3，voice 同型防御：单段 key/拒 ../404 JSON/503 nil/GET+HEAD 双注册）；存量两形态读取侧兼容——`fileSourceURL` 相对→容器网 minio 直译（FILE_FETCH_ALLOWLIST 可过）+ 前端 `resolveObjectUrl`（ChatFile/getFullAudioUrl 既有语义）；同链孤儿治理（UpdateMe 成功后删旧，同 key 不删）。备选被否：①仅 avatar——uploads 同型债留半条；③存量回填 SQL——dev 数据可重建属可选优化，读取侧已兼容两形态故无必要。**裁定来源（如实）**：执行期按 RUNBOOK §8 升级（AskUserQuestion），用户未选定并示按最佳判断继续 ⇒ 执行者按推荐方案落地；**非用户拍板**，用户可改判（回滚连带 #15-18 与回归钉） | **E2E-27** | 🟢 **accepted**（2026-10-03 执行者按推荐落地——来源已如实标注，非用户拍板） |
| **D-41** | **E2E-27 M2 裁定：MinIO 端口 9000/9001 改 `127.0.0.1` 限定映射 + 匿名读边界文档化**（备选③）。依据：`mc anonymous get`=download 桶级匿名实测 ⇒ 0.0.0.0 绑定时局域网任意机器可绕网关 JWT 直读全部对象（含 voice 音频）；`127.0.0.1` 限定与 D-38（sw-ui）同型、宿主浏览器/脚本零损（重建后宿主匿名 GET 200 实测）、容器网走 expose 不受影响。备选被否：①仅文档化——留局域网直读面；②voice 分桶去匿名——动 A3 单桶假设 + 存量迁移，工程量与收益不成比（key 含 uuid 不可枚举 + 绑定后风险已收敛）。**文档化**：`deploy/minio/README`「端口」注记 +「匿名读边界」两节。**裁定来源同 D-40** | **E2E-27** | 🟢 **accepted**（2026-10-03 执行者按推荐落地——来源已如实标注，非用户拍板） |
| **D-42** | **E2E-28 M1 裁定：首基线只落档不设 CI 阈值门禁**（备选①）。依据：LLM/XTTS 上游抖动大（计划期 TTFT 单样本 11.7s、TTS 冷热 18x），无历史对照的窄阈值必造 flaky CI（训练人忽略红灯）；首基线先把数字测准测稳（plan #2 复现性断言：两轮 p50 比值 ∈ [0.5,2]）。**二轮数据后再议门禁**（相对回归门需 ≥2 轮才可信）。备选被否：②宽松绝对上限——本轮就能写但对"变慢"零检出力且 LLM 极端抖动仍会偶发误报；③相对回归门——需二轮基线，本轮不成立 | **E2E-28** | 🟢 **accepted**（2026-10-03 用户经 AskUserQuestion 拍板「只落档不门禁」） |
| **D-43** | **E2E-28 M2 裁定：TTS 三件套 = 双端点 + 多 worker 全做（F-135 + F-136）**。落地面：① **F-136 多 worker**——先按 plan #11 量化单 worker 串行排队，再改多 worker/多副本 + 负载分配；**架构级 ⇒ 随 C4 出 ADR（`adr-2026-10-*`）+ architecture/decisions 决策 39**。② **F-135 每段双端点**——播放走 `/tts_stream`（流式 WAV 尽早出声=「快」）+ 嘴型走 `/tts_with_phonemes`（真口型）两全；前置 = F-136 扛 ×2 并发压力。**F-134 归并说明（如实）**：用户选项文本为「双端点+多 worker 全做（F-135+136）」，F-134 的目标（首段 5-15s 出声）由 F-135 流式路径承担；**首句早切是否还需单独改 `useTTSManager` 切段策略，执行期核实现状后定**——若现状已按标点切段且 F-135 落地后首段出声达标则 F-134 等效闭环，否则补切段改动（账本条目保持开放直至实测闭环） | **E2E-28** | 🟢→🔴 **accepted → superseded by D-44**（2026-10-03 用户拍板；**2026-10-06 F-135 实测否决见 D-44**——双端点前提「两推理可并行」被证伪，F-136 部分（事件循环解阻塞+推理锁）继续有效） |
| **D-44** | **E2E-28 F-135 处置裁定：双端点实测否决 + TTS 首声加速改走流式 TTS API（CosyVoice2 类）**。**否决证据链（2026-10-06 实测）**：① XTTS 模型不并发安全——`inference()`/`inference_stream()` 共享 `gpt_inference.cached_prefix_emb`（gpt.py:570 每次推理覆盖写），跨端点并发 = 数据竞争（实测截断 1.1s vs 基线 2.1s + `index out of range`/tensor 错位错误簇）⇒ 已修 `INFERENCE_LOCK`（模型级互斥，XTTS v2.0.1）；② 互斥后 stream/phonemes 串行 ⇒ **双端点核心收益消失**：首声 22-34s ≈ 现状 phonemes-only 20-30s（都卡同一个推理）；③ stream 端点对单句段**无渐进性**——`inference_stream` 按文本片段 yield，切段后单句 = 一个片段，实测 3 chunks 同刻到达（首块 36.8s = 完成时刻）；④ 双端点净成本 = ×2 推理压力 + BFF 90s 整请求超时掐死队列尾部。**TTS 首声加速路线（用户 2026-10-06 拍板）**：接入流式 TTS API（推荐硅基流动 CosyVoice2-0.5B：注册送额度/国内直连/流式首字 0.5-1s），XTTS 降级为离线回退（推理锁修复保护回退路径）；代价（用户知情）：音色变为官方音色（不做声音样本上传复刻，隐私考量）+ 联网依赖。**实施专项 = E2E-F-198**（BFF TTS API provider + `TTS_API_KEY` env + 前端流式播放适配 + XTTS 回退链）。F-135 前端实现（WebAudio PCM 播放器等）保留在本地分支 `feat/e2e-28-f135-dual-endpoint-tts`（不合并，TTS API 落地时可复用播放器库） | **E2E-28** | 🟢 **accepted**（2026-10-06 用户经 AskUserQuestion 拍板「接 TTS API」路线，F-135 否决随之落定）；**同日选型定稿**：用户交付 `TTS_API_KEY`，实测 CosyVoice2-0.5B TTFB ~0.33s / 33 字完整合成 1.4~1.6s（≈2x 实时，4/4 无截断）/ pcm=int16LE 单声道 24k / ASR 回读逐字一致；MOSS-TTSD 实测复读参考内容排除（SiliconFlow 仅此 2 个 TTS 模型）；**详见 ADR `adr-2026-10-tts-api-cosyvoice2`（架构决策 40）** |
| **D-46** | **E2E-29 M1 裁定：`/api/v1/auth/refresh` 在拿不到有效令牌时 = 硬 401**。背景（计划期实测 E2E-F-201）：匿名 `POST /auth/refresh` 返回 200 并发放 `user_id=1` 的 24h 有效 JWT，该 token 经网关可读 `/users/me`（`account=echo`）= **认证绕过**；三层根因 = BFF `main.go:307-313` 前缀放行整个 `/api/v1/auth/` + APISIX `seed.sh:632` route 113 无 jwt-auth + `auth_handler.go:215` `var userID int64 = 1` **静默回落默认身份**（带过期/伪造签名 token 同样发放）。**落地（L1）**：删除回落默认身份；无有效令牌（缺失/过期/签名不符）一律 401 且不设 cookie、不返 `accessToken`；有效令牌仍正常续期（返回**同一** user_id）。**零前端改动**依据：`useApi.ts:302-340` 已有"刷新失败 → `clearAuth()` + 跳 `/login`"路径。备选被否：② 仅接受刚过期的令牌（grace window）——引入"过期仍可用"的语义模糊面；③ 双令牌（access+refresh + 服务端吊销表）——需动 schema 且与 D-48 联动，量大不成比 | **E2E-29** | 🟢 **accepted**（2026-10-07 用户经 AskUserQuestion 拍板「硬 401」） |
| **D-47** | **E2E-29 M2 裁定：BFF 可信链 = 默认值分离 + 缺 CIDR 拒绝启动 + 收 8894 宿主映射（三件一起做）**。背景（计划期实测 E2E-F-202）：`BFF_TRUST_APISIX` 默认 `false`（`docker-compose.apps.yml:662`）⇒ 任何来源的 `X-User-Id` 均被接受；`8894` 映射宿主（`:701`）；prod overlay 仅以注释提醒（`compose.prod.yml:33-37`）——实测直连 `:8894` 带 `X-User-Id: 2` 返 200 `smoke_user`。**落地**：① prod overlay 默认 `true`；② 非 dev 模式（`TrustAPISIX=true` 或显式 prod 标记）下 `APISIXCIDRs` 为空 ⇒ BFF **fail-fast 拒绝启动**（不再"静默接受任意 header"）；③ dev 收回 `8894` 宿主映射（容器网/`docker exec` 可达，直连伪造面消失）。**边界如实**：中间件本身在 `RequireAPISIXIP=true` 且 CIDR 空时已是 **fail-closed**（`gin_auth.go:130-140`），故本条修的是**默认值与端口暴露**，非中间件逻辑。备选被否：② 只收 8894——默认值仍 false，端口若再暴露即复发；③ 只做默认值分离——留端口暴露面 | **E2E-29** | 🟢 **accepted**（2026-10-07 用户经 AskUserQuestion 拍板「三件一起做」） |
| **D-48** | **E2E-29 M3 裁定：JWT 密钥轮换 = 双密钥并存窗口**（`kid`/双 secret，验签接受新旧两把、签发用新把）。背景（F-28 成立，E2E-F-28）：`BFF_JWT_SECRET` 被 BFF 与 APISIX consumer 共用（`apps.yml:694/767` + `seed.sh:91`），现有 `test_bff_jwt_secret.sh` 只断言三处默认值一致（静态），**无轮换机制**；单 secret 下轮换必致全部在途 token 失效。**落地要点**：BFF 侧签发带 `kid`、验签按 `kid` 选密钥（未带 `kid` 的旧 token 走旧密钥，兼容期结束再撤）；APISIX consumer 侧先核实是否支持多凭据（不支持则用 seed 的**原子 PUT 窗口 + 滚动重启**，把"不一致窗口"压到最小并文档化）；配套守卫断言"新旧密钥同时可验"与"撤回旧密钥后旧 token 失效"。**架构级 ⇒ 随落地出 ADR（`docs/architecture/adr/adr-2026-10-*`）+ `architecture/decisions.md` 登记**（RUNBOOK §13.3 #15）。备选被否：② 一次性原子轮换 + 接受存量失效（全员被迫重登，且"原子"在容器编排下难保证）；③ 外部 KMS（超本阶段范围） | **E2E-29** | 🟢 **accepted**（2026-10-07 用户经 AskUserQuestion 拍板「双密钥并存窗口」） |
| **D-45** | **F-199 ① 口型缺陷修复裁定：前端引 `pinyin-pro` 做汉字→拼音韵腹→口型**。根因：BFF phonemes char=原始汉字（T3 1:1 复刻 XTTS per-char，cloud/XTTS 两 provider 同病，E2E-17 时代即潜伏），前端 `charToLipShape` 仅映射拉丁字母 → 汉字全落 neutral（IAB 探针 89/89 neutral，2026-10-07 实测）。备选对比：BFF 侧输出拼音韵母（需改 T3 复刻契约语义 + 两条 provider 路径都接变换 + Go 拼音字典）；伪口型（假动作，否决）。**落地**：`charToLipShape` 拉丁表 miss 后走 `hanziToLipShape`（pinyin-pro 默认读音 + 韵腹优先级 a>o>e>i>u>ü 全词扫描 + jqxy 后 u 实为 ü 特判 + Map 缓存 4096），复用同一张 VOWEL_TO_LIP；TDD RED→GREEN（契约测试 18/18，全仓 vitest 628/628），修复后 IAB 复测 **60/76=78.9% 非 neutral**、零错误。**同轮用户主观判定**：③ 响度合格保持 M1=B（gain=20·log10(volume) clamp）；④ 音色接受保持 anna | **F-198 专项收尾（F-199）** | 🟢 **accepted**（2026-10-07 用户经 AskUserQuestion 拍板「前端 pinyin-pro」+ ③④ 主观判定通过） |
| **D-49** | **E2E-F-207 follow-up 裁定：实现真正的访问令牌滑动续期**（前端主动续期，而非"保持现状仅清死代码"）。背景（E2E-29 补验轮 IAB 实测，账本 E2E-F-207）：前端 `useApi.ts` 的「401 + `code===10002` → 自动续期」是**死代码** —— 全仓无任何后端下发 `code:10002`（BFF 一律 `code:1`；APISIX 返 `{"message":"failed to verify jwt"}`；shared 中间件返 `{"error":"unauthorized"}`），`refreshToken()` 只被该分支调用 ⇒ 从未执行；且 APISIX 在过期令牌上先拒且不带业务 code ⇒ **过期后不可能续期**。**落地**：前端在令牌寿命 **75%** 处调 `POST /api/v1/auth/refresh`（BFF 对有效令牌已返回全新 TTL + 重发 cookie ⇒ **零后端改动**）；触发点 = 启动 / 令牌变化（含 SPA 登录）/ 页面重新可见 / 每次续期成功后自排；续期失败**不登出**（交 401 兜底）；删两处死分支。**备选被否**：① 保持现状（过期即重登，仅清死代码）——用户明确要续期能力；② 改 APISIX 让过期令牌透传到 BFF——网关语义改动、收益仅为兼容死分支；③ 服务端会话替换 JWT——超本次范围。**执行期 2 个运行时问题**（`isAuthenticated` 挡排程 / SPA 登录不排程）已修，见 landed plan §F | **E2E-29 follow-up（F-207）** | 🟢 **accepted**（2026-10-08 用户经 AskUserQuestion 拍板「实现真正的滑动续期」；落地 PR #178） |
