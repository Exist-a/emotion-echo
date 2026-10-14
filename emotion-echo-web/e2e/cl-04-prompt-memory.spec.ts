/**
 * CL-04 · Phase 7（T-14）回归钉 —— 多轮「AI 记得上文」的**人可见钉**。
 *
 * 背景（CL-04 前）：对话 prompt 只有 `[system, 最后一条 user]` ⇒ 多轮＝无记忆单轮。
 * 本 spec 在**真实 UI** 里发两轮：
 *   第 1 轮埋入「姓名 + 岗位」；第 2 轮**只问**「还记得我叫什么、应聘什么岗位吗」——
 *   只有 prompt 真带历史才答得出（这正是 AGENTS §2.4「数据对不对」式的强断言，
 *   而不是"HTTP 200 就算过"）。
 *
 * 人工取证（[V]，供人核）：`docs/closure-roadmap/stages/cl-04-prompt-assembly/evidence/iab-t14-multiturn.png`
 *
 * 运行（需 dev 栈 + 本地前端；**不要**让 webServer 自起，用已有 dev server）：
 *   BASE_URL=http://127.0.0.1:3000 pnpm exec playwright test e2e/cl-04-prompt-memory.spec.ts --project=chromium
 *
 * ⚠️ 两个端点别混：`:3000` 只是前端页面（SSR HTML，不代理 API）；**API 必须走网关 `:19080`**。
 * 本 spec 走 UI 路径，所以不需要直接调 API —— 登录用页面上的「演示账号」按钮。
 */
import { test, expect } from '@playwright/test'

// 埋入用的唯一信息（不常见组合，避免与预置会话内容撞车造成假通过）
const NAME = '阿澈'
const JOB = '数据分析师'

test.describe('CL-04/T-14 · AI 记得上文（多轮对话）', () => {
  test('第二轮回复必须复述第一轮埋入的姓名与岗位', async ({ page }) => {
    test.setTimeout(300_000)

    // 1) 演示账号一键登录（与 IAB 人工验收同一路径）
    await page.goto('/login')
    await page.waitForLoadState('domcontentloaded')
    await page.getByRole('button', { name: /演示账号/ }).click()
    // ⚠️ 必须 exact：页面上有两个含「对话」的标题（h1「对话」+ h3「最近的对话」），
    // 不加 exact 会 strict-mode 二义（本钉子首次跑就踩了这个）。
    await expect(page.getByRole('heading', { name: '对话', exact: true })).toBeVisible({ timeout: 30_000 })

    // 2) 第 1 轮：埋入姓名 + 岗位
    const firstBox = page.getByPlaceholder(/请输入想倾诉的内容/)
    await expect(firstBox).toBeVisible({ timeout: 30_000 })
    await firstBox.fill(`我叫${NAME}，准备应聘${JOB}，下周有个面试，一想到被追问细节就睡不好。`)
    await page.getByRole('button', { name: /^发送/ }).first().click()

    // 3) 等第 1 轮落定：输入框 placeholder 变为「想说点什么？」（会话已建立），再等流式收尾
    const nextBox = page.getByPlaceholder(/想说点什么/)
    await expect(nextBox).toBeVisible({ timeout: 90_000 })
    await page.waitForTimeout(20_000)

    // 4) 第 2 轮：只问「是否记得」，不提供任何信息
    await nextBox.fill('你还记得我叫什么、应聘的是什么岗位吗？')
    await page.getByRole('button', { name: '发送消息' }).click()

    // 5) 断言：**最新一条** AI 回复里必须出现埋入的姓名（+ 岗位相关的弱匹配）
    const lastArticle = page.locator('article').last()
    await expect(lastArticle).toContainText(NAME, { timeout: 90_000 })
    await expect(lastArticle).toContainText(/数据|分析/, { timeout: 90_000 })
  })
})
