import { describe, it, expect } from 'vitest'
import { vhToPx } from './vhToPx'

describe('vhToPx', () => {
  it('服务端环境下返回 0（window 未定义）', () => {
    // 在 vitest 的 jsdom 默认环境中有 window；用 mock 模拟 undefined
    const orig = (globalThis as any).window
    // @ts-ignore — 故意删 window
    delete (globalThis as any).window
    try {
      expect(vhToPx(50)).toBe(0)
    } finally {
      ;(globalThis as any).window = orig
    }
  })

  it('正常环境（100vh 视口）下返回正确 px', () => {
    // 默认 jsdom window.innerHeight = 768
    Object.defineProperty(globalThis.window, 'innerHeight', { configurable: true, value: 1080 })
    const got = vhToPx(50) // 50 * 1080 / 100 = 540
    expect(got).toBe(540)
  })

  it('vh=0 → 0', () => {
    Object.defineProperty(globalThis.window, 'innerHeight', { configurable: true, value: 800 })
    expect(vhToPx(0)).toBe(0)
  })

  it('vh=100 = 100% viewportHeight', () => {
    Object.defineProperty(globalThis.window, 'innerHeight', { configurable: true, value: 800 })
    expect(vhToPx(100)).toBe(800)
  })

  it('vh 四舍五入到整数像素', () => {
    Object.defineProperty(globalThis.window, 'innerHeight', { configurable: true, value: 1000 })
    // 33vh = 330 — 整数
    expect(vhToPx(33)).toBe(330)
  })
})
