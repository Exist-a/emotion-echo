/**
 * apisix-gateway.spec.ts — E2E-25 网关 APISIX 回归钉
 *
 * 钉住本阶段验证过的网关行为，防止后续阶段/重构引入回退：
 *   1. /apisix-health 自健康路由 200（N1：route 205 无 upstream 曾恒 503）
 *   2. catch-all 未带 token → 401（jwt-auth 验签链路活着，#13）
 *   3. 篡改签名的 token → 401（#13 负向）
 *   4. auth 白名单路由无 token 可达且不是 jwt 401（#15；login 的 401 是业务层
 *      "invalid username or password" JSON，非 APISIX 拦截页）
 *   5. 有效 token → /users/me 200 且 userId 正确（X-User-Id 注入链路，#13/#14）
 *   6. 客户端伪造 X-User-Id 被覆盖：带伪造 header 仍返回真实身份（#14）
 *   7. CORS preflight 六头齐全且 allow_origins 精确回显（#19，F-139 症状回归钉）
 *   8. 白名单路由限流拒绝码 = 429 非 503（N2 语义钉：仅在限流触发后可观测，
 *      这里只静态验证 preflight 不被限流误伤；429 语义由 dev 运行时验证留证）
 *
 * 判定：[A] 自动可判（HTTP 状态码 + 响应头）。
 * 前置：dev 栈在跑（RUNBOOK §2.1），网关 localhost:19080 可达。
 */
import { test, expect } from '@playwright/test'

const API_BASE = 'http://localhost:19080'

// login 是限流路由（60/min/IP，N2 修复后拒绝码 429）。回归钉连续跑时会自己把
// 配额打穿——token 按 worker 缓存，login 每个项目只发一次。
let tokenPromise: Promise<string> | null = null
function getEchoToken(request: import('@playwright/test').APIRequestContext): Promise<string> {
  if (!tokenPromise) {
    tokenPromise = (async () => {
      const res = await request.post(`${API_BASE}/api/v1/auth/login`, {
        data: { username: 'echo', password: 'echo123' },
      })
      if (res.status() === 429) throw new Error('login 被限流（429）——等 60s 窗口再跑 spec')
      expect(res.status()).toBe(200)
      return (await res.json()).data.accessToken as string
    })()
  }
  return tokenPromise
}

test('网关自健康路由 /apisix-health 返回 200（N1 回归钉）', async ({ request }) => {
  const res = await request.get(`${API_BASE}/apisix-health`)
  expect(res.status()).toBe(200)
  expect(await res.text()).toContain('ok')
})

test('catch-all 未带 token → 401（jwt-auth 验签链路）', async ({ request }) => {
  const res = await request.get(`${API_BASE}/api/v1/users/me`)
  expect(res.status()).toBe(401)
})

test('篡改签名的 token → 401（负向）', async ({ request }) => {
  const token = await getEchoToken(request)
  // ⚠️ 必须改签名的**第一个**字符：末字符只携带 base64 填充位，替换后解码字节
  // 可能不变（等价签名）⇒ 偶发 200 假失败。首字符影响首字节高位，必变。
  const head = token.slice(0, token.lastIndexOf('.') + 1)
  const sig = token.slice(token.lastIndexOf('.') + 1)
  const flipped = sig[0] === 'A' ? 'B' : 'A'
  const tampered = head + flipped + sig.slice(1)
  expect(tampered).not.toBe(token)
  const res = await request.get(`${API_BASE}/api/v1/users/me`, {
    headers: { Authorization: `Bearer ${tampered}` },
  })
  expect(res.status()).toBe(401)
})

test('auth 白名单路由无 token 不被 jwt 拦截（#15：401 必须是业务层 JSON）', async ({ request }) => {
  // login 用不存在的随机用户：业务层 401 + JSON body（若是 APISIX 拦截页则 body 非 JSON）。
  // ⚠️ 必须随机——固定不存在用户反复失败会触发登录锁定（authlock，D-28）返 423。
  const randUser = `__no_such_${Math.random().toString(36).slice(2, 10)}__`
  const res = await request.post(`${API_BASE}/api/v1/auth/login`, {
    data: { username: randUser, password: 'x' },
  })
  expect(res.status()).toBe(401)
  const body = await res.json()
  expect(body.message).toContain('invalid username or password')

  // security-questions 无 token → 非 401（200/400 业务响应）
  const sq = await request.post(`${API_BASE}/api/v1/auth/security-questions`, {
    data: { username: 'echo' },
  })
  expect(sq.status()).not.toBe(401)

  // client-error 无 token → 非 401（E2E-F-148：未登录白屏恰是最需上报的场景）
  const ce = await request.post(`${API_BASE}/api/v1/client-error`, {
    data: { message: 'pin-probe' },
  })
  expect(ce.status()).not.toBe(401)
})

test('有效 token → /users/me 200 且 userId=1；伪造 X-User-Id 被覆盖（#13/#14）', async ({ request }) => {
  const token = await getEchoToken(request)

  const me = await request.get(`${API_BASE}/api/v1/users/me`, {
    headers: { Authorization: `Bearer ${token}` },
  })
  expect(me.status()).toBe(200)
  expect((await me.json()).data.user.userId).toBe(1)

  // 负向：带伪造 X-User-Id: 999 的有效请求仍返回真实身份（post-function 无条件覆盖）
  const forged = await request.get(`${API_BASE}/api/v1/users/me`, {
    headers: { Authorization: `Bearer ${token}`, 'X-User-Id': '999' },
  })
  expect(forged.status()).toBe(200)
  expect((await forged.json()).data.user.userId).toBe(1)
})

test('CORS preflight 六头齐全且 origin 精确回显（#19 回归钉）', async ({ request }) => {
  const res = await request.fetch(`${API_BASE}/api/v1/users/me`, {
    method: 'OPTIONS',
    headers: {
      Origin: 'http://127.0.0.1:3000',
      'Access-Control-Request-Method': 'GET',
      'Access-Control-Request-Headers': 'authorization',
    },
  })
  expect(res.status()).toBe(200)
  expect(res.headers()['access-control-allow-origin']).toBe('http://127.0.0.1:3000')
  expect(res.headers()['access-control-allow-methods']).toContain('GET')
  expect(res.headers()['access-control-allow-credentials']).toBe('true')
  expect(res.headers()['access-control-allow-headers'].toLowerCase()).toContain('authorization')
  expect(res.headers()['access-control-expose-headers']).toContain('X-User-Id')
  expect(res.headers()['access-control-max-age']).toBe('600')
})
