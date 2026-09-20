---
stage: e2e-09
title: 注册流程
status: partial
date: 2026-09-19
verdict: PARTIAL
superseded-note: 2026-09-19 治理轮：由 done 降为 partial（0 张截图、无汇总行、缺「收口自检」章节、2 点未执行）
---

# E2E-09 注册流程 — 执行报告

## 1. 执行摘要

| 维度 | 结果 |
|------|------|
| 测试点 | 12/14 PASS，2 个留账（#12/#13 需 dev DB） |
| Playwright 回归钉 | 11/11 PASS（chromium） |
| Vitest 契约测试 | 23/23 PASS（3 个套件，含 #10 bcrypt 契约） |
| 发现的 bug | 1 个（typecheck TS2322，已修） |
| 范围外发现 | 1 个（typecheck plugin 预存问题，未修） |

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 登录/注册 tab 切换无状态残留 | [A]+[V] | PASS | Playwright: 切换后登录字段清空，切回注册字段保留（reactive 预期行为） |
| 2 | 注册表单不再出现验证码字段 | [A] | PASS | Playwright: `.code-field` 0 个 / `获取验证码` 0 个 / `验证码会打印` 0 个 |
| 3 | 用户名校验边界 | [A] | PASS | Playwright: 空用户名 → 弹框不出现（前端拦截） |
| 4 | 密码强度校验 | [A] | PASS | Playwright: 弱密码 `123` → 弹框不出现（前端 `< 6` 拦截） |
| 5 | 重复用户名 | [A]+[V] | PASS | Playwright: 注册成功 → 同名再注册 → 断言"已存在"错误提示 + URL 留在 /login |
| 6 | 密保弹框出现 | [A]+[V] | PASS | Playwright: 提交注册 → `.sq-overlay` 可见 + `.sq-dialog` 可见 |
| 7 | 弹框含用途提示 | [V] | PASS | Playwright: `找回密码` 文案可见 |
| 8 | 弹框不可跳过 | [A] | PASS | Playwright: 无"跳过"按钮 + 关闭后 URL 仍在 `/login` |
| 9 | 密保答案必填 | [A] | PASS | Playwright: 空答案提交 → `答案不能为空` 可见 + 弹框仍在 |
| 10 | 密保答案不明文落库 | [A] | PASS | Vitest 契约: authlogic.go 使用 password.Hash()（bcrypt）+ AnswerHash 字段 + TrimSpace/ToLower 标准化 |
| 11 | 注册成功 | [A]+[V] | PASS | Playwright: 填写答案 → 提交 → URL 跳转到 `/chat/conversation` |
| 12 | 注册后可直接用于找回密码 | [A] | BLOCKED | 未执行 —— 需端到端联动 E2E-07；阻塞原因为阶段内决定不扩范围（账本 E2E-F-77 留账） |
| 13 | 并发注册同名 | [A] | BLOCKED | 未执行 —— 需并发请求 + DB UNIQUE 断言（账本 E2E-F-78 留账） |
| 14 | 卡片空间未被撑破 | [V] | PASS | Playwright: `.login-card` overflow = `hidden` |

汇总：PASS 12 / FAIL 0 / BLOCKED 2 / N/A 0

> **2026-09-19 治理补账说明**：上表结论**按原报告转录**，仅两处形式修正 ——
> ① #12/#13 的结果列原写 `⬜ 未覆盖`（非法基值，审计器 A11 报 WARN），改为 `BLOCKED` + 原因移入证据列（语义未变，二者本就是"未执行"）；
> ② 补写汇总行（原报告缺失）。
> 真实缺口在证据形态：`screenshots/` 为 0 张 ⇒ #1/#5/#6/#7/#11/#14 的 `[V]` 半只有 DOM/文案断言、**无视觉证据**。故阶段状态由 `done` 降为 `partial`。

## 3. 改动清单

| 文件 | 动作 | 说明 |
|------|------|------|
| `SecurityQuestionDialog.vue` | 新增 | Teleport 弹框组件，不可跳过，答案必填校验 |
| `SecurityQuestionDialog.test.ts` | 新增 | 8 个 source 契约测试 |
| `registration-security-question.test.ts` | 新增 | 12 个注册改造契约测试 |
| `registration.spec.ts` (e2e/) | 新增 | Playwright 回归钉 10 测试点 |
| `login/index.vue` | 改 | 删除验证码 UI/逻辑，提交后弹密保弹框，密码强度校验 |
| `login/registration-security-question.test.ts` | 改 | 新增 #10 bcrypt 契约测试 3 条（authlogic.go 源码断言） |
| `e2e/registration.spec.ts` | 改 | 新增 #5 重复用户名 Playwright 测试 |
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

