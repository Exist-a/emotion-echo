/**
 * CL-08a · T-20 / T-21 回归钉 —— 「我的记忆」自述入口 + 跨会话语义（人可见）
 *
 * 两个测试：
 *   1. 侧边栏入口 → 自述 → 固定回执「记忆已更新」→ 关系图渲染（canvas 可见）+ 截图
 *   2. 自述埋入姓名/职业 → **新会话**问起 → AI 复述（只有记忆注入生效才答得出）
 *
 * 运行（需 dev 栈 + 已起的 web 容器；API 走网关，UI 走 :3000）：
 *   BASE_URL=http://127.0.0.1:3000 pnpm exec playwright test e2e/cl-08a-memory.spec.ts --project=chromium
 *
 * ⚠️ `:3000` 只是前端页面（不代理 API）；本 spec 走 UI 路径，不需要直接调网关。
 * ⚠️ 该页**不触发** AI 回答 / 情绪分析 / 数字人 —— 回执是固定文案。
 */
import { test, expect } from '@playwright/test'

// 唯一埋入信息（避免与预置会话/旧记忆撞车导致假通过）
const NAME = '云舟'
const JOB = '插画师'

async function loginAsDemo(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.waitForLoadState('domcontentloaded')
  await page.getByRole('button', { name: /演示账号/ }).click()
  await expect(page.getByRole('heading', { name: '对话', exact: true })).toBeVisible({ timeout: 30_000 })
}

/**
 * 提交一条自述并等回执。
 *
 * 为什么要重试一次：服务刚重建时首个 gRPC 调用会撞 5s 截止（E2E-F-115 既有账），
 * 页面会显示「这次没记录成功」（draft 保留）。重试一次即可，不掩盖真实故障
 * （第二次仍失败 ⇒ 断言直接红）。
 */
async function submitNote(page: import('@playwright/test').Page, text: string) {
  const input = page.locator('#memory-input')
  await expect(input).toBeVisible({ timeout: 30_000 })
  await input.fill(text)

  const ok = page.getByText('记忆已更新').first()
  const fail = page.getByText('这次没记录成功').first()

  await page.getByRole('button', { name: '记录' }).click()
  await expect(ok.or(fail)).toBeVisible({ timeout: 30_000 })
  if (await fail.isVisible().catch(() => false)) {
    await page.getByRole('button', { name: '记录' }).click()
    await expect(ok).toBeVisible({ timeout: 30_000 })
  }
}

test.describe('CL-08a · 我的记忆', () => {
  test('自述 → 固定回执 → 关系图渲染', async ({ page }) => {
    test.setTimeout(180_000)
    await loginAsDemo(page)

    // 1) 侧边栏入口（在「对话」「心理测验」下面）
    await page.getByRole('link', { name: '我的记忆' }).click()
    await expect(page).toHaveURL(/\/chat\/memory/)
    await expect(page.getByRole('heading', { name: '我的记忆', exact: true })).toBeVisible({ timeout: 30_000 })

    // 2) 写一条自述 + 固定回执（该页不调大模型回答）
    await submitNote(page, '今天想记一件事：我最近开始学着接受自己慢一点。')

    // 4) 关系图渲染（ECharts 画在 canvas 上；空图会显示「还没有记忆」）
    await expect(page.locator('canvas').first()).toBeVisible({ timeout: 60_000 })

    // 5) [V] 证据截图
    await page.screenshot({
      path: '../docs/closure-roadmap/stages/cl-08-memory/evidence/t20-memory-page.png',
      fullPage: false,
    })
  })

  test('跨会话：自述里的姓名与职业能在新会话被复述', async ({ page }) => {
    test.setTimeout(300_000)
    await loginAsDemo(page)

    // 1) 在「我的记忆」埋入姓名 + 职业
    await page.getByRole('link', { name: '我的记忆' }).click()
    await submitNote(page, `我叫${NAME}，是一名${JOB}。`)

    // 2) 抽取是异步的：固定等 20s。
    //    注意：关系图画在 canvas 上，无法用 DOM 文本断言"实体已出现"，因此这里只能用
    //    时间窗（经验值：LLM 抽取 + 摘要重建 ≈ 5~10s）。若未来换成 SVG 渲染，应改为轮询。
    await page.getByRole('link', { name: '对话' }).click()
    await expect(page.getByRole('heading', { name: '对话', exact: true })).toBeVisible({ timeout: 30_000 })
    await page.waitForTimeout(20_000)

    // 3) 新会话提问（不提供任何信息）
    const box = page.getByPlaceholder(/请输入想倾诉的内容/)
    await expect(box).toBeVisible({ timeout: 30_000 })
    await box.fill('你还记得我叫什么名字、做什么工作吗？')
    await page.getByRole('button', { name: /^发送/ }).first().click()

    // 4) 断言：最新一条 AI 回复复述了自述里的姓名（+ 职业弱匹配）
    const lastArticle = page.locator('article').last()
    await expect(lastArticle).toContainText(NAME, { timeout: 120_000 })
    await expect(lastArticle).toContainText(/插画师|插画/, { timeout: 120_000 })
  })
})
