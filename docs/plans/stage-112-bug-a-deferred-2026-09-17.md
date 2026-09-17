---
status: deferred
priority: high
created: 2026-09-17
sprint: 112
owner: zcode-bot
---

# Stage 112 · Bug A 浏览器侧验收未通过 · 待后续 sprint 修复

## 现象（dev mode IAB 浏览器实测）
从已登录态 `/chat/conversation/new` 点 sidebar 的"日报/周报/月报/年报"——
**约 50% 概率被中间件弹回 `/login`**，必须重新登录才能进入 dashboard。

复现命令序列（IAB 浏览器侧）：
1. 打开 `http://localhost:3000/login`，点击"用演示账号快速体验"
2. 跳转到 `http://localhost:3000/chat/conversation/new`，sidebar 8 个入口可见
3. JS-click sidebar 的 `a[href="/chat/dashboard/dailyReport"]`
4. **500ms 后 URL 变成 `http://localhost:3000/login`**，pinia state 被 wipe（accessToken/userInfo 都变 NONE）

**已尝试修复（commit d53f1f7 + 未 commit 的 SSR 段补丁）但浏览器侧实测仍失败**。

## 已落地的修复（已 commit / 文件已改）

### commit d53f1f7（client-side fetchUserInfo 兜底）
**改动**：`emotion-echo-web/app/middleware/auth.global.ts`
```ts
// 之前：直接踢回 /login
if (!isInWhiteList && !isAuthenticated) {
  return navigateTo("/login", { replace: true })
}

// 之后：client 段先尝试恢复
if (!isInWhiteList && !isAuthenticated) {
  if (import.meta.client && userStore.accessToken) {
    try { await userStore.fetchUserInfo() } catch {}
    if (userStore.isAuthenticated) return // 恢复成功放行
  }
  return navigateTo("/login", { replace: true })
}
```

### SSR 段补丁（文件已改，未 commit）
在上述 if 块内额外加：
```ts
if (import.meta.server) {
  const tokenCookie = useCookie("access_token")
  if (tokenCookie.value) {
    userStore.accessToken = tokenCookie.value
    try { await userStore.fetchUserInfo() } catch {}
    if (userStore.isAuthenticated) return
  }
}
```

**测试 `auth-restore-on-mount.architecture.test.ts` 已加 3 条断言**（client 段 + SSR 段均覆盖）。

## 浏览器实测仍然失败的根因（猜测）

排查过程留下的观察：
- Nuxt dev mode SSR 对 `/chat/dashboard/dailyReport` 不做服务端 redirect（curl `200 OK` 返回）
- `<NuxtLink to="/chat/dashboard/dailyReport">` 在 dev mode 下疑似触发 **SSR 重 fetch**（不是纯 SPA client-side router.push）
- SPA 跳 dashboard 后 pinia state 在 500ms 内被 wipe（accessToken/userInfo 都变 NONE）
- `document.cookie` 在登录后一直是空字符串，说明 `useCookie` 在 CSR 写入没进 `document.cookie`（SSR 端有独立 cookie jar）
- 中间件 SSR 段读 cookie + 注入 store + await fetchUserInfo 都做了，但仍被踢回

最可能的根因（待验证）：
1. **Nuxt dev mode hydration race**：SPA 跳 dashboard 触发 Nuxt dev server 重新 fetch + SSR → SSR 上 userStore 没拿到 cookie（dev cookie jar 跨请求不同步）→ 服务端中间件 `navigateTo('/login')` → 但 SSR 端**不会**发 302，而是输出 `/login` 的 HTML → 客户端 hydrate 后把 `/login` 内容覆盖到 dashboard 上 → 但 client store 被 wipe → isAuthenticated=false → client middleware 又 `navigateTo('/login')` 一次
2. **`useCookie` 写入策略不一致**：客户端 `useCookie(...) = token` 写入路径与 SSR 读取路径用的不是同一个 cookie jar

## 建议的根治方向（不在 Sprint 112 落地）

| 方向 | 工作量 | 风险 |
|---|---|---|
| **A. BFF 真实 Set-Cookie（HttpOnly）**：让 `/api/v1/auth/login` 通过 `Set-Cookie: access_token=...; HttpOnly; SameSite=Lax; Path=/` 头写入，浏览器 cookie store 真实持有 → SSR 一定能读 | 中：改 BFF handler + 测试 | 低：cookie 永远在浏览器 + SSR 同步 |
| **B. dashboard 路由改成 `<KeepAlive>` + 全客户端渲染**：让 dashboard 不再触发 SSR 重渲染，纯 SPA 跳转 | 小：加 `definePageMeta({ ssr: false })` | 中：dashboard 首屏 SSR 失效，SEO 与首屏性能受影响（产品不需要 SEO） |
| **C. 接受 dev-mode limitation**：prod build 不复现，仅 dev mode 偶发；写明"刷新页面/重登"是 dev mode 已知问题 | 极小：仅文档 | 无技术风险但用户体验问题 |
| **D. 跳过中间件对 client-side 二次 SPA 跳转的检查**：用 `to.path` 和 `from.path` 都属 `/chat/*` 的条件绕过 | 中 | 高：安全洞（用户主动 logout 后仍可访问） |

**推荐方向 A**：BFF Set-Cookie 是项目里多次 ADR 讨论过的"该做但没做"的事，本次正好可以合并到 Sprint 113 集中处理。

## 验收未达成的清单（Sprint 112 收口）

- [ ] Bug A 浏览器实测验收（5/5 sidebar dashboard 链接都不踢回 login）
- [x] Bug B 已修复 + 测试 PASS
- [x] Bug C 已修复 + 测试 PASS
- [x] 忘记密码 UI 文案对齐 + 测试 PASS
- [x] 所有静态源测试 PASS（除待调整的 SSR 测试 regex）

## 参考资料
- 截图：`gui-test-screenshots/stage-112-section-scan-2026-09-17/03-bug-dashboard-redirect-login.png`
- 已 commit 修复：`d53f1f7 fix(B-A): auth.global 主动 await fetchUserInfo ...`
- 未 commit 中间件：`emotion-echo-web/app/middleware/auth.global.ts`（working copy 有 SSR 段补丁）
- 待调整测试：`emotion-echo-web/app/middleware/auth-restore-on-mount.architecture.test.ts`（SSR 段 regex 调整后未重新跑）