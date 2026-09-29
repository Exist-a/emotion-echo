/**
 * clientErrorReporter.ts — 浏览器侧未捕获异常上报（E2E-F-148）
 *
 * 为什么需要：项目原有 58 处 console.*，服务端日志里完全看不到浏览器发生了什么。
 * 本轮不引第三方 SDK（Sentry 之类会新增外部数据出口、需隐私评审），而是
 * POST 到 BFF 的 /api/v1/client-error —— 那里会写进与业务日志同一条结构化
 * 日志流，经 promtail 进 Loki，**并带 trace_id**，可与后端同一时刻的日志对齐。
 *
 * 关键约束：
 *  - 永不抛错、永不 await：上报本身出问题绝不能影响主流程
 *  - 采样：只上报 error / unhandledrejection；高频场景（资源加载失败）不重复刷
 *  - 限流：同一 msg 在滑动窗口内只报一次，避免死循环把日志打爆
 */
/** 同一错误在窗口内只上报一次（防死循环刷爆日志） */
const DEDUP_WINDOW_MS = 10_000
const lastSent = new Map<string, number>()

/** 单条 payload 上限，防止把整页 HTML 塞进日志 */
const MAX_MSG = 2000
const MAX_STACK = 4000

function clip(s: string, max: number): string {
  if (!s) return ''
  return s.length > max ? s.slice(0, max) + '...[truncated]' : s
}

function shouldSend(key: string): boolean {
  const now = Date.now()
  const prev = lastSent.get(key)
  if (prev !== undefined && now - prev < DEDUP_WINDOW_MS) return false
  lastSent.set(key, now)
  // 顺手清理过期项，避免 Map 无限增长
  if (lastSent.size > 100) {
    for (const [k, t] of lastSent) {
      if (now - t >= DEDUP_WINDOW_MS) lastSent.delete(k)
    }
  }
  return true
}

function describe(err: unknown): { msg: string; stack: string } {
  if (err instanceof Error) {
    return { msg: clip(err.message, MAX_MSG), stack: clip(err.stack ?? '', MAX_STACK) }
  }
  if (typeof err === 'string') return { msg: clip(err, MAX_MSG), stack: '' }
  try {
    return { msg: clip(JSON.stringify(err) ?? String(err), MAX_MSG), stack: '' }
  } catch {
    return { msg: clip(String(err), MAX_MSG), stack: '' }
  }
}

/**
 * 上报目标 base URL。
 *
 * ⚠️ 必须在**安装时**（Nuxt plugin 上下文内）取好并传进来，不能在 window 事件
 * 回调里现调 `useRuntimeConfig()` —— 2026-09-29 IAB 实测踩到：事件回调不在
 * Nuxt 实例上下文内，`useRuntimeConfig()` 抛错，而 reportClientError 的
 * try/catch 会把它**静默吞掉**，表现为"监听器装了却一条都发不出去"，
 * 单元测试因为 stub 了 useRuntimeConfig 完全测不出来。
 */
let baseUrl = ''

/**
 * 发送一条客户端错误。非阻塞、永不 reject。
 * @param kind error | unhandledrejection
 */
export async function reportClientError(
  kind: 'error' | 'unhandledrejection',
  err: unknown,
  extra?: { url?: string; line?: number; col?: number }
): Promise<void> {
  try {
    const { msg, stack } = describe(err)
    const key = `${kind}:${msg}`
    if (!msg && !stack) return
    if (!shouldSend(key)) return

    if (!baseUrl) {
      // 没装或装失败（理论上不会，见 installClientErrorReporter）——
      // 直接放弃上报，绝不为了"发出去"而抛给调用方
      return
    }
    // 刻意用裸 fetch，不走 composables/useApi 的封装：
    // 1) useApi 带鉴权 / 重试 / 401 跳转等包装 —— 上报失败时再触发跳转或重试，
    //    就变成"错误上报引发更多错误"的循环；
    // 2) 这个端点不要求登录（未登录白屏恰恰最需要被记录）。
    // ⚠️ baseUrl 本身**已经含** /api/v1 前缀（NUXT_PUBLIC_API_BASE_URL=
    // http://localhost:19080/api/v1，决策 11/12：APISIX 是唯一业务入口）。
    // 早期版本这里又拼了一次 /api/v1 ⇒ 实际打到 /api/v1/api/v1/client-error
    // ⇒ 落到 catch-all route 100 ⇒ jwt-auth 401。
    // （2026-09-29 IAB 实测：网关 access log 里 url=/api/v1/api/v1/client-error
    //   route_id=100 status=401 —— curl 测不出来，因为 curl 用的是手写路径。）
    await fetch(`${baseUrl}/client-error`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        kind,
        msg,
        stack,
        url: extra?.url ?? (typeof window !== 'undefined' ? window.location.href : ''),
        line: extra?.line ?? 0,
        col: extra?.col ?? 0
      }),
      // 上报不该被缓存，也不该拖住页面卸载
      keepalive: true
    })
  } catch {
    // 上报失败静默吞掉：绝不能因为上报把错误再抛一遍
  }
}

/**
 * 安装全局捕获（nuxt plugin 调用）。
 *
 * @param apiBase API base URL（形如 http://localhost:19080/api/v1）。
 *   必须在 Nuxt plugin 内取（getApiBaseUrl() 依赖 runtimeConfig）。
 * 返回卸载函数，便于测试里复原。
 */
export function installClientErrorReporter(apiBase: string): () => void {
  if (typeof window === 'undefined') return () => {}
  baseUrl = (apiBase || '').replace(/\/+$/, '')
  // 正向标记：2026-09-29 IAB 排查时，"监听器到底装没装"没有任何可观测点，
  // 只能靠加日志猜。留一个显式标记，排查与线上排障都能直接读。
  ;(window as any).__CLIENT_ERROR_REPORTER_READY__ = true

  const onError = (ev: ErrorEvent) => {
    void reportClientError('error', ev.error ?? ev.message, {
      url: ev.filename,
      line: ev.lineno,
      col: ev.colno
    })
  }
  const onRejection = (ev: PromiseRejectionEvent) => {
    void reportClientError('unhandledrejection', ev.reason)
  }

  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onRejection)

  return () => {
    window.removeEventListener('error', onError)
    window.removeEventListener('unhandledrejection', onRejection)
  }
}
