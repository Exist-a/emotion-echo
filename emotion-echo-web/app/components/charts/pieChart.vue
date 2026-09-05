<template>
  <BaseChart :option="chartOption" :title="title" :height="height" />
</template>

<script setup lang="ts">
import BaseChart from "./BaseChart.vue";
import { pieChartOption } from "~/configs/chartConfig/pieChartConfig";
import type { pieChartDataItem } from "~/types/charts/pieChartType";

interface Props {
  data: pieChartDataItem[];
  title?: string;
  height?: number;
}

const props = withDefaults(defineProps<Props>(), {
  height: 300,
});

const chartOption = computed(() => pieChartOption(props.data, props.title));
</script>
