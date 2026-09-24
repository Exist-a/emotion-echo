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
| **D-26** | 端侧化混合推理主方案 = 按 v0.2 选型（WebLLM 主力 + MindChat 双轨验证）/ v0.3 阶段切分实施「端侧优先 + 云端兜底」；分项 D-26.1~5 待 §十二 5 项决策拍板 | **端侧化（Lane O 阶段一）** | 🟡 **proposed**（2026-09-24 立项；阶段一不依赖决策门可先行；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-hybrid-main.md` + 计划 `docs/plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md`。**E2E-17 收口时 D-26 让位本方案**；并行隔离见 `docs/_meta/parallel-tracks.md`） |
| **D-26.2** | 端侧主力模型 = **WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC（Apache 2.0）**—— 用户 2026-09-24 会话口头授权选 B 方案（待正式拍板转 accepted）；不选 A MindChat 核心理由 = GPL-3.0 copyleft 锁定商用 + 自编译成本高 + 无 head-to-head 优势证据 + 与项目 Apache-2.0 默认 license 冲突 + MindChat 2024-02-06 后未更新（19 个月）；分级加载（0.6B / 1.7B / 4B）| **端侧化（Lane O T2#3）** | 🟡 **proposed**（2026-09-24 立项；详见 ADR `docs/architecture/adr/adr-2026-09-on-device-model-selection-qwen3.md` + 决策材料 `docs/plans/on-device-model-selection-decision-material-2026-09-24.md`。Apache 2.0 NOTICE 致谢 + WebLLM Dynamic Import + §六握手（`package.json` 改动登记）|
| **D-27** | **Redis = 保留并接入业务（不下线）**：① E2E-20 多实例修复的 `RedisLimiterBackend`（登录锁定/验证码防枚举/APISIX 跨节点限流）；② **记忆系统存储**（与会话记忆待决策联动，见 `docs/plans/conversation-memory-pending-decision-2026-09-24.md` §D-c）；③ token 获取/缓存等后续业务接入。盘点事实（2026-09-24 E2E-18 #6/#7）：当前**零业务引用**（6 svc `redis.NewClient`/`InitRedis` 零命中、`connected_clients:1`、`LimiterBackend`/`InitRedis` 0 caller）⇒ 现状是「**保留待接入**」而非「已接入」——各接入点落地时按所在阶段 TDD + ADR | **E2E-18** | ✅ 已决议（2026-09-24 用户拍板：「redis 正常需要接入业务，所以要保留，比如可以和记忆系统操作、token 获取」；备选=下线（被否，E2E-20 要重拉纯往返）/ 立即接入（超出本阶段边界）） |
