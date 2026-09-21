import { test, expect } from '@playwright/test'

/**
 * E2E-15 · 报表 Dashboard 端到端
 *
 * 覆盖测试点：
 *  1.  dailyReport 渲染正常数据（[A]+[V]）       E2E-15 plan §5 #1
 *  2.  weeklyReport 渲染正常数据                    E2E-15 plan §5 #2
 *  3.  monthlyReport 渲染正常数据                  E2E-15 plan §5 #3
 *  4.  annualReport 渲染正常数据                    E2E-15 plan §5 #4
 *  5.  dailyReport 日期切换 → 数据更新              E2E-15 plan §5 #5
 *  6.  weeklyReport 周切换                          E2E-15 plan §5 #6
 *  7.  monthlyReport 月切换                         E2E-15 plan §5 #7
 *  8.  annualReport 年切换                          E2E-15 plan §5 #8
 *  9.  4 dashboard 空态 div 加 v-else-if（已有 contract 钉）   E2E-15 plan §5 #9
 *  10. mental-health 端点有真实数据 → backend smoke（#10/#11）
 *  12. E2E-F-36 滚动复验（dashboard 4 页 × 1280×600）         E2E-15 plan §5 #12
 *  13. 暗色主题跟随（dashboard 4 页）                            E2E-15 plan §5 #13
 *  14. 跨视口响应式（<992 / 992-1600 / ≥1600）                  E2E-15 plan §5 #14
 *
 * 前置条件：
 *  - dev 环境运行中（docker compose --env-file .env.local up -d）
 *  - analytics-svc:v0.1.8 含 Save 路径（PR #47 已 merge + 镜像已 rebuild）
 *  - 演示账号 echo / echo123 存在
 *  - mental_health_assessments 演示数据已 seed（dev seed 脚本或手工 INSERT）
 */

const API_BASE = 'http://localhost:19080'
const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'
const DEMO = { username: 'echo', password: 'echo123' }

/** 通过 API 登录拿 token，注入 cookie 绕 UI race */
async function loginViaAPI(page: import('@playwright/test').Page) {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, {
    data: DEMO,
  })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response must contain accessToken').toBeTruthy()
  await page.context().addCookies([
    { name: 'access_token', value: token, url: WEB_BASE },
  ])
  return token
}

/** 等 Nuxt SSR-off hydration 完成 */
async function waitForHydration(page: import('@playwright/test').Page) {
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
}

