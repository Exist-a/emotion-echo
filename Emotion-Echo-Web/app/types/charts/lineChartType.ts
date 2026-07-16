export interface LineSeriesItem {
  name: string;
  data: number[];
}

export interface lineChartProps {
  XData: string[];
  YData: number[];
  seriesData?: LineSeriesItem[]; // 多条线数据，传此值时 YData 被忽略
  width?: number | string;
  height: number | string;
  title?: string;
}
export interface lineChartItem {
  title?: string;
  XData: string[];
  YData: number[];
  seriesData?: LineSeriesItem[];
  chartType: "line";
}
