---
stage: e2e-07
title: E2E-07 找回/重置密码 执行记录
type: report
status: done
started: 2026-09-19
completed: 2026-10-13
superseded-note: 2026-09-19 治理轮：由 done 降为 partial（原报告无逐测试点结果、0 张截图、回归钉未记录运行）；2026-10-13 收口轮：逐点复验 + 回归钉首次真跑 12/12 + 修 3 处真缺陷后回升 done（BLOCKED 1/13 = 7.7% ≤ 1/3）。
---

# E2E-07 找回/重置密码 执行记录

> **2026-10-13 收口轮**：本阶段被 R-02 降为 `partial` 的根因是「6 个测试点无证据 + 回归钉从未运行」。
> 本轮**逐点复验**：回归钉 `password-recovery.spec.ts` **首次真正运行** → **12 passed / 1 skipped**；
> 并因此抓到 **3 处真缺陷**（见 §八）。**BLOCKED 6 → 1（仅 #6 [M]），13 点 = 12 PASS + 1 BLOCKED。**

## 一、阶段目标

把找回密码从"手机号短信验证码"改造为**密保问题流程**（D-01=C），端到端跑通。

## 二、环境基线

| 项 | 值 |
|----|-----|
| 操作系统 | Windows 10 |
| Go 版本 | 1.26.1 |
| Node 版本 | 24（收口轮实测） |
| Docker 栈 | 25 容器 healthy（APISIX `:19080` 通） |
| 前端 | 本地 `pnpm dev`（`:3000`，CORS 白名单内） |
| 测试账号 | test_user / test123456（密保答案：kitty，questionOrder=1） |
| 夹具准备 | **注册接口直建**（`POST /api/v1/auth/register` 带 `securityQuestions`）；原 `scripts/seed_security_question.sh` 已失效（打 `:8888`，该宿主端口早被收掉，且只打印手改 SQL） |

## 三、执行结果

### 3.1 改造清单

| 层 | 文件 | 改动 | 状态 |
|----|------|------|------|
| proto | `proto/user.proto` | 新增 `GetSecurityQuestionsByUsername` RPC | ✅ |
| user-svc logic | `internal/logic/authlogic.go` | 新增方法 + 答案归一化（trim+忽略大小写） | ✅ |
| user-svc handler | `internal/handler/auth_handler.go` | 新增 `GetSecurityQuestionsHandler` | ✅ |
| user-svc gRPC | `internal/grpcserver/user_server.go` | 新增 gRPC 实现 | ✅ |
| user-svc 路由 | `main.go` | 注册 `GET /security-questions` | ✅ |
| user-svc 拦截器 | `internal/grpcserver/server.go` | 匿名跳过清单新增 | ✅ |
| BFF downstream | `internal/downstream/user.go` | 接口 + HTTP 实现 | ✅ |
| BFF downstream | `internal/downstream/user_grpc.go` | gRPC 实现 | ✅ |
| BFF handler | `internal/handler/auth_handler.go` | `security-questions` case + resetToken 签发 | ✅ |
| BFF auth | `internal/auth/jwt.go` | 新增 `SignResetToken` / `ParseResetToken` | ✅ |
| APISIX | `deploy/apisix/seed.sh` | route 118 白名单 | ✅ |
| 前端路由 | `app/lib/apiRoutes.ts` | 新增 2 个路由 | ✅ |
| 前端 store | `app/stores/user.ts` | 新增 2 个方法 + 导出 | ✅ |
| 前端状态 | `app/composables/forgetPwdState.ts` | securityVerified + resetToken | ✅ |
| verify.vue | `app/pages/login/forget/verify.vue` | 验证码 → 密保问题 | ✅ |
| modify.vue | `app/pages/login/forget/modify.vue` | 使用 resetToken | ✅ |
| middleware | `app/middleware/forgetPwd.ts` | securityVerified 检查 | ✅（**收口轮修正：服务端不判断**，见 §八） |
| 架构测试 | `username-only-copy.architecture.test.ts` | 4 个新断言 | ✅ |
| E2E spec | `e2e/password-recovery.spec.ts` | 13 个测试点 | ✅（**收口轮修正 4 处**，见 §八） |
| 测试夹具 | `scripts/seed_security_question.sh` | 新建 | ⚠️ **已失效**（见 §二；改用注册接口） |

