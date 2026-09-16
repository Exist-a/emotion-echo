import type { ChartItem } from '~/types/charts/common'
import type { EmotionTrend } from '~/types/api'
import { getEmotionLabel } from './emotion'
import { getIntentLabel } from './intent'

/**
 * 把 EmotionTrend (周/月/年报表 API 响应) 转成 ChartItem[] 喂给 chartsCard.
 *
 * 修复 Stage 104: 之前 lineChart 接收的是 YData = series.flatMap(s => s.data),
 * 把多条情绪曲线拍平成一维数组,导致图例与曲线对不上. 这里改为传 seriesData.
 */
export function trendToChartItems(trend: EmotionTrend | null): ChartItem[] {
  if (!trend) return []
  const items: ChartItem[] = []

  // 折线图: 多 series 叠加 → seriesData
  if (trend.series?.length) {
    items.push({
      chartType: 'line',
      title: '情绪趋势',
      XData: trend.dates,
      YData: [],
      seriesData: trend.series.map((s) => ({ name: s.name, data: s.data })),
    })
  }

  if (trend.emotionDistribution?.length) {
    items.push({
      chartType: 'pie',
      title: '情绪分布',
      data: trend.emotionDistribution.map((item) => ({
        ...item,
        name: getEmotionLabel(item.name),
      })),
    })
  }

  if (trend.intentDistribution?.length) {
    items.push({
      chartType: 'pie',
      title: '意图分布',
      data: trend.intentDistribution.map((item) => ({
        name: getIntentLabel(item.intent),
        value: item.count,
      })),
    })
  }

  return items
}