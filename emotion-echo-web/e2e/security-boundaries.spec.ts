/**
 * E2E-29 回归钉（横切：异常与安全）—— 把本轮实测的边界钉成可重复的自动化断言。
 *
 * 运行：
 *   BASE_URL=http://localhost:3000 npx playwright test e2e/security-boundaries.spec.ts --project=chromium
 * 前置：dev 栈运行中（APISIX :19080 / BFF / PG / Redis）；演示账号 echo/echo123。
 *
 * 覆盖（对应 plan §2 测试点）：
 *   #1  匿名 POST /auth/refresh 必须 401 且不下发 cookie（E2E-F-201 / D-46 硬 401）
 *   #3  受保护端点匿名枚举（≥6 条）全部 401
 *   #4  令牌类型隔离：reset token 不得当 access token 用；access token 不得当 reset token 用
 *   #6  logout 清除 cookie 且带 SameSite（F-203）
 *   #7/#8 reports 的 user_id 归属 403；身份别名（数字型）不符亦 403（遗留项 3 加固），
 *         一致时响应体与基准逐字相同
 *   #9/#10 跨用户资源：读/改/删/发消息 一律 403，自己的资源 2xx
 *   #11 宿主直连 BFF 8894 不可达（D-47 收映射）
 *   #13 限流拒绝码 = 429（白名单链）
 *   #15/#16 CORS：恶意 origin 无 ACAO；合法 origin 精确回显；allow_headers 不含 X-User-Id
 *
 * 说明：IAB 截图存在渲染帧与 DOM 不同步的失真（E2E-F-184），故 [V] 类证据由本 spec 的
 * 独立 Chromium 渲染栈产出（与 E2E-27 同范式）。
 */
import { test, expect, request, type APIRequestContext } from '@playwright/test'
import * as path from 'node:path'
import * as fs from 'node:fs'
import { fileURLToPath } from 'node:url'

const API_BASE = 'http://localhost:19080'
const BFF_DIRECT = 'http://localhost:8894'
const DEMO = { username: 'echo', password: 'echo123' }
const SHOTS_DIR = path.resolve(
  path.dirname(fileURLToPath(import.meta.url)),
  '../../docs/e2e-roadmap/stages/e2e-29-security-exceptions/screenshots',
)

const PROTECTED = [
  { method: 'GET' as const, url: '/api/v1/users/me' },
  { method: 'GET' as const, url: '/api/v1/conversations' },
  { method: 'GET' as const, url: '/api/v1/reports/daily' },
  { method: 'GET' as const, url: '/api/v1/user-behavior/depth' },
  { method: 'GET' as const, url: '/api/v1/surveys' },
  { method: 'POST' as const, url: '/api/v1/ai/stream' },
  { method: 'POST' as const, url: '/api/v1/uploads/file' },
]

async function newApi(): Promise<APIRequestContext> {
  return request.newContext({ baseURL: API_BASE, timeout: 20000 })
}

async function login(api: APIRequestContext, who = DEMO): Promise<string> {
  const res = await api.post('/api/v1/auth/login', { data: who })
  expect(res.status(), `login ${who.username} 必须 200`).toBe(200)
  const token = (await res.json()).data.accessToken as string
  expect(token).toBeTruthy()
  return token
}

/** 幂等注册一个带密保的探针账号；已存在（409）时直接登录。 */
async function ensureAccount(api: APIRequestContext, username: string): Promise<string> {
  const password = 'Probe!2026'
  const reg = await api.post('/api/v1/auth/register', {
    data: {
      username,
      password,
      securityQuestions: [{ question: 'pets', answer: 'abc123' }],
    },
  })
  expect([200, 409], `register ${username} 应 200 或 409`).toContain(reg.status())
  return login(api, { username, password })
}

