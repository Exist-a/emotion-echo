import { test, expect } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

/** 截图归档目录——带 project.name 后缀防双 project 覆盖（E2E-14 教训） */
const SCREENSHOT_DIR = join(process.cwd(), 'screenshots')
mkdirSync(SCREENSHOT_DIR, { recursive: true })

/**
 * E2E-18 缓存层回归钉
 *
 * 目的：LRU 默认启用（WORKER_LRU_CAPACITY fallback 0→1024，E2E-18 #3）
 * + ai-svc 镜像 rebuild（v0.1.8）之后，聊天主链路不回归。
 *
 * 覆盖测试点：
 * #10 回归钉：双 project 全绿
 * #11 主链路视觉证据（截图归档）
 *
 * 前置条件：
 * - dev 环境运行中（RUNBOOK §2.1，必带 --env-file .env.local --profile dev）
 * - ai-svc v0.1.8（含 LRU 默认启用修复）healthy
 * - 演示账号 echo / echo123 存在
 *
 * 断言边界（诚实声明）：
 * - LRU 属 fusion worker 后台路径（情绪融合去重），浏览器不可直接观测；
 *   其"启用"证据在启动日志（plan #4）与 /metrics（plan #5），不在本 spec。
 * - 本 spec 断言的是"主链路在缓存层改动后不回归"这一集成面。
 */

const API_BASE = 'http://localhost:19080'
const DEMO = { username: 'echo', password: 'echo123' }

async function loginViaAPI(page: import('@playwright/test').Page) {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, {
    data: DEMO,
  })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response must contain accessToken').toBeTruthy()
  await page.context().addCookies([
    { name: 'access_token', value: token, url: 'http://localhost:3000' },
  ])
}

async function waitForHydration(page: import('@playwright/test').Page) {
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
}

test.describe('E2E-18 缓存层回归钉', () => {
  test('#10a 发消息后 AI 回复出现（ai-svc rebuild 后主链路不回归）', async ({
    page,
  }) => {
    // 记录请求，断言 /ai/stream 真被调用（区分"回复出现"与"mock 静默兜底"）
    const streamRequests: string[] = []
    page.on('request', (req) => {
      if (req.method() === 'POST' && req.url().includes('/api/v1/ai/stream')) {
        streamRequests.push(req.url())
      }
    })

    await loginViaAPI(page)
    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('缓存层回归测试：请回复任意一句话')
    await page.locator('button.send-btn[type="submit"]').first().click({
      force: true,
    })

    // AI 气泡出现且非空
    const aiBubble = page.locator('.dialog-ai .bubble-ai').first()
    await expect(aiBubble).toBeVisible({ timeout: 15_000 })
    await expect(async () => {
      const text = (await aiBubble.textContent())?.trim() ?? ''
      expect(text.length).toBeGreaterThan(0)
    }).toPass({ timeout: 20_000 })

    // /ai/stream 至少触发一次
    expect(streamRequests.length, '必须触发 POST /api/v1/ai/stream').toBeGreaterThanOrEqual(1)

    // 截图归档（plan #11 [V] 视觉证据）
    const suffix = test.info().project.name
    await page.screenshot({
      path: join(SCREENSHOT_DIR, `11-chat-smoke-after-lru-${suffix}.png`),
      fullPage: false,
    })
  })

  test('#10b 连续两条消息无 429（限流中间件与 LRU 启用不误伤正常流量）', async ({
    page,
  }) => {
    const clientErrors: number[] = []
    page.on('response', (resp) => {
      const url = resp.url()
      if (url.includes('/api/v1/') && resp.status() >= 400) {
        clientErrors.push(resp.status())
      }
    })

    await loginViaAPI(page)
    await page.goto('/chat/conversation/new')
    await waitForHydration(page)

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
    await textarea.fill('连续消息第 1 条')
    await page.locator('button.send-btn[type="submit"]').first().click({
      force: true,
    })
    const aiBubble = page.locator('.dialog-ai .bubble-ai').first()
    await expect(aiBubble).toBeVisible({ timeout: 15_000 })

    await textarea.fill('连续消息第 2 条')
    await page.locator('button.send-btn[type="submit"]').first().click({
      force: true,
    })
    await expect(async () => {
      const bubbles = await page.locator('.dialog-ai .bubble-ai').count()
      expect(bubbles).toBeGreaterThanOrEqual(2)
    }).toPass({ timeout: 20_000 })

    // 正常用户流量不应被限流误伤（429 是限流信号）
    expect(
      clientErrors.filter((s) => s === 429),
      '正常两条消息不得触发 429',
    ).toEqual([])
    // 也不允许出现 5xx（ai-svc rebuild 后链路健康）
    expect(
      clientErrors.filter((s) => s >= 500),
      '不得出现 5xx（rebuild 后链路健康）',
    ).toEqual([])
  })
})
