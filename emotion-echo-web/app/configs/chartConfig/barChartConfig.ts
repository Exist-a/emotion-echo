// import type { barChartDataItem } from "~/types/charts/barChartType";

export const barChartOption = (
  XData: string[],
  YData: number[],
  title: string | undefined
) => ({
  tooltip: {
    trigger: "axis",
    axisPointer: {
      type: "shadow",
    },
  },
  grid: {
    left: "3%",
    right: "4%",
    bottom: "3%",
    containLabel: true,
  },
  xAxis: [
    {
      type: "category",
      data: XData,
      axisTick: {
        alignWithLabel: true,
      },
    },
  ],
  yAxis: [
    {
      type: "value",
    },
  ],
  series: [
    {
      name: title,
      type: "bar",
      barWidth: "60%",
      data: YData,
    },
  ],
});
