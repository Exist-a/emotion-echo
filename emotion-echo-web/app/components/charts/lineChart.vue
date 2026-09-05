<template>
  <BaseChart :option="chartOption" :title="title" :height="height" />
</template>

<script setup lang="ts">
import BaseChart from "./BaseChart.vue";
import { lineChartOption } from "~/configs/chartConfig/lineChartConfig";
import type { LineSeriesItem } from "~/types/charts/lineChartType";

interface Props {
  XData: string[];
  YData: number[];
  seriesData?: LineSeriesItem[];
  title?: string;
  height?: number;
}

const props = withDefaults(defineProps<Props>(), {
  height: 300,
});

const chartOption = computed(() =>
  lineChartOption(props.XData, props.YData, props.title, props.seriesData)
);
</script>
