---
stage: e2e-07
title: E2E-07 找回/重置密码 执行记录
type: report
status: partial
started: 2026-09-19
completed: 2026-09-19
superseded-note: 2026-09-19 治理轮：由 done 降为 partial（原报告无逐测试点结果、0 张截图、回归钉未记录运行）
---

# E2E-07 找回/重置密码 执行记录

## 一、阶段目标

把找回密码从"手机号短信验证码"改造为**密保问题流程**（D-01=C），端到端跑通。

## 二、环境基线

| 项 | 值 |
|----|-----|
| 操作系统 | Windows 10 |
| Go 版本 | 1.26.1 |
| Node 版本 | 20 |
| Docker Desktop | 运行中 |
| 测试账号 | test_user / test123456（密保答案：kitty） |

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
| middleware | `app/middleware/forgetPwd.ts` | securityVerified 检查 | ✅ |
| 架构测试 | `username-only-copy.architecture.test.ts` | 4 个新断言 | ✅ |
| E2E spec | `e2e/password-recovery.spec.ts` | 13 个测试点 | ✅ |
| 测试夹具 | `scripts/seed_security_question.sh` | 新建 | ✅ |

### 3.2 测试验证

| 层 | 结果 |
|----|------|
| user-svc 单元测试 | ✅ 4 新测试通过 |
| user-svc handler 测试 | ✅ 3 新测试通过 |
| BFF handler 测试 | ✅ 3 新测试通过 |
| 前端 vitest | ✅ 370/370 通过 |
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
- [x] E2E Playwright spec 编写

## 六、判定

**partial**（2026-09-19 治理轮由 `done` 降级）。功能实现与 API 层实测已具备，但**原报告未按 plan 的 13 个测试点逐点记录结果、`screenshots/` 为 0 张、回归钉只记"编写"未记运行**，不满足 [RUNBOOK](../../RUNBOOK.md) §7 收口契约。逐点结论见 §七。

---

## 七、测试点结果（2026-09-19 治理补账）

> **本节性质**：2026-09-19 治理轮补写。原报告 §三 只有"改造清单 / 测试验证 / 端到端实测"三张表，**未按 plan 的 13 个测试点逐点记录**。
> 本节按其**已有记录 + 账本既有实测**转录，**不新增、不修改任何验证结论**；无记录者标 `BLOCKED` 并注明原因。
> 标注 `BLOCKED` 的含义是"**该点证据未记录**"，**不是**"功能坏了"。本轮另实跑了当前可执行的自动测试作为补充证据（见 #13）。
> 取证补拍（6 个 BLOCKED 点 + 全部 `[V]` 截图）按用户 2026-09-19 决议归入独立轮次，见 §十与账本 E2E-F-90。

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 登录页"忘记密码"入口可达 | [A]+[V] | BLOCKED | 原报告未记录本点执行证据（§五 DoD 只勾"E2E spec 编写"，无运行输出；截图缺失 → 视觉证据未取证） |
| 2 | 步骤 1：输入用户名 → 展示该用户密保问题 | [A]+[V] | PASS | 原报告 §3.3 端到端实测 → 获取密保问题返回"你的第一只宠物叫什么？"（仅回显问题文案，未回显答案）。**[A] 半通过；[V] 半未取证（截图缺失）**；原报告未做"不泄露答案"的字段级断言 |
| 3 | 不存在的用户名 → 防枚举响应 | [A] | PASS | 账本 E2E-F-60（R-01，2026-09-18 端到端实测）→ 未知用户返回 401，与已存在用户的错误响应不可区分 |
| 4 | 步骤 2：答案正确 → 进入改密页 | [A] | BLOCKED | 原报告 §3.3 只记 API 层"验证密保答案 → 返回 resetToken"，**未记页面跳转**；无运行输出 |
| 5 | 步骤 2：答案错误 → 拒绝且不泄露答案 | [A] | PASS | 账本 E2E-F-60（R-01 实测）→ 错答案返回 401 |
| 6 | 错误次数限制 | [A] | BLOCKED | 原报告未记录。相关背景：账本 E2E-F-25 记登录失败锁定为 in-memory 实现、多实例下静默失效（归 E2E-20），本点的限流语义需重跑才能定 |
| 7 | 步骤 3：改密成功 | [A] | PASS | 原报告 §3.3 → 重置密码返回 `success: true`；数据库 `password_hash` 已更新 |
| 8 | 新密码可登录 | [A] | PASS | 原报告 §3.3 → 新密码登录返回 `accessToken` |
| 9 | 旧密码失效 | [A] | PASS | 原报告 §3.3 → 旧密码登录返回 `invalid username or password` |
| 10 | 路由守卫：跳过步骤 | [A] | BLOCKED | 原报告未记录。`app/middleware/forgetPwd.ts` 出现在 §3.1 改造清单中，但**存在性不构成行为证据**（RUNBOOK §4.1） |
| 11 | localStorage 步骤持久化 | [A] | BLOCKED | 原报告未记录运行证据（§3.1 列了 `forgetPwdState.ts` 的 securityVerified/resetToken，同属存在性） |
| 12 | 密码强度校验 | [A] | BLOCKED | 原报告未记录（§3.3 用的是合法密码 `test123456`，未测弱密码被拒） |
| 13 | 页面文案无"手机号/邮箱" | [A] | PASS | **本轮实跑**（2026-09-19）：`pnpm vitest run app/pages/login/forget/username-only-copy.architecture.test.ts` → `Test Files 1 passed / Tests 7 passed`，含密保文案断言 |

