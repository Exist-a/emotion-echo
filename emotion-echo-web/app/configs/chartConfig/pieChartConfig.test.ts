import { describe, it, expect } from 'vitest'
import { pieChartOption } from './pieChartConfig'

// pieChartConfig 公共合同:
//   - 空数据 → 返回 { series: [] } (供 BaseChart 走空态)
//   - 有数据 → option.series[0].type === 'pie', 且 data 顺序与传入一致
//   - 有数据时 option 必须含 color 数组, 长度 >= data.length, 每项为合法 CSS 颜色
//   - legend.textStyle.color 与 pie 默认调色板对齐主品牌色系 (Stage 104 收口)
//   - 任意 data 项 name 为空字符串时, 不能影响整体渲染 (健壮性)
describe('pieChartConfig', () => {
  const sample = [
    { name: '开心', value: 12 },
    { name: '难过', value: 8 },
    { name: '平静', value: 5 },
  ]

  it('returns an empty-series option when data is empty', () => {
    const opt = pieChartOption([], '情绪分布') as any
    expect(opt.series).toEqual([])
  })

  it('returns an empty-series option when data is undefined-like', () => {
    const opt = pieChartOption(null as any, '情绪分布') as any
    expect(opt.series).toEqual([])
  })

  it('renders a pie series with the same data items passed in', () => {
    const opt = pieChartOption(sample, '情绪分布') as any
    expect(Array.isArray(opt.series)).toBe(true)
    expect(opt.series[0].type).toBe('pie')
    expect(opt.series[0].data).toEqual(sample)
  })

  it('exposes a color palette covering all data items (Stage 104 visual fix)', () => {
    const opt = pieChartOption(sample, '情绪分布') as any
    // color 字段可能在 series 顶层或 series[0] 上;两种位置都接受
    const colors = opt.color ?? opt.series[0].color
    expect(Array.isArray(colors)).toBe(true)
    expect(colors.length).toBeGreaterThanOrEqual(sample.length)
    for (const c of colors) {
      // 合法 CSS 颜色 (十六进制 / rgb / hsl)
      expect(c).toMatch(/^(#[0-9a-fA-F]{3,8}|rgba?\(|hsla?\()/)
    }
  })

  it('keeps legend color readable on light surface (no pure white text)', () => {
    const opt = pieChartOption(sample, '情绪分布') as any
    if (opt.legend?.textStyle?.color) {
      const c = String(opt.legend.textStyle.color).toLowerCase()
      expect(c).not.toBe('#fff')
      expect(c).not.toBe('#ffffff')
      expect(c).not.toBe('white')
    }
  })

  it('handles an item with empty name without throwing', () => {
    expect(() => pieChartOption([{ name: '', value: 1 }, ...sample], '情绪分布')).not.toThrow()
  })
})