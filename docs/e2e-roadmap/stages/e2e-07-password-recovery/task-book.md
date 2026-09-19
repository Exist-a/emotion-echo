---
stage: e2e-07
title: E2E-07 找回密码任务书
type: task-book
status: done
created: 2026-09-19
author: ZCode
---

# E2E-07 找回密码 — 任务书

## 0. 调研依据

**已读代码文件**（AGENTS.md 文档撰写规则 §①）：

| 文件 | 关键发现 |
|------|---------|
| `emotion-echo-web/app/pages/login/forget/verify.vue` | 仍是"输入验证码"页面，含 `formInfo.verificationCode`、`getVerificationCode` 按钮、6 位数字校验 |
| `emotion-echo-web/app/pages/login/forget/modify.vue` | 发送 `{username, verificationCode, newPassword}` 到 `/auth/reset-password` |
| `emotion-echo-web/app/pages/login/forget/success.vue` | 成功页，调 `resetState()` 清 localStorage |
| `emotion-echo-web/app/composables/forgetPwdState.ts` | localStorage 键：`forgetPwdStep`/`forgetPwdAccount`/`forgetPwdCode`，含 `verificationCode` ref |
| `emotion-echo-web/app/middleware/forgetPwd.ts` | 步骤守卫：modify 需 step≥1，success 需 step≥2 |
| `emotion-echo-web/app/lib/apiRoutes.ts` | **缺少** `authVerifySecurityAnswer` 路由定义 |
| `emotion-echo-web/app/stores/user.ts` | **缺少** `verifySecurityAnswer` 方法 |
| `emotion-echo-web-bff/internal/handler/auth_handler.go:110-111,458-488` | BFF 已实现 `verifySecurityAnswer`，调 `VerifySecurityAnswerByUsername`，防枚举统一 401 |
| `emotion-echo-web-bff/internal/downstream/user.go:54-83,233-253` | `UserClient` 接口已定义 `VerifySecurityAnswerByUsername`，HTTP/gRPC 双实现 |
| `emotion-echo-user-svc/main.go:158` | 路由已注册：`noAuth.POST("/verify-security-answer", ...)` |
| `emotion-echo-user-svc/internal/logic/authlogic.go:155-211` | `VerifySecurityAnswer` + `VerifySecurityAnswerByUsername` 已实现，bcrypt 校验 |
| `emotion-echo-user-svc/internal/model/security_answer.go` | `SecurityAnswer` 模型：`user_id` + `question_order` 联合主键，`answer_hash` bcrypt |
| `deploy/apisix/seed.sh:557` | route 117 白名单已配置 `/api/v1/auth/verify-security-answer` |

**已读 ADR / 决策**（AGENTS.md §②）：

| 文档 | 相关决策 |
|------|---------|
| `docs/e2e-roadmap/decisions.md` D-01 | 选 C 密保问题；密保不可跳过、注册必设、弹框 UI、删除验证码步骤 |
| `docs/e2e-roadmap/decisions.md` D-05 | 注册密保改为可选（后端回退），E2E-09 再做完整 |
| `docs/e2e-roadmap/roadmap.md` | R-01 已完成：三层根因全修（APISIX + BFF gRPC + user-svc 拦截器） |

**已读测试文件**（AGENTS.md §①）：

| 文件 | 覆盖情况 |
|------|---------|
| `user-svc/internal/logic/authlogic_test.go:315-393` | 5 个负向测试（错误答案/用户不存在/无效 order/空用户名） |
| `bff/internal/handler/security_answer_handler_test.go` | 5 个 handler 测试（错答案 401/未知用户 401/正确 200/缺答案 400/order 越界 400） |
| `emotion-echo-web/e2e/` | **无** password-recovery spec |

---

## 1. 现状与目标差距分析

### 1.1 后端链路（✅ 已就绪）

```
前端 → BFF POST /auth/verify-security-answer → downstream.VerifySecurityAnswerByUsername
     → user-svc POST /verify-security-answer → AuthLogic.VerifySecurityAnswer
     → SecurityAnswerRepo.GetByUserID → bcrypt 校验
```

**状态**：全链路已通，有 10 个单元测试覆盖。APISIX route 117 白名单已配。

### 1.2 前端链路（❌ 需改造）

| 组件 | 现状 | 目标 | 差距 |
|------|------|------|------|
| `apiRoutes.ts` | 无 `authVerifySecurityAnswer` | 新增路由定义 | **缺失** |
| `stores/user.ts` | 无 `verifySecurityAnswer` 方法 | 新增方法调用 BFF | **缺失** |
| `verify.vue` | 输入验证码（6 位数字） | 输入密保答案（1~2 题） | **核心改造** |
| `forgetPwdState.ts` | `verificationCode` ref + localStorage `forgetPwdCode` | 改为 `securityAnswer` / 答案相关 | **需改造** |
| `modify.vue` | 发送 `verificationCode` 到 reset-password | 发送密保验证凭证 | **需改造** |
| `middleware/forgetPwd.ts` | step 1=确认账号，step 2=验证码验证 | step 1=确认账号+密保验证，step 2=改密 | **语义调整** |
| 架构测试 | 只锁"无手机号/邮箱" | 新增密保文案断言 | **缺失** |
| E2E spec | 不存在 | `password-recovery.spec.ts` | **缺失** |

