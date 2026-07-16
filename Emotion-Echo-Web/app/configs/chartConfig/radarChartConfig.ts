import type { RadarIndicator } from "~/types/charts/radarChartType";

export const radarChartOption = (
  indicators: RadarIndicator[],
  data: number[],
  title?: string
) => ({
  tooltip: {
    trigger: "item",
  },
  radar: {
    indicator: indicators.map((i) => ({ name: i.name, max: i.max })),
    shape: "polygon",
    splitNumber: 5,
    axisName: {
      color: "#333",
    },
    splitLine: {
      lineStyle: {
        color: [
          "rgba(238, 197, 102, 0.1)",
          "rgba(238, 197, 102, 0.2)",
          "rgba(238, 197, 102, 0.4)",
          "rgba(238, 197, 102, 0.6)",
          "rgba(238, 197, 102, 0.8)",
        ].reverse(),
      },
    },
    splitArea: {
      show: false,
    },
    axisLine: {
      lineStyle: {
        color: "rgba(238, 197, 102, 0.5)",
      },
    },
  },
  series: [
    {
      name: title || "雷达图",
      type: "radar",
      data: [
        {
          value: data,
          name: "当前状态",
          areaStyle: {
            color: "rgba(64, 158, 255, 0.3)",
          },
          lineStyle: {
            color: "#409eff",
            width: 2,
          },
          itemStyle: {
            color: "#409eff",
          },
        },
      ],
    },
  ],
});
