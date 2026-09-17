---
stage: e2e-09
title: 注册流程
type: verification + transformation
status: pending
created: 2026-09-17
depends-on: [e2e-06, e2e-07, e2e-04]
blocks: []
gate: [register-verification-code-disposition]   # 见 RUNBOOK §9，未落定不得开工
related-findings: [E2E-F-01]
---

# E2E-09 注册流程

## 1. 阶段目标

跑通注册全流程，并接入 D-01 新增的**密保问题设定步骤**。当前注册依赖"验证码"，而验证码**无真实投递渠道**（dev 靠 `BFF_DEV_RETURN_CODE=1` 回显）——本阶段需明确该步骤在密保方案下的最终形态。

## 2. 范围与边界

### 做

| 功能 | 现状 | 目标 |
|------|------|------|
| 注册 tab 切换 | `web/app/pages/login/index.vue`（登录/注册同页 tab） | 验证 |
| 用户名/密码校验 | `registerInfo` + 前端校验（`:138`） | 验证边界（长度/字符/空值） |
| 验证码发送 | `userStore.sendVerificationCode({type:'register'})`；BFF 60s 冷却 + 防枚举 | **评估去留**：密保方案下是否还需要验证码步骤？ |
| 验证码校验 | BFF `verifyVerificationCode` → `user.Register` | 同上 |
| 密保问题设定 | ❌ 无 | **新增**（D-01=C 需要用户在注册时设定） |
| 注册后行为 | 返回 LoginData（自动登录？） | 验证 |
| 重复用户名 | `users.username UNIQUE` | 验证错误提示友好 |

涉及文件：

| 层 | 文件 |
|----|------|
| 页面 | `web/app/pages/login/index.vue`（注册 tab） |
| 状态 | `web/app/stores/user.ts`（`register()` / `sendVerificationCode()`） |
| BFF | `bff/internal/handler/auth_handler.go`（`register` / `verification-code`） |
| 用户服务 | `user-svc` `Register`（bcrypt） |

### 不做（边界）

- 第三方登录（OAuth 已删，ADR-21）
- 邮箱/手机号采集（D-01 已排除，现 users 表 phone/email 字段在 E2E-06 删除）
- 注册后的新手引导

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-06（密保字段就位；phone/email 已删） | ⏳ |
| E2E-07（密保问题读写链路已验证） | ⏳ |
| E2E-04（前端 lint/typecheck 门槛，注册页改动受保护） | ⏳ |
| **待决策**：密保方案下验证码步骤的去留 | ⚠️ 需明确 |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | 登录/注册 tab 切换无状态残留 | 来回切换 3 次，断言表单字段清空/保留符合预期 | 截图 | ⬜ |
| 2 | 用户名校验边界 | 空/超长/特殊字符 → 断言拒绝且提示清晰 | 断言 | ⬜ |
| 3 | 密码强度校验 | 弱密码 → 断言拒绝 | 断言 | ⬜ |
| 4 | 重复用户名 | 用已存在用户名注册 → 断言明确错误提示 | 断言 + 截图 | ⬜ |
| 5 | 验证码步骤（若保留） | 发送 → 60s 冷却断言 → 错误码断言 | 断言 | ⬜ |
| 6 | **密保问题设定步骤** | 断言注册表单含密保问题选择 + 答案输入；提交后 DB 断言哈希已存 | 截图 + DB 断言 | ⬜ |
| 7 | 密保答案不可明文落库 | 查 DB 断言为 bcrypt 哈希 | 查询输出 | ⬜ |
| 8 | 注册成功 | 断言成功并进入预期后续（自动登录 or 跳登录页） | 截图 | ⬜ |
| 9 | 注册后可直接用于找回密码 | 新账号 → 走 E2E-07 流程 → 断言密保问题可答对 | 端到端 | ⬜ |
| 10 | 注册防枚举 | 已存在/不存在用户名的响应不可区分 | 断言 | ⬜ |
| 11 | 并发注册同名 | 并发提交同名 → 断言只有一个成功（UNIQUE 约束生效） | 断言 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 11 个测试点通过
- [ ] 密保问题设定步骤落地且答案哈希存储
- [ ] 新注册账号可完整走通找回密码（E2E-07 联动验证）
- [ ] 回归钉写入 `emotion-echo-web/e2e/registration.spec.ts`

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 验证码步骤去留未定 → 本阶段范围可能变动 | **先决策再开工**；若保留验证码但无投递渠道，需明确其定位（仅演示？） |
| 注册必须设密保会抬高注册门槛，影响体验 | 评估是否允许"跳过密保"（则找回密码需降级路径）；此决策需用户确认 |
| 用户名 UNIQUE 冲突的错误映射可能暴露技术细节（500 vs 友好提示） | 测试点 4/11 覆盖 |

## 7. 产出物

- 注册页密保设定 UI + BFF/user-svc 支持
- Playwright spec：`emotion-echo-web/e2e/registration.spec.ts`
- 执行记录：`stages/e2e-09-registration/report.md`