### 1.3 关键决策点（待确认）

| 决策点 | 建议 | 理由 |
|--------|------|------|
| 密保验证结果如何传递给 modify.vue | localStorage 存 `securityVerified: true` + `verifiedUsername` | 与现有 `forgetPwdStep` 模式一致，不引入新依赖 |
| 答案归一化规则 | `trim` + 忽略大小写 | 业界常见做法，降低用户输入摩擦 |
| 1~2 个问题的 UI 形态 | 先展示问题列表，每题一个输入框，全部填写后提交 | 简单直观，避免多步骤复杂性 |
| 不存在用户的响应 | 统一提示"用户名或密保答案错误"（防枚举） | 与 BFF 策略一致 |

---

## 2. 改造清单（TDD 红→绿→重构）

### Phase 1：基础设施（RED → GREEN）

#### Task 1.1：新增前端 API 路由

**文件**：`emotion-echo-web/app/lib/apiRoutes.ts`

```typescript
// 新增
authVerifySecurityAnswer: { method: 'POST', path: '/auth/verify-security-answer' } as ApiRoute,
```

**TDD**：
- 🔴 RED：写测试断言 `API_ROUTES.authVerifySecurityAnswer` 存在且 path 正确
- 🟢 GREEN：添加路由定义

#### Task 1.2：新增 user store 方法

**文件**：`emotion-echo-web/app/stores/user.ts`

新增 `verifySecurityAnswer(params)` 方法，调用 `post(API_ROUTES.authVerifySecurityAnswer.path, params)`。

**TDD**：
- 🔴 RED：写测试断言 store 有 `verifySecurityAnswer` 方法，且调用正确 path
- 🟢 GREEN：实现方法

#### Task 1.3：更新 forgetPwdState.ts

**文件**：`emotion-echo-web/app/composables/forgetPwdState.ts`

改动：
- `verificationCode` ref → `securityVerified` ref (boolean)
- localStorage 键：`forgetPwdCode` → `forgetPwdSecurityVerified`
- 新增 `verifiedUsername` ref（记录已验证的用户名，防止篡改）
- `resetState()` 清理新键

**TDD**：
- 🔴 RED：写测试断言新 ref 存在、localStorage 键正确、resetState 清理
- 🟢 GREEN：重构 composable

---

### Phase 2：核心页面改造（RED → GREEN）

#### Task 2.1：改造 verify.vue（核心）

**文件**：`emotion-echo-web/app/pages/login/forget/verify.vue`

**改动**：
1. 删除验证码相关 UI（`verificationCode` 输入框、`getVerificationCode` 按钮、倒计时）
2. 新增密保问题区域：
   - 输入用户名后，调 BFF 获取该用户的密保问题（**需新增 BFF 端点**，见 Task 2.2）
   - 展示 1~2 个问题，每题一个输入框
   - "继续"按钮：调 `verifySecurityAnswer` 校验全部答案
3. 更新表单校验规则（移除验证码规则，新增答案非空规则）
4. 成功后设置 `securityVerified = true` + `verifiedUsername = username` + `updateStep(1)`

**UI 文案**：
- 标题：`先确认一下这是你的账户`
- 副标题：`回答你注册时设置的密保问题`
- 按钮：`验证`

**TDD**：
- 🔴 RED：
  - 架构测试：断言页面包含"密保问题"文案
  - 组件测试：模拟获取问题 → 断言问题渲染 → 模拟输入答案 → 断言调用 verifySecurityAnswer
- 🟢 GREEN：实现页面改造

#### Task 2.2：新增 BFF 获取密保问题端点

**已确认**（2026-09-19 调研）：BFF、user-svc、APISIX **均不存在**获取密保问题的端点。当前代码库仅支持"验证密保答案"（`verify-security-answer`），不支持"获取密保问题"。

**需全链路新增**：

