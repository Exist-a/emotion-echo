import type { pieChartItem } from "../charts/pieChartType";
import type { lineChartItem } from "../charts/lineChartType";
import type { barChartItem } from "./barChartType";
import type { RadarChartItem } from "./radarChartType";

export type ChartItem = pieChartItem | lineChartItem | barChartItem | RadarChartItem;
export type chartType = "pie" | "line" | "bar" | "radar";

// 通用图表基础属性
export interface ChartBaseProps {
  title?: string;
  height?: number;
}
