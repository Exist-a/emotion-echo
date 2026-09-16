import { describe, it, expect } from 'vitest'
import { trendToChartItems } from './trendReportCharts'
import type { EmotionTrend } from '~/types/api'

// trendToChartItems 公共合同:
//   - null → []
//   - 有 series → 1 条 line chartItem, seriesData 数组, 长度与 trend.series 一致
//   - 有 emotionDistribution → 1 条 pie chartItem, data 名称走 getEmotionLabel
//   - 有 intentDistribution → 1 条 pie chartItem, data 名称走 getIntentLabel
//   - 任意字段缺失时 → 跳过对应 chartItem (不抛错)
describe('trendToChartItems', () => {
  const baseTrend: EmotionTrend = {
    type: 'weekly',
    dates: ['2026-09-10', '2026-09-11', '2026-09-12'],
    series: [
      { name: '开心', data: [1, 2, 3] },
      { name: '难过', data: [0, 1, 1] },
    ],
    summary: 's',
    emotionDistribution: [{ name: 'happy', value: 10 }, { name: 'sad', value: 5 }],
    conversationCount: 7,
    messageCount: 42,
  }

  it('returns [] for null input', () => {
    expect(trendToChartItems(null)).toEqual([])
  })

  it('emits a line chart with seriesData (not flatMap YData)', () => {
    const items = trendToChartItems(baseTrend)
    const line = items.find((i) => i.chartType === 'line') as any
    expect(line).toBeTruthy()
    expect(line.seriesData).toBeDefined()
    expect(line.seriesData.length).toBe(2)
    expect(line.seriesData[0]).toEqual({ name: '开心', data: [1, 2, 3] })
    // YData 必须留空, 让 chartsCard 走 seriesData 路径
    expect(line.YData).toEqual([])
  })

  it('emits a pie chart for emotionDistribution with localized labels', () => {
    const items = trendToChartItems(baseTrend)
    const pies = items.filter((i) => i.chartType === 'pie')
    expect(pies.length).toBeGreaterThanOrEqual(1)
    const emoPie = pies.find((i: any) => i.title === '情绪分布') as any
    expect(emoPie).toBeTruthy()
    // name 应该是 localized label,不是原始 'happy'/'sad'
    const names = emoPie.data.map((d: any) => d.name)
    expect(names.every((n: string) => n !== 'happy' && n !== 'sad')).toBe(true)
  })

  it('emits a pie chart for intentDistribution when present', () => {
    const trend: EmotionTrend = {
      ...baseTrend,
      intentDistribution: [{ intent: 'vent', count: 3 }, { intent: 'advice', count: 2 }],
    }
    const items = trendToChartItems(trend)
    const intentPie = items.find((i: any) => i.title === '意图分布') as any
    expect(intentPie).toBeTruthy()
    expect(intentPie.data.length).toBe(2)
    expect(intentPie.data[0]).toHaveProperty('value')
    expect(intentPie.data[0]).toHaveProperty('name')
  })

  it('skips sections gracefully when fields are missing', () => {
    const partial: EmotionTrend = {
      ...baseTrend,
      emotionDistribution: [],
      intentDistribution: undefined,
    }
    const items = trendToChartItems(partial)
    const line = items.find((i) => i.chartType === 'line')
    expect(line).toBeTruthy()
    // emotionDistribution 为空, 不应产出'情绪分布'饼图
    expect(items.find((i: any) => i.title === '情绪分布')).toBeUndefined()
    expect(items.find((i: any) => i.title === '意图分布')).toBeUndefined()
  })
})