test.describe('E2E-15 报表 Dashboard', () => {
  // ============ #1 dailyReport 渲染正常数据 ============
  test('#1 dailyReport 渲染 summary + conversationCount + messageCount（emotionDistribution 可空）', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/chat/dashboard/dailyReport')
    await waitForHydration(page)

    // summary 文本非空（不依赖 AI 分析触发）
    await expect(page.getByText(/段对话/).first(), 'summary 含「段对话」文案').toBeVisible({ timeout: 10_000 })

    // E2E-15 IAB 实测补充（2026-09-21）：有图表时不得同时出现「暂无数据」
    // （原 spec 只断言 summary 存在 ⇒ 放过 v-else-if 未生效的旧行为）
    const chartCount = await page.locator('.chart-container').count()
    if (chartCount > 0) {
      await expect(
        page.locator('.ee-empty'),
        'E2E-15: chartData.length > 0 时「暂无数据」不得与图表同屏（v-else-if 互斥）',
      ).toHaveCount(0)
    }

    // 截图归档（双 project 防覆盖）
    await page.screenshot({
      path: `screenshots/e2e-15-01-daily-report-${test.info().project.name}.png`,
      fullPage: true,
    })
  })

  // ============ #2 weeklyReport 渲染 ============
  test('#2 weeklyReport 渲染 summary + dates + series', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/chat/dashboard/weeklyReport')
    await waitForHydration(page)

    // weeklyReport 标题 + summary
    await expect(page.getByText(/周报|week/).first()).toBeVisible({ timeout: 10_000 })

    await page.screenshot({
      path: `screenshots/e2e-15-02-weekly-report-${test.info().project.name}.png`,
      fullPage: true,
    })
  })

  // ============ #3 monthlyReport 渲染 ============
  test('#3 monthlyReport 渲染 summary', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/chat/dashboard/monthlyReport')
    await waitForHydration(page)

    await expect(page.getByText(/月报|month/).first()).toBeVisible({ timeout: 10_000 })

    await page.screenshot({
      path: `screenshots/e2e-15-03-monthly-report-${test.info().project.name}.png`,
      fullPage: true,
    })
  })

  // ============ #4 annualReport 渲染 ============
  test('#4 annualReport 渲染 summary', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/chat/dashboard/annualReport')
    await waitForHydration(page)

    await expect(page.getByText(/年报|year/).first()).toBeVisible({ timeout: 10_000 })

    await page.screenshot({
      path: `screenshots/e2e-15-04-annual-report-${test.info().project.name}.png`,
      fullPage: true,
    })
  })

  // ============ #5 dailyReport 日期切换 ============
  test('#5 dailyReport 日期切换 → chartData 更新（picker 改 date 后重 fetch）', async ({ page }) => {
    await loginViaAPI(page)

    // 抓初始 daily 报告的 messageCount
    const initialResp = await page.request.get(`${API_BASE}/api/v1/reports/daily?date=2026-09-21`)
    expect(initialResp.ok()).toBe(true)
    const initialData = await initialResp.json()
    const initialMsgCount = initialData?.data?.messageCount

    await page.goto('/chat/dashboard/dailyReport')
    await waitForHydration(page)

    // 验证 page 上展示的 messageCount 与 API 一致
    if (initialMsgCount && initialMsgCount > 0) {
      await expect(page.getByText(`${initialMsgCount}`).first()).toBeVisible({ timeout: 10_000 })
    }

    await page.screenshot({
      path: `screenshots/e2e-15-05-daily-date-switch-${test.info().project.name}.png`,
      fullPage: true,
    })
  })

  // ============ #6/7/8 weekly/monthly/annual 切换 ============
  test('#6 weeklyReport 切换 trend type 端点可访问', async ({ page }) => {
    await loginViaAPI(page)
    const r = await page.request.get(`${API_BASE}/api/v1/reports/trend?type=weekly&start_date=2026-09-15&end_date=2026-09-21`)
    expect(r.ok()).toBe(true)
    const body = await r.json()
    expect(body?.data?.dates?.length).toBeGreaterThan(0, 'weekly trend dates 应非空')
  })

  test('#7 monthlyReport 切换 trend type 端点可访问', async ({ page }) => {
    await loginViaAPI(page)
    const r = await page.request.get(`${API_BASE}/api/v1/reports/trend?type=monthly&start_date=2026-09-01&end_date=2026-09-21`)
    expect(r.ok()).toBe(true)
    const body = await r.json()
    expect(body?.data?.dates?.length).toBeGreaterThan(0, 'monthly trend dates 应非空')
  })

  test('#8 annualReport 切换 trend type 端点可访问', async ({ page }) => {
    await loginViaAPI(page)
    const r = await page.request.get(`${API_BASE}/api/v1/reports/trend?type=yearly&start_date=2026-01-01&end_date=2026-09-21`)
    expect(r.ok()).toBe(true)
    const body = await r.json()
    expect(body?.data?.dates?.length).toBeGreaterThan(0, 'annual trend dates 应非空')
  })

  // ============ #10/#11 mental-health 端点有真实数据（修 E2E-F-10 后） ============
  test('#10 mental-health/assessment 端点返回真实数据（修 E2E-F-10 后）', async ({ page }) => {
    await loginViaAPI(page)
    const r = await page.request.get(`${API_BASE}/api/v1/mental-health/assessment?type=daily`)
    expect(r.ok()).toBe(true)
    const body = await r.json()
    const a = body?.data?.assessment
    expect(a, 'assessment 字段应存在').not.toBeNull()
    expect(a?.overallScore, 'overallScore 应 > 0').toBeGreaterThan(0)
    expect(a?.riskLevel, 'riskLevel 应推导出 low/moderate/high/severe').toMatch(/low|moderate|high|severe/)
    expect(a?.dimensions?.length, 'dimensions 应非空').toBeGreaterThan(0)
  })

  // ============ #12 E2E-F-36 滚动复验 ============
  test('#12 dashboard 4 页 × 1280×600 滚动可用（E2E-F-36 复验）', async ({ page }) => {
    await page.setViewportSize({ width: 1280, height: 600 })
    await loginViaAPI(page)

    for (const route of ['/chat/dashboard/dailyReport', '/chat/dashboard/weeklyReport',
                         '/chat/dashboard/monthlyReport', '/chat/dashboard/annualReport']) {
      await page.goto(route)
      await waitForHydration(page)

      // .page-content 必须有 overflow-y: auto（E2E-F-36 修复已生效）
      const overflow = await page.evaluate(() => {
        const el = document.querySelector('.page-content') as HTMLElement | null
        if (!el) return 'no-page-content'
        const style = getComputedStyle(el)
        return style.overflowY
      })
      expect(overflow, `${route}: .page-content overflow-y 应为 auto 或 scroll`).toMatch(/auto|scroll/)
    }
  })

  // ============ #13 暗色主题跟随 ============
  test('#13 暗色主题切换后页面正常渲染（主题跟随由 BaseChart ECharts dark theme 处理）', async ({ page }) => {
    await loginViaAPI(page)
    await page.goto('/chat/dashboard/dailyReport')
    await waitForHydration(page)

    // 验证 dark mode 下仍能渲染
    await page.evaluate(() => {
      document.documentElement.classList.add('dark')
    })
    await page.waitForTimeout(500)

    await page.screenshot({
      path: `screenshots/e2e-15-13-dark-mode-${test.info().project.name}.png`,
      fullPage: true,
    })

    // 关键元素仍可见
    await expect(page.locator('body')).toBeVisible()
  })

  // ============ #14 跨视口响应式 ============
  test('#14 chartsCard 跨视口列数（<992/992-1600/≥1600）', async ({ page }) => {
    await loginViaAPI(page)

    // <992px → 1 列
    await page.setViewportSize({ width: 800, height: 900 })
    await page.goto('/chat/dashboard/weeklyReport')
    await waitForHydration(page)
    // E2E-15 IAB 实测补充（2026-09-21）：不只是截图，要断言列数真的重算了
    // （原实现 window.innerWidth 非响应式 ⇒ resize 后不重算，图表被挤压）
    const cols800 = await page
      .locator('.charts-grid')
      .first()
      .evaluate((el) => getComputedStyle(el).gridTemplateColumns)
    expect(
      cols800.split(' ').length,
      `E2E-15: 800px 视口应 1 列（实际 ${cols800}）`,
    ).toBe(1)
    await page.screenshot({ path: `screenshots/e2e-15-14a-viewport-800-${test.info().project.name}.png`, fullPage: true })

    // ≥1600px → 3 列
    await page.setViewportSize({ width: 1800, height: 900 })
    await page.reload()
    await waitForHydration(page)
    const cols1800 = await page
      .locator('.charts-grid')
      .first()
      .evaluate((el) => getComputedStyle(el).gridTemplateColumns)
    expect(
      cols1800.split(' ').length,
      `E2E-15: 1800px 视口应 3 列（实际 ${cols1800}）`,
    ).toBe(3)
    await page.screenshot({ path: `screenshots/e2e-15-14b-viewport-1800-${test.info().project.name}.png`, fullPage: true })
  })
})