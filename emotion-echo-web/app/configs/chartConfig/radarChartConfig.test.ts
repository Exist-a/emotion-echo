// E2E-14：雷达图必须为轴标签留出空间
//
// 实测（2026-09-20）：结果弹窗与我的空间在窄视口（Pixel 5，393px）下，
// 轴标签「尽责性」「神经质」「外向性」被裁成「性」「神」「向性」。
// 根因：radarChartOption 未设 radar.radius，ECharts 默认取容器较小边的 ~75~80%，
// 五边形画满后标签无处可放。
//
// 契约：radius 必须显式设为 ≤68%（标签空间），且 axisName 字号收敛。
import { describe, it, expect } from 'vitest'
import { radarChartOption } from './radarChartConfig'

const INDICATORS = [
  { name: '开放性', max: 30 },
  { name: '尽责性', max: 30 },
  { name: '外向性', max: 30 },
  { name: '宜人性', max: 30 },
  { name: '神经质', max: 30 },
]

describe('radarChartOption', () => {
  it('显式设置 radius，为轴标签预留空间（否则窄视口标签被裁剪）', () => {
    const opt = radarChartOption(INDICATORS, [18, 18, 18, 18, 18], '人格维度') as any
    expect(opt.radar.radius, 'radar.radius 必须显式声明').toBeDefined()
    const pct = parseInt(String(opt.radar.radius), 10)
    expect(Number.isNaN(pct), 'radius 应为百分比字符串').toBe(false)
    expect(pct, 'radius 需 ≤68% 才能容纳中文轴标签').toBeLessThanOrEqual(68)
    expect(pct, 'radius 也不应过小（图形可读性）').toBeGreaterThanOrEqual(45)
  })

  it('轴标签字号收敛到 12px 以内（避免长中文标签溢出）', () => {
    const opt = radarChartOption(INDICATORS, [1, 2, 3, 4, 5]) as any
    const fontSize = opt.radar.axisName.fontSize
    expect(fontSize, 'axisName.fontSize 必须显式声明').toBeDefined()
    expect(fontSize).toBeLessThanOrEqual(12)
  })

  it('正常传递 indicator 与 data', () => {
    const opt = radarChartOption(INDICATORS, [6, 10, 22, 18, 30]) as any
    expect(opt.radar.indicator).toHaveLength(5)
    expect(opt.radar.indicator[0]).toEqual({ name: '开放性', max: 30 })
    expect(opt.series[0].data[0].value).toEqual([6, 10, 22, 18, 30])
    expect(opt.series[0].type).toBe('radar')
  })
})
