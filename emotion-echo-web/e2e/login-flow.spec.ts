import { test, expect } from '@playwright/test'

test.describe('login flow', () => {
  test('happy-path-3: quick login triggers POST /auth/login and navigates', async ({ page }) => {
    // 收集所有请求
    const requests: { method: string; url: string; status?: number }[] = []
    page.on('request', (req) => {
      requests.push({ method: req.method(), url: req.url() })
    })
    page.on('response', (res) => {
      const entry = requests.find(r => r.url === res.url() && !r.status)
      if (entry) entry.status = res.status()
    })

    // 收集控制台日志
    const consoleLogs: string[] = []
    page.on('console', (msg) => {
      if (msg.type() === 'error' || msg.type() === 'warn') {
        consoleLogs.push(`[${msg.type()}] ${msg.text()}`)
      }
    })

    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    // 等按钮可用
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 20_000 })
    await expect(quickBtn).toBeEnabled({ timeout: 10_000 })
    await page.waitForTimeout(500)

    // 点击
    await quickBtn.click()

    // 等待足够时间
    await page.waitForTimeout(15_000)

    // 分析结果
    const loginReq = requests.find(r =>
      r.method === 'POST' && r.url.includes('/auth/login')
    )
    const profileReq = requests.find(r =>
      r.method === 'GET' && r.url.includes('/user/profile')
    )

    // 输出诊断信息
    const diagnostics = {
      loginReq: loginReq || null,
      profileReq: profileReq || null,
      currentUrl: page.url(),
      consoleLogs: consoleLogs.slice(-10),
      allApiRequests: requests.filter(r => r.url.includes('/api/v1'))
    }

    // 断言
    expect(loginReq, '应有 POST /auth/login 请求').toBeTruthy()
    expect(loginReq?.status, 'login 应返回200').toBe(200)
    expect(profileReq, '应有 GET /user/profile 请求').toBeTruthy()
    expect(page.url()).toMatch(/\/chat\/conversation/)
  })

  test('happy-path-2: 页面元素完整性', async ({ page }) => {
    await page.goto('/login')
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 20_000 })
    await expect(page.getByRole('tab', { name: '登录' })).toBeVisible()
    await expect(page.getByRole('tab', { name: '注册' })).toBeVisible()
    await expect(page.locator('input[type="password"]')).toBeVisible()
  })
})
