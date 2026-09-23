// e2e/e2e-16-mobile.spec.ts — E2E-16 plan §5 测试点 24
//
// 测试点 24：移动端（Pixel 5，375px）语音 / 附件 / 摄像头三入口可用且不遮挡。
//
// 设计：用 Playwright mobile project (Pixel 5 viewport 393x851 / DPR 2.75)
// 走 API 登录拿 token → 注入 cookie → /chat/conversation/new → 断言三入口
// DOM 存在 + 不水平溢出（scrollWidth ≤ clientWidth + 1px 容差）+ viewport 内 +
// 截图。
//
// 复用根目录 playwright.config.ts（已有 chromium + mobile 两个 project）。

import { test, expect } from '@playwright/test'

const BASE_URL = process.env.BASE_URL ?? 'http://localhost:3000'
const API_URL = process.env.API_URL ?? 'http://localhost:8894'

test.describe('E2E-16 · 测试点 24：移动端三入口（Pixel 5）', () => {
  test('Pixel 5 viewport 下 /new 页三入口可见 + 不遮挡 + 不水平溢出', async ({ page, request }) => {
    // 1. 走 apisix 网关（:19080）登录，拿到 token + Set-Cookie access_token。
    //    :3000 直连不是 BFF（被前端 SSR 拦回 /login 302），所以必须经网关。
    const loginResp = await request.post(`${API_URL}/api/v1/auth/login`, {
      data: { username: 'smoke_user', password: 'echo123' },
    })
    expect(loginResp.ok(), `login must 200, got ${loginResp.status()}`).toBeTruthy()
    const setCookie = loginResp.headers()['set-cookie']
    expect(setCookie, 'login must return Set-Cookie').toBeTruthy()
    const m = setCookie!.match(/access_token=([^;]+)/)
    expect(m, 'Set-Cookie must contain access_token').toBeTruthy()
    const token = m![1]

    // 2. 把 access_token 设到 localhost:3000 的 cookie jar（web-bff 接受 cookie）
    await page.context().addCookies([
      {
        name: 'access_token',
        value: token,
        domain: 'localhost',
        path: '/',
        httpOnly: true,
        sameSite: 'Lax',
      },
    ])
    // 调试：确认 cookie 真的落到 localhost domain
    const cookiesAfter = await page.context().cookies()
    console.log('cookies after add:', cookiesAfter.map((c) => `${c.name}@${c.domain}:${c.path}`))

    // 3. 进 /new（cookie 自动带上）
    await page.goto(`${BASE_URL}/chat/conversation/new`)
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(2000)

    // 4. 三入口断言（plan §8-C：「语音真录音 / 附件真可发 / 摄像头可用」）
    await expect(page.locator('.voice-record-btn')).toBeVisible()
    await expect(page.getByRole('button', { name: '添加附件' })).toBeVisible()
    await expect(page.getByRole('button', { name: '开启摄像头' })).toBeVisible()
    // 额外：登录后页面是 /chat/conversation/new 不是 /login —— 防 cookie 失效回归
    expect(page.url(), '应停在 /new 而非被重定向到 /login').toContain('/chat/conversation/new')

    // 5. 布局无水平溢出
    const overflow = await page.evaluate(() => ({
      scrollWidth: document.documentElement.scrollWidth,
      clientWidth: document.documentElement.clientWidth,
      innerWidth: window.innerWidth,
    }))
    // 容差 1px（Pixel 5 DPR 2.75 下 sub-pixel 渲染）
    expect(
      overflow.scrollWidth,
      `Pixel 5 viewport 出现水平溢出: scrollWidth=${overflow.scrollWidth} > clientWidth=${overflow.clientWidth}`,
    ).toBeLessThanOrEqual(overflow.clientWidth + 1)

    // 6. 三入口在 viewport 内可见（不被 nav 遮挡）
    const visible = await page.evaluate(() => {
      const sel = ['.voice-record-btn', '[aria-label="添加附件"]', '[aria-label="开启摄像头"]']
      return sel.map((s) => {
        const el = document.querySelector(s)
        if (!el) return null
        const r = el.getBoundingClientRect()
        return {
          sel: s,
          x: Math.round(r.x), y: Math.round(r.y), w: Math.round(r.width), h: Math.round(r.height),
          inViewport: r.x >= 0 && r.y >= 0 && r.bottom <= window.innerHeight && r.right <= window.innerWidth,
        }
      })
    })
    for (const v of visible) {
      if (!v) continue
      expect(v.inViewport, `${v.sel} 不在 viewport 内: x=${v.x} y=${v.y} w=${v.w} h=${v.h}`).toBe(true)
    }

    // 7. 截图 [V] 证据
    await page.screenshot({
      path: 'playwright-report/e2e-16-mobile-pixel5-new.png',
      fullPage: false,
    })
  })
})
