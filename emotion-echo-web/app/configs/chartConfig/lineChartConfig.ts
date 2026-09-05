export const lineChartOption = (
  XData: string[],
  YData: number[],
  title: string | undefined,
  seriesData?: { name: string; data: number[] }[]
) => ({
  tooltip: {
    trigger: "axis",
  },
  legend: {
    data: seriesData ? seriesData.map((s) => s.name) : [title],
    bottom: 0,
  },
  xAxis: {
    type: "category",
    boundaryGap: false,
    data: XData,
  },
  yAxis: {
    type: "value",
  },
  series: seriesData
    ? seriesData.map((s) => ({
        name: s.name,
        data: s.data,
        type: "line",
        smooth: true,
      }))
    : [
        {
          name: title,
          data: YData,
          type: "line",
          areaStyle: {},
        },
      ],
});
