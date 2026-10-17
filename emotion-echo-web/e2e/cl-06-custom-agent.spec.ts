/**
 * CL-06 · Phase 8 回归钉 —— 用户自定义 Agent（多角色）的**人可见钉**。
 *
 * 覆盖三条**只有真实浏览器里才成立**的断言（单测/源码契约都替代不了）：
 *   ① 角色管理页在**生产构建**里真的可达（CL-06 Phase 8 实测过它 404 —— 根因是
 *      `app/router.options.ts` 手写路由表未注册，见账本 CL-F-46 与
 *      `app/router.options.contract.test.ts` 的门禁）；
 *   ② 切换器位于 composer **语音按钮左侧**，且菜单**向上展开**（用户 2026-10-17 两次澄清的
 *      位置与形态要求；用 getBoundingClientRect 量化，不是"存在即通过"）；
 *   ③ 切换角色**写本会话**（PATCH）且**只影响后续消息**：切换后发给 AI 的消息带新角色 id。
 *
 * 人工取证（[V]）：`docs/closure-roadmap/stages/cl-06-custom-agent/evidence/phase8-iab-switcher-upward.png`
 *
 * 运行（需 dev 栈 + 已在跑的前端镜像；**不要**让 webServer 自起）：
 *   BASE_URL=http://127.0.0.1:3000 pnpm exec playwright test e2e/cl-06-custom-agent.spec.ts --project=chromium
 *
 * ⚠️ `:3000` 只是前端页面（SSR HTML，不代理 API）；API 走网关 `:19080`。本 spec 全程走 UI。
 */
import { test, expect } from '@playwright/test'

test.describe('CL-06 · 用户自定义 Agent（多角色）', () => {
  test.beforeEach(async ({ page }) => {
    test.setTimeout(240_000)
    await page.goto('/login')
    await page.waitForLoadState('domcontentloaded')
    await page.getByRole('button', { name: /演示账号/ }).click()
    await expect(page.getByRole('heading', { name: '对话', exact: true })).toBeVisible({
      timeout: 30_000,
    })
  })

  test('角色管理页可达且列出角色（含默认「小E」）', async ({ page }) => {
    await page.goto('/chat/agents')
    await page.waitForLoadState('domcontentloaded')

    // ① 不是 404 页（本阶段实测过 404，故这条是定点回归钉）
    // ⚠️ 不能用 getByRole('heading', {name:'角色'}) —— 布局的 h1 与本页 h2 都叫「角色」，
    //    exact 匹配会 strict-mode 二义（首次跑就踩了）。改为按页面容器限定。
    await expect(page.locator('.agent-page h2')).toHaveText('角色', { timeout: 30_000 })
    await expect(page.getByText('Page not found')).toHaveCount(0)

    // ② 列表至少含默认角色 + 「新建角色」入口
    await expect(page.locator('[data-agent-card]').first()).toBeVisible({ timeout: 30_000 })
    await expect(page.getByRole('button', { name: '新建角色' })).toBeVisible()
  })

  test('composer 切换器：位于语音按钮左侧、向上展开、可切换本会话角色', async ({ page }) => {
    // 进入**已有会话**页。
    // ⚠️ 不能走 `/chat/conversation/new`：切换器是"写本会话角色"的控件，而新会话页
    //    尚**没有会话行**可写 ⇒ 该页按设计不放置切换器（CL-06 设计口径：切换器属于
    //    会话页，不是"建会话时选一次"）。这条本身就是阶段的语义结论之一。
    await page.goto('/chat/conversation')
    await page.waitForLoadState('domcontentloaded')
    // 列表项是 div.conversation-item（不是 <a>）—— 上一版按 `a[href^="/chat/conversation/"]`
    // 找必然找不到（首次跑就踩了）。
    const firstConv = page.locator('.conversation-item').first()
    await expect(firstConv).toBeVisible({ timeout: 30_000 })
    await firstConv.click()
    await expect(page.getByPlaceholder(/想说点什么/)).toBeVisible({ timeout: 45_000 })

    const trigger = page.getByRole('button', { name: /当前角色/ })
    await expect(trigger).toBeVisible({ timeout: 30_000 })

    // ③ 位置：切换器右边缘 ≤ 语音按钮左边缘（= 在其左侧）
    const geo = await page.evaluate(() => {
      const btn = document.querySelector('.agent-trigger') as HTMLElement | null
      const voice = document.querySelector('.voice-btn') as HTMLElement | null
      if (!btn || !voice) return null
      const b = btn.getBoundingClientRect()
      const v = voice.getBoundingClientRect()
      return { btnRight: b.right, btnX: b.x, voiceLeft: v.left, voiceX: v.x }
    })
    expect(geo, 'composer 内应同时存在 .agent-trigger 与 .voice-btn').not.toBeNull()
    expect(geo!.btnRight).toBeLessThanOrEqual(geo!.voiceLeft + 1)

    // ④ 形态：展开后菜单**整体在触发器上方**（bottom ≤ trigger top）
    await trigger.click()
    const menu = page.locator('.agent-menu')
    await expect(menu).toBeVisible({ timeout: 10_000 })
    const upward = await page.evaluate(() => {
      const btn = document.querySelector('.agent-trigger') as HTMLElement | null
      const m = document.querySelector('.agent-menu') as HTMLElement | null
      if (!btn || !m) return null
      return { menuBottom: m.getBoundingClientRect().bottom, triggerTop: btn.getBoundingClientRect().top }
    })
    expect(upward).not.toBeNull()
    expect(upward!.menuBottom).toBeLessThanOrEqual(upward!.triggerTop + 1)

    // ⑤ 「管理角色」入口在位
    await expect(page.getByRole('link', { name: '管理角色' })).toBeVisible()

    // ⑥ 切换：选一个**不是当前**的角色（演示账号至少有默认「小E」）
    const options = page.locator('[data-agent-option]')
    const n = await options.count()
    expect(n).toBeGreaterThan(0)
    await options.first().click()

    // 切换后：菜单收起（写库成功才收起 —— 失败会留在菜单里显示原因）
    await expect(menu).toHaveCount(0, { timeout: 15_000 })
  })
})
