---
stage: e2e-09
title: 注册流程
status: done
date: 2026-09-19
verdict: PARTIAL
---

# E2E-09 注册流程 — 执行报告

## 1. 执行摘要

| 维度 | 结果 |
|------|------|
| 测试点 | 10/14 PASS，4 个未覆盖（#5/#10/#12/#13） |
| Playwright 回归钉 | 10/10 PASS（chromium） |
| Vitest 契约测试 | 20/20 PASS（2 个新套件） |
| 发现的 bug | 1 个（typecheck TS2322，已修） |
| 范围外发现 | 1 个（typecheck plugin 预存问题，未修） |

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 登录/注册 tab 切换无状态残留 | [A]+[V] | PASS | Playwright: 切换后登录字段清空，切回注册字段保留（reactive 预期行为） |
| 2 | 注册表单不再出现验证码字段 | [A] | PASS | Playwright: `.code-field` 0 个 / `获取验证码` 0 个 / `验证码会打印` 0 个 |
| 3 | 用户名校验边界 | [A] | PASS | Playwright: 空用户名 → 弹框不出现（前端拦截） |
| 4 | 密码强度校验 | [A] | PASS | Playwright: 弱密码 `123` → 弹框不出现（前端 `< 6` 拦截） |
| 5 | 重复用户名 | [A]+[V] | ⬜ 未覆盖 | 需 dev 环境 + DB 断言，本次未执行 |
| 6 | 密保弹框出现 | [A]+[V] | PASS | Playwright: 提交注册 → `.sq-overlay` 可见 + `.sq-dialog` 可见 |
| 7 | 弹框含用途提示 | [V] | PASS | Playwright: `找回密码` 文案可见 |
| 8 | 弹框不可跳过 | [A] | PASS | Playwright: 无"跳过"按钮 + 关闭后 URL 仍在 `/login` |
| 9 | 密保答案必填 | [A] | PASS | Playwright: 空答案提交 → `答案不能为空` 可见 + 弹框仍在 |
| 10 | 密保答案不明文落库 | [A] | ⬜ 未覆盖 | 需 DB 查询断言 bcrypt hash，本次未执行 |
| 11 | 注册成功 | [A]+[V] | PASS | Playwright: 填写答案 → 提交 → URL 跳转到 `/chat/conversation` |
| 12 | 注册后可直接用于找回密码 | [A] | ⬜ 未覆盖 | 需端到端联动 E2E-07，本次未执行 |
| 13 | 并发注册同名 | [A] | ⬜ 未覆盖 | 需并发请求 + DB UNIQUE 断言，本次未执行 |
| 14 | 卡片空间未被撑破 | [V] | PASS | Playwright: `.login-card` overflow = `hidden` |

## 3. 改动清单

| 文件 | 动作 | 说明 |
|------|------|------|
| `SecurityQuestionDialog.vue` | 新增 | Teleport 弹框组件，不可跳过，答案必填校验 |
| `SecurityQuestionDialog.test.ts` | 新增 | 8 个 source 契约测试 |
| `registration-security-question.test.ts` | 新增 | 12 个注册改造契约测试 |
| `registration.spec.ts` (e2e/) | 新增 | Playwright 回归钉 10 测试点 |
| `login/index.vue` | 改 | 删除验证码 UI/逻辑，提交后弹密保弹框，密码强度校验 |
| `stores/user.ts` | 改 | 删除 `sendVerificationCode`（死代码） |
| `types/api.ts` | 改 | `RegisterParams`: `verificationCode` → `securityQuestions[]` |
| `types/login/loginType.ts` | 改 | 删除 `verificationCode` 字段 |
| `verificationCodeCountDown.ts` + test | 删 | 死代码清理 |

## 4. 发现并修复的 bug

### Bug 1：typecheck TS2322 — answers[i] 可能 undefined

- **严重度**：中（CI typecheck 失败）
- **根因**：`data.answers[i]` 类型为 `string | undefined`，赋值给 `answer: string` 不兼容
- **影响**：CI web-test workflow typecheck 步骤失败
- **修复**：`data.answers[i]` → `data.answers[i] ?? ''`
- **文件**：`emotion-echo-web/app/pages/login/index.vue:213`

### Bug 2：test 缺少 beforeAll import（发现但未计入 bug）

- **严重度**：低（typecheck 报错，vitest 不受影响）
- **根因**：`registration-security-question.test.ts` 使用 `beforeAll` 但未从 vitest import
- **修复**：import 补 `beforeAll`
- **文件**：`emotion-echo-web/app/pages/login/registration-security-question.test.ts`

## 5. 产出物

| 产出物 | 路径 |
|--------|------|
| 密保弹框组件 | `emotion-echo-web/app/components/SecurityQuestionDialog.vue` |
| 组件契约测试（8） | `emotion-echo-web/app/components/SecurityQuestionDialog.test.ts` |
| 注册改造契约测试（12） | `emotion-echo-web/app/pages/login/registration-security-question.test.ts` |
| Playwright 回归钉（10） | `emotion-echo-web/e2e/registration.spec.ts` |
| 本报告 | `docs/e2e-roadmap/stages/e2e-09-registration/report.md` |

## 6. 调研依据

- 已读：`login/index.vue`（完整）、`stores/user.ts`（lines 160-250）、`types/api.ts`（完整）、`SecurityQuestionDialog.vue`（新建）、`auth_handler.go`（lines 160-215）、`authlogic.go`（lines 89-153）
- 已查：E2E-09 plan.md、E2E-07 report.md、E2E-08 report.md（模板参考）
- 后端无需改动：BFF register handler 已支持 securityQuestions（D-05 决议）

## 7. 结论

E2E-09 **partial done**。核心注册流程（验证码删除 + 密保弹框 + 注册成功）已实现并通过 Playwright 验证。4 个测试点（#5 重复用户名、#10 不明文落库、#12 找回密码联动、#13 并发同名）未覆盖，均需 dev 环境 DB 访问，留作后续补充。

**未解决项**：
- 测试点 #5/#10/#12/#13 待补充
- `vue-router@4.6.4` volar 插件兼容性问题（预存，非本次引入）
