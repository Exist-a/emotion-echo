---
stage: e2e-07
title: 找回/重置密码
type: verification + transformation
status: pending
created: 2026-09-17
depends-on: [e2e-06, e2e-01, e2e-04]
blocks: []
related-findings: [E2E-F-01]
---

# E2E-07 找回/重置密码

## 1. 阶段目标

把找回密码从"手机号短信时代的三步向导 + 无投递渠道的验证码"改造为**密保问题流程**（D-01=C），并端到端跑通。

## 2. 范围与边界

### 做

**改造部分**（D-01=C，见 [decisions.md](../decisions.md)）：

| 层 | 现状 | 改造后 |
|----|------|--------|
| 数据库 | 无密保字段 | 由 E2E-06 提供 `security_question` + `security_answer_hash` |
| `verify.vue` | 输入"6 位数字验证码"（`formInfo.verificationCode`） | 输入**密保问题答案** |
| BFF | `POST /auth/verification-code`（仅 in-memory 存码，dev 靠 `BFF_DEV_RETURN_CODE=1` 回显） | `POST /auth/verify-security-answer` |
| user-svc | `POST /api/v1/users/reset-password` 已有 | 复用，加答案校验前置 |
| 架构测试 | `username-only-copy.architecture.test.ts` 锁死"不得出现手机号/邮箱字样" | **需新增**密保问题文案断言（不能直接复用现有断言） |

**验证部分**：三步向导（`index.vue` 步骤条 → `verify.vue` → `modify.vue` → `success.vue`）端到端 + 路由守卫 + localStorage 步骤持久化。

涉及文件：

| 层 | 文件 |
|----|------|
| 页面 | `web/app/pages/login/forget/{index,verify,modify,success}.vue` |
| 状态 | `web/app/composables/forgetPwdState.ts`（localStorage: `forgetPwdStep`/`Account`/`Code`） |
| 守卫 | `web/app/middleware/forgetPwd.ts` |
| BFF | `bff/internal/handler/auth_handler.go` |
| 用户服务 | `user-svc` `reset-password` |

### 不做（边界）

- 短信/邮件投递（D-01 已排除）
- 运维离线重置脚本（A 方案已排除）
- 密保问题的**注册侧设定**流程（归 E2E-09）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-06 完成（密保字段就位） | ⏳ **硬前置** |
| E2E-01 完成（会话状态稳定，找回后要能登录） | ⏳ |
| 测试账号已设置密保问题（需 E2E-09 或种子数据） | ⏳ |
| `BFF_DEV_RETURN_CODE=1` 的作用重新评估（密保流程不再需要验证码回显） | 需确认是否移除 |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | 登录页"忘记密码"入口可达 | 点击 → 断言进入 `/login/forget` | 截图 | ⬜ |
| 2 | 步骤 1：输入用户名 → 展示该用户的密保问题 | 断言问题文案出现（**不泄露答案**） | 截图 | ⬜ |
| 3 | 不存在的用户名 → 防枚举响应 | 断言错误文案与已存在用户**不可区分**（或统一提示） | 断言 | ⬜ |
| 4 | 步骤 2：答案正确 → 进入改密页 | 断言跳转 `modify` | 截图 | ⬜ |
| 5 | 步骤 2：答案错误 → 拒绝且不泄露答案 | 断言停留 + 错误提示 | 截图 | ⬜ |
| 6 | 错误次数限制 | 连续错误 N 次后断言被限流/锁定 | 断言 | ⬜ |
| 7 | 步骤 3：改密成功 | 断言进入 `success` 页 | 截图 | ⬜ |
| 8 | 新密码可登录 | 登出 → 用新密码登录 → 成功 | 截图 | ⬜ |
| 9 | 旧密码失效 | 用旧密码登录 → 断言失败 | 断言 | ⬜ |
| 10 | 路由守卫：跳过步骤 | 直接导航 `/login/forget/modify`（未完成 verify）→ 断言被拦回上一步 | 断言 URL | ⬜ |
| 11 | localStorage 步骤持久化 | 刷新页面 → 断言停留在当前步骤；完成后断言被清理 | 断言 | ⬜ |
| 12 | 密码强度校验 | 提交弱密码 → 断言拒绝 | 断言 | ⬜ |
| 13 | 页面文案无"手机号/邮箱" | 现有架构测试仍绿 + 新增密保文案断言 | 测试输出 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 13 个测试点通过
- [ ] 改造按 TDD 走（先红后绿）：验证码→密保问题的前后端改动都有失败测试先行
- [ ] 防枚举与限流行为有断言（安全语义不能被改造破坏）
- [ ] 回归钉写入 `emotion-echo-web/e2e/password-recovery.spec.ts`
- [ ] `forgetPwdState.ts` 的 localStorage 键名更新（`Code` → 答案相关）或删除

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 密保答案的安全性弱（可猜、无投递验证） | 明确记录为**已知安全权衡**（D-01 用户已接受）；加错误次数限制；答案 bcrypt 存储 |
| `username-only-copy.architecture.test.ts` 会拦截含"邮箱"字样文案 | 新文案避免该词；新增独立断言文件而非修改原文件语义 |
| `BFF_DEV_RETURN_CODE=1` 移除后可能影响其他流程（注册验证码） | 先确认该开关的完整影响面（注册流程 E2E-09 仍可能需要） |
| 密保问题在旧用户上未设置 | 定义降级路径（提示"未设置密保，请联系管理员"—注意系统**无 admin 概念**，文案需另定） |

## 7. 产出物

- 改造后的 `verify.vue` / BFF 端点 / user-svc 校验
- Playwright spec：`emotion-echo-web/e2e/password-recovery.spec.ts`
- 执行记录：`stages/e2e-07-password-recovery/report.md`
