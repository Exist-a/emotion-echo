import type { pieChartDataItem } from "~/types/charts/pieChartType";

export const pieChartOption = (
  data: pieChartDataItem[],
  title: string | undefined
) => {
  // 空数据检查 - 返回空状态配置
  if (!data || data.length === 0) {
    return {
      series: [],
    };
  }

  return {
    responsive: true,
    tooltip: { trigger: "item" },
    legend: {
      orient: "vertical",
      left: "left",
    },
    series: [
      {
        name: title,
        type: "pie",
        radius: ["40%", "70%"],
        data: data,
        itemStyle: {
          borderRadius: 10,
          borderColor: "#fff",
          borderWidth: 2,
        },
        label: {
          show: false,
        },

        labelLine: {
          show: false,
        },
      },
    ],
  };
};
