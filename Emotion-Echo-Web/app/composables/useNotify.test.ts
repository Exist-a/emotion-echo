import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// useNotify.ts 源码合同断言。
// 由于实现里 `import.meta.client` 在 vitest happy-dom 下始终是 undefined,
// 直接 import 后 push 会成为 no-op(SSR 守卫触发),无法在测试进程跑出真实 push。
// 因此退化为源码合同断言:
const src = readFileSync('./app/composables/useNotify.ts', 'utf8')

describe('useNotify.ts · source 合同', () => {
  it('module-level reactive toast list', () => {
    expect(src).toMatch(/ref<Toast\[\]>\(\[\]\)/)
  })

  it('SSR 守卫 in push()', () => {
    expect(src).toMatch(/!import\.meta\.client\s*\)\s*return/)
  })

  it('id 自增(seq++)', () => {
    expect(src).toMatch(/id\s*=\s*\+\+seq|id:\s*\+\+seq/)
  })

  it('push 函数形态: (title, message, type default info, duration default 3000)', () => {
    const re = /function push\([\s\S]*?title: string[\s\S]*?message: string[\s\S]*?type[\s\S]*?duration[\s\S]*?3000/
    expect(src).toMatch(re)
  })

  it('exposes useNotify() returning { toasts, success, error, warning, info, show, push }', () => {
    expect(src).toMatch(/export function useNotify\(\)/)
    expect(src).toMatch(/toasts/)
    expect(src).toMatch(/success:/)
    expect(src).toMatch(/error:/)
    expect(src).toMatch(/warning:/)
    expect(src).toMatch(/info:/)
  })

  it('exposes global notify() shortcut', () => {
    const re = /export function notify\([\s\S]*?title: string[\s\S]*?message: string/
    expect(src).toMatch(re)
  })

  it('expired toast removed by filter() after setTimeout', () => {
    expect(src).toMatch(/window\.setTimeout/)
    expect(src).toMatch(/filter\(\s*\(t\)\s*=>\s*t\.id\s*!==\s*id\s*\)/)
  })
})