| 层 | 改动 |
|----|------|
| user-svc model | `SecurityAnswerRepo` 新增 `GetQuestionsByUserID(ctx, userID)` 方法 |
| user-svc logic | `AuthLogic.GetSecurityQuestionsByUsername(username)` → 返回问题列表（不含 answer_hash） |
| user-svc handler | `GET /security-questions?username=xxx` → 返回 `{questions: [{question, order}]}` |
| user-svc main.go | 注册路由：`noAuth.GET("/security-questions", ...)` |
| BFF downstream | `UserClient` 接口新增 `GetSecurityQuestionsByUsername(ctx, username)` |
| BFF downstream 实现 | HTTP: `GET /api/v1/users/security-questions?username=xxx`；gRPC: 新增 RPC |
| BFF handler | switch 新增 `case "security-questions"` → `getSecurityQuestions(c)` |
| BFF handler 实现 | 调 downstream，返回问题列表 |
| APISIX | route 118 白名单：`put_auth_route 118 "/api/v1/auth/security-questions"` |
| proto（如走 gRPC） | 新增 `GetSecurityQuestionsByUsername` RPC 定义 |

**TDD**：
- 🔴 RED：
  - user-svc test：断言返回问题列表、不含 answer_hash、用户不存在返回空列表
  - BFF test：断言 200 + 问题列表、未知用户 200 + 空列表（防枚举）
- 🟢 GREEN：实现全链路

**安全注意**：获取密保问题端点必须**防枚举**——用户不存在时返回空列表（而非 404），与 verify-security-answer 策略一致。

#### Task 2.3：改造 modify.vue

**文件**：`emotion-echo-web/app/pages/login/forget/modify.vue`

**改动**：
- 移除 `verificationCode` 读取
- 改用 `securityVerified` + `verifiedUsername`
- 提交 reset-password 时，传递 `securityVerified: true`（或后端改为验证 token）
- 校验 `securityVerified === true` 才允许提交

**关键决策**：reset-password 端点如何确认密保已验证？
- **方案 A**（简单）：前端传 `securityVerified: true`，后端信任（不安全）
- **方案 B**（推荐）：verify-security-answer 成功后返回一个短期 token，modify 传 token 给 reset-password，后端校验 token

**建议采用方案 B**，与现有 `verificationCode` 的语义对齐（验证码本身就是一种 token）。

**TDD**：
- 🔴 RED：测试断言未验证时提交被拒、验证后可提交
- 🟢 GREEN：实现改造

#### Task 2.4：更新 middleware/forgetPwd.ts

**文件**：`emotion-echo-web/app/middleware/forgetPwd.ts`

**改动**：
- step 语义调整：step 1 = 密保验证完成（原"确认账号"）
- 校验 `securityVerified` 而非仅 `currentStep`

**TDD**：
- 🔴 RED：测试断言未验证时访问 modify 被重定向
- 🟢 GREEN：更新守卫逻辑

---

### Phase 3：测试与收口（REFACTOR）

#### Task 3.1：架构测试更新

**文件**：`emotion-echo-web/app/pages/login/forget/username-only-copy.architecture.test.ts`（或新建）

新增断言：
- ✅ 包含"密保问题"相关文案
- ❌ 不包含"验证码"、"手机号"、"邮箱"字样

#### Task 3.2：E2E Playwright spec

**文件**：`emotion-echo-web/e2e/password-recovery.spec.ts`（新建）

覆盖 plan.md 的 13 个测试点：

| # | 测试点 | 实现方式 |
|---|--------|---------|
| 1 | 登录页"忘记密码"入口可达 | 点击 → 断言 URL = `/login/forget` |
| 2 | 步骤 1：输入用户名 → 展示密保问题 | 填用户名 → 点获取 → 断言问题文案出现 |
| 3 | 不存在用户名 → 防枚举 | 填不存在用户名 → 断言错误文案不泄露用户存在性 |
| 4 | 答案正确 → 进入改密页 | 填正确答案 → 断言跳转 `/login/forget/modify` |
| 5 | 答案错误 → 拒绝 | 填错误答案 → 断言停留 + 错误提示 |
| 6 | 错误次数限制 | 连续错误 N 次 → 断言被限流 |
| 7 | 改密成功 | 填新密码 → 断言进入 success 页 |
| 8 | 新密码可登录 | 登出 → 用新密码登录 → 成功 |
| 9 | 旧密码失效 | 用旧密码登录 → 断言失败 |
| 10 | 路由守卫跳过步骤 | 直接访问 modify → 断言被拦回 |
| 11 | localStorage 持久化 | 刷新 → 断言停留当前步骤 |
| 12 | 密码强度校验 | 提交弱密码 → 断言拒绝 |
| 13 | 文案无手机号/邮箱 | 架构测试绿 |

#### Task 3.3：测试夹具准备

**问题**：注册页尚未收集密保（E2E-09 范围），如何创建"已设密保"的测试账号？

**方案**：写一个脚本 `scripts/seed_security_question.sh`，通过 user-svc API 直接插入密保数据：
```bash
# 调 user-svc internal API 或直接 psql 插入
psql -c "INSERT INTO emotion_echo_user.user_security_answers 
         (user_id, question_order, question, answer_hash) 
         VALUES ($USER_ID, 1, '你的第一只宠物叫什么？', '$BCRYPT_HASH')"
```