E2E-09 **partial**（2026-09-19 治理轮由 done 降级）。12/14 测试点的结论维持不变，核心注册流程（验证码删除 + 密保弹框 + 注册成功 + 重复用户名校验 + bcrypt 哈希）已通过 Playwright + Vitest 契约测试验证。降级原因：`screenshots/` 为 0 张（6 个含 `[V]` 的测试点缺视觉证据）+ #12/#13 未执行 + 原报告缺 §10 模板必填章节与汇总行。取证补拍见 §10 与账本 E2E-F-90。

## 8. 修复清单与回归钉

### 8.1 修复清单（TDD 记录）

| # | 缺陷 | 先行失败测试 | 修复锚点 | 本轮复跑证据（2026-09-19） |
|---|------|-------------|---------|--------------------------|
| Bug 1 | typecheck TS2322：`data.answers[i]` 可能 `undefined` 赋给 `answer: string` | CI typecheck 报错即"红" | `emotion-echo-web/app/pages/login/index.vue:213`（`?? ''`） | 未单独复跑 typecheck；改动点未变 |
| Bug 2 | `registration-security-question.test.ts` 用了 `beforeAll` 但未 import | typecheck 报错 | 同上测试文件 import 行 | ✅ **本轮实跑**：`pnpm vitest run app/pages/login/registration-security-question.test.ts app/components/SecurityQuestionDialog.test.ts` → `Test Files 2 passed / Tests 23 passed` |

### 8.2 回归钉

| spec | 用例数 | 原报告记录 | 本轮（2026-09-19）状态 |
|------|--------|-----------|----------------------|
| `emotion-echo-web/e2e/registration.spec.ts` | 10 | 11/11 PASS（chromium） | **未重跑**（取证轮次补跑，见 §10） |
| `emotion-echo-web/app/components/SecurityQuestionDialog.test.ts` | 8 | 8/8（契约） | ✅ 本轮实跑通过（随 §8.1 Bug 2 命令） |
| `emotion-echo-web/app/pages/login/registration-security-question.test.ts` | 15 | 12 + 3 bcrypt 契约（原报告记 23/23 共 3 套件） | ✅ 本轮实跑通过（含 #10 bcrypt 契约断言） |
| 前端全量 vitest | — | 原报告未记全量 | ✅ 本轮实跑：`pnpm test` → `Test Files 49 passed / Tests 410 passed`（2026-09-19） |

## 9. 发现与分类（范围外，已记账）

| 发现 | 分类 | 处置 |
|------|------|------|
| `vue-router@4.6.4` volar 插件兼容性问题（预存，非本轮引入） | 范围外 | 原报告已在结论中登记；不属 E2E 阶段 |
| BFF 层密保校验曾 fail-open（R-01 已修，属 E2E-F-41） | 范围外 | 归 R-01，本轮不重复处置 |

## 10. 待决策 / 升级项

| # | 事项 | 处置 |
|---|------|------|
| 1 | #12（注册→找回密码联动）/ #13（并发同名）未执行 | 账本 E2E-F-77 / E2E-F-78 留账；**环境当前可用**（实测 8 容器 healthy），执行成本低 —— 建议随取证轮次一并执行 |
| 2 | 取证补拍：6 个 `[V]` 点半缺视觉证据 + 回归钉未重跑 | 用户 2026-09-19 决议：归独立取证轮次，账本 E2E-F-90 |
| 3 | 环境基线不可追溯 | 原报告未记录启动命令/容器清单/配置差异（RUNBOOK §2 要求）。补账不臆造，随取证轮次补齐 |

### 截图清单（补拍 2026-09-20）
| 文件 | 视口 | 覆盖 |
|------|------|------|
| `screenshots/01-register-page.png` | 1280×720 | #1（登录/注册 tab 切换，注册页渲染） |

## 11. 收口自检

- [x] report.md 存在且含 §10 模板必填章节（2026-09-19 补账后：测试点结果 ✓ / 收口自检 ✓）
- [x] 汇总行非占位符，且计数与测试点表行数一致（PASS 12 + BLOCKED 2 = 14 = 表行数）
- [x] 结果列仅含合法四值（#12/#13 由 `⬜ 未覆盖` 修正为 `BLOCKED`）
- [x] 阶段状态三处一致：roadmap / plan / report 均为 `partial`（2026-09-19）
- [x] 账本对账：本阶段相关未解决条目 = E2E-F-01 / E2E-F-77 / E2E-F-78 / E2E-F-90 ⇒ 阶段只能为 `partial`
- [x] 修复项与契约测试可复现：本轮实跑 23 条相关契约 + 全量 410 条前端测试均通过
- [ ] 截图归档 —— **未完成**：`screenshots/` 为 0 张
- [ ] 全量 Playwright 重跑与 #12/#13 —— **未完成**（归取证轮次）
- [ ] §2.5 收口自检三连 —— **未执行**（阶段处于 partial）
