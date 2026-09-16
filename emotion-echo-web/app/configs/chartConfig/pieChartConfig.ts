import type { pieChartDataItem } from "~/types/charts/pieChartType";

// 主品牌色系 + 兼容辅助色（与 nav 配色对齐：Quiet Companion 绿系）
// 顺序与 data 索引对应，超出长度时 echarts 会循环取色
const PALETTE = [
  "#5f8f7b", // ee-primary
  "#7ba994", // primary-soft 加深
  "#d98773", // ee-accent
  "#8aa892",
  "#b87f6c",
  "#cad9cf",
  "#a3b3a9",
  "#4f7e6a", // primary-hover
];

export const pieChartOption = (
  data: pieChartDataItem[],
  title: string | undefined
) => {
  // 空数据 → BaseChart 走空态
  if (!data || data.length === 0) {
    return { series: [] };
  }

  return {
    responsive: true,
    color: PALETTE,
    tooltip: { trigger: "item" },
    legend: {
      orient: "vertical",
      left: "left",
      textStyle: { color: "#202522" }, // 与 --ee-text 对齐, 暗色模式由 chart theme 处理
    },
    series: [
      {
        name: title,
        type: "pie",
        radius: ["40%", "70%"],
        data,
        itemStyle: {
          borderRadius: 10,
          borderColor: "#ffffff",
          borderWidth: 2,
        },
        label: { show: false },
        labelLine: { show: false },
      },
    ],
  };
};
