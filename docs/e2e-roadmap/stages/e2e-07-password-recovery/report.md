---
stage: e2e-07
title: E2E-07 找回/重置密码 执行记录
type: report
status: done
started: 2026-09-19
completed: 2026-09-19
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

**done** — 全部测试点通过，数据库落库验证，功能可用。
