import { test, expect, type Page } from '@playwright/test'
import { mkdirSync } from 'node:fs'
import { join } from 'node:path'

/**
 * CL-05 消息链路闭环 · 人可见回归钉（Phase 7 / Phase 8）
 *
 * 覆盖：
 *  - T-20① 上滚翻到更早页（游标分页真生效）
 *  - T-20② 刷新后消息仍在（缓存接线 + 服务端回读）
 *  - T-20③ 断网发送 → 恢复 → 自动补发（幂等，不重复入库；含 T-19）
 *  - T-21  发送后聊天区**自动滚到底**（账本 CL-F-10 回归钉）
 *  - T-22  建会话→发送→刷新 ⇒ 会话列表**只 +1**（账本 CL-F-11 回归钉）
 *
 * 前置：dev 栈（`--env-file .env.local --profile dev`）+ 前端 :3000 + demo 账号。
 * 基址：业务 HTTP 走网关 :19080 + Bearer（AGENTS §2.4）；`page.request` 不经
 *      `page.route` ⇒ 拦网要覆盖两条路径（memory 教训）。
 *
 * ⚠️ 等待策略：**不用固定 sleep 等 hydration**。Nuxt dev server 首次访问某路由要
 * 现场编译（实测 >2.5s），固定 sleep 会在冷启动时拿到**空页面**并误判成功能坏了
 * （本轮首跑 4 个 case 就栽在这上面）。一律改成等内容就绪。
 */

const SCREENSHOT_DIR = join(process.cwd(), 'screenshots')
mkdirSync(SCREENSHOT_DIR, { recursive: true })

const API_BASE = 'http://localhost:19080'
const DEMO = { username: 'echo', password: 'echo123' }
const READY_TIMEOUT = 45_000

/**
 * 登录一次、**全文件复用 token**。
 *
 * 为什么不在每个 test 里各登一次：网关有 limit-count（60 次/60s，key=remote_addr，
 * E2E-31 F-214）。一个文件 5 个 test 各登一次 + 各自的探测请求，很容易把**自己的
 * 限流闸门**打成 429 —— 那是**测试自己造的假红**（本轮实测：429 而非功能坏）。
 * cookie 仍按 test 注入（每个 test 有独立 browser context）。
 */
let cachedToken = ''
async function loginViaAPI(page: Page): Promise<string> {
  if (!cachedToken) {
    const resp = await page.request.post(`${API_BASE}/api/v1/auth/login`, { data: DEMO })
    expect(resp.ok(), `login API must succeed (got ${resp.status()})`).toBe(true)
    cachedToken = ((await resp.json())?.data?.accessToken ?? '') as string
    expect(cachedToken, 'login must return accessToken').toBeTruthy()
  }
  await page.context().addCookies([{ name: 'access_token', value: cachedToken, url: 'http://localhost:3000' }])
  return cachedToken
}

/** 带 429 重试的 GET（网关限流是**环境属性**，不该让回归钉假红） */
async function apiGet(page: Page, url: string, token: string) {
  for (let i = 0; i < 4; i++) {
    const r = await page.request.get(url, { headers: { Authorization: `Bearer ${token}` } })
    if (r.status() !== 429) return r
    await page.waitForTimeout(3000)
  }
  return page.request.get(url, { headers: { Authorization: `Bearer ${token}` } })
}

/** 进会话详情页并**等内容就绪**（dev 冷启动要现场编译） */
async function gotoConversation(page: Page, id: string) {
  await page.goto(`/chat/conversation/${id}`)
  await page.waitForLoadState('domcontentloaded')
  await expect(page.locator('main.chat-main')).toBeVisible({ timeout: READY_TIMEOUT })
  // 消息区出现任意一条即视为渲染完成（该会话本身有历史消息）
  await expect(page.locator('.dialog-list .dialog').first()).toBeVisible({ timeout: READY_TIMEOUT })
  await expect(dialogComposer(page)).toBeVisible({ timeout: READY_TIMEOUT })
}

/** 进 /new 页并等 composer 就绪 */
async function gotoNew(page: Page) {
  await page.goto('/chat/conversation/new')
  await page.waitForLoadState('domcontentloaded')
  await expect(dialogComposer(page)).toBeVisible({ timeout: READY_TIMEOUT })
}

/** 聊天输入框 —— 详情页是 `textarea.composer-input`，/new 页是 `textarea.sender-input` */
function dialogComposer(page: Page) {
  return page.locator('textarea.composer-input, textarea.sender-input').first()
}

/**
 * 发消息：优先**点发送按钮**（最接近真实用户操作），点不到再退回回车。
 * 为什么不用 `fill` + 直接断言"发了" —— textarea 上挂了
 * `@keydown.enter.exact.prevent`，用 Enter 依赖焦点/修饰键语义，容易假失败；
 * 按钮路径更稳。
 */
