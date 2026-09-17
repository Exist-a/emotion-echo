import { test, expect } from '@playwright/test'

/**
 * E2E: dashboard flow · A3 + A4 端到端验证
 *
 * SSR 模式下需要等待 hydration 完成。
 */

test.describe('dashboard flow · A3 + A4 chartData 验证', () => {
  test('happy-path-1: quick-login → dailyReport 页面渲染成功', async ({ page }) => {
    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 20_000 })
    await expect(quickBtn).toBeEnabled({ timeout: 10_000 })
    await page.waitForTimeout(500)
    await quickBtn.click()

    await page.waitForURL(/\/chat\/conversation/, { timeout: 15_000 })

    await page.goto('/chat/dashboard/dailyReport')
    await page.waitForLoadState('domcontentloaded')

    const heading = page.getByRole('heading', { name: /日报|daily/i }).first()
    await expect(heading).toBeVisible({ timeout: 15_000 })
  })

  test('happy-path-2: 4 个 dashboard 页面均可访问 + 渲染至少 1 个 chart 容器', async ({ page }) => {
    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 20_000 })
    await expect(quickBtn).toBeEnabled({ timeout: 10_000 })
    await page.waitForTimeout(500)
    await quickBtn.click()

    await page.waitForURL(/\/chat\/conversation/, { timeout: 15_000 })

    const pages = [
      { path: '/chat/dashboard/dailyReport', label: '日报' },
      { path: '/chat/dashboard/weeklyReport', label: '周报' },
      { path: '/chat/dashboard/monthlyReport', label: '月报' },
      { path: '/chat/dashboard/annualReport', label: '年报' }
    ]

    for (const p of pages) {
      await page.goto(p.path)
      await page.waitForLoadState('domcontentloaded')

      const chartContainer = page.locator('canvas, svg.chart, [class*="chart"], [class*="Chart"]').first()
      await expect(chartContainer, `页面 ${p.label} (${p.path}) 必须至少 1 个 chart 容器可见`)
        .toBeVisible({ timeout: 10_000 })
    }
  })
})
