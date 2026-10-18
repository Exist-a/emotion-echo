// app/utils/dropdownLayout.test.ts
//
// CL-06 · UI 轮：浮层定位的**纯函数**契约。
//
// 为什么单独抽出来测：这段"往下放不下就翻到上面"的算术是浮层最容易悄悄坏掉的部分
// —— 面板贴到窗口底边、被裁一半、或者翻上去顶出屏幕，都不会报错，只有人眼看着不对。
// 抽出纯函数后，边界（正好放得下 / 正好放不下 / 上下都窄）都能钉住。
import { describe, it, expect } from 'vitest'
import { computeDropdownLayout } from './dropdownLayout'

const anchor = (top: number, bottom: number) => ({ top, bottom, left: 100, width: 240 })

describe('computeDropdownLayout · 向下展开', () => {
  it('下方空间充裕 ⇒ 贴在锚点下方，高度用满上限', () => {
    const l = computeDropdownLayout(anchor(100, 140), 720)
    expect(l.flipUp).toBe(false)
    expect(l.top).toBe(146) // bottom + gap
    expect(l.bottom).toBe('auto')
    expect(l.maxHeight).toBe(264)
  })

  it('下方刚好等于上限也算充裕（不无谓地翻上去）', () => {
    const l = computeDropdownLayout(anchor(100, 450), 720) // below = 720-450-6 = 264
    expect(l.flipUp).toBe(false)
    expect(l.maxHeight).toBe(264)
  })
})

describe('computeDropdownLayout · 空间不够时翻到上方', () => {
  it('下方比上限窄且上方更宽裕 ⇒ 向上展开，且高度取上方可用空间', () => {
    const l = computeDropdownLayout(anchor(500, 540), 720) // below=174, above=494
    expect(l.flipUp).toBe(true)
    expect(l.top).toBe('auto')
    expect(l.maxHeight).toBe(264)
    // 面板底边落在锚点上方 gap 处 ⇒ 到底边的距离 = 720 - (500 - 6) = 226
    expect(l.bottom).toBe(226)
  })

  it('上方空间也要被上限截断（不会因为上方很高就长出一整屏）', () => {
    const l = computeDropdownLayout(anchor(700, 714), 720) // above=694
    expect(l.flipUp).toBe(true)
    expect(l.maxHeight).toBe(264)
  })

  it('上方没比下方宽裕 ⇒ 不翻（翻了只会更糟）', () => {
    const l = computeDropdownLayout(anchor(60, 100), 720) // below=614, above=54
    expect(l.flipUp).toBe(false)
  })
})

describe('computeDropdownLayout · 窗口极窄时不越界', () => {
  it('上下都窄 ⇒ 仍给出可用的最小高度，且不把面板顶出屏幕', () => {
    const l = computeDropdownLayout(anchor(200, 240), 260) // below=14, above=194
    expect(l.flipUp).toBe(true)
    expect(l.maxHeight).toBeGreaterThanOrEqual(132) // 最小可读高度
    expect(l.bottom).toBeGreaterThanOrEqual(0)
  })

  it('锚点贴底且上方也窄 ⇒ 不产生负高度', () => {
    const l = computeDropdownLayout(anchor(250, 258), 260)
    expect(l.maxHeight).toBeGreaterThan(0)
    expect(Number.isFinite(l.maxHeight)).toBe(true)
  })
})
