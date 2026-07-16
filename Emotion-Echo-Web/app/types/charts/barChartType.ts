export interface barChartProps {
    
  XData: string[];
  YData: number[];
  width?: number | string;
  height: number | string;
  title?: string;
}
export interface barChartItem {
    title?:string;
  XData: string[];
  YData: number[];
  chartType:"bar";
}
