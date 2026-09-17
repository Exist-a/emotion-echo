import { test, expect } from '@playwright/test'

// Sprint 110 · A8 真修复验证截图 (用于 stage-110 doc)
test('A8 真修复截图存证', async ({ page }) => {
  // API login
  await page.goto('/login')
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
  const loginResp = await page.request.post('http://localhost:19080/api/v1/auth/login', {
    data: { username: 'echo', password: 'echo123' }
  })
  const loginBody = await loginResp.json()
  const token = loginBody.data.accessToken
  await page.context().addCookies([{ name: 'access_token', value: token, url: 'http://localhost:3000' }])

  await page.goto('/chat/conversation/new')
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(5000)

  const textarea = page.locator('textarea').first()
  await textarea.waitFor({ state: 'visible', timeoutMs: 10000 })
  await textarea.fill('你好')
  const sendBtn = page.locator('button.send-btn[type="submit"]').first()
  await sendBtn.waitFor({ state: 'visible', timeoutMs: 5000 })
  await sendBtn.click()
  await page.waitForTimeout(15000)

  // 截图存证
  const fs = await import('node:fs')
  const buf = await page.screenshot()
  fs.writeFileSync('D:/源码/Emotion-Echo/gui-test-screenshots/a8-fixed-final.png', buf)

  // 断言
  await expect(page.locator('.dialog-ai').first()).toBeVisible({ timeout: 5000 })
})
