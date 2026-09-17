import { describe, it, expect } from 'vitest'
import { lineChartOption } from './lineChartConfig'

// lineChartConfig 公共合同:
//   - 仅传 XData/YData → 1 条 series, name = title
//   - 传 seriesData (多情绪叠加) → N 条 series, 每条 name/data 来自入参
//   - 传 seriesData 时, 不再把多 series 错误拍平为单 series
//   - legend.data 包含所有 series.name (图例/曲线一一对应)
//   - 每条 series.type === 'line'
//   - 仅有 1 条 series 时, 默认带 areaStyle (周/月/年报整体趋势观感)
describe('lineChartConfig', () => {
  it('renders a single-series line when only XData/YData provided', () => {
    const opt = lineChartOption(['周一', '周二', '周三'], [1, 2, 3], '每日趋势') as any
    expect(Array.isArray(opt.series)).toBe(true)
    expect(opt.series.length).toBe(1)
    expect(opt.series[0].type).toBe('line')
    expect(opt.series[0].name).toBe('每日趋势')
    expect(opt.series[0].data).toEqual([1, 2, 3])
  })

  it('renders multiple series when seriesData provided (Stage 104 多情绪叠加修复)', () => {
    const seriesData = [
      { name: '开心', data: [1, 2, 3] },
      { name: '难过', data: [4, 5, 6] },
      { name: '平静', data: [7, 8, 9] },
    ]
    const opt = lineChartOption(['Mon','Tue','Wed'], [], '情绪趋势', seriesData) as any
    expect(opt.series.length).toBe(3)
    for (let i = 0; i < seriesData.length; i++) {
      expect(opt.series[i].type).toBe('line')
      expect(opt.series[i].name).toBe(seriesData[i]!.name)
      expect(opt.series[i].data).toEqual(seriesData[i]!.data)
    }
    // legend.data 必须覆盖所有 series.name (图例/曲线对应)
    expect(opt.legend?.data).toEqual(['开心', '难过', '平静'])
  })

  it('ignores YData when seriesData is provided (prevents data loss)', () => {
    const seriesData = [{ name: '开心', data: [1, 2] }]
    const opt = lineChartOption(['a','b'], [99, 99], 't', seriesData) as any
    expect(opt.series.length).toBe(1)
    expect(opt.series[0].data).toEqual([1, 2]) // 不是 [99, 99]
  })

  it('RED-guard: caller MUST pass seriesData when EmotionTrend.series has >1 entries', () => {
    // 模拟 EmotionTrend: dates + [{name:'开心',data:[1,2,3]},{name:'难过',data:[4,5,6]}]
    // 调用方正确的写法是 seriesData=trend.series (而不是 YData=series.flatMap(...))
    const trendLike = [
      { name: '开心', data: [1, 2, 3] },
      { name: '难过', data: [4, 5, 6] },
    ]
    // 错误写法 (历史 bug): flatMap 把多 series 拍平成一维
    const wrongY = trendLike.flatMap((s) => s.data)
    const wrongOpt = lineChartOption(['Mon','Tue','Wed'], wrongY, 't', undefined) as any
    // 单 series 长度为 6,图例只有 1 个 't',与 2 条实际曲线对不上
    expect(wrongOpt.series.length).toBe(1)
    expect(wrongOpt.series[0].data.length).toBe(6)

    // 正确写法: seriesData=trend.series
    const rightOpt = lineChartOption(['Mon','Tue','Wed'], [], 't', trendLike) as any
    expect(rightOpt.series.length).toBe(2)
    expect(rightOpt.legend?.data).toEqual(['开心', '难过'])
  })

  it('uses areaStyle only for the single-series case', () => {
    const single = lineChartOption(['a'], [1], 't') as any
    expect(single.series[0].areaStyle).toBeTruthy()

    const multi = lineChartOption(['a'], [], 't', [{ name: 'A', data: [1] }]) as any
    expect(multi.series[0].areaStyle).toBeFalsy()
  })

  it('handles empty XData gracefully (no throw)', () => {
    expect(() => lineChartOption([], [], 't')).not.toThrow()
  })
})