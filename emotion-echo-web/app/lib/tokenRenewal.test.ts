/**
 * 访问令牌滑动续期逻辑单测（plan docs/plans/sliding-token-renewal.md，E2E-F-207 follow-up）。
 *
 * 覆盖：JWT 时间解析 / 续期时点计算 / 调度器（注入时钟与定时器，无真实等待）。
 */
import { describe, it, expect, vi } from 'vitest'
import {
  parseJwtTimes,
  nextRenewalDelayMs,
  createTokenRenewal,
  RENEW_AT_FRACTION,
  RENEW_MIN_LEAD_MS,
} from './tokenRenewal'

/** 造一把形如 JWT 的令牌（仅 payload 有意义）。 */
function makeToken(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) => Buffer.from(JSON.stringify(o)).toString('base64url')
  return `${b64({ alg: 'HS256', typ: 'JWT' })}.${b64(payload)}.sig`
}

const IAT_S = 1_000_000_000
const IAT_MS = IAT_S * 1000
const TTL_MS = 86_400_000 // 24h
const EXP_MS = IAT_MS + TTL_MS
const TOKEN_24H = makeToken({ user_id: 1, key: 'user', iat: IAT_S, exp: IAT_S + 86400 })

describe('parseJwtTimes', () => {
  it('解析 iat/exp（秒 → 毫秒）', () => {
    expect(parseJwtTimes(TOKEN_24H)).toEqual({ iatMs: IAT_MS, expMs: EXP_MS })
  })

  it('空令牌 / 非 3 段 / 非法 payload → 双 null', () => {
    expect(parseJwtTimes(null)).toEqual({ iatMs: null, expMs: null })
    expect(parseJwtTimes('')).toEqual({ iatMs: null, expMs: null })
    expect(parseJwtTimes('a.b')).toEqual({ iatMs: null, expMs: null })
    expect(parseJwtTimes('a.@@@not-base64@@@.c')).toEqual({ iatMs: null, expMs: null })
  })

  it('缺 iat 时 iatMs 为 null 而 expMs 仍可解析', () => {
    const t = makeToken({ exp: IAT_S + 100 })
    expect(parseJwtTimes(t)).toEqual({ iatMs: null, expMs: (IAT_S + 100) * 1000 })
  })
})

describe('nextRenewalDelayMs', () => {
  it('刚签发 → 距续期点 = 寿命的 75%', () => {
    const expected = Math.round(TTL_MS * RENEW_AT_FRACTION)
    expect(nextRenewalDelayMs(TOKEN_24H, IAT_MS)).toBe(expected)
    expect(expected).toBe(64_800_000) // 18h
  })

  it('已过续期点 → 0（立即续期）', () => {
    expect(nextRenewalDelayMs(TOKEN_24H, IAT_MS + 70_000_000)).toBe(0)
  })

  it('刚好在续期点 → 0', () => {
    expect(nextRenewalDelayMs(TOKEN_24H, IAT_MS + 64_800_000)).toBe(0)
  })

  it('已过期 / 无令牌 / 非法令牌 → null（无从续期）', () => {
    expect(nextRenewalDelayMs(TOKEN_24H, EXP_MS)).toBeNull()
    expect(nextRenewalDelayMs(TOKEN_24H, EXP_MS + 1)).toBeNull()
    expect(nextRenewalDelayMs(null, IAT_MS)).toBeNull()
    expect(nextRenewalDelayMs('garbage', IAT_MS)).toBeNull()
    expect(nextRenewalDelayMs(makeToken({ exp: IAT_S + 100 }), IAT_MS)).not.toBeNull() // 有 exp 即可
  })

  it('极短寿命：最小提前量生效 → 立即续期（不返回负值）', () => {
    const t = makeToken({ iat: IAT_S, exp: IAT_S + 10 }) // 10s
    expect(nextRenewalDelayMs(t, IAT_MS)).toBe(0)
    expect(RENEW_MIN_LEAD_MS).toBe(60_000)
  })

  it('缺 iat：退化为"剩余时间的 25% 处"', () => {
    const t = makeToken({ exp: IAT_S + 3600 }) // 1h，无 iat
    // 剩余 = expMs - nowMs；renewAt = expMs - 0.25*剩余 ⇒ delay = 0.75*剩余
    const now = IAT_MS
    const remaining = (IAT_S + 3600) * 1000 - now
    expect(nextRenewalDelayMs(t, now)).toBe(Math.round(remaining * RENEW_AT_FRACTION))
  })
})

describe('createTokenRenewal 调度器', () => {
  /** 手写定时器，便于确定性驱动。 */
  function harness(token: string | null, renewImpl?: () => Promise<string | null>) {
    let scheduled: { fn: () => void; ms: number } | null = null
    let cleared = 0
    const renew = vi.fn(renewImpl ?? (async () => 'new-token'))
    const r = createTokenRenewal({
      getToken: () => token,
      renew,
      now: () => IAT_MS,
      setTimer: (fn, ms) => {
        scheduled = { fn, ms }
        return 1
      },
      clearTimer: () => {
        cleared += 1
      },
    })
    return {
      r,
      renew,
      fire: async () => {
        const s = scheduled
        scheduled = null
        s?.fn()
        // 让 async 续期链走完
        await Promise.resolve()
        await Promise.resolve()
        await Promise.resolve()
      },
      get scheduledMs() {
        return scheduled?.ms ?? null
      },
      get cleared() {
        return cleared
      },
    }
  }

  it('schedule 按计算出的延迟排程', () => {
    const h = harness(TOKEN_24H)
    h.r.schedule()
    expect(h.r.isScheduled()).toBe(true)
    expect(h.scheduledMs).toBe(64_800_000)
  })

  it('无令牌 → 不排程', () => {
    const h = harness(null)
    h.r.schedule()
    expect(h.r.isScheduled()).toBe(false)
    expect(h.scheduledMs).toBeNull()
  })

  it('到点触发续期，成功后重排下一次', async () => {
    const h = harness(TOKEN_24H)
    h.r.schedule()
    await h.fire()
    expect(h.renew).toHaveBeenCalledTimes(1)
    expect(h.r.isScheduled()).toBe(true) // 重排
  })

  it('续期失败（返回 null）→ 不重排、不抛', async () => {
    const h = harness(TOKEN_24H, async () => null)
    h.r.schedule()
    await h.fire()
    expect(h.renew).toHaveBeenCalledTimes(1)
    expect(h.r.isScheduled()).toBe(false)
  })

  it('续期抛异常 → 不重排、不冒泡', async () => {
    const h = harness(TOKEN_24H, async () => {
      throw new Error('boom')
    })
    h.r.schedule()
    await h.fire()
    expect(h.r.isScheduled()).toBe(false)
  })

  it('stop 清掉已排程的定时器', () => {
    const h = harness(TOKEN_24H)
    h.r.schedule()
    h.r.stop()
    expect(h.r.isScheduled()).toBe(false)
    expect(h.cleared).toBeGreaterThan(0)
  })

  it('重复 schedule 不叠加（先清后排）', () => {
    const h = harness(TOKEN_24H)
    h.r.schedule()
    h.r.schedule()
    expect(h.r.isScheduled()).toBe(true)
    expect(h.cleared).toBeGreaterThan(0)
  })
})
