import { expect, type APIResponse, type Page } from '@playwright/test'

/**
 * E2E-F-214：回归钉在 dev 限流下 flaky 的**根因是请求预算，不是断言**。
 *
 * 事实（2026-10-09 实测，APISIX Redis 计数键 `plugin-limit-count:v1:/apisix/routes/100:<addr>`）：
 *   - APISIX route 100（`/api/v1/*` catch-all）与 route 110（auth 白名单）各挂
 *     `limit-count`：`count = 60` / `time_window = 60`，key = `remote_addr`。
 *   - `quiz + survey-scoring + personality` 双 project 一次性连跑共发出 route100 **112** 次、
 *     route110 **50** 次（原本 24 用例 × 2 project 各登录一次）；其中某一分钟 route100 达 **72** 次
 *     ⇒ 超出 60 ⇒ 429 ⇒ 页面无数据 ⇒ 断言随机假红（实测复现：`[mobile] personality #4`）。
 *
 * 本模块把连跑的请求压到阈值之下，**不改产品限流配置**（仍是 60/60s）：
 *   1. `loginOnce` —— worker 内复用登录令牌（access token TTL 24h）。原本 48 次登录是纯冗余。
 *   2. `gwGet` 的**种子数据缓存** —— `/api/v1/surveys`（列表）与 `/api/v1/surveys/{id}`（详情）
 *      是只读种子数据，用例执行期间不会被写入，可跨用例复用（结果类接口 `/surveys/results*`
 *      每次提交都变化，**禁止缓存**）。
 *   3. `acquireSlot` —— 进程级滑动窗口预算，把经网关的请求压到 ≤ `BUDGET`/60s。
 *   4. `gateBrowserRequests` —— 页面**自身**发出的网关请求也走预算
 *      （`page.request` 不经 `page.route`，故显式调用必须走 `gwGet` / `gwPost`）。
 *
 * 为什么预算取 55 而不是 60：APISIX 的 `limit-count`（redis policy）是**固定窗口**，跨窗口边界
 * 的 60s 区间理论上可放过近 2×count；而我们这里限制的是**任意滑动 60s 窗口**，故 55 对 60 有
 * 安全余量。（实测每 project 受控请求约 57 次，缓存后约 38 次 ⇒ 需求 0.63 req/s < 预算 0.92 req/s，
 * 闸门不再阻塞；若阻塞会经 `E2E_GATE_DEBUG=1` 打印每次等待毫秒。）
 */

/** 网关（APISIX）地址；与各 spec 的 `API_BASE` 同源，前端 `NUXT_PUBLIC_API_BASE_URL` 亦然 */
export const GATEWAY = process.env.API_BASE ?? 'http://localhost:19080'
/** 前端站点地址（SSR 由 web 容器提供，不经网关） */
export const WEB_BASE = process.env.BASE_URL ?? 'http://localhost:3000'

/** 演示账号（种子数据） */
export const DEMO = { username: 'echo', password: 'echo123' }

/** 窗口内允许发出的请求上限——低于 APISIX 的 60，留出安全余量 */
const BUDGET = 55
const WINDOW_MS = 60_000

/** 滑动窗口内已发出的请求时间戳（worker 进程内共享） */
const issued: number[] = []

/** 供证据收集：本 worker 的闸门统计 */
export const gateStats = { issued: 0, blocked: 0, waitedMs: 0, cacheHits: 0 }

/** 取一个预算令牌；窗口满则等到最早的请求滑出窗口再取 */
export async function acquireSlot(): Promise<void> {
  for (;;) {
    const now = Date.now()
    while (issued.length > 0 && now - issued[0] >= WINDOW_MS) issued.shift()
    if (issued.length < BUDGET) {
      issued.push(now)
      gateStats.issued++
      return
    }
    const waitMs = Math.max(WINDOW_MS - (now - issued[0]) + 25, 25)
    gateStats.blocked++
    gateStats.waitedMs += waitMs
    if (process.env.E2E_GATE_DEBUG) {
      process.stdout.write(
        `[gateway-budget] 窗口已满(${issued.length}/${BUDGET})，等待 ${waitMs}ms\n`,
      )
    }
    await new Promise((resolve) => setTimeout(resolve, waitMs))
  }
}

// 证据收集：worker 退出时打印闸门统计（E2E_GATE_DEBUG=1）
process.once('exit', () => {
  if (process.env.E2E_GATE_DEBUG) {
    process.stdout.write(
      `[gateway-budget] 汇总 issued=${gateStats.issued} blocked=${gateStats.blocked} ` +
        `waitedMs=${gateStats.waitedMs} cacheHits=${gateStats.cacheHits}\n`,
    )
  }
})