async function sendMessage(page: Page, text: string) {
  const composer = dialogComposer(page)
  await composer.click()
  await composer.fill(text)
  await expect(composer).toHaveValue(text)
  const sendBtn = page
    .locator('button[aria-label="发送消息"], button[aria-label="发送"], button.send-btn')
    .first()
  if (await sendBtn.count()) {
    await expect(sendBtn).toBeEnabled({ timeout: 10_000 })
    await sendBtn.click()
    return
  }
  await page.keyboard.press('Enter')
}

/** 用**真实滚轮**滚到顶（比直接设 scrollTop 更接近用户，且一定会触发 scroll 事件） */
async function wheelToTop(page: Page) {
  const main = page.locator('main.chat-main')
  const box = await main.boundingBox()
  if (box) {
    await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
    await page.mouse.wheel(0, -60_000)
  }
  await main.evaluate((el) => { el.scrollTop = 0 })
}

async function listConversations(page: Page, token: string) {
  // limit 必须够大：用 10 会把结果**截断在 10**，于是"前后条数差"永远看不出来
  // （本轮 T-22 首跑就栽在这 —— 会话其实建出来了，只是被 limit 挡住）
  const r = await apiGet(page, `${API_BASE}/api/v1/conversations?limit=100`, token)
  expect(r.ok(), `conversations list must be ok (got ${r.status()})`).toBe(true)
  return ((await r.json())?.data?.list ?? []) as Array<{ id: string }>
}

async function conversationIds(page: Page, token: string): Promise<Set<string>> {
  return new Set((await listConversations(page, token)).map((c) => String(c.id)))
}

async function messageIds(page: Page, token: string, convId: string) {
  const r = await apiGet(page, `${API_BASE}/api/v1/conversations/${convId}/messages?limit=200`, token)
  if (!r.ok()) return [] as string[]
  return (((await r.json())?.data?.list ?? []) as Array<{ id: string }>).map((m) => String(m.id))
}

/** 会话列表里"新增的 id"（不用条数 —— limit 会封顶，账号有 ~500 个会话，长度恒等于 limit） */
/**
 * 挑一个"首屏取不全"的会话（消息数 > 前端首屏 20）—— 否则首页已取全、
 * "上滚加载更早"无从观察（T-20① 首跑的假失败）。最多探 5 个、命中即停：
 * 网关 limit-count 60/60s（F-214），探太多会被自己的限流打成 429。
 */
async function paginatableConversation(page: Page, token: string, min = 21) {
  const convs = await listConversations(page, token)
  let best = { id: '', count: -1 }
  for (const c of convs.slice(0, 5)) {
    const n = (await messageIds(page, token, c.id)).length
    if (n > best.count) best = { id: c.id, count: n }
    if (best.count >= min) break
  }
  return best
}

async function addedConversationIds(page: Page, token: string, before: Set<string>) {
  const after = await conversationIds(page, token)
  return [...after].filter((id) => !before.has(id))
}