### 3.2 测试验证

| 层 | 结果 |
|----|------|
| user-svc 单元测试 | ✅ 4 新测试通过 |
| user-svc handler 测试 | ✅ 3 新测试通过 |
| BFF handler 测试 | ✅ 3 新测试通过 |
| 前端 vitest | ✅ 370/370 通过（收口轮复测：全仓 **658/658**） |
| 架构测试 | ✅ 7/7 通过 |
| 全仓 Go 测试 | ✅ 6 模块全部通过 |

### 3.3 端到端实测

| 步骤 | 结果 | 证据 |
|------|------|------|
| 获取密保问题 | ✅ | 返回"你的第一只宠物叫什么？" |
| 验证密保答案 | ✅ | 返回 resetToken |
| 重置密码 | ✅ | success: true |
| 旧密码登录 | ❌ | invalid username or password |
| 新密码登录 | ✅ | 返回 accessToken |
| 数据库密码落库 | ✅ | password_hash 已更新 |
| 密保问题存库 | ✅ | user_security_answers 有记录 |

### 3.4 决策落地

| 决策 | 选择 | 实现 |
|------|------|------|
| 答案归一化 | A: trim + 忽略大小写 | `strings.ToLower(strings.TrimSpace(answer))` |
| 验证结果传递 | A: resetToken | JWT 5分钟有效 |
| 答案提交方式 | A: 逐题验证 | 最后一题成功后获取 resetToken |

## 四、发现与遗留

| 编号 | 说明 | 归属 |
|------|------|------|
| - | 输入框样式已改进（圆角 + focus 高亮） | 已完成 |
| - | 浏览器缓存导致旧版本显示，需强制刷新 | 已知限制 |

## 五、DoD 核对

- [x] 密保问题流程端到端跑通
- [x] 数据库落库验证
- [x] 答案归一化（trim + 忽略大小写）
- [x] resetToken 安全机制
- [x] 防枚举（用户不存在返回空列表）
- [x] 前端 vitest 全绿
- [x] 全仓 Go 测试全绿
- [x] 架构测试更新
- [x] E2E Playwright spec 编写 **且首次真跑通过（2026-10-13）**

## 六、判定

**done**（2026-10-13 收口轮）。功能实现、API 层与**页面层**端到端均已实测；回归钉 **12/12 通过**；`[V]` 证据 2 张；逐点结果见 §七。

---

## 七、测试点结果（2026-10-13 收口轮逐点复验）

