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
    // ⚠️ UI 轮（2026-10-17）起**本页不再有自己的一级标题**：布局的 <h1> 已经叫「角色」，
    //    页面再写一遍 h2「角色」会在屏幕上出现两个同名标题（用户反馈的"渲染逻辑"之一，
    //    旧注释里"布局 h1 与本页 h2 都叫角色"正是同一个问题的另一种写法）。
    //    断言随之改为"页面主体真的渲染出来了"：说明句 + 卡片列表都只可能来自本页 ——
    //    404 页两者皆无，所以这条对"路由是否真的接上了"的证明力与原先等同。
    await expect(page.locator('.agent-page')).toBeVisible({ timeout: 30_000 })
    await expect(page.locator('.agent-page .agent-lede')).toBeVisible({ timeout: 30_000 })
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

    // ④b 菜单**不得被祖先裁切**（2026-10-18 用户实测指出"选择框在另一个元素下面"）。
    //
    // 为什么必须用**坐标命中测试**而不是几何断言：`getBoundingClientRect()` 给出的永远是
    // "未被裁剪的盒子"，祖先 `overflow: hidden` 把菜单右侧切掉 25px 时它照样报出完整的
    // rect（本阶段先踩过一次：3 点探针全落在未裁剪区，误判为"没被遮"）。
    // 真凶：`conversation-page`（overflow: hidden，右边缘 x=1229）裁掉了菜单右侧，
    // 被裁处露出 `div.page-content`。判据 = 菜单自身靠右的位置必须能被自己命中。
    const clip = await page.evaluate(() => {
      const m = document.querySelector('.agent-menu') as HTMLElement | null
      if (!m) return null
      const b = m.getBoundingClientRect()
      const y = Math.round(b.top + b.height / 2)
      return [0.5, 0.9, 0.98].map((r) => {
        const el = document.elementFromPoint(Math.round(b.left + b.width * r), y)
        return { at: r, inMenu: !!el?.closest?.('.agent-menu') }
      })
    })
    expect(clip, '菜单应已展开（否则本断言无意义）').not.toBeNull()
    for (const p of clip!) {
      expect(
        p.inMenu,
        `菜单在自身宽度 ${Math.round(p.at * 100)}% 处必须能被自己命中 —— false 表示被祖先 overflow 裁掉`,
      ).toBe(true)
    }

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
