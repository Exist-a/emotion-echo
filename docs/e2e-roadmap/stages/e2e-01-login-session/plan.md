---
stage: e2e-01
title: 登录会话持久化
type: verification
status: done
created: 2026-09-17
depends-on: []
blocks: [e2e-07]
gate: []
related-findings: []
---

# E2E-01 登录会话持久化

## 1. 阶段目标

验证「登录 → 会话保持 → 刷新恢复 → 登出清除 → 过期重定向」整条会话生命周期在真实浏览器里正确，而不仅是单测绿。近期多轮 bug（`d53f1f7` / `0d6d1cc` / `d3ee175` / `15bf087` 一串 B-A 系列）都出在"Sprint 说修好了、浏览器里仍被踢回登录页"，本阶段要一次性钉死。

## 2. 范围与边界

### 做

- 登录成功后 **HttpOnly cookie** 的写入与属性校验（HttpOnly / Path / SameSite / 过期时间）
- 刷新页面后的会话恢复链路
- 登出后的 cookie 清除与跳转
- remember-me 行为（token **不得**落 localStorage，P0-R2-1 约定）
- 未登录访问受保护页 → 跳 `/login`；已登录访问 `/login` → 跳回
- JWT 过期后的重定向行为
- SSR 模式下的会话恢复时序（项目 `ssr: true`，`auth.global.ts` 在服务端和客户端都执行）

涉及文件：

| 层 | 文件 |
|----|------|
| 前端中间件 | `emotion-echo-web/app/middleware/auth.global.ts` |
| 前端状态 | `emotion-echo-web/app/stores/user.ts`、`app/lib/clientAccessToken.ts` |
| 前端 API | `emotion-echo-web/app/composables/useApi.ts` |
| BFF 鉴权 | `emotion-echo-web-bff/internal/handler/auth_handler.go`、`internal/auth/jwt.go` |
| 网关 | `deploy/apisix/seed.sh`（jwt-auth consumer 与白名单路由） |

### 不做（边界）

- 注册流程（E2E-09）
- 找回密码流程（E2E-07）
- 密保问题（D-01，归 E2E-06/07）
- JWT 密钥轮换（E2E-29）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| dev 模式容器全 healthy | ✅ 已启动（2026-09-17：6 服务 + 前端） |
| 演示账号可用 | `echo` / `echo123`（走标准 `POST /api/v1/auth/login`） |
| IAB 内置浏览器可用 | 待启用 |

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定（详见 [RUNBOOK.md](../../RUNBOOK.md) §4）。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 登录成功响应设置 cookie | [A] | Playwright `context.cookies()` 断言 token cookie 存在 | 断言输出 | ⬜ |
| 2 | cookie 是 HttpOnly | [A] | 断言 `httpOnly === true`；`document.cookie` 读不到该 token | 断言输出（双断言） | ⬜ |
| 3 | 刷新页面会话保持 | [A]+[V] | `page.reload()` 后断言未跳 `/login`；截图确认用户信息仍显示 | 断言 + 截图 | ⬜ |
| 4 | 直接访问受保护页（冷启动） | [A] | 新 context 登录 → 直接导航 `/chat/conversation/new`，断言 URL 不变 | 断言 URL | ⬜ |
| 5 | 未登录访问受保护页 | [A] | 无 cookie 上下文导航 `/chat/user`，断言重定向 `/login` | 断言 URL | ⬜ |
| 6 | 已登录访问 `/login` | [A] | 断言被重定向回应用内 | 断言 URL | ⬜ |
| 7 | 登出清除会话 | [A]+[V] | 断言 cookie 被清除 + 跳 `/login`；返回键访问受保护页仍被拒 | 断言 + 截图 | ⬜ |
| 8 | remember-me 不落 localStorage | [A] | `page.evaluate` 读 localStorage，断言**无 token 类键**（允许用户名） | `evaluate` 输出 | ⬜ |
| 9 | JWT 过期行为 | [A] | 构造过期 token（用 `BFF_JWT_SECRET` 签一个 `exp` 已过的 JWT，或篡改签名）注入 cookie，断言判未登录并跳 `/login` | 断言 + 截图 | ⬜ |
| 10 | 会话恢复不发散（SPA 时序） | [A]+[V] | 刷新后录制 2s 内 URL 序列，断言不出现"先跳登录页又弹回"的闪烁 | URL 序列 + 截图 | ⬜ |
| 11 | dev 覆盖项声明 | [M] | 明确记录本阶段验证的是 dev 配置（`BFF_DEV_RETURN_CODE=1` 等），prod 差异归 E2E-25/29 | report.md 声明 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 11 个测试点有结果（PASS/FAIL/BLOCKED/N/A，均为合法取值且 BLOCKED 不超过 1/3）
- [ ] 范围内 bug 按 TDD 修复
- [ ] 回归钉写入 `emotion-echo-web/e2e/login-flow.spec.ts`（扩展现有 2 条用例）并跑过且绿
- [ ] 范围外发现只记入 `discovered-unresolved.md`，不修
- [ ] 按 [RUNBOOK.md](../../RUNBOOK.md) §7 收口契约完成 8 项 + 自检三连

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| `BFF_DEV_RETURN_CODE=1` 等 dev 覆盖项让行为与 prod 不同 | 明确记录本阶段验证的是 dev 配置；prod 差异归 E2E-25/29 |
| cookie 域/端口问题（历史 Chrome 拒 localhost，Stage 105 修过 CORS 双 host） | 同时用 `127.0.0.1:3000` 与 `localhost:3000` 各跑 1 遍 |
| 现有 `login-flow.spec.ts` 用 API 直登设置 cookie，绕过真实 UI | 保留该用法做锚点，另加真实 UI 登录用例 |

## 7. 产出物

- Playwright spec：`emotion-echo-web/e2e/login-flow.spec.ts`（扩展）
- 执行记录：`stages/e2e-01-login-session/report.md`
- 截图：`stages/e2e-01-login-session/screenshots/`
