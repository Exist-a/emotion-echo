export interface RadarIndicator {
  name: string;
  max: number;
}

export interface RadarChartProps {
  indicators: RadarIndicator[];
  data: number[];
  title?: string;
  height?: number;
}

export interface RadarChartItem {
  indicators: RadarIndicator[];
  data: number[];
  chartType: "radar";
  title?: string;
}
