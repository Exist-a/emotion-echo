<template>
  <BaseChart :option="chartOption" :title="title" :height="height" />
</template>

<script setup lang="ts">
import BaseChart from "./BaseChart.vue";
import { radarChartOption } from "~/configs/chartConfig/radarChartConfig";
import type { RadarIndicator } from "~/types/charts/radarChartType";

interface Props {
  indicators: RadarIndicator[];
  data: number[];
  title?: string;
  height?: number;
}

const props = withDefaults(defineProps<Props>(), {
  height: 300,
});

const chartOption = computed(() =>
  radarChartOption(props.indicators, props.data, props.title)
);
</script>
