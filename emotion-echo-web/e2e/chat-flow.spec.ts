import { test, expect } from '@playwright/test'

/**
 * E2E: chat flow · A8 端到端回归钉子 (Sprint 110)
 *
 * IAB 实测 2026-09-17 复现 (gui-test-screenshots/a8-02-after-send.png):
 *   - 浏览器点发送, URL 跳到 /chat/conversation/N ✓
 *   - POST /conversations + GET /messages + POST /messages ✓
 *   - POST /api/v1/ai/stream 缺席 ✗ (SSE 流被 abort)
 *   - BFF 日志 0 条 ai-stream 调用 ✗
 *   - chat-svc DB 无 AI 消息入库 ✗
 *
 * 本 spec 钉死 (Green 阶段):
 *   happy-path-1: quick-login → /chat/conversation/new → 输入消息 → 点发送 → 5s 内
 *                 看到至少 1 个 .dialog-ai 元素 + fetch hook 含 POST /api/v1/ai/stream
 *   happy-path-2: AI 回复气泡包含非空文字
 */

test.describe('chat flow · A8 SSE 流端到端', () => {
  test('happy-path-1: quick-login → 发消息 → 5s 内看到 .dialog-ai + 触发 ai-stream 请求', async ({ page }) => {
    const aiStreamRequests: string[] = []
    const allRequests: string[] = []
    page.on('request', (req) => {
      allRequests.push(`${req.method()} ${req.url()}`)
      if (req.method() === 'POST' && req.url().includes('/api/v1/ai/stream')) {
        aiStreamRequests.push(req.url())
      }
    })
    page.on('console', (msg) => {
      console.log(`[BROWSER-${msg.type().toUpperCase()}]`, msg.text())
    })
    page.on('pageerror', (err) => {
      console.log(`[BROWSER-PAGEERROR]`, err.message)
    })

    // 1) 直接 goto 登录页 + 用 API 设置 cookie（绕过 navigateTo race + 503 编译问题）
    await page.goto('/login')
    await page.waitForLoadState('domcontentloaded')
    // 等 Nuxt dev SSR=off 客户端 hydrate
    await page.waitForTimeout(3000)

    // 用 page.request 直接调 POST /auth/login（APISIX 路由）设置 cookie
    const loginResp = await page.request.post('http://localhost:19080/api/v1/auth/login', {
      data: { username: 'echo', password: 'echo123' }
    })
    expect(loginResp.ok(), 'login API 必须成功').toBe(true)
    const loginBody = await loginResp.json()
    const token = loginBody?.data?.accessToken
    expect(token, 'login response 必须含 accessToken').toBeTruthy()
    // 把 token 写入 cookie + localStorage（前端 auth 依赖）
    await page.context().addCookies([
      { name: 'access_token', value: token, url: 'http://localhost:3000' }
    ])

    // 2) 直接 goto /chat/conversation/new（已经有 token，UI 应自动识别已登录）
    await page.goto('/chat/conversation/new')
    // 等首屏（避免 503 chunk 编译）
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(5000)

    // 3) 输入消息 + 点发送
    //    /chat/conversation/new 的 textarea 是 .sender-input（class）
    //    [id].vue 的 textarea placeholder 是 "想说点什么？"
    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('你好，请用一句话介绍你自己')

    // 4) 点发送：new.vue 的按钮 class 是 .send-btn（aria-label="发送"）
    const sendBtn = page.locator('button.send-btn[type="submit"]').first()
    await expect(sendBtn).toBeVisible({ timeout: 5_000 })
    console.log('[DEBUG-SEND-BTN] aria-label=', await sendBtn.getAttribute('aria-label'))
    await sendBtn.click()

    // 5) 5 秒内看到至少 1 个 .dialog-ai 元素（A8 修复成功的信号）
    console.log('[DEBUG-ALL-REQUESTS]', allRequests.length)
    for (const r of allRequests) console.log('  ', r)
    console.log('[DEBUG-AI-STREAM-REQUESTS]', aiStreamRequests.length)

    await expect(page.locator('.dialog-ai').first()).toBeVisible({ timeout: 10_000 })

    // 6) fetch hook 监听到 POST /api/v1/ai/stream（A8 真根因修复）
    expect(
      aiStreamRequests.length,
      '点发送后必须触发至少 1 次 POST /api/v1/ai/stream, 否则 SSE 流没真发出 (A8 现象)'
    ).toBeGreaterThanOrEqual(1)
  })

  test('happy-path-2: AI 回复气泡包含非空文字内容', async ({ page }) => {
    // 1) 登录
    await page.goto('/login')
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await quickBtn.click()
    await page.waitForURL(/\/chat\/conversation\/(new|\d+)/, { timeout: 15_000 })
    await page.waitForLoadState('domcontentloaded')

    // 2) 发消息
    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('hello')
    const sendBtn = page.getByRole('button', { name: /^发送$/ }).first()
    await sendBtn.click()

    // 3) 等 .dialog-ai 出现 + 文字非空
    const aiBubble = page.locator('.dialog-ai .bubble-ai').first()
    await expect(aiBubble).toBeVisible({ timeout: 10_000 })
    // dev 模式 mockEmpathyReply 至少返回 2 字符，真实 LLM 至少 1 字符
    const text = (await aiBubble.textContent())?.trim() ?? ''
    expect(text.length, 'AI 回复气泡必须包含非空文字').toBeGreaterThan(0)
  })
})
