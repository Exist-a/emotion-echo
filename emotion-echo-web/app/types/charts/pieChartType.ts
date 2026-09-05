// 引入 ECharts 官方类型（无需自定义 Instance）
import type { ECharts, EChartsOption } from 'echarts';

// 饼图单个数据项类型
export interface pieChartDataItem {
  name: string;
  value: number;
  color?: string;
}

// 饼图组件 Props 类型（简化 customOption 为官方 EChartsOption）
// 必须要宽高，在父卡片元素里获取获取尺寸传入
export interface pieChartProps{
  data: pieChartDataItem[];
  width?: number|string;
  height: number|string;
  title?: string;
}

export interface pieChartItem{
  data: pieChartDataItem[];
  chartType:"pie";
  title?:string;
}
// 复用官方 ECharts 实例类型，无需自定义
export type EChartsInstance = ECharts;