> **取证方式**：回归钉 `emotion-echo-web/e2e/password-recovery.spec.ts` **首次真正运行**
> （`BASE_URL=http://localhost:3000 npx playwright test e2e/password-recovery.spec.ts --project=chromium`
> → **12 passed / 1 skipped**，1.3m），并按需以网关直测交叉验证。
> 原 6 个 `BLOCKED` 是"**证据未记录**"（非功能坏了）；本轮补齐后仅剩 #6（显式 `[M]`）。

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 登录页"忘记密码"入口可达 | [A]+[V] | PASS | spec `#1` 通过（点击后 URL → `/login/forget`）；截图 `screenshots/01-forgot-password.png` |
| 2 | 步骤 1：输入用户名 → 展示该用户密保问题 | [A]+[V] | PASS | spec `#2` 通过（`.question-text` 可见且含"宠物/城市/问题"，**未回显答案**）；截图 `screenshots/02-security-question-chromium.png`（2026-10-13 新增） |
| 3 | 不存在的用户名 → 防枚举响应 | [A] | PASS | spec `#3` 通过；**API 实测**：`POST /auth/security-questions {username:nonexistent_user_xyz}` → **200 + `{"questions":[]}`**（与"存在但无密保"同一响应，不可区分） |
| 4 | 步骤 2：答案正确 → 进入改密页 | [A] | PASS | spec `#4` 通过（URL → `/login/forget/modify`） |
| 5 | 步骤 2：答案错误 → 拒绝且不泄露答案 | [A] | PASS | spec `#5` 通过（**停留 verify 页** + `.notify-card` 提示）；API 对照：错答案 **401 `security answer verification failed`**（不泄露） |
| 6 | 错误次数限制 | [M] | **BLOCKED** | spec 内为 `test.skip('#6 … [M] 需人工裁定')` —— 限流语义需产品裁定（关联 E2E-F-25 登录锁定的多实例语义） |
| 7 | 步骤 3：改密成功 | [A] | PASS | spec `#7` 通过（URL → `/login/forget/success` + "密码已更新"） |
| 8 | 新密码可登录 | [A] | PASS | spec `#8` 通过（`newpwd123` 登录返回 accessToken） |
| 9 | 旧密码失效 | [A] | PASS | spec `#9` 通过（旧密码 `invalid username or password`） |
| 10 | 路由守卫：跳过步骤 | [A] | PASS | spec `#10` 通过（直接访问 `/login/forget/modify` → 被重定向 `/login/forget/verify`） |
| 11 | localStorage 步骤持久化 | [A] | PASS | spec `#11` 通过（写入 localStorage 后刷新/跳转 → 可进 `/login/forget/modify`）。**修前 FAIL，真因见 §八** |
| 12 | 密码强度校验 | [A] | PASS | spec `#12` 通过（弱密码 `123` → 停留 modify 页，未提交） |
| 13 | 页面文案无"手机号/邮箱" | [A] | PASS | 真跑 `npx playwright test e2e/password-recovery.spec.ts -g "#13"` → **1 passed**；另 `pnpm vitest run app/pages/login/forget/username-only-copy.architecture.test.ts` → **Tests 7 passed** |

汇总：PASS 12 / FAIL 0 / BLOCKED 1 / N/A 0

**判定分布**：`[A]` 13 个（#1/#2 兼 `[V]`）。`BLOCKED` **1/13 = 7.7% ≤ 1/3** ⇒ 依 RUNBOOK §4 本阶段**可判 done**。

## 八、修复清单（TDD 记录）

本阶段为**改造实施**，无独立缺陷修复 commit。**2026-10-13 收口轮**因"回归钉首次运行"而暴露并修复 **3 处真缺陷 + 4 处 spec 自身缺陷**：

| 类别 | 位置 | 问题 | 修法 | RED → GREEN |
|------|------|------|------|-------------|
| **真缺陷 ①** | `app/composables/useApi.ts:320` | **任何 401 都被当成"会话过期"** ⇒ `clearAuth()` + 跳 `/login`。而"密保答案错误"按防枚举设计**统一返 401** ⇒ 用户答错一题就被**踢回登录页**（`#5` 实测 URL 变 `/login`） | 仅当**请求带了令牌**时才按会话失效处理；**匿名请求的 401 视为业务错误**（抛出，不清态/不跳转） | `#5` FAIL（URL=/login）→ PASS（停留 verify） |
| **真缺陷 ②** | `app/middleware/forgetPwd.ts` | 步骤守卫在**服务端**也执行，而 `useForgetPwdState()` 只在客户端读 localStorage ⇒ 服务端恒判"步骤不足" ⇒ **刷新 `/login/forget/modify` 必被弹回 verify**，"步骤持久化"（#11）永远不成立 | 中间件**服务端直接放行**（`if (import.meta.server) return`），判断交给客户端（此时 localStorage 可见） | `#11` FAIL（URL 停在 verify）→ PASS |
| **真缺陷 ③** | 同上（连带） | `#12` 同因（改密页进不去 ⇒ 输入框不存在） | 同 ② | `#12` FAIL → PASS |
| spec 缺陷 | `e2e/password-recovery.spec.ts` | ① 通知 locator `.el-notification, .notify, [role="alert"]` **早已失效**（Element Plus 已迁自研 toast → `.notify-card`/`role=status`）⇒ `#3/#5` 恒"找不到提示"；② `#7` `getByText('密码已更新')` **strict-mode 命中 2 元素**；③ `#11/#12` 在 `page.evaluate`（浏览器上下文）里引用 **Node 侧常量 `TEST_USERNAME`**（`ReferenceError`） | 修 locator → `.notify-card`；`#7` 加 `.first()`；`#11/#12` 改为 `page.evaluate((u)=>{…}, TEST_USERNAME)` 传参 | 修后 `#3/#7/#11/#12` 全 PASS |

