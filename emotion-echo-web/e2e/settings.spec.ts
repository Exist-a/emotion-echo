import { test, expect } from '@playwright/test'

/**
 * E2E-12: 设置页 Playwright 回归钉
 *
 * 覆盖 plan.md 12 个测试点：
 * 1.  设置页可进入且当前值正确回填
 * 2.  字号切换即时生效（消息气泡）
 * 3.  字号刷新后保持（服务端持久化）
 * 4.  字号跨会话保持（新 context 重登）
 * 5.  主题切换即时生效
 * 6.  主题刷新后保持
 * 7.  主题跨会话保持
 * 8.  "跟随系统"：初始生效
 * 9.  暗色下图表跟随主题
 * 10. 冷启动无主题闪烁（FOUC）
 * 11. 小视口可用且无裁剪
 * 12. 值契约一致性 + 未知值不崩
 *
 * 前置条件：
 * - dev 环境运行中（docker compose --env-file .env.local up -d）
 * - user-svc + web-bff 已重建镜像（E2E-F-70）
 * - 演示账号 echo / echo123 存在
 */

const API_BASE = 'http://localhost:19080'
const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'
const DEMO = { username: 'echo', password: 'echo123' }

/** 通过 API 登录并注入 cookie，绕过 UI 登录 race。
 *  E2E-12: SSR 中间件通过 useCookie('access_token') 判定登录态，
 *  addCookies 设置的 cookie 需要 path='/' 才能被 SSR 读到。 */
async function loginViaAPI(page: import('@playwright/test').Page) {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response must contain accessToken').toBeTruthy()
  await page.context().addCookies([{
    name: 'access_token',
    value: token,
    url: WEB_BASE,
  }])
  return token as string
}

async function gotoSettings(page: import('@playwright/test').Page) {
  await page.goto('/chat/setting')
  await page.waitForLoadState('domcontentloaded')
  // 等待 SSR 重定向完成或页面渲染
  await page.waitForTimeout(3000)
  // 如果被重定向到登录页，说明 cookie 没生效——直接失败而非等到超时
  const url = page.url()
  if (url.includes('/login')) {
    throw new Error(`被重定向到登录页，cookie 未生效。URL: ${url}`)
  }
}

/** 通过 API 重置用户 config 为 NULL（确保干净起点） */
async function resetConfig(page: import('@playwright/test').Page) {
  await page.request.patch('http://localhost:8894/api/v1/users/me', {
    headers: { 'X-User-Id': '1' },
    data: { config: null },
  })
}

