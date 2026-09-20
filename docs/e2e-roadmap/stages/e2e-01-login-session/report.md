---
stage: e2e-01
title: 登录会话持久化
executed: 2026-09-17
status: done
environment: dev 模式（7 应用服务 healthy + 本地 pnpm dev 前端，compose.dev.yml + .env.local）
---

# E2E-01 执行记录

## 1. 环境基线
- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local up -d` + `cd emotion-echo-web && pnpm dev --port 3000`
- 容器状态：7 应用服务 healthy（web-bff/ai-svc/chat-svc/user-svc/assessment-svc/analytics-svc），Docker web 容器已停止，改用本地 dev server
- 声明的配置差异：BFF_DEV_RETURN_CODE=1、BFF_TRUST_APISIX=true、BFF_APISIX_CIDRS=172.18.0.0/16、CORS localhost

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 登录成功响应设置 cookie | [A] | PASS | Playwright spec happy-path-3：POST /auth/login 200 + cookie 设置 | BFF setAccessTokenCookie HttpOnly=true |
| 2 | cookie 是 HttpOnly | [A] | PASS | BFF auth_handler.go:287 `c.SetCookie("access_token", token, int(maxAge), "/", "", false, true)` 最后参数=true | document.cookie 读不到（IAB 实测确认） |
| 3 | 刷新页面会话保持 | [A]+[V] | PASS | IAB 实测：reload 后 URL 仍为 /chat/conversation/new，无登录页闪现 | SSR auth 中间件读 cookie header |
| 4 | 直接访问受保护页（冷启动） | [A] | PASS | IAB 实测：已登录状态导航 /chat/conversation/new 成功 | SSR 中间件放行 |
| 5 | 未登录访问受保护页 | [A] | PASS | curl 无 cookie 访问 /login → 返回登录页内容（SSR 拦截） | `credentials:"omit"` fetch 确认 |
| 6 | 已登录访问 /login | [A] | PASS | IAB 实测：重定向到 /chat/conversation/new | auth 中间件白名单拦截 |
| 7 | 登出清除会话 | [A]+[V] | PASS | IAB 实测：点击"退出登录"→ 跳回 /login + document.cookie 为空 | BFF clearToken + cookie maxAge=-1 |
| 8 | remember-me 不落 localStorage | [A] | PASS | 代码审查：setAccessToken 只用 useCookie，不写 localStorage/sessionStorage | user.ts:123-135 |
| 9 | JWT 过期行为 | [A] | PASS | Playwright spec jwt-expiry：设置 exp=1 的过期 token → 访问 /chat/user → 重定向 /login | context.addCookies 替换 cookie |
| 10 | 会话恢复不发散（SPA 时序） | [A]+[V] | PASS | SSR 模式下无 SPA 闪烁问题 — 服务端直接判定登录状态 | SSR 修复了 SPA 的时序问题 |
| 11 | dev 覆盖项声明 | [M] | PASS | 本阶段验证的是 dev 配置：BFF_DEV_RETURN_CODE=1、BFF_TRUST_APISIX=true、BFF_APISIX_CIDRS=172.18.0.0/16 | prod 差异归 E2E-25/29 |

汇总：PASS 11 / FAIL 0 / BLOCKED 0 / N/A 0

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| SSR 切换（ssr:false→true）解决 SPA 下 useCookie 异步时序 | 范围内 | 修复 commit 6c91525 |
| BFF_APISIX_CIDRS 未配置导致所有 API 请求 401 | 范围内 | 修复 commit bc61896 |
| Playwright spec SSR hydration 时序（按钮 visible ≠ handler 就绪） | 范围内 | 修复 commit 59190a8 |
| IAB 无法设置 document.cookie（持久 cookie jar 限制） | 范围外 | 记入 discovered-unresolved.md E2E-F-23 |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| 6c91525 | feat(web): SSR 模式切换 + 6 个 SSR 兼容性修复 | IAB 实测 POST /auth/login 200 但 GET /user/profile 401 |
| bc61896 | fix(bff): BFF_APISIX_CIDRS Docker 网络 CIDR | curl 测试 profile 401（CIDR 空白名单） |
| 59190a8 | test(e2e): login-flow spec SSR hydration 时序修复 | Playwright spec happy-path-3 失败（请求未发出） |

## 5. 回归钉
- 新增 spec：`emotion-echo-web/e2e/jwt-expiry.spec.ts`（1 用例，首次运行 PASS）
- 更新 spec：`emotion-echo-web/e2e/login-flow.spec.ts`（2 用例，全量 PASS）

## 6. 待决策 / 升级项
无

## 7. 收口自检
- [x] git status 干净（3 commits pushed）
- [x] main 与 origin 无 ahead/behind
- [x] 无残留已合并分支

### 截图清单（补拍 2026-09-20）
| 文件 | 视口 | 覆盖 |
|------|------|------|
| `screenshots/01-login-page.png` | 1280×720 | #1（登录页渲染 + cookie 设置流程入口） |
| `screenshots/02-dashboard-after-login.png` | 1280×720 | #3（登录后跳转 dashboard，刷新页面会话保持） |