/**
 * 把浏览器**自身**发出的网关请求也纳入预算。
 * 必须在 `page.goto` 之前调用（`test.beforeEach` 最合适）。
 */
export async function gateBrowserRequests(page: Page): Promise<void> {
  await page.route(`${GATEWAY}/**`, async (route) => {
    await acquireSlot()
    await route.continue()
  })
}

let cachedToken: string | null = null

/**
 * 登录一次并在 worker 内复用。
 *
 * 覆盖度说明（诚实记录）：登录成功的断言由**每个 worker 首次调用**承担（原实现为每用例一次，
 * 48 次）；因此本改动把「login 端点可用」的运行时断言从 48 次降为 1 次/worker。令牌本身
 * （TTL 24h）与 cookie 注入方式完全不变，用例的业务断言一条未动。登录端点另有独立覆盖
 * （`login-flow.spec.ts` / Go 侧 auth handler 单测）。
 */
export async function loginOnce(page: Page): Promise<string> {
  if (cachedToken) {
    await page.context().addCookies([{ name: 'access_token', value: cachedToken, url: WEB_BASE }])
    return cachedToken
  }
  await acquireSlot()
  const resp = await page.request.post(`${GATEWAY}/api/v1/auth/login`, { data: DEMO })
  expect(resp.ok(), 'login API must succeed').toBe(true)
  const token = (await resp.json())?.data?.accessToken as string
  expect(token, 'login response must contain accessToken').toBeTruthy()
  cachedToken = token
  await page.context().addCookies([{ name: 'access_token', value: token, url: WEB_BASE }])
  return token
}

/** 响应视图：`page.request` 的真实响应与种子缓存命中两者同形，调用方无感 */
export interface GwResponse {
  ok(): boolean
  status(): number
  json(): Promise<any>
  text(): Promise<string>
}

function wrap(resp: APIResponse): GwResponse {
  return {
    ok: () => resp.ok(),
    status: () => resp.status(),
    json: () => resp.json(),
    text: () => resp.text(),
  }
}

function authHeaders(token?: string) {
  return token ? { Authorization: `Bearer ${token}` } : undefined
}

/** 种子数据（量表定义）缓存：只读、跨用例不变；不含 `/surveys/results*`（提交即变） */
const seedCache = new Map<string, { status: number; body: any }>()

function isCacheableSeedGet(path: string): boolean {
  return /^\/api\/v1\/surveys(\/\d+)?$/.test(path)
}

function fromCache(entry: { status: number; body: any }): GwResponse {
  return {
    ok: () => entry.status >= 200 && entry.status < 300,
    status: () => entry.status,
    // 深拷贝：避免某个用例就地修改 body 后污染其余用例
    json: async () => JSON.parse(JSON.stringify(entry.body)),
    text: async () => JSON.stringify(entry.body),
  }
}

/** 经预算闸门的 GET（替代裸 `page.request.get`）；只读种子数据命中 worker 内缓存 */
export async function gwGet(page: Page, path: string, token?: string): Promise<GwResponse> {
  const cacheable = isCacheableSeedGet(path)
  if (cacheable) {
    const hit = seedCache.get(path)
    if (hit) {
      gateStats.cacheHits++
      return fromCache(hit)
    }
  }
  await acquireSlot()
  const resp = await page.request.get(`${GATEWAY}${path}`, { headers: authHeaders(token) })
  if (cacheable && resp.ok()) {
    const entry = { status: resp.status(), body: await resp.json() }
    seedCache.set(path, entry)
    return fromCache(entry)
  }
  return wrap(resp)
}

/** 经预算闸门的 POST（替代裸 `page.request.post`）；POST 一律不缓存 */
export async function gwPost(
  page: Page,
  path: string,
  opts: { token?: string; data?: unknown; timeout?: number } = {},
): Promise<GwResponse> {
  await acquireSlot()
  const resp = await page.request.post(`${GATEWAY}${path}`, {
    headers: authHeaders(opts.token),
    data: opts.data,
    timeout: opts.timeout,
  })
  return wrap(resp)
}

/** 仅测试用：重置预算窗口、令牌与种子缓存 */
export function __resetGatewayState(): void {
  issued.length = 0
  cachedToken = null
  seedCache.clear()
  gateStats.issued = 0
  gateStats.blocked = 0
  gateStats.waitedMs = 0
  gateStats.cacheHits = 0
}
