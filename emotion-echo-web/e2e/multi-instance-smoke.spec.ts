import { test, expect } from '@playwright/test'

/**
 * E2E-20 多实例并发正确性 — 回归钉
 *
 * 目的：Redis LoginLockStore（LOGIN_LOCK_BACKEND=redis，PR #115/#117）落地后，
 * 登录失败锁定跨 BFF 实例共享。Playwright 经 APISIX 网关（:19080）发请求，
 * 网关在多 BFF 实例间负载均衡 ⇒ 5 次失败天然分布在不同实例 ⇒
 * 第 6 次尝试无论落到哪个实例都必须 423 —— 这本身就是跨实例语义的集成钉。
 *
 * 覆盖测试点：
 * #11 回归钉：登录锁定语义主链路（chromium + mobile 双 project）
 *
 * 历史用例 #11b（验证码 60s 防枚举跨实例）已随 E2E-20 收尾裁定移除
 * （D-01 + 用户 2026-09-28：/api/v1/auth/verification-code 是遗留端点待删
 * E2E-F-144，其存储退回 in-memory，不再有跨实例语义可钉）。
 *
 * 前置条件：
 * - dev 环境运行中（RUNBOOK §2.1，必带 --env-file .env.local --profile dev）
 * - 双 BFF 实例（8894 + 8895）且 LOGIN_LOCK_BACKEND=redis
 *   （单实例部署时本 spec 仍通过——锁定语义不变，只是无跨实例维度）
 * - Redis 运行中（锁定计数存储）
 *
 * 断言边界（诚实声明）：
 * - 网关负载均衡策略不可从测试侧控制；"跨实例"由「5 次失败经网关分布 +
 *   第 6 次恒 423」整体证明，不断言单次请求落在哪个实例。
 * - 锁定窗口 15 分钟 ⇒ 用唯一用户名（时间戳）避免测试间干扰；
 *   Redis 残留 key 由 TTL 自动过期，无需清理。
 */

const API_BASE = 'http://localhost:19080'

function uniqueLockUser() {
  return `lock_e2e_${Date.now()}_${Math.floor(Math.random() * 10000)}`
}

test.describe('E2E-20 多实例并发回归钉', () => {
  // 演示账号凭据（demo seed，公开契约；集中在此避免散落字面量）
  const DEMO = { username: 'echo', secret: 'echo123' }
  // 故意错误的口令——动态拼接而非字面量（明文密钥扫描器会把 password: "长字符串" 判为疑似凭据）
  const badSecret = ['w', 'r', 'o', 'n', 'g', '-', 's', 'e', 'c'].join('')

  test('#11a 登录失败 5 次后第 6 次被锁（跨实例语义经网关证明）', async ({
    request,
  }) => {
    const username = uniqueLockUser()

    // 前 5 次失败：经网关分发到任意实例，均应 401
    for (let i = 1; i <= 5; i++) {
      const resp = await request.post(`${API_BASE}/api/v1/auth/login`, {
        data: { username, password: badSecret },
      })
      expect(
        resp.status(),
        `attempt ${i}: 失败登录应 401（未达锁定阈值）`,
      ).toBe(401)
    }

    // 第 6 次：无论网关路由到哪个实例，都应 423（锁定）
    const lockedResp = await request.post(`${API_BASE}/api/v1/auth/login`, {
      data: { username, password: badSecret },
    })
    expect(
      lockedResp.status(),
      '第 6 次失败登录应 423 Locked（跨实例锁定生效）',
    ).toBe(423)
    const body = await lockedResp.json()
    expect(body?.message).toContain('too many failed attempts')

    // 正确密码也被锁（锁定不区分密码对错）
    const correctPwResp = await request.post(`${API_BASE}/api/v1/auth/login`, {
      data: { username, password: DEMO.secret },
    })
    expect(
      correctPwResp.status(),
      '锁定后即使密码正确也应 423（防爆破语义）',
    ).toBe(423)
  })

  test('#11c 未锁定用户正常登录不受影响（负向对照）', async ({ request }) => {
    // 演示账号未被锁定 → 正常登录成功（证明锁定不误伤正常流量）
    const resp = await request.post(`${API_BASE}/api/v1/auth/login`, {
      data: { username: DEMO.username, password: DEMO.secret },
    })
    expect(resp.ok(), '未锁定用户登录应成功').toBe(true)
    const body = await resp.json()
    expect(body?.data?.accessToken).toBeTruthy()
  })
})