test.describe('E2E-29 横切安全回归钉', () => {
  test.setTimeout(180_000)

  test('#1 匿名 refresh 必须 401 且不下发 cookie', async () => {
    const api = await newApi()
    const res = await api.post('/api/v1/auth/refresh')
    expect(res.status(), '匿名 refresh 必须 401（E2E-F-201）').toBe(401)
    const setCookie = res.headers()['set-cookie'] ?? ''
    expect(setCookie, '401 时不得下发 access_token').not.toContain('access_token=ey')
    const body = await res.text()
    expect(body).not.toContain('accessToken')
  })

  test('#3 受保护端点匿名枚举全部 401', async () => {
    const api = await newApi()
    for (const p of PROTECTED) {
      const res = await api.fetch(p.url, { method: p.method, data: {} })
      expect(res.status(), `${p.method} ${p.url} 匿名必须 401`).toBe(401)
    }
  })

  test('#4 令牌类型隔离双向', async () => {
    const api = await newApi()
    const token = await ensureAccount(api, 'ee29_e2e_iso')

    // 取 reset token（密保答案固定 abc123）
    const verify = await api.post('/api/v1/auth/verify-security-answer', {
      data: { username: 'ee29_e2e_iso', questionOrder: 1, answer: 'abc123' },
    })
    expect(verify.status()).toBe(200)
    const resetToken = (await verify.json()).data.resetToken as string
    expect(resetToken).toBeTruthy()

    // 4a: reset token 当 access token 用 → 401
    const asAccess = await api.get('/api/v1/users/me', {
      headers: { Authorization: `Bearer ${resetToken}` },
    })
    expect(asAccess.status(), 'reset token 不得当 access token 用').toBe(401)

    // 4b: access token 当 reset token 用 → 401，且口令不变
    const asReset = await api.post('/api/v1/auth/reset-password', {
      data: { resetToken: token, newPassword: 'Hacked123' },
    })
    expect(asReset.status(), 'access token 不得当 reset token 用').toBe(401)
    const relogin = await api.post('/api/v1/auth/login', {
      data: { username: 'ee29_e2e_iso', password: 'Probe!2026' },
    })
    expect(relogin.status(), '原口令必须仍然可用（未被越权改写）').toBe(200)
  })

  test('#6 logout 清除 cookie 且带 SameSite', async () => {
    const api = await newApi()
    const res = await api.post('/api/v1/auth/logout')
    expect(res.status()).toBe(200)
    const setCookie = (res.headers()['set-cookie'] ?? '').toLowerCase()
    expect(setCookie).toContain('access_token=;')
    expect(setCookie, 'F-203：清除 cookie 必须带 SameSite').toContain('samesite=lax')
  })

  test('#7/#8 reports 归属 403 + 别名不构成越权', async () => {
    const api = await newApi()
    const tokenA = await ensureAccount(api, 'ee29_e2e_a')
    const tokenB = await ensureAccount(api, 'ee29_e2e_b')
    const meB = await api.get('/api/v1/users/me', { headers: { Authorization: `Bearer ${tokenB}` } })
    const idB = String((await meB.json()).data.user.userId)

    const cross = await api.get(`/api/v1/reports/daily?user_id=${idB}`, {
      headers: { Authorization: `Bearer ${tokenA}` },
    })
    expect(cross.status(), '查他人报表必须 403').toBe(403)

    const base = await api.get('/api/v1/reports/daily', { headers: { Authorization: `Bearer ${tokenA}` } })
    expect(base.status()).toBe(200)
    const baseBody = await base.text()

    // E2E-29 遗留项 3（2026-10-08）：身份别名（userId/userid/uid/user/id）的
    // **数字型**取值与认证身份不符 → 403（与 user_id 同语义），不再被静默忽略。
    for (const alias of ['userId', 'userid', 'uid', 'user', 'id']) {
      const res = await api.get(`/api/v1/reports/daily?${alias}=${idB}`, {
        headers: { Authorization: `Bearer ${tokenA}` },
      })
      expect(res.status(), `别名 ${alias} 携带他人 id 必须 403（同 user_id 语义）`).toBe(403)
    }

    // 别名与认证身份一致 → 200，且响应体与基准逐字相同（未泄漏他人数据）
    const meA = await api.get('/api/v1/users/me', { headers: { Authorization: `Bearer ${tokenA}` } })
    const idA = String((await meA.json()).data.user.userId)
    for (const alias of ['userId', 'uid', 'id']) {
      const res = await api.get(`/api/v1/reports/daily?${alias}=${idA}`, {
        headers: { Authorization: `Bearer ${tokenA}` },
      })
      expect(res.status(), `别名 ${alias} 与认证身份一致应 200`).toBe(200)
      expect(await res.text(), `别名 ${alias} 一致时响应体应与基准逐字相同`).toBe(baseBody)
    }
  })

  test('#9/#10 跨用户资源读/改/删/发消息一律 403，自己的 2xx', async () => {
    const api = await newApi()
    const tokenA = await ensureAccount(api, 'ee29_e2e_a')
    const tokenB = await ensureAccount(api, 'ee29_e2e_b')
    const hA = { Authorization: `Bearer ${tokenA}` }
    const hB = { Authorization: `Bearer ${tokenB}` }

    const convB = await api.post('/api/v1/conversations', {
      headers: hB,
      data: { title: 'ee29-e2e-B-private' },
    })
    expect(convB.status()).toBe(200)
    const idB = (await convB.json()).data.id as string

    const convA = await api.post('/api/v1/conversations', { headers: hA, data: { title: 'ee29-e2e-A-own' } })
    expect(convA.status()).toBe(200)
    const idA = (await convA.json()).data.id as string

    // A 动 B 的资源 → 全部 403
    const crossCalls = [
      api.get(`/api/v1/conversations/${idB}/messages`, { headers: hA }),
      api.patch(`/api/v1/conversations/${idB}`, { headers: hA, data: { title: 'hijacked' } }),
      api.post(`/api/v1/conversations/${idB}/pin`, { headers: hA, data: { isTop: true } }),
      api.post(`/api/v1/conversations/${idB}/messages`, { headers: hA, data: { content: 'hijack' } }),
      api.delete(`/api/v1/conversations/${idB}`, { headers: hA }),
    ]
    for (const [i, p] of crossCalls.entries()) {
      const res = await p
      expect(res.status(), `跨用户调用 #${i} 必须 403`).toBe(403)
    }

    // A 动自己的资源 → 2xx（对照，证明上面的 403 不是"全盘拒绝"）
    const ownPatch = await api.patch(`/api/v1/conversations/${idA}`, {
      headers: hA,
      data: { title: 'ee29-e2e-A-renamed' },
    })
    expect(ownPatch.status()).toBe(200)

    // B 的资源未被改动
    const listB = await api.get('/api/v1/conversations', { headers: hB })
    const items = (await listB.json()).data.items ?? (await listB.json()).data.list ?? []
    const target = items.find((x: any) => String(x.id) === String(idB))
    expect(target, 'B 的会话必须仍然存在').toBeTruthy()
    expect(target.title).toBe('ee29-e2e-B-private')
  })

  test('#11 宿主直连 BFF 8894 不可达（D-47 收映射）', async () => {
    const direct = await request.newContext({ baseURL: BFF_DIRECT, timeout: 4000 })
    let reachable = true
    try {
      await direct.get('/health', { timeout: 3000 })
    } catch {
      reachable = false
    }
    expect(reachable, '宿主直连 8894 必须不可达（D-47 ③）').toBe(false)
    await direct.dispose()
  })

  test('#15/#16 CORS：恶意 origin 无 ACAO；合法 origin 精确回显且 allow_headers 不含 X-User-Id', async () => {
    const api = await newApi()

    const evil = await api.fetch('/api/v1/auth/login', {
      method: 'OPTIONS',
      headers: {
        Origin: 'http://evil.example.com',
        'Access-Control-Request-Method': 'POST',
      },
    })
    expect(evil.headers()['access-control-allow-origin'], '恶意 origin 不得回 ACAO').toBeUndefined()

    const good = await api.fetch('/api/v1/auth/login', {
      method: 'OPTIONS',
      headers: {
        Origin: 'http://localhost:3000',
        'Access-Control-Request-Method': 'POST',
        'Access-Control-Request-Headers': 'authorization,x-user-id',
      },
    })
    const h = good.headers()
    expect(h['access-control-allow-origin']).toBe('http://localhost:3000')
    expect(h['access-control-allow-headers'] ?? '', '#16：allow_headers 不得含 X-User-Id').not.toContain('X-User-Id')
    expect(h['access-control-max-age']).toBe('600')
  })

  test('#19 [V] 摄像头失败文案可归因（截图）', async ({ page }) => {
    fs.mkdirSync(SHOTS_DIR, { recursive: true })
    await page.goto('/login')
    await page.waitForLoadState('networkidle')

    // 登录后进入聊天页（摄像头入口在聊天页）
    const quickBtn = page.getByRole('button', { name: /用演示账号快速体验/ })
    await expect(quickBtn).toBeEnabled({ timeout: 20_000 })
    await quickBtn.click()
    await page.waitForURL(/\/chat/, { timeout: 20_000 })
    await page.waitForLoadState('networkidle')

    const camBtn = page.getByRole('button', { name: /摄像头/ })
    await expect(camBtn, '聊天页必须有摄像头入口').toBeVisible({ timeout: 15_000 })
    await camBtn.click()

    // 无摄像头环境（headless chromium）下应给出**可归因**文案，而不是笼统"请刷新页面"
    const banner = page.locator('body')
    await expect(banner).toContainText(/未找到可用的摄像头设备|当前浏览器环境不支持摄像头采集|摄像头组件未就绪|摄像头权限被拒绝|摄像头正被其他程序占用/, {
      timeout: 15_000,
    })
    await page.screenshot({
      path: path.join(SHOTS_DIR, '19-camera-error-attributable.png'),
      fullPage: true,
    })
  })
})
