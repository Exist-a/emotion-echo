import type { pieChartDataItem } from '~/types/charts/pieChartType'

// 主品牌色系 + 兼容辅助色（与 nav 配色对齐：Quiet Companion 绿系）
// 顺序与 data 索引对应，超出长度时 echarts 会循环取色
const PALETTE = [
  '#5f8f7b', // ee-primary
  '#7ba994', // primary-soft 加深
  '#d98773', // ee-accent
  '#8aa892',
  '#b87f6c',
  '#cad9cf',
  '#a3b3a9',
  '#4f7e6a', // primary-hover
]

export const pieChartOption = (data: pieChartDataItem[], title: string | undefined) => {
  // 空数据 → BaseChart 走空态
  if (!data || data.length === 0) {
    return { series: [] }
  }

  return {
    responsive: true,
    color: PALETTE,
    tooltip: { trigger: 'item' },
    // ⚠️ 图例曾用 `orient:'vertical', left:'left'`，而饼图是默认居中 + 70% 半径。
    // 在 /chat/user 的三列窄卡片（约 266×288）里，两者**必然相交** ——
    // 「下午 (12:00-18:00)」「夜间 (18:00-24:00)」两行文字直接压在橙色/浅绿色块上
    // （E2E-F-165，2026-09-30 IAB 实测截图）。DOM 快照与 canvas 像素分析都看不出来，
    // 只有渲染结果能看出。
    // 现改为：图例**横排放底部**（在绘图区之外），饼图上移并收小，
    // 保证圆环外接矩形与图例占位矩形不相交。数值由测试里的几何断言守住。
    legend: {
      orient: 'horizontal',
      left: 'center',
      bottom: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: '#202522', fontSize: 10 }, // 与 --ee-text 对齐, 暗色模式由 chart theme 处理
    },
    series: [
      {
        name: title,
        type: 'pie',
        // center/radius 的取值与"图例横排底部"配套：圆环整体落在
        // [标题带下沿, 图例带上沿] 之间，不与二者相交。
        center: ['50%', '43%'],
        radius: ['24%', '41%'],
        data,
        itemStyle: {
          borderRadius: 10,
          borderColor: '#ffffff',
          borderWidth: 2,
        },
        label: { show: false },
        labelLine: { show: false },
      },
    ],
  }
}
