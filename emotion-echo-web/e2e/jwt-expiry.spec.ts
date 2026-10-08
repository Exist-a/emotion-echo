/**
 * 过期令牌行为回归钉（E2E-29 #5 加严，2026-10-08 补验轮）。
 *
 * 背景（本轮实测结论）：前端 `useApi.ts` 的"401 + `code===10002` → 自动 refresh → 重试"
 * 分支是**死代码** —— 全仓没有任何后端会下发 `code:10002`（BFF 一律 `code:1`；APISIX
 * 过期令牌返 `{"message":"failed to verify jwt"}` 无 `code` 字段；shared 中间件返
 * `{"error":"unauthorized"}`）。因此真实行为是：**令牌过期/无效 → 直接登出并跳 /login，
 * 绝不尝试 /auth/refresh**。本 spec 把该契约钉死（防止将来误以为存在"静默续期"）。
 *
 * 令牌：形如过期 JWT（`exp=1` = 1970）但签名无效 —— 网关按"验证失败"拒绝，与真实
 * 过期（签名有效、`exp` 已过）走同一条 401 路径（后者本轮已由 IAB 实测 + curl 单独验证）。
 * 不在 spec 里持有真实密钥（AGENTS §四红线）。
 */
import { test, expect } from '@playwright/test'

const EXPIRED_JWT =
  'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJrZXkiOiJ1c2VyIiwiZXhwIjoxfQ.invalid'

async function loginQuick(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.waitForLoadState('networkidle')
  const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
  await expect(quickBtn).toBeEnabled({ timeout: 15_000 })
  await quickBtn.click()
  await page.waitForURL(/\/chat/, { timeout: 15_000 })
}

test('过期令牌冷启动 → 跳 /login 且不尝试刷新', async ({ page, context }) => {
  const refreshCalls: string[] = []
  page.on('request', (r) => {
    if (r.url().includes('/auth/refresh')) refreshCalls.push(r.url())
  })

  await loginQuick(page)

  // 用过期令牌覆盖 HttpOnly cookie（Playwright 的 addCookies 可写 HttpOnly；
  // 页面内 JS 因 HttpOnly 无法覆盖 —— 本轮 IAB 实测已确认，属安全正效应）
  const cookies = await context.cookies()
  const tokenCookie = cookies.find((c) => c.name === 'access_token')
  expect(tokenCookie, '应有 access_token cookie').toBeTruthy()
  await context.addCookies([
    {
      name: 'access_token',
      value: EXPIRED_JWT,
      domain: tokenCookie!.domain,
      path: '/',
      httpOnly: tokenCookie!.httpOnly,
      secure: tokenCookie!.secure,
      sameSite: tokenCookie!.sameSite as any,
    },
  ])

  // 冷启动：整页加载 → store init() 读 cookie 发现 exp 已过 → clearToken → 中间件跳登录
  await page.goto('/chat/conversation')
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(2000)

  expect(page.url(), '过期令牌必须跳 /login').toContain('/login')
  await expect(page.locator('input[type="password"]'), '必须显示登录表单（真登出，非白屏）').toBeVisible()
  expect(refreshCalls, '过期不得尝试 /auth/refresh（10002 分支为死代码）').toHaveLength(0)

  // 应用侧登录态已清空（store + localStorage）。
  // 注：HttpOnly cookie 客户端删不掉（JS 写同名 HttpOnly cookie 被浏览器拒绝，本轮 IAB 实测确认）。
  // 属已知边界（report §6）：显式登出走 /auth/logout（服务端 Set-Cookie 清），401 触发的
  // clearAuth 场景令牌本已失效，故无安全影响；此处只断言"应用侧已登出"。
  const lsUser = await page.evaluate(() => localStorage.getItem('user_info'))
  expect(lsUser, '登出后不得残留 user_info（应用侧已清空）').toBeFalsy()
})

test('受保护接口返 401（网关真实体，无 code:10002）→ 登出跳登录，不尝试刷新', async ({ page }) => {
  const refreshCalls: string[] = []
  let intercepted = 0
  page.on('request', (r) => {
    if (r.url().includes('/auth/refresh')) refreshCalls.push(r.url())
  })

  await loginQuick(page)

  // 复刻网关对过期令牌的真实响应体（本轮 curl 实测：无 code 字段）——
  // 命中 useApi 的"其他 401"分支（clearAuth + 跳登录），而非 10002 续期分支
  await page.route('**/api/v1/**', (route) => {
    if (route.request().url().includes('/auth/refresh')) return route.continue()
    intercepted += 1
    return route.fulfill({
      status: 401,
      contentType: 'application/json',
      body: JSON.stringify({ message: 'failed to verify jwt' }),
    })
  })

  await page.goto('/chat/user') // 我的空间在挂载时发多个受保护请求
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(2500)

  expect(intercepted, '负向对照：本测试必须真的拦截到受保护请求（否则断言空转）').toBeGreaterThan(0)
  expect(page.url(), '401 后必须跳 /login').toContain('/login')
  expect(refreshCalls, '无 code:10002 时不得尝试刷新').toHaveLength(0)
})
