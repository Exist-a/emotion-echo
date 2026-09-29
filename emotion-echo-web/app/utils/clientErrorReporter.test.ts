// clientErrorReporter.test.ts — 前端错误上报契约（E2E-F-148）
//
// 这组用例守三条线，任一破掉都会造成真实事故：
//  1. 上报本身**永不抛错** —— 否则"上报失败 → 抛新错 → 再上报"形成循环
//  2. 同一条错误在窗口内**只报一次** —— 死循环的组件能把日志瞬间打爆
//  3. payload **必须截断** —— 前端常把整段 HTML / 大对象塞进 error 对象
//
// 为什么用静态源扫描 + fetch 替身而非要真实浏览器：
//  happy-dom 下 window.addEventListener 与 ErrorEvent 的行为与真实浏览器有差
// （见 E2E-11 的教训），而本组要验的是"我们自己的代码有没有做对"，不是浏览器
// 有没有派发事件 —— 所以只测我们能控制的那一层：函数行为 + 落库前的内容。

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))
const SRC = readFileSync(join(__dirname, 'clientErrorReporter.ts'), 'utf-8')
// 只看代码行：断言"不得出现某写法"时，注释里为了说明原因提到该写法会造成误判
// （本文件已因这个吃过两次亏，故固化为 helper 而不是逐次改写措辞）
const SRC_CODE = SRC.split(/\r?\n/)
  .filter(l => !l.trim().startsWith('*') && !l.trim().startsWith('//') && !l.trim().startsWith('/*'))
  .join('\n')

// 模块内部状态（DEDUP_WINDOW 缓存）跨用例会串，统一用 vi.resetModules 隔离
async function freshModule() {
  vi.resetModules()
  return await import('./clientErrorReporter')
}

describe('clientErrorReporter · 静态源契约', () => {
  it('不得在 window 环境之外注册监听（SSR 会崩）', () => {
    expect(SRC).toContain("typeof window === 'undefined'")
  })

  it('上报端点只拼 /client-error（baseUrl 已含 /api/v1 前缀，不能拼两次）', () => {
    // 拼两次 ⇒ 打到 /api/v1/api/v1/client-error ⇒ 落 catch-all route 100
    // ⇒ jwt-auth 401。2026-09-29 IAB 实测抓到，curl 测不出来。
    expect(SRC_CODE).toContain('${baseUrl}/client-error')
    expect(SRC_CODE).not.toContain('${baseUrl}/api/v1/client-error')
    expect(SRC_CODE).toContain("method: 'POST'")
  })

  // 2026-09-29 IAB 实测：base URL 必须在**安装时**取。原先在 window 事件回调里
  // 现调 useRuntimeConfig()，不在 Nuxt 上下文 ⇒ 抛错 ⇒ 被 try/catch 静默吞掉
  // ⇒ 表现为「监听器装了却一条都发不出去」，而单测 stub 掉了 useRuntimeConfig
  // 所以完全测不出来（IAB 真实事件才抓到）。
  it('base URL 由 install 时传入，事件回调里不得再取 runtimeConfig', () => {
    expect(SRC_CODE).toMatch(/export function installClientErrorReporter\(apiBase: string\)/)
    expect(SRC_CODE).not.toContain('useRuntimeConfig(')
    expect(SRC_CODE).not.toContain('getApiBaseUrl(')
  })

  it('用裸 fetch 而非 useApi（避免鉴权包装/401 跳转造成上报循环）', () => {
    expect(SRC).toContain('await fetch(')
    expect(SRC).not.toMatch(/useApi\(\)\.post|\{\s*\$fetch\s*\}/)
  })


  it('上报调用外层有 try/catch —— 绝不 reject', () => {
    // 唯一的 fetch 调用必须处在 try 内（文件里 catch 分支存在且吞掉）
    expect(SRC).toMatch(/catch\s*\{[\s\S]*?\n\s*\}/)
    const tryIdx = SRC.indexOf('export async function reportClientError')
    const body = SRC.slice(tryIdx, SRC.indexOf('export function installClientErrorReporter'))
    expect(body).toContain('try {')
    expect(body).toContain('catch')
  })
})

describe('clientErrorReporter · 去重与截断行为', () => {
  let fetchMock: ReturnType<typeof vi.fn>

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue({ ok: true })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('useRuntimeConfig', () => ({
      public: { API_BASE_URL: 'http://localhost:19080/api/v1' }
    }))
    vi.stubGlobal('window', {
      location: { href: 'http://localhost:3000/chat' },
      addEventListener: vi.fn(),
      removeEventListener: vi.fn()
    })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('未安装（baseUrl 空）时静默放弃上报，不抛给调用方', async () => {
    const { reportClientError } = await freshModule() // 故意不调 install
    await expect(reportClientError('error', new Error('boom'))).resolves.toBeUndefined()
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it('同一条错误在去重窗口内只发一次', async () => {
    const { reportClientError } = await install()
    await reportClientError('error', new Error('boom'))
    await reportClientError('error', new Error('boom'))
    await reportClientError('error', new Error('boom'))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('不同错误各报一次', async () => {
    const { reportClientError } = await install()
    await reportClientError('error', new Error('a'))
    await reportClientError('error', new Error('b'))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  /** 取第 n 次 fetch 调用的 body（mock.calls 元素在 strict 下可能是 undefined，故收口到一处断言） */
  const API_BASE = 'http://localhost:19080/api/v1'

  /** 先装上报器再触发（与生产同序：plugin 里装 → 之后才有事件） */
  async function install() {
    const mod = await freshModule()
    mod.installClientErrorReporter(API_BASE)
    return mod
  }

  function bodyOf(n: number): any {
    const call = fetchMock.mock.calls[n]
    if (!call) throw new Error(`fetch 未被调用第 ${n} 次`)
    const init = call[1] as RequestInit | undefined
    if (!init || typeof init.body !== 'string') throw new Error('fetch 未带 string body')
    return JSON.parse(init.body)
  }

  it('超大 message / stack 被截断', async () => {
    const { reportClientError } = await install()
    const huge = 'x'.repeat(50_000)
    await reportClientError('error', { message: huge, stack: huge })
    const body = bodyOf(0)
    expect(body.msg.length).toBeLessThan(2100)
    expect(body.msg).toContain('truncated')
    expect(body.stack.length).toBeLessThan(4100)
  })

  it('fetch 失败时静默吞掉，不 reject', async () => {
    fetchMock.mockRejectedValue(new Error('network down'))
    const { reportClientError } = await install()
    await expect(reportClientError('error', new Error('boom'))).resolves.toBeUndefined()
  })

  it('最终 URL 恰为 <API_BASE>/client-error，无重复前缀', async () => {
    const { reportClientError } = await install()
    await reportClientError('error', new Error('url-shape-check'))
    const call = fetchMock.mock.calls[0]
    if (!call) throw new Error('fetch 未被调用')
    expect(call[0]).toBe(API_BASE + '/client-error')
  })

  it('上报内容带 kind/url/line/col，便于在 Loki 里按维度筛', async () => {
    const { reportClientError } = await install()
    await reportClientError('unhandledrejection', new Error('nope'), {
      url: 'http://localhost:3000/dashboard',
      line: 12,
      col: 3
    })
    const body = bodyOf(0)
    expect(body.kind).toBe('unhandledrejection')
    expect(body.url).toBe('http://localhost:3000/dashboard')
    expect(body.line).toBe(12)
    expect(body.col).toBe(3)
  })
})
