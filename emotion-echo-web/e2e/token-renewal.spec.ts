/**
 * 访问令牌滑动续期回归钉（E2E-F-207 follow-up；设计见 docs/plans/sliding-token-renewal.md）。
 *
 * 背景：前端原「401 + `code===10002` → 自动续期」是死代码（无后端下发 10002，且
 * APISIX 先拒过期令牌）⇒ 令牌一过期就登出。现改为**过期前主动续期**（寿命 75% 处）。
 *
 * 前置：dev 栈运行中，且 `:3000` 由 **dev 构建**（`nuxi dev`）提供 —— 验证句柄
 * `window.__tokenRenewal` 仅在 `import.meta.dev` 下暴露（生产构建不挂）。
 */
import { test, expect, type Page } from '@playwright/test'

async function loginQuick(page: Page) {
  await page.goto('/login')
  await page.waitForLoadState('networkidle')
  const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
  await expect(quickBtn).toBeEnabled({ timeout: 15_000 })
  await quickBtn.click()
  await page.waitForURL(/\/chat/, { timeout: 15_000 })
}

test('已登录 → 调度器已排程，且续期真的换到全新 TTL 令牌（滑动生效）', async ({ page }) => {
  const refreshCalls: string[] = []
  page.on('request', (r) => {
    if (r.url().includes('/auth/refresh')) refreshCalls.push(r.url())
  })

  await loginQuick(page)

  const hasHandle = await page.evaluate(
    () => typeof (window as any).__tokenRenewal !== 'undefined',
  )
  expect(hasHandle, 'dev 构建下应暴露 __tokenRenewal（生产构建不适用本钉）').toBe(true)

  expect(
    await page.evaluate(() => (window as any).__tokenRenewal.isScheduled()),
    '已登录应已排程续期',
  ).toBe(true)

  const payload = await page.evaluate(async () => {
    const t = await (window as any).__tokenRenewal.renewNow()
    if (!t) return null
    const b64 = t.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    return JSON.parse(atob(b64))
  })

  expect(refreshCalls.length, '续期应真的调用 /auth/refresh').toBeGreaterThan(0)
  expect(payload, '续期应返回新令牌').toBeTruthy()
  expect(payload.exp - payload.iat, '新令牌必须是全新 TTL（滑动续期核心）').toBe(86400)
  expect(
    await page.evaluate(() => (window as any).__tokenRenewal.isScheduled()),
    '续期后应重排下一次',
  ).toBe(true)
})

test('未登录 → 调度器不排程（不空转）', async ({ page }) => {
  await page.goto('/login')
  await page.waitForLoadState('networkidle')
  await page.context().clearCookies()
  await page.evaluate(() => {
    localStorage.clear()
    sessionStorage.clear()
  })
  await page.goto('/login')
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(1000)

  const st = await page.evaluate(() => ({
    hasHandle: typeof (window as any).__tokenRenewal !== 'undefined',
    scheduled: (window as any).__tokenRenewal
      ? (window as any).__tokenRenewal.isScheduled()
      : null,
  }))
  expect(st.hasHandle).toBe(true)
  expect(st.scheduled, '无令牌时不应排程').toBe(false)
})
