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

// ============================================================================
// E2E-F-165（2026-09-30 IAB 复验抓到）：图例与圆环**实际相交**。
//
// 现场：/chat/user「昼夜使用模式」卡片。DOM 快照完全正常（标题、canvas 都在），
// canvas 像素分析也证明画布非空 —— **只有截图能看出来**。这正是
// "有 canvas + 有像素 ≠ 版面正确"（memory: frontend-visual-evidence-failure-modes）。
//
// 根因：legend 是 `orient:'vertical', left:'left'`，饼图用默认
// `center:['50%','50%']` + `radius` 最大 70%。卡片窄（约 266×288）时，
// 圆环左侧必然压到左下角那条竖排图例上。
//
// 这里的断言不是"断言配置长什么样"，而是**算几何**：把 option 换算成
// 「圆环外接矩形」与「图例占位矩形」，要求两者不相交。改配置只要不满足
// 几何约束就红 —— 无论你怎么调 center / radius / legend。
// ============================================================================
describe('pieChartConfig · 图例不得与圆环重叠（E2E-F-165）', () => {
  // 真实卡片尺寸：/chat/user 三列栅格下 canvas 约 266×288（IAB 实测）
  const CARD_W = 266
  const CARD_H = 288
  const TITLE_H = 45 // BaseChart 顶部标题带高度
  const LEGEND_ROW_H = 16 // 单行图例高度
  const LEGEND_GAP = 4

  const dayNight = [
    { name: '凌晨 (0:00-6:00)', value: 3 },
    { name: '上午 (6:00-12:00)', value: 41 },
    { name: '下午 (12:00-18:00)', value: 19 },
    { name: '夜间 (18:00-24:00)', value: 33 },
  ]

  // ECharts 的 radius 可能是单值或 [内, 外] 数组；这里统一取**外半径**（末位）
  const outerRadiusOf = (v: unknown, fallback = 70): number => {
    const raw = Array.isArray(v) ? v[v.length - 1] : v
    const m = typeof raw === 'string' ? raw.match(/([\d.]+)%/) : null
    return m ? parseFloat(m[1]!) : fallback
  }
  const pctOf = (v: string | number | undefined, fallback = 50): number => {
    const m = typeof v === 'string' ? v.match(/([\d.]+)%/) : null
    return m ? parseFloat(m[1]!) : fallback
  }

  it('圆环外接矩形与图例占位矩形不相交', () => {
    const opt = pieChartOption(dayNight, '昼夜使用模式') as any
    const s = opt.series[0]
    const legend = opt.legend ?? {}

    // 图例按最坏情况估高：横向会换行，竖向按条目数
    const perItemW = 14 + Math.max(...dayNight.map((d) => d.name.length)) * 7
    const itemsPerRow = Math.max(1, Math.floor(CARD_W / perItemW))
    const legendRows = Math.ceil(dayNight.length / itemsPerRow)
    const legendH = legendRows * LEGEND_ROW_H

    // 竖排图例贴着左边且占满绘图区高度 ⇒ 必然与居中圆环相交
    const legendRect =
      legend.orient === 'vertical'
        ? { x: 0, y: TITLE_H, w: perItemW, h: CARD_H - TITLE_H }
        : { x: 0, y: CARD_H - legendH, w: CARD_W, h: legendH }

    // 圆环外接矩形（ECharts 的 radius 百分比以 min(宽, 高) 为基准）
    const outerR = (outerRadiusOf(s.radius) / 100) * Math.min(CARD_W, CARD_H - TITLE_H)
    const cx = (pctOf(s.center?.[0]) / 100) * CARD_W
    const cy = TITLE_H + (pctOf(s.center?.[1]) / 100) * (CARD_H - TITLE_H)
    const pieRect = { x: cx - outerR, y: cy - outerR, w: outerR * 2, h: outerR * 2 }

    const overlap =
      pieRect.x < legendRect.x + legendRect.w &&
      pieRect.x + pieRect.w > legendRect.x &&
      pieRect.y < legendRect.y + legendRect.h &&
      pieRect.y + pieRect.h > legendRect.y

    expect(
      overlap,
      `圆环矩形 ${JSON.stringify(pieRect)} 与图例矩形 ${JSON.stringify(legendRect)} 相交`,
    ).toBe(false)
  })

  it('圆环不得压到标题带', () => {
    const opt = pieChartOption(dayNight, '昼夜使用模式') as any
    const s = opt.series[0]
    const outerR = (outerRadiusOf(s.radius) / 100) * Math.min(CARD_W, CARD_H - TITLE_H)
    const cy = TITLE_H + (pctOf(s.center?.[1]) / 100) * (CARD_H - TITLE_H)
    expect(cy - outerR, '圆环顶端应在标题带下沿之下').toBeGreaterThanOrEqual(TITLE_H)
  })

  it('图例不应竖排贴左（该布局在窄卡片里必然与圆环相交）', () => {
    const opt = pieChartOption(dayNight, '昼夜使用模式') as any
    const isVerticalLeft =
      opt.legend?.orient === 'vertical' && (opt.legend?.left === 'left' || opt.legend?.left === 0)
    expect(isVerticalLeft, '竖排左置图例在 266px 宽卡片里会压住圆环').toBe(false)
  })
})