test.describe('CL-05 消息链路人可见钉', () => {
  test.setTimeout(180_000)

  test('T-20① 上滚到顶 → 自动加载更早一页（消息条数增加）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const target = await paginatableConversation(page, token)
    test.skip(!target.id || target.count <= 20, `无 >20 条消息的会话 ⇒ 首页已取全、上翻无从观察（best=${JSON.stringify(target)}）`)

    await gotoConversation(page, target.id)
    const items = page.locator('.dialog-list .dialog')
    const before = await items.count()
    expect(before).toBeGreaterThan(0)

    // 滚到顶触发 onMessagesScroll → loadMoreMessages（首页默认只取最新一页）
    await wheelToTop(page)
    await expect
      .poll(async () => items.count(), { timeout: 20_000, message: '上滚后应加载到更早的消息' })
      .toBeGreaterThan(before)

    await page.screenshot({ path: join(SCREENSHOT_DIR, 'cl05-t20-1-load-earlier.png') })
  })

  test('T-20② 刷新后消息仍在（不丢）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const target = await paginatableConversation(page, token)
    test.skip(!target.id, '数据前置不满足：无可用会话')

    await gotoConversation(page, target.id)
    const items = page.locator('.dialog-list .dialog')
    const first = await items.count()
    expect(first).toBeGreaterThan(0)

    await page.reload()
    await page.waitForLoadState('domcontentloaded')
    await expect(items.first()).toBeVisible({ timeout: READY_TIMEOUT })
    const second = await items.count()
    expect(second, '刷新后消息条数不得减少（缓存 + 服务端回读）').toBeGreaterThanOrEqual(first)
  })

  test('T-21 发送后聊天区**自动滚到底**（CL-F-10 回归钉）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const target = await paginatableConversation(page, token)
    test.skip(!target.id || target.count < 5, '数据前置不满足：需要一个会溢出的会话')

    await gotoConversation(page, target.id)
    const main = page.locator('main.chat-main')

    // 先滚到顶，制造"用户不在底部"的初始态
    await main.evaluate((el) => { el.scrollTop = 0 })
    const overflows = await main.evaluate((el) => el.scrollHeight > el.clientHeight + 120)
    expect(overflows, '该会话消息应足以让聊天区溢出滚动（否则本钉无效）').toBe(true)

    await sendMessage(page, `CL05滚底钉-${Date.now()}`)

    // 关键断言：**流式增长过程中**视口就应跟随到底（旧实现只看条数 ⇒ 这里必失败）
    await expect
      .poll(async () => main.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight), {
        timeout: 30_000,
        message: '发送后视口必须被带到末尾（CL-F-10 根因：只 watch length 会漏流式增长）',
      })
      .toBeLessThanOrEqual(160)

    await page.screenshot({ path: join(SCREENSHOT_DIR, 'cl05-t21-autoscroll.png') })
  })

  test('T-22 建会话→发送→刷新 ⇒ 会话列表只 +1（CL-F-11 回归钉）', async ({ page }) => {
    const token = await loginViaAPI(page)

    // 阻断 AI 流（只数"建了几个会话"，不等 LLM）；建会话请求必须放行
    await page.route('**/api/v1/ai/stream', (route) => route.abort())

    // ⚠️ 用"新增 id 集合"而不是"条数差"：列表 limit 会封顶（账号 ~500 会话）
    // ⇒ 长度差恒为 0，是本轮 T-22 假失败的根因。
    const before = await conversationIds(page, token)
    await gotoNew(page)
    await sendMessage(page, `CL05双对话钉-${Date.now()}`)

    await expect
      .poll(async () => (await addedConversationIds(page, token, before)).length, {
        timeout: 30_000,
        message: '一次发送应恰好建 1 个会话',
      })
      .toBe(1)

    // 刷新（用户上报的触发动作）后再数一次：不得再多出一个
    await page.reload()
    await page.waitForLoadState('domcontentloaded')
    await expect(dialogComposer(page)).toBeVisible({ timeout: READY_TIMEOUT })
    expect(
      (await addedConversationIds(page, token, before)).length,
      '刷新后会话列表只应 +1（CL-F-11：不得再冒出一个）',
    ).toBe(1)
  })

  test('T-20③ 断网发送 → 恢复 → 自动补发且**不重复入库**（含 T-19 幂等）', async ({ page }) => {
    const token = await loginViaAPI(page)
    const target = await paginatableConversation(page, token)
    test.skip(!target.id, '数据前置不满足：无可用会话')

    let blocked = 0
    const sendPattern = `**/api/v1/conversations/${target.id}/messages`
    await page.route(sendPattern, (route) => {
      if (route.request().method() === 'POST') {
        blocked += 1
        return route.abort()
      }
      return route.continue()
    })

    await gotoConversation(page, target.id)
    const marker = `CL05幂等钉-${Date.now()}`
    await sendMessage(page, marker)
    await expect.poll(() => blocked, { timeout: 15_000, message: '发送请求应被拦下（模拟断网）' }).toBeGreaterThan(0)

    // "恢复网络" + 触发 online（plugins/init.ts 的监听重扫补发）
    await page.unroute(sendPattern)
    await page.evaluate(() => window.dispatchEvent(new Event('online')))

    await expect
      .poll(
        async () => {
          const r = await apiGet(page, `${API_BASE}/api/v1/conversations/${target.id}/messages?limit=200`, token)
          const list = ((await r.json())?.data?.list ?? []) as Array<{ content?: string }>
          return list.filter((m) => (m.content ?? '').includes(marker)).length
        },
        { timeout: 30_000, intervals: [1500, 1500, 2000, 3000], message: '断网期间的消息应在恢复后自动补发入库' },
      )
      .toBe(1)

    // 再等一拍，确认**没有**第二次入库（幂等：复用 client_msg_id 唯一索引）
    await page.waitForTimeout(4000)
    const r = await apiGet(page, `${API_BASE}/api/v1/conversations/${target.id}/messages?limit=200`, token)
    const list = ((await r.json())?.data?.list ?? []) as Array<{ content?: string }>
    expect(
      list.filter((m) => (m.content ?? '').includes(marker)).length,
      '同一 clientMsgId 补发多次也只能入库 1 行（T-19 幂等）',
    ).toBe(1)
  })
})