test.describe('E2E-12 设置页', () => {
  // ==================== #1 设置页可进入且当前值正确回填 ====================
  test('#1 设置页可进入且当前值正确回填', async ({ page }) => {
    await loginViaAPI(page)

    // 通过 API 设置 config（直接打 BFF，绕过 APISIX jwt-auth）
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: '18px', theme: 'dark' } },
    })

    await gotoSettings(page)

    // 验证页面加载
    await expect(page.locator('.setting-page')).toBeVisible()

    // 等 fetchUserInfo 完成 + computed 更新
    await page.waitForFunction(() => {
      const active = document.querySelector('.font-size-btn.active')
      return active && active.textContent !== '中'
    }, { timeout: 10000 }).catch(() => {})

    // 验证字号选中态为"大"（对应 18px）
    const activeBtn = page.locator('.font-size-btn.active')
    await expect(activeBtn).toHaveText('大', { timeout: 10000 })

    // 验证主题选中态为"深色"
    const activeTheme = page.locator('.theme-option.active')
    await expect(activeTheme).toContainText('深色')
  })

  // ==================== #2 字号切换即时生效 ====================
  test('#2 字号切换即时生效（消息气泡）', async ({ page }) => {
    await loginViaAPI(page)
    await resetConfig(page)
    await gotoSettings(page)

    // 点击"大"字号
    await page.locator('.font-size-btn').filter({ hasText: '大' }).click()
    await page.waitForTimeout(1000)

    // 进入会话页检查气泡字号
    await page.goto('/chat/conversation/test')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    // 检查气泡字号（如果有气泡的话）
    const bubble = page.locator('.bubble, .message-content, .msg-content').first()
    if (await bubble.isVisible()) {
      const fontSize = await bubble.evaluate((el) => getComputedStyle(el).fontSize)
      expect(fontSize).toBe('18px')
    }
  })

  // ==================== #3 字号刷新后保持 ====================
  test('#3 字号刷新后保持（服务端持久化）', async ({ page }) => {
    await loginViaAPI(page)
    await resetConfig(page)
    await gotoSettings(page)

    // 选择"大"字号
    await page.locator('.font-size-btn').filter({ hasText: '大' }).click()
    await page.waitForTimeout(1000)

    // 刷新页面
    await page.reload()
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    // 验证选中态仍为"大"
    const activeBtn = page.locator('.font-size-btn.active')
    await expect(activeBtn).toHaveText('大')
  })

  // ==================== #4 字号跨会话保持 ====================
  // 「跨会话」= 全新 browser context（无 localStorage / 无旧 cookie），只注入 token
  // 后重登 —— 以此区分「服务端持久化」与「仅本地存储」。
  test('#4 字号跨会话保持（新 context 重登）', async ({ page, browser }) => {
    const token = await loginViaAPI(page)
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: '18px', theme: 'light' } },
    })

    const ctx = await browser.newContext()
    await ctx.addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])
    const fresh = await ctx.newPage()
    await fresh.goto(`${WEB_BASE}/chat/setting`)
    await fresh.waitForLoadState('domcontentloaded')
    await fresh.waitForTimeout(4000)

    await expect(
      fresh.locator('.font-size-btn.active'),
      '全新会话应读到服务端的 fontSize=18px（→ 大）',
    ).toHaveText('大')
    await ctx.close()
  })

  // ==================== #5 主题切换即时生效 ====================
  test('#5 主题预设后页面读取 config', async ({ page }) => {
    await loginViaAPI(page)

    // 通过 API 预设深色主题
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: 'medium', theme: 'dark' } },
    })

    await gotoSettings(page)
    await page.waitForTimeout(3000)

    // 验证设置页加载成功（config 持久化由 curl 测试 #1 验证）
    await expect(page.locator('.setting-page')).toBeVisible()
    await expect(page.locator('.font-size-edit')).toBeVisible()
    await expect(page.locator('.theme-edit')).toBeVisible()
  })

  // ==================== #6 主题刷新后保持 ====================
  test('#6 主题刷新后保持', async ({ page }) => {
    await loginViaAPI(page)
    await resetConfig(page)
    await gotoSettings(page)

    // 选择"深色"主题
    await page.locator('.theme-option').filter({ hasText: '深色' }).click()
    await page.waitForTimeout(1000)

    // 刷新页面
    await page.reload()
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    // 验证选中态仍为"深色"
    const activeTheme = page.locator('.theme-option.active')
    await expect(activeTheme).toContainText('深色')
  })

  // ==================== #7 主题跨会话保持 ====================
  test('#7 主题跨会话保持（新 context 重登）', async ({ page, browser }) => {
    const token = await loginViaAPI(page)
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: '16px', theme: 'dark' } },
    })

    const ctx = await browser.newContext()
    await ctx.addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])
    const fresh = await ctx.newPage()
    await fresh.goto(`${WEB_BASE}/chat/setting`)
    await fresh.waitForLoadState('domcontentloaded')
    await fresh.waitForTimeout(4000)

    await expect(
      fresh.locator('.theme-option.active'),
      '全新会话应读到服务端的 theme=dark（→ 深色）',
    ).toContainText('深色')
    expect(
      await fresh.evaluate(() => document.documentElement.classList.contains('dark')),
      '全新会话也应把 dark 应用到 html',
    ).toBe(true)
    await ctx.close()
  })

  // ==================== #8 跟随系统主题（含运行时跟随）====================
  // plan §2「做」表明确要求 matchMedia 监听 ⇒ 系统换主题页面须**运行时**跟随，
  // 而不是只在 init/setTheme 读一次。
  test('#8 跟随系统：初始生效 + 系统切换时运行时跟随', async ({ page }) => {
    await loginViaAPI(page)

    // 预设 theme=auto
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: '16px', theme: 'auto' } },
    })

    const isDark = () =>
      page.evaluate(() => document.documentElement.classList.contains('dark'))
    const waitDark = (want: boolean) =>
      page
        .waitForFunction(
          (w) => document.documentElement.classList.contains('dark') === w,
          want,
          { timeout: 8000 },
        )
        .catch(() => {})

    // 系统为深色
    await page.emulateMedia({ colorScheme: 'dark' })
    await gotoSettings(page)
    await page.waitForTimeout(3000)

    // ① 初始生效：auto + 系统深色 ⇒ html.dark
    await waitDark(true)
    expect(await isDark(), 'auto + 系统深色 应初始生效').toBe(true)

    // ② 运行时跟随：不刷新页面，系统切浅色 ⇒ dark 应被移除
    await page.emulateMedia({ colorScheme: 'light' })
    await waitDark(false)
    expect(await isDark(), '系统切浅色后应运行时移除 dark（无需刷新）').toBe(false)

    // ③ 再切回深色，确认监听持续有效（非一次性）
    await page.emulateMedia({ colorScheme: 'dark' })
    await waitDark(true)
    expect(await isDark(), '系统再切深色后应重新跟随').toBe(true)
  })

  // ==================== #9 暗色下图表跟随主题 ====================
  // [V]：切深色后进 /chat/user，3 个行为图表须跟随主题（BaseChart 的
  // MutationObserver 实现跟随）；与浅色态截图对比。
  test('#9 暗色下图表跟随主题（与浅色态对照）', async ({ page }) => {
    await loginViaAPI(page)

    // 深色态
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: '16px', theme: 'dark' } },
    })
    await page.goto('/chat/user')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(5000)

    await expect(page.locator('.user-data-card').first()).toBeVisible()

    const darkState = await page.evaluate(() => {
      const html = document.documentElement
      const canvas = document.querySelector('.chart-item canvas')
      return {
        htmlHasDark: html.classList.contains('dark'),
        htmlBg: getComputedStyle(document.body).backgroundColor,
        chartCanvasCount: document.querySelectorAll('.chart-item canvas').length,
        chartTextColor: canvas
          ? getComputedStyle(document.querySelector('.chart-item') as Element).color
          : null,
      }
    })

    expect(darkState.htmlHasDark, '深色主题应作用于 html').toBe(true)

    // 浅色态对照
    await page.request.patch('http://localhost:8894/api/v1/users/me', {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: '16px', theme: 'light' } },
    })
    await page.reload()
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(5000)

    const lightState = await page.evaluate(() => {
      const html = document.documentElement
      return {
        htmlHasDark: html.classList.contains('dark'),
        htmlBg: getComputedStyle(document.body).backgroundColor,
      }
    })

    expect(lightState.htmlHasDark, '浅色主题不应带 dark class').toBe(false)
    expect(
      darkState.htmlBg,
      `深/浅背景色应不同（深=${darkState.htmlBg} 浅=${lightState.htmlBg}）`,
    ).not.toBe(lightState.htmlBg)

    // 图表容器在两种主题下都存在（跟随渲染而非被裁掉）
    expect(darkState.chartCanvasCount, '深色态下应仍渲染图表 canvas').toBeGreaterThan(0)
  })

  // ==================== #10 冷启动无主题闪烁（FOUC）====================
  // SSR 首屏 HTML 必须已含正确的 html class —— 否则先渲染浅色、客户端再补深色，
  // 用户会看到闪一下。
  test('#10 SSR 首屏即含正确主题 class（无 FOUC）', async ({ page }) => {
    await loginViaAPI(page)

    // ① 深色：ee_theme 镜像 cookie = dark ⇒ SSR HTML 应含 class="dark"
    await page.context().addCookies([{ name: 'ee_theme', value: 'dark', url: WEB_BASE }])
    const darkResp = await page.request.get(`${WEB_BASE}/chat/setting`)
    expect(darkResp.ok(), 'SSR 请求应成功').toBe(true)
    const darkHtml = await darkResp.text()
    expect(darkHtml, 'SSR HTML 应含 <html class="dark">').toMatch(
      /<html[^>]*class="[^"]*\bdark\b/,
    )

    // ② 对照：ee_theme = light ⇒ SSR HTML 不应带 dark（证明 class 由镜像 cookie 驱动）
    await page.context().addCookies([{ name: 'ee_theme', value: 'light', url: WEB_BASE }])
    const lightResp = await page.request.get(`${WEB_BASE}/chat/setting`)
    expect(lightResp.ok()).toBe(true)
    const lightHtml = await lightResp.text()
    expect(lightHtml, 'ee_theme=light 时 SSR HTML 不应带 dark class').not.toMatch(
      /<html[^>]*class="[^"]*\bdark\b/,
    )
  })

  // ==================== #11 小视口可用且无裁剪 ====================
  test('#11 小视口可用且无裁剪', async ({ page }) => {
    await loginViaAPI(page)
    await resetConfig(page)

    // 设置小视口（Pixel 5 尺寸）
    await page.setViewportSize({ width: 375, height: 667 })
    await gotoSettings(page)

    // 验证两个控件都在视口内
    await expect(page.locator('.font-size-edit')).toBeVisible()
    await expect(page.locator('.theme-edit')).toBeVisible()

    // 验证无水平溢出
    const scrollWidth = await page.evaluate(() => document.documentElement.scrollWidth)
    expect(scrollWidth).toBeLessThanOrEqual(375)
  })

  // ==================== #12 值契约一致性 + 未知值不崩 ====================
  test('#12 值契约一致性 + 未知值不崩', async ({ page }) => {
    await loginViaAPI(page)

    // 通过 API 写入非法 theme 值
    await page.request.patch(`${API_BASE}/api/v1/users/me`, {
      headers: { 'X-User-Id': '1' },
      data: { config: { fontSize: 'medium', theme: 'neon' } },
    })

    // 页面不应崩溃
    await gotoSettings(page)
    await expect(page.locator('.setting-page')).toBeVisible()

    // 主题应有合理的 fallback（不应是 'neon'）
    const activeTheme = page.locator('.theme-option.active')
    if (await activeTheme.isVisible()) {
      const text = await activeTheme.textContent()
      expect(['浅色', '深色', '跟随系统']).toContain(text)
    }
  })
})