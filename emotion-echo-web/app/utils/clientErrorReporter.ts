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
import { getApiBaseUrl } from '~/lib/apiBaseUrl'

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

    // 刻意用裸 fetch，不走 composables/useApi 的封装：
    // 1) useApi 带鉴权 / 重试 / 401 跳转等包装 —— 上报失败时再触发跳转或重试，
    //    就变成"错误上报引发更多错误"的循环；
    // 2) 这个端点不要求登录（未登录白屏恰恰最需要被记录）。
    const base = getApiBaseUrl(useRuntimeConfig())
    await fetch(`${base}/api/v1/client-error`, {
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
 * 返回卸载函数，便于测试里复原。
 */
export function installClientErrorReporter(): () => void {
  if (typeof window === 'undefined') return () => {}

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