**关联**：本阶段链路在 R-01 轮曾被查出**三层同时阻断**（APISIX 白名单缺路由 117、BFF gRPC 客户端为 `not implemented` 桩、user-svc 拦截器匿名跳过清单漏配），由 R-01 修复并以**负向对照**证明测试有效（账本 E2E-F-60/E2E-F-71/E2E-F-72）。该项归属 R-01。

## 九、回归钉

| spec | 用例数 | 运行状态 |
|------|--------|---------|
| `emotion-echo-web/e2e/password-recovery.spec.ts` | 13（12 跑 + 1 skip `[M]`） | ✅ **2026-10-13 首次真跑：12 passed / 1 skipped（1.3m）**，chromium |
| `emotion-echo-web/app/pages/login/forget/username-only-copy.architecture.test.ts` | 7 | ✅ 通过 |

**夹具注意（本轮实测的坑，供后续参考）**：`#7/#8/#9` 会把 `test_user` 的密码改成 `newpwd123` **且不还原** ⇒ **跑完必须复位**（本轮用 `verify-security-answer` + `reset-password` 流程复位回 `test123456`，过程中顺带验证了该 API 链路）。该 non-idempotency 是 spec 的已知弱点（未改，避免扩大改动面）。

## 十、待决策 / 升级项

| # | 事项 | 处置 |
|---|------|------|
| 1 | **#6 错误次数限制 [M]** | 需产品裁定限流语义（与 E2E-F-25 登录锁定的多实例语义同源）；spec 内显式 `test.skip` 并标注 `[M]`，**不伪装成 PASS** |
| 2 | **E2E-F-144 验证码端点残留** | 本阶段无关联改动；已**转「专项 · 验证码端点清理」**（D-01/D-28 遗留物，删除方案明确，待独立小 PR） |
| 3 | `scripts/seed_security_question.sh` 已失效（打 `:8888`） | 记账本 **E2E-F-222**（夹具脚本陈旧，改用注册接口带 `securityQuestions`） |

### 截图清单
| 文件 | 覆盖 |
|------|------|
| `screenshots/01-forgot-password.png` | #1（入口可达） |
| `screenshots/02-security-question-chromium.png` | #2（密保问题展示且不泄露答案；2026-10-13 新增） |

## 十一、收口自检

- [x] report.md 存在且含 RUNBOOK §10 模板必填章节（测试点结果 / 收口自检）
- [x] 汇总行非占位符，且计数与测试点表行数一致（PASS 12 + BLOCKED 1 = 13 = 表行数）
- [x] 阶段状态三处一致：roadmap / plan / report 均为 `done`（2026-10-13）
- [x] `BLOCKED` 1/13 = 7.7% ≤ 1/3（RUNBOOK §4 允许判 done）
- [x] 账本对账：本阶段名下未解决条目 E2E-F-144 已转专项 ⇒ A5 干净
- [x] 截图归档：2 张（覆盖 2 个 `[V]` 测试点）
- [x] 回归钉运行记录：`password-recovery.spec.ts` 12 passed / 1 skipped（2026-10-13）
- [x] §2.5 收口自检三连
