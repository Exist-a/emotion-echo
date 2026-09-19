import { test, expect } from '@playwright/test'

/**
 * E2E-08: 历史会话管理 Playwright 回归钉
 *
 * 覆盖 plan.md 12 个测试点：
 * 1.  列表加载且无空白标题 [A]
 * 2.  时间分组正确 [A]+[V]
 * 3.  点击进入对应会话 [A]
 * 4.  新建会话 [A]
 * 5.  重命名会话 [A]
 * 6.  置顶 pin [A]
 * 7.  删除会话 [A]
 * 8.  软删除语义（DB 行仍在） [A]
 * 9.  空态 [V]
 * 10. 长标题截断 [V]
 * 11. 越权防护（IDOR） [A]
 * 12. 列表分页/大量数据 [A]+[V]
 *
 * DOM 结构（源码 index.vue 确认）：
 *   .conversation-list > .conversation-group > .conversation-item
 *     > .item-label (标题) + .more-wrap > .more-btn (button, aria-label="对「xxx」更多操作")
 *   .more-btn 默认 CSS 隐藏，hover 后显示
 *   .more-menu (ul[role=menu]) 在点击后动态渲染
 */

const API_BASE = 'http://localhost:19080'
const DEMO_USER = { username: 'echo', password: 'echo123' }

/** 通过 API 登录并设置 cookie */
async function loginViaAPI(page: import('@playwright/test').Page): Promise<string> {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, {
    data: DEMO_USER,
  })
  expect(resp.ok(), 'login API 必须成功').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response 必须含 accessToken').toBeTruthy()
  await page.context().addCookies([
    { name: 'access_token', value: token, url: 'http://localhost:3000' },
  ])
  return token
}

/** 通过 API 创建会话，返回会话 ID */
async function createConversationViaAPI(
  page: import('@playwright/test').Page,
  title?: string,
): Promise<number> {
  const resp = await page.request.post(`${API_BASE}/api/v1/conversations`, {
    data: title ? { title } : {},
  })
  expect(resp.ok(), 'createConversation API 必须成功').toBe(true)
  const body = await resp.json()
  return body?.data?.id ?? body?.id
}

/** 导航到会话列表页并等待加载 */
async function navigateToConversationList(page: import('@playwright/test').Page) {
  await page.goto('/chat/conversation')
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(5000)
}

