import { test, expect } from '@playwright/test'

test('expired JWT redirects to login', async ({ page, context }) => {
  // 先登录获取 cookie
  await page.goto('/login')
  await page.waitForLoadState('networkidle')
  const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
  await expect(quickBtn).toBeEnabled({ timeout: 15_000 })
  await quickBtn.click()
  await page.waitForURL(/\/chat/, { timeout: 15_000 })

  // 用过期 token 替换 cookie
  const cookies = await context.cookies()
  const tokenCookie = cookies.find(c => c.name === 'access_token')
  expect(tokenCookie, '应有 access_token cookie').toBeTruthy()

  // 设置过期 token（exp=1 = 1970年）
  const expiredToken = 'eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJrZXkiOiJ1c2VyIiwiZXhwIjoxfQ.invalid'
  await context.addCookies([{
    name: 'access_token',
    value: expiredToken,
    domain: tokenCookie!.domain,
    path: '/',
    httpOnly: tokenCookie!.httpOnly,
    secure: tokenCookie!.secure,
    sameSite: tokenCookie!.sameSite as any,
  }])

  // 访问受保护页
  await page.goto('/chat/user')
  await page.waitForLoadState('networkidle')
  await page.waitForTimeout(3000)

  // 应被重定向到登录页
  expect(page.url()).toContain('/login')
})
