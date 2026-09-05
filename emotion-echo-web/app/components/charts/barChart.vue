<template>
  <BaseChart :option="chartOption" :title="title" :height="height" />
</template>

<script setup lang="ts">
import BaseChart from "./BaseChart.vue";
import { barChartOption } from "~/configs/chartConfig/barChartConfig";

interface Props {
  XData: string[];
  YData: number[];
  title?: string;
  height?: number;
}

const props = withDefaults(defineProps<Props>(), {
  height: 300,
});

const chartOption = computed(() => barChartOption(props.XData, props.YData, props.title));
</script>