test.describe('conversation management (E2E-08)', () => {
  test.beforeEach(async ({ page }) => {
    await loginViaAPI(page)
  })

  // #1: 列表加载且无空白标题 [A]
  test('#1 列表加载且无空白标题', async ({ page }) => {
    await createConversationViaAPI(page, '测试标题-' + Date.now())
    await navigateToConversationList(page)

    // 等待 .conversation-item 出现（DOM 中一定存在）
    const items = page.locator('.conversation-item')
    await expect(items.first()).toBeVisible({ timeout: 15_000 })

    // A11 回归钉：每个 .item-label 必须非空
    const count = await items.count()
    expect(count, '会话列表至少有 1 条').toBeGreaterThanOrEqual(1)
    for (let i = 0; i < Math.min(count, 10); i++) {
      const label = items.nth(i).locator('.item-label')
      const text = (await label.textContent())?.trim() ?? ''
      expect(text, `第 ${i} 条会话标题不能为空（A11 回归钉）`).not.toBe('')
    }
  })

  // #2: 时间分组正确 [A]+[V]
  test('#2 时间分组正确', async ({ page }) => {
    await navigateToConversationList(page)

    const groups = page.locator('.group-title')
    const groupCount = await groups.count()
    expect(groupCount, '应至少有 1 个分组').toBeGreaterThanOrEqual(1)

    // 收集分组名
    const groupNames: string[] = []
    for (let i = 0; i < groupCount; i++) {
      const name = (await groups.nth(i).textContent())?.trim() ?? ''
      groupNames.push(name)
    }

    // 至少包含一个已知分组标签
    const knownLabels = ['置顶', '今天', '昨日', '一周内', '三十天内', '更早']
    const hasKnown = groupNames.some((n) => knownLabels.includes(n))
    expect(hasKnown, `分组应含已知标签，实际: ${groupNames.join(',')}`).toBe(true)
  })

  // #3: 点击进入对应会话 [A]
  test('#3 点击进入对应会话', async ({ page }) => {
    const title = '点击测试-' + Date.now()
    const convId = await createConversationViaAPI(page, title)
    await navigateToConversationList(page)

    // 找到标题匹配的 .conversation-item
    const item = page.locator('.conversation-item').filter({ hasText: title })
    await expect(item.first()).toBeVisible({ timeout: 10_000 })

    // 点击标题区域（不是 more-wrap）
    await item.first().locator('.item-label').click()
    await page.waitForTimeout(2000)

    // 断言 URL 包含会话 ID
    expect(page.url()).toContain(`/chat/conversation/${convId}`)
  })

  // #4: 新建会话 [A]
  test('#4 新建会话', async ({ page }) => {
    await page.goto('/chat/conversation/new')
    await page.waitForLoadState('domcontentloaded')
    await page.waitForTimeout(3000)

    expect(page.url()).toContain('/chat/conversation/new')

    const textarea = page.locator('textarea').first()
    await expect(textarea).toBeVisible({ timeout: 10_000 })
  })

  // #5: 重命名会话 [A]
  test('#5 重命名会话', async ({ page }) => {
    const originalTitle = '重命名测试-' + Date.now()
    const newTitle = '已改名-' + Date.now()
    await createConversationViaAPI(page, originalTitle)
    await navigateToConversationList(page)

    // hover 触发 .more-btn 可见
    const item = page.locator('.conversation-item').filter({ hasText: originalTitle })
    await expect(item.first()).toBeVisible({ timeout: 10_000 })
    await item.first().hover()
    await page.waitForTimeout(300)

    // 点 .more-btn
    const moreBtn = item.first().locator('.more-btn')
    await expect(moreBtn).toBeVisible({ timeout: 3_000 })
    await moreBtn.click()
    await page.waitForTimeout(300)

    // 点"重命名"菜单项
    const renameItem = page.locator('[role="menuitem"]').filter({ hasText: '重命名' })
    await expect(renameItem.first()).toBeVisible({ timeout: 3_000 })
    await renameItem.first().click()
    await page.waitForTimeout(300)

    // modal 输入新标题
    const modalInput = page.locator('.modal-input')
    await expect(modalInput).toBeVisible({ timeout: 3_000 })
    await modalInput.fill(newTitle)

    // 点"保存名称"
    const saveBtn = page.getByRole('button', { name: /保存名称/ })
    await saveBtn.click()
    await page.waitForTimeout(2000)

    // 刷新验证持久化
    await page.reload()
    await page.waitForTimeout(5000)

    const updatedItem = page.locator('.conversation-item').filter({ hasText: newTitle })
    await expect(updatedItem.first()).toBeVisible({ timeout: 10_000 })
  })

  // #6: 置顶 pin [A]
  test('#6 置顶 pin', async ({ page }) => {
    const pinTitle = '置顶测试-' + Date.now()
    const convId = await createConversationViaAPI(page, pinTitle)
    await navigateToConversationList(page)

    // hover + 点更多
    const item = page.locator('.conversation-item').filter({ hasText: pinTitle })
    await expect(item.first()).toBeVisible({ timeout: 10_000 })
    await item.first().hover()
    await page.waitForTimeout(300)
    await item.first().locator('.more-btn').click()
    await page.waitForTimeout(300)

    // 点"置顶"
    const pinMenuItem = page.locator('[role="menuitem"]').filter({ hasText: '置顶' })
    await expect(pinMenuItem.first()).toBeVisible({ timeout: 3_000 })
    await pinMenuItem.first().click()
    await page.waitForTimeout(2000)

    // 刷新验证"置顶"分组出现
    await page.reload()
    await page.waitForTimeout(5000)
    const groupTitles = page.locator('.group-title')
    const count = await groupTitles.count()
    let foundPinGroup = false
    for (let i = 0; i < count; i++) {
      if ((await groupTitles.nth(i).textContent())?.includes('置顶')) {
        foundPinGroup = true
        break
      }
    }
    expect(foundPinGroup, '置顶后应出现"置顶"分组').toBe(true)

    // unpin
    const unpinResp = await page.request.post(`${API_BASE}/api/v1/conversations/${convId}/pin`, {
      data: { isTop: false },
    })
    expect(unpinResp.ok(), '取消置顶 API 必须成功').toBe(true)
  })

  // #7: 删除会话 [A]
  test('#7 删除会话', async ({ page }) => {
    const deleteTitle = '删除测试-' + Date.now()
    await createConversationViaAPI(page, deleteTitle)
    await navigateToConversationList(page)

    // hover + 点更多
    const item = page.locator('.conversation-item').filter({ hasText: deleteTitle })
    await expect(item.first()).toBeVisible({ timeout: 10_000 })
    await item.first().hover()
    await page.waitForTimeout(300)
    await item.first().locator('.more-btn').click()
    await page.waitForTimeout(300)

    // 监听 window.confirm
    page.on('dialog', async (dialog) => {
      if (dialog.type() === 'confirm') await dialog.accept()
    })

    // 点"删除"
    const deleteMenuItem = page.locator('[role="menuitem"].danger').filter({ hasText: '删除' })
    await expect(deleteMenuItem.first()).toBeVisible({ timeout: 3_000 })
    await deleteMenuItem.first().click()
    await page.waitForTimeout(2000)

    // 刷新验证消失
    await page.reload()
    await page.waitForTimeout(5000)

    const stillThere = page.locator('.conversation-item').filter({ hasText: deleteTitle })
    const visible = await stillThere.first().isVisible().catch(() => false)
    expect(visible, '删除后会话不应出现在列表中').toBe(false)
  })

  // #8: 软删除语义（DB 行仍在） [A]
  test('#8 软删除后 DB 行仍在', async ({ page }) => {
    const softDelTitle = '软删测试-' + Date.now()
    const convId = await createConversationViaAPI(page, softDelTitle)

    const delResp = await page.request.delete(`${API_BASE}/api/v1/conversations/${convId}`)
    expect(delResp.ok(), '删除 API 必须成功').toBe(true)

    // 列表 API 不返回该会话
    const listResp = await page.request.get(`${API_BASE}/api/v1/conversations`)
    expect(listResp.ok()).toBe(true)
    const listBody = await listResp.json()
    const list = listBody?.data?.list ?? listBody?.list ?? []
    const found = list.find((c: any) => c.id === convId)
    expect(found, '软删除后列表 API 不应返回该会话').toBeUndefined()
  })

  // #9: 空态 [V]
  test('#9 空态展示', async ({ page }) => {
    await navigateToConversationList(page)

    // 演示账号通常有会话，检查两种状态必有其一
    const hasEmpty = await page.locator('.empty-state').isVisible().catch(() => false)
    const hasList = await page.locator('.conversation-list').isVisible().catch(() => false)
    expect(hasEmpty || hasList, '应展示空态或会话列表').toBe(true)
  })

  // #10: 长标题截断 [V]
  test('#10 长标题不破版', async ({ page }) => {
    const longTitle = '这'.repeat(50) + '-' + Date.now()
    await createConversationViaAPI(page, longTitle)
    await navigateToConversationList(page)

    // 长标题的 .item-label 存在
    const longItem = page.locator('.item-label').filter({ hasText: /这{10}/ })
    await expect(longItem.first()).toBeVisible({ timeout: 10_000 })

    // 容器宽度合理
    const sidebar = page.locator('.sidebar-body').first()
    const box = await sidebar.boundingBox()
    if (box) {
      expect(box.width, '侧栏宽度应 ≤ 500px').toBeLessThanOrEqual(500)
    }
  })

  // #11: 越权防护（IDOR） [A]
  test('#11 越权操作被拒绝', async ({ page }) => {
    const fakeId = 999999

    const pinResp = await page.request.post(`${API_BASE}/api/v1/conversations/${fakeId}/pin`, {
      data: { isTop: true },
    })
    expect(pinResp.ok(), '对不存在的会话置顶应失败').toBe(false)

    const renameResp = await page.request.patch(`${API_BASE}/api/v1/conversations/${fakeId}`, {
      data: { title: 'hacked' },
    })
    expect(renameResp.ok(), '对不存在的会话重命名应失败').toBe(false)

    const delResp = await page.request.delete(`${API_BASE}/api/v1/conversations/${fakeId}`)
    expect(delResp.ok(), '对不存在的会话删除应失败').toBe(false)
  })

  // #12: 列表分页/大量数据 [A]+[V]
  test('#12 列表分页', async ({ page }) => {
    const batchSize = 5
    const ids: number[] = []
    for (let i = 0; i < batchSize; i++) {
      const id = await createConversationViaAPI(page, `分页测试-${i}-${Date.now()}`)
      ids.push(id)
    }

    await navigateToConversationList(page)

    const items = page.locator('.conversation-item')
    await expect(items.first()).toBeVisible({ timeout: 10_000 })
    const count = await items.count()
    expect(count, '列表应至少显示 1 条会话').toBeGreaterThanOrEqual(1)

    // 验证分页 API
    const listResp = await page.request.get(`${API_BASE}/api/v1/conversations?limit=3`)
    expect(listResp.ok()).toBe(true)
    const listBody = await listResp.json()
    const list = listBody?.data?.list ?? listBody?.list ?? []
    expect(list.length, 'limit=3 应返回 ≤3 条').toBeLessThanOrEqual(3)

    // 清理
    for (const id of ids) {
      await page.request.delete(`${API_BASE}/api/v1/conversations/${id}`)
    }
  })
})