**TDD**：
- 🔴 RED：测试断言脚本可执行、插入后可验证
- 🟢 GREEN：实现脚本

#### Task 3.4：清理遗留

- 评估 `verificationCodeCountDown` composable 是否还有其他使用方，若无则标记废弃
- 清理 `BFF_DEV_RETURN_CODE` 相关逻辑（确认无其他流程使用后）

---

## 3. 依赖与前置

| 依赖 | 状态 | 说明 |
|------|------|------|
| 后端 verify-security-answer 全链路 | ✅ 就绪 | R-01 已修复三层根因 |
| user_security_answers 表 | ✅ 就绪 | E2E-06 已创建 |
| APISIX route 117 白名单 | ✅ 就绪 | seed.sh 已配置 |
| 测试账号密保数据 | ❌ 待创建 | 需写 seed 脚本 |
| BFF 获取密保问题端点 | ❌ 待实现 | 全链路需新增（user-svc + BFF + APISIX route 118） |

---

## 4. 风险与缓解

| 风险 | 影响 | 缓解 |
|------|------|------|
| BFF 无获取密保问题端点（已确认） | verify.vue 无法展示问题 | Task 2.2 需全链路新增（user-svc + BFF + APISIX），增加后端工作量 |
| reset-password 端点不支持 token 验证 | modify.vue 安全性弱 | 采用方案 B，verify 返回 token |
| 架构测试拦截含"邮箱"文案 | 新文案需避开 | 已确认密保文案不含"邮箱" |
| 未设密保用户无法找回 | 预期行为 | D-01 决议：不处理存量用户 |

---

## 5. 执行顺序（建议）

```
Phase 0 (后端新增)     → 最先做，因为 verify.vue 依赖获取密保问题端点
  Task 2.2 获取密保问题端点（全链路：user-svc → BFF → APISIX route 118）

Phase 1 (前端基础设施) → 可并行，依赖 Phase 0 的 API 可用
  Task 1.1 apiRoutes
  Task 1.2 store method
  Task 1.3 forgetPwdState

Phase 2 (核心页面改造) → 串行依赖 Phase 1
  Task 2.1 verify.vue（核心改造）
  Task 2.3 modify.vue
  Task 2.4 middleware

Phase 3 (测试收口)     → 依赖 Phase 2
  Task 3.1 架构测试
  Task 3.2 E2E spec
  Task 3.3 测试夹具
  Task 3.4 清理遗留
```

**关键路径**：Phase 0 → Task 2.1 → Task 2.3 → Task 3.2 → Task 3.3

---

## 6. 验收标准（DoD）

- [ ] 13 个测试点全部通过（E2E spec）
- [ ] 改造按 TDD 走：每个 Task 有 RED → GREEN 记录
- [ ] 架构测试绿：无"手机号/邮箱/验证码"字样，有"密保问题"文案
- [ ] 防枚举行为有断言（不存在用户 vs 错误答案不可区分）
- [ ] `forgetPwdState.ts` 键名已更新，localStorage 清理正确
- [ ] `password-recovery.spec.ts` 写入 `emotion-echo-web/e2e/`
- [ ] 测试夹具脚本可复现（`scripts/seed_security_question.sh`）
- [ ] 端到端实测：错答案 401 / 正确答案 200 / 改密成功 / 新密码可登录

---

## 7. 附录：关键代码位置速查

| 组件 | 文件 | 行号 |
|------|------|------|
| BFF 路由分发 | `bff/internal/handler/auth_handler.go` | 110-111 |
| BFF handler 实现 | `bff/internal/handler/auth_handler.go` | 458-488 |
| BFF downstream 接口 | `bff/internal/downstream/user.go` | 54-83 |
| BFF downstream 实现 | `bff/internal/downstream/user.go` | 233-253 |
| user-svc 路由 | `user-svc/main.go` | 158 |
| user-svc logic | `user-svc/internal/logic/authlogic.go` | 155-211 |
| user-svc model | `user-svc/internal/model/security_answer.go` | 全文 |
| APISIX route 117 | `deploy/apisix/seed.sh` | 557 |
| 前端 verify.vue | `emotion-echo-web/app/pages/login/forget/verify.vue` | 全文 |
| 前端 modify.vue | `emotion-echo-web/app/pages/login/forget/modify.vue` | 全文 |
| 前端 forgetPwdState | `emotion-echo-web/app/composables/forgetPwdState.ts` | 全文 |
| 前端 middleware | `emotion-echo-web/app/middleware/forgetPwd.ts` | 全文 |
| 前端 apiRoutes | `emotion-echo-web/app/lib/apiRoutes.ts` | 25-32 |
| 前端 user store | `emotion-echo-web/app/stores/user.ts` | 170-179 |