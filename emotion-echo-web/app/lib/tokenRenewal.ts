/**
 * 访问令牌滑动续期（sliding token renewal）。
 *
 * 背景（E2E-29 补验轮 E2E-F-207）：前端原有一条「401 + `code===10002` → 自动续期」
 * 分支，但**全仓无任何后端下发 10002**（BFF 一律 `code:1`；APISIX 在过期令牌上先返
 * 401 且不带业务 code），故该分支从未执行 ⇒ 令牌一过期就登出。且因为 APISIX 挡在
 * BFF 之前，**过期后已无法续期**（D-46：过期/无效一律 401）——只能**过期前主动续期**。
 *
 * 本模块：在令牌寿命的 `RENEW_AT_FRACTION`（默认 75%）处调后端 refresh 换新令牌。
 * 纯逻辑与调度分离，便于注入时钟/定时器做确定性单测。
 * 设计见 `docs/plans/sliding-token-renewal.md`。
 */

/** 在令牌寿命的该比例处续期（0.75 = 用掉 75% 时续）。 */
export const RENEW_AT_FRACTION = 0.75

/** 最小提前量：避免续期点与 exp 重合（极短寿命令牌仍能立即续）。 */
export const RENEW_MIN_LEAD_MS = 60_000

export interface JwtTimes {
  iatMs: number | null
  expMs: number | null
}

const NO_TIMES: JwtTimes = { iatMs: null, expMs: null }

function base64UrlDecode(segment: string): string {
  const b64 = segment.replace(/-/g, '+').replace(/_/g, '/')
  const padded = b64 + '='.repeat((4 - (b64.length % 4)) % 4)
  return atob(padded)
}

/** 从 JWT 解析 `iat` / `exp`（秒 → 毫秒）；解析不出则为 null。 */
export function parseJwtTimes(token: string | null): JwtTimes {
  if (!token) return NO_TIMES
  const parts = token.split('.')
  if (parts.length !== 3) return NO_TIMES
  try {
    const payload = JSON.parse(base64UrlDecode(parts[1]!))
    const iatMs = typeof payload?.iat === 'number' ? payload.iat * 1000 : null
    const expMs = typeof payload?.exp === 'number' ? payload.exp * 1000 : null
    return { iatMs, expMs }
  } catch {
    return NO_TIMES
  }
}

/**
 * 计算距下一次续期应等待的毫秒数。
 *
 * 返回 `null` = 无需/无法续期：无令牌、解析不出 `exp`、或**已过期**
 * （过期后只能由 401 → 登出兜底，见 D-46）。
 * 返回 `0` = 已到/已过续期点，应立即续期。
 */
export function nextRenewalDelayMs(token: string | null, nowMs: number): number | null {
  const { iatMs, expMs } = parseJwtTimes(token)
  if (expMs === null) return null
  if (nowMs >= expMs) return null

  // 缺 iat 时退化为"把剩余时间当作寿命"（即剩余时间的 25% 处续期）
  const lifetime = iatMs !== null && iatMs < expMs ? expMs - iatMs : expMs - nowMs
  const lead = Math.max(lifetime * (1 - RENEW_AT_FRACTION), RENEW_MIN_LEAD_MS)
  const renewAt = expMs - lead
  return Math.max(0, Math.round(renewAt - nowMs))
}

export interface TokenRenewalOptions {
  /** 读取当前令牌（无则 null）。 */
  getToken: () => string | null
  /** 执行续期；成功返回新令牌，失败返回 null 或抛异常。 */
  renew: () => Promise<string | null>
  now?: () => number
  setTimer?: (fn: () => void, ms: number) => any
  clearTimer?: (handle: any) => void
  onRenewed?: (token: string) => void
  onError?: (err: unknown) => void
}

export interface TokenRenewal {
  /** 按当前令牌重新排程（会先清掉上一次，不叠加）。 */
  schedule(): void
  /** 清掉已排程的定时器。 */
  stop(): void
  isScheduled(): boolean
}

/**
 * 创建续期调度器。失败（返回 null / 抛异常）**不重排**、**不冒泡**——
 * 避免失败后紧密重试；真正的失效由请求 401 → 登出兜底。
 */
export function createTokenRenewal(opts: TokenRenewalOptions): TokenRenewal {
  const now = opts.now ?? (() => Date.now())
  const setTimer = opts.setTimer ?? ((fn: () => void, ms: number) => setTimeout(fn, ms))
  const clearTimer = opts.clearTimer ?? ((handle: any) => clearTimeout(handle))

  let handle: any = null
  let inflight = false

  function schedule(): void {
    if (handle !== null) {
      clearTimer(handle)
      handle = null
    }
    const delay = nextRenewalDelayMs(opts.getToken(), now())
    if (delay === null) return
    handle = setTimer(onFire, delay)
  }

  function stop(): void {
    if (handle !== null) {
      clearTimer(handle)
      handle = null
    }
  }

  async function onFire(): Promise<void> {
    handle = null
    if (inflight) return
    inflight = true
    let ok = false
    try {
      const token = await opts.renew()
      if (token) {
        ok = true
        opts.onRenewed?.(token)
      }
    } catch (err) {
      opts.onError?.(err)
    } finally {
      inflight = false
    }
    if (ok) schedule()
  }

  return { schedule, stop, isScheduled: () => handle !== null }
}
