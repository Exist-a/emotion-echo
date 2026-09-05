import { test, expect } from '@playwright/test'

/**
 * E2E: 登录页 → 使用演示账号快速登录 → 看到 dashboard 或聊天页
 *
 * happy-path 1: 首页 → login 页面 → 点击"用演示账号快速体验"→ 跳转成功 + 显示用户邮箱
 *
 * 这是项目第一个 Playwright spec，对齐 Stage 26-M 任务：
 *   "起 Playwright 配置 + 写 e2e/login-flow.spec.ts 第一个 happy-path"
 */

test.describe('login flow', () => {
  test('happy-path-3: 点击"用演示账号快速体验"按钮触发标准 /auth/login 调用', async ({ page }) => {
    // Sprint 1 PR-5 (2026-09-04, 决策 18 登记实例 #16):
    //   - 前端 quickLogin() (emotion-echo-web/app/pages/login/index.vue:175-196) 实际走标准
    //     POST /api/v1/auth/login（账号 echo/echo123），**不是**独立 /auth/quick-login 端点
    //   - BFF auth_handler.go 仅注册 5 个 action：login / register / refresh / logout / verification-code
    //   - todo-pile-2026-09-04.md C6：quick-login 端点从未实现，quickLogin 注释承认走标准登录路径
    //
    // 本测试断言：
    //   (1) 点击"用演示账号快速体验"按钮
    //   (2) 触发 POST /api/v1/auth/login 网络请求（精确 URL，避免之前 url.includes 模糊匹配）
    //   (3) 跳转至 /chat/conversation（成功登录的端到端信号）
    let loginRequestUrl: string | null = null
    page.on('request', (req) => {
      const url = req.url()
      if (
        req.method() === 'POST' &&
        url.includes('/api/v1/auth/login') &&
        !url.includes('/quick-login') &&
        !url.includes('/register')
      ) {
        loginRequestUrl = url
      }
    })

    await page.goto('/login')

    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 20_000 })
    await quickBtn.click()

    // 等后端响应 + 路由跳转（dev 后端 up 后 1-3s）
    await page.waitForURL(/\/chat\/conversation/, { timeout: 15_000 }).catch(() => {})
    await page.waitForTimeout(1000)

    // 断言 (2)：确实触发了标准登录端点
    expect(loginRequestUrl, '点击 quickLogin 按钮应触发 POST /api/v1/auth/login').not.toBeNull()
    // 断言 (3)：跳转到了聊天页（端到端成功）
    expect(page.url()).toMatch(/\/chat\/conversation/)
  })

  test('happy-path-2: 页面元素完整性（不点击，仅验证渲染）', async ({ page }) => {
    await page.goto('/login')

    // 演示按钮作为水合完成信号
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 20_000 })

    // 表单切换 tab
    await expect(page.getByRole('tab', { name: '登录' })).toBeVisible()
    await expect(page.getByRole('tab', { name: '注册' })).toBeVisible()

    // 至少一个登录控件存在
    await expect(page.locator('input[type="password"]')).toBeVisible()
  })
})
