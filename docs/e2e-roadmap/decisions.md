---
status: active
priority: high
created: 2026-09-17
last-refresh: 2026-09-17
type: e2e-transformation-decisions
---

# 改造项决策记录（transformation decisions）

> 建档预探查发现 3 个"设计意图与实现不符"的改造项。此类必须**实测前先对齐方向**，否则会修错方向。
> 每项记录：现状 → 候选方案 → 决议 → 影响面。

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

### 落地要点

| 层 | 改动 |
|----|------|
| 数据库 | `users` 表新增密保字段（`security_question` + `security_answer_hash`，答案同 password_hash 用 bcrypt），或独立 `user_security_answers` 表支持多问题。**归属 E2E-03 数据库改造** |
| 注册流程 | 注册时增加"设定密保问题"步骤（归属 E2E-06） |
| 找回流程 | `verify.vue` 从"输入验证码"改为"回答问题"，`modify.vue` 保留改密；BFF 的 `verification-code` 端点改为 `verify-security-answer`。**归属 E2E-04** |
| 后端 | `user-svc` model/repository/logic 加密保字段读写 + bcrypt 校验 |
| 测试 | 架构测试需同步更新（`username-only-copy.architecture.test.ts` 现锁死"不得出现手机号/邮箱"字样，密保问题文案需另立断言） |

### 附带效应

用户指出"数据库本身也要改了"——本项目数据库确实需要一次改造（死字段清理 + 密保字段 + 迁移治理），已单列为 **E2E-03 数据库改造** 改造阶段，排在 E2E-04 之前（D-01 依赖它的字段）。

---

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
