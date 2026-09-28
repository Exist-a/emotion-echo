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

// 模块内部状态（DEDUP_WINDOW 缓存）跨用例会串，统一用 vi.resetModules 隔离
async function freshModule() {
  vi.resetModules()
  return await import('./clientErrorReporter')
}

describe('clientErrorReporter · 静态源契约', () => {
  it('不得在 window 环境之外注册监听（SSR 会崩）', () => {
    expect(SRC).toContain("typeof window === 'undefined'")
  })

  it('上报端点固定为 /api/v1/client-error', () => {
    expect(SRC).toContain('/api/v1/client-error')
    expect(SRC).toContain("method: 'POST'")
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

  it('同一条错误在去重窗口内只发一次', async () => {
    const { reportClientError } = await freshModule()
    await reportClientError('error', new Error('boom'))
    await reportClientError('error', new Error('boom'))
    await reportClientError('error', new Error('boom'))
    expect(fetchMock).toHaveBeenCalledTimes(1)
  })

  it('不同错误各报一次', async () => {
    const { reportClientError } = await freshModule()
    await reportClientError('error', new Error('a'))
    await reportClientError('error', new Error('b'))
    expect(fetchMock).toHaveBeenCalledTimes(2)
  })

  /** 取第 n 次 fetch 调用的 body（mock.calls 元素在 strict 下可能是 undefined，故收口到一处断言） */
  function bodyOf(n: number): any {
    const call = fetchMock.mock.calls[n]
    if (!call) throw new Error(`fetch 未被调用第 ${n} 次`)
    const init = call[1] as RequestInit | undefined
    if (!init || typeof init.body !== 'string') throw new Error('fetch 未带 string body')
    return JSON.parse(init.body)
  }

  it('超大 message / stack 被截断', async () => {
    const { reportClientError } = await freshModule()
    const huge = 'x'.repeat(50_000)
    await reportClientError('error', { message: huge, stack: huge })
    const body = bodyOf(0)
    expect(body.msg.length).toBeLessThan(2100)
    expect(body.msg).toContain('truncated')
    expect(body.stack.length).toBeLessThan(4100)
  })

  it('fetch 失败时静默吞掉，不 reject', async () => {
    fetchMock.mockRejectedValue(new Error('network down'))
    const { reportClientError } = await freshModule()
    await expect(reportClientError('error', new Error('boom'))).resolves.toBeUndefined()
  })

  it('上报内容带 kind/url/line/col，便于在 Loki 里按维度筛', async () => {
    const { reportClientError } = await freshModule()
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