汇总：PASS 7 / FAIL 0 / BLOCKED 6 / N/A 0

**判定分布**：`[A]` 13 个（#1/#2 兼 `[V]`）。`BLOCKED` 6/13 = 46% > 1/3 —— 依 RUNBOOK §4 本阶段**不得判 done**，与 §六 的 `partial` 一致。

## 八、修复清单（TDD 记录）

本阶段为**改造实施**（密保问题流程替换短信验证码），无独立缺陷修复 commit，故无 Red→Green 记录。

需注意的关联修复：本阶段链路在 R-01 轮被查出**三层同时阻断**（APISIX 白名单缺 `/verify-security-answer` 路由 117、BFF gRPC 客户端为 `not implemented` 桩、user-svc 拦截器匿名跳过清单漏配），由 R-01 修复并以**负向对照**证明测试有效（账本 E2E-F-60/E2E-F-71/E2E-F-72）。该项归属 R-01，不在本阶段范围内。

## 九、回归钉

| spec | 用例数 | 运行状态 |
|------|--------|---------|
| `emotion-echo-web/e2e/password-recovery.spec.ts` | 13 测试点 | ⚠️ **原报告只记"编写"（§五 DoD 第 9 项），未记录任何运行输出** —— 收口契约要求"跑过至少一次且绿"，此项目前**不成立** |
| `emotion-echo-web/app/pages/login/forget/username-only-copy.architecture.test.ts` | 7 | ✅ 2026-09-19 本轮实跑通过（`Tests 7 passed`），见 §七 #13 |

## 十、待决策 / 升级项

| # | 事项 | 处置 |
|---|------|------|
| 1 | 取证补拍：6 个 `BLOCKED` 点的执行证据 + 全部 `[V]` 截图 | 用户 2026-09-19 决议：**归入独立轮次**（轻量补账优先），登记为账本 E2E-F-90。**环境当前可用**（2026-09-19 实测 `docker ps` → 8 个 emotion-echo 容器 healthy），故本例属**主动推迟取证**，不是环境阻塞 |
| 2 | 回归钉 `password-recovery.spec.ts` 从未记录运行结果 | 同上，随取证轮次一并补跑（该 spec 是 13 点的主要自动化证据来源） |

## 十一、收口自检

- [x] report.md 存在且含 §10 模板必填章节（2026-09-19 补账后：测试点结果 / 收口自检）
- [x] 汇总行非占位符，且计数与测试点表行数一致（PASS 7 + BLOCKED 6 = 13 = 表行数）
- [x] 阶段状态三处一致：roadmap / plan / report 均为 `partial`（2026-09-19）
- [x] 账本对账：本阶段相关未解决条目 = E2E-F-01 / E2E-F-15 / E2E-F-90 ⇒ 阶段只能为 `partial`，不得为 `done`
- [ ] 截图归档 —— **未完成**：`screenshots/` 为 0 张，2 个含 `[V]` 的测试点无视觉证据
- [ ] 回归钉运行记录 —— **未完成**：`password-recovery.spec.ts` 无运行输出（见 §九）
- [ ] §2.5 收口自检三连 —— **未执行**（本阶段处于 partial，收口动作待取证轮次完成后统一执行）
