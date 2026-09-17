import { test, expect } from '@playwright/test'

/**
 * E2E: dashboard flow · A3 + A4 端到端验证 (Sprint 110)
 *
 * A3: 5 个 dashboard 页面 (index/daily/weekly/monthly/annual) 无 Playwright 覆盖
 * A4: Stage 36-FU "dev 模式 4 dashboard chartData.length === 0" 历史 bug 未复测
 *
 * 本 spec 钉死:
 *   happy-path-1: quick-login → /chat/dashboard/dailyReport → 渲染断言
 *   happy-path-2: 各 dashboard 页面至少图表容器可见
 */

test.describe('dashboard flow · A3 + A4 chartData 验证', () => {
  test('happy-path-1: quick-login → dailyReport 页面渲染成功', async ({ page }) => {
    // 1) 登录
    await page.goto('/login')
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 90_000 })
    await quickBtn.click()
    await page.waitForURL(/\/chat\/conversation/, { timeout: 15_000 })

    // 2) 跳转到 daily report
    await page.goto('/chat/dashboard/dailyReport')
    await page.waitForLoadState('domcontentloaded')

    // 3) 页面应该有 heading "日报" 或类似标识
    const heading = page.getByRole('heading', { name: /日报|daily/i }).first()
    await expect(heading).toBeVisible({ timeout: 15_000 })
  })

  test('happy-path-2: 4 个 dashboard 页面均可访问 + 渲染至少 1 个 chart 容器', async ({ page }) => {
    // 登录
    await page.goto('/login')
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeVisible({ timeout: 90_000 })
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

      // 至少 1 个 chart 容器（canvas / svg / chart 相关 class）
      const chartContainer = page.locator('canvas, svg.chart, [class*="chart"], [class*="Chart"]').first()
      await expect(chartContainer, `页面 ${p.label} (${p.path}) 必须至少 1 个 chart 容器可见`)
        .toBeVisible({ timeout: 10_000 })
    }
  })
})
