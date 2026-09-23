import { test, expect } from '@playwright/test'

/**
 * E2E-F-124 回归钉：/chat/conversation/:id 路由必须加载并渲染已有消息
 *
 * 触发场景（用户 2026-09-22 第二轮复测发现）：
 *   - 后端 GET /api/v1/conversations/280/messages 返 200 + 2 条
 *   - 前端 /280 路由渲染后 .dialog/<article> 0 个
 *   - 与 /new 页（同样 cookie 状态）能正常渲染对比，差异只在路由参数
 *
 * 推测根因（待本测试验证）：messageStore 在 /280 路由首次进入时未触发 loadMoreMessages，
 * 或 onMounted 内的 currentSessionId 比较有 race。
 *
 * 本测试（GREEN 前必 RED）：
 *   - API 创建会话 + 2 条消息
 *   - 直接 goto /chat/conversation/:id（不走 /new 流）
 *   - 断言 .dialog 元素 >= 2（消息真实渲染）
 */

const API_BASE = 'http://localhost:19080'
const DEMO = { username: 'echo', password: 'echo123' }

async function loginViaAPI(page: import('@playwright/test').Page): Promise<string> {
  const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const body = await resp.json()
  const token = body?.data?.accessToken
  expect(token, 'login response must contain accessToken').toBeTruthy()
  await page.context().addCookies([
    { name: 'access_token', value: token, url: 'http://localhost:3000' },
  ])
  return token
}

async function waitForHydration(page: import('@playwright/test').Page) {
  await page.waitForLoadState('domcontentloaded')
  await page.waitForTimeout(3000)
}

test.describe('E2E-F-124 /chat/conversation/:id 直接路由必须渲染已有消息', () => {
  test('通过 sidebar 之外的路径进入 /chat/conversation/:id, 历史消息必须渲染', async ({
    page,
  }) => {
    const token = await loginViaAPI(page)

    // 1) 准备数据：API 创建 conv + 2 消息
    const createConv = await page.request.post(`${API_BASE}/api/v1/conversations`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {},
    })
    expect(createConv.ok(), 'create conv API must succeed').toBe(true)
    const convBody = await createConv.json()
    const convId = convBody?.data?.id
    expect(convId, 'create conv response must contain id').toBeTruthy()

    // 发送 2 条不同 contentType 的消息
    const uuid1 = crypto.randomUUID()
    const uuid2 = crypto.randomUUID()
    await page.request.post(`${API_BASE}/api/v1/conversations/${convId}/messages`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { content: 'hello world', contentType: 'text', clientMsgId: uuid1 },
    })
    await page.request.post(`${API_BASE}/api/v1/conversations/${convId}/messages`, {
      headers: { Authorization: `Bearer ${token}` },
      data: {
        content: 'http://localhost:9000/avatars/uploads/t19c.txt',
        contentType: 'file',
        fileName: 't19c.txt',
        clientMsgId: uuid2,
      },
    })

    // 2) 直接 goto /chat/conversation/:id（不走 /new 流、不走 sidebar 切换）
    //    这是 F-124 的关键场景：用户从外部进入或刷新页面
    await page.goto(`/chat/conversation/${convId}`)
    await waitForHydration(page)

    // 3) 断言：.dialog 元素必须 >= 2（消息已渲染）
    //    RED 阶段此断言会失败（0 articles）—— 锁定 bug
    const dialogs = page.locator('article.dialog')
    await expect(dialogs, `F-124: /${convId} 路由必须渲染 >= 2 条消息 article`).toHaveCount(
      2,
      { timeout: 10_000 },
    )

    // 4) 内容断言：第一条 text 消息内容可见
    await expect(page.getByText('hello world').first()).toBeVisible({ timeout: 5_000 })
    // 第二条 file 消息：文件名 t19c.txt 应可见
    await expect(page.getByText('t19c.txt').first()).toBeVisible({ timeout: 5_000 })
  })
})