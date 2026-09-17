<template>
  <div class="charts-card-container">
    <div class="charts-grid" :style="gridStyle">
      <div
        v-for="(item, index) in data"
        :key="item.title || index"
        class="chart-item"
      >
        <!-- 饼图 -->
        <component
          :is="pieChart"
          v-if="item.chartType === 'pie'"
          :data="(item as pieChartItem).data"
          :height="300"
          :title="item.title"
        />

        <!-- 折线图 -->
        <component
          :is="lineChart"
          v-else-if="item.chartType === 'line'"
          :height="300"
          :XData="(item as lineChartItem).XData"
          :YData="(item as lineChartItem).YData"
          :series-data="(item as lineChartItem).seriesData"
          :title="item.title"
        />

        <!-- 柱状图 -->
        <component
          :is="barChart"
          v-else-if="item.chartType === 'bar'"
          :height="300"
          :XData="(item as barChartItem).XData"
          :YData="(item as barChartItem).YData"
          :title="item.title"
        />

        <!-- 雷达图 -->
        <component
          :is="radarChart"
          v-else-if="item.chartType === 'radar'"
          :height="300"
          :indicators="(item as RadarChartItem).indicators"
          :data="(item as RadarChartItem).data"
          :title="item.title"
        />

        <!-- 未知图表类型 -->
        <div v-else class="chart-placeholder">
          <div class="ee-empty">暂无数据</div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import {
  defineAsyncComponent,
  computed,
  ref,
  onMounted,
  onUnmounted,
} from "vue";
import type { ChartItem } from "~/types/charts/common";
import type { pieChartItem } from "~/types/charts/pieChartType";
import type { lineChartItem } from "~/types/charts/lineChartType";
import type { barChartItem } from "~/types/charts/barChartType";
import type { RadarChartItem } from "~/types/charts/radarChartType";

// 定义props
const props = defineProps<{
  data: ChartItem[];
}>();

// 动态导入图表组件
const pieChart = defineAsyncComponent(
  () => import("~/components/charts/pieChart.vue")
);

const lineChart = defineAsyncComponent(
  () => import("~/components/charts/lineChart.vue")
);

const barChart = defineAsyncComponent(
  () => import("~/components/charts/barChart.vue")
);

const radarChart = defineAsyncComponent(
  () => import("~/components/charts/RadarChart.vue")
);

// 响应式布局配置 - 调整断点使最小宽度更大
const gridConfig = ref({
  columns: 3, // 默认3列（最多3列）
  gap: "24px", // 增加间距
  breakpoints: {
    xs: 1, // < 992px 显示1列（原768px调整到992px）
    sm: 2, // 992px - 1600px 显示2列
    md: 3, // >= 1600px 显示3列
  },
});

// 计算当前列数（响应式）- 调整断点值
const currentColumns = computed(() => {
  if (typeof window === "undefined") return gridConfig.value.columns;

  const width = window.innerWidth;

  // 调整断点：最小宽度更大
  if (width < 992) return gridConfig.value.breakpoints.xs; // 1列（992px以下）
  if (width < 1600) return gridConfig.value.breakpoints.sm; // 2列（992-1600px）
  return gridConfig.value.breakpoints.md; // 3列（1600px以上）
});

// 计算网格样式
const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${currentColumns.value}, minmax(0, 1fr))`,
  gap: gridConfig.value.gap,
}));

// 窗口大小变化处理函数
const handleResize = () => {
  // currentColumns是计算属性，会自动更新
};

// 监听窗口大小变化
onMounted(() => {
  if (typeof window !== "undefined") {
    window.addEventListener("resize", handleResize);
  }
});

onUnmounted(() => {
  if (typeof window !== "undefined") {
    window.removeEventListener("resize", handleResize);
  }
});

// 获取图表组件（备用方法）
const getChartComponent = (type: string) => {
  switch (type) {
    case "pie":
      return pieChart;
    case "line":
      return lineChart;
    case "bar":
      return barChart;
    case "radar":
      return radarChart;
    default:
      return null;
  }
};

// 暴露方法给父组件
defineExpose({
  updateGridConfig: (config: Partial<typeof gridConfig.value>) => {
    Object.assign(gridConfig.value, config);
  },
  getChartComponent,
});
</script>

<style scoped lang="scss">
.charts-card-container {
  width: 100%;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  padding: 24px;
  box-sizing: border-box;
  margin-top: 24px;

  @media (max-width: 1200px) { padding: 20px; }
  @media (max-width: 992px) { padding: 16px; }
  @media (max-width: 768px) { padding: 12px; }
}

.charts-grid {
  display: grid;
  transition: grid-template-columns 0.3s ease;

  // 中屏设备（992px以下）单列由 JS currentColumns + gridStyle 接管, CSS 不重复
  @media (max-width: 992px) { gap: 20px; }
  @media (max-width: 768px) { gap: 16px; }
  @media (max-width: 576px) { gap: 12px; }
}

.chart-item {
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: var(--ee-shadow-soft);
  overflow: hidden;
  transition: transform 0.3s ease, box-shadow 0.3s ease;
  padding: 20px;
  min-width: 0;

  &:hover {
    transform: translateY(-4px);
    box-shadow: var(--ee-shadow-soft);

    @media (max-width: 768px) { transform: none; }
  }

  :deep(.chart-header) {
    padding: 16px 20px;
    background: var(--ee-surface-muted);
    border-bottom: 1px solid var(--ee-border);

    h3 {
      margin: 0;
      font-size: 16px;
      font-weight: 600;
      color: var(--ee-text);
    }
  }

  :deep(.chart-content) {
    padding: 16px;

    @media (max-width: 992px) { padding: 12px; }
  }

  @media (max-width: 992px) {
    margin-bottom: 16px;
    &:last-child { margin-bottom: 0; }
  }
  @media (max-width: 768px) { margin-bottom: 12px; }
}

.chart-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px;
  background: var(--ee-surface-muted);
  border-radius: var(--ee-radius-lg);

  @media (max-width: 992px) { min-height: 250px; }
  @media (max-width: 768px) { min-height: 200px; }
}

.fade-enter-active,
.fade-leave-active { transition: opacity 0.5s ease; }
.fade-enter-from,
.fade-leave-to { opacity: 0; }

// 单/双图表时大屏居中布局由 chartsCard.vue script 的 countClass 动态加类
// 暗色模式由 html.dark 下 global.scss 重定义 --ee-* token 接管,此处不重复
</style>
