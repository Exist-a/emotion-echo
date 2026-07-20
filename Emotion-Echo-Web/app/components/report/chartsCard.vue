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
          v-if="item.chartType === 'pie'"
          :is="pieChart"
          :data="(item as pieChartItem).data"
          :height="300"
          :title="item.title"
        />

        <!-- 折线图 -->
        <component
          v-else-if="item.chartType === 'line'"
          :is="lineChart"
          :height="300"
          :XData="(item as lineChartItem).XData"
          :YData="(item as lineChartItem).YData"
          :seriesData="(item as lineChartItem).seriesData"
          :title="item.title"
        />

        <!-- 柱状图 -->
        <component
          v-else-if="item.chartType === 'bar'"
          :is="barChart"
          :height="300"
          :XData="(item as barChartItem).XData"
          :YData="(item as barChartItem).YData"
          :title="item.title"
        />

        <!-- 雷达图 -->
        <component
          v-else-if="item.chartType === 'radar'"
          :is="radarChart"
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
  () => import("~/components/charts/radarChart.vue")
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
  if (width < 1600) return gridConfig.value.breakpoints.sm; // 2列（992px-1600px）
  return gridConfig.value.breakpoints.md; // 3列（1600px以上）
});

// 计算网格样式
const gridStyle = computed(() => ({
  gridTemplateColumns: `repeat(${currentColumns.value}, 1fr)`,
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
  background-color: #fff;
  border-radius: $radius-lg;
  padding: 24px; // 增加内边距
  box-sizing: border-box;
  margin-top: 40px;

  // 响应式调整
  @media (max-width: 1200px) {
    padding: 20px;
  }

  @media (max-width: 992px) {
    padding: 16px;
  }

  @media (max-width: 768px) {
    padding: 12px;
  }
}

.charts-grid {
  display: grid;
  transition: grid-template-columns 0.3s ease;

  // 中屏设备（992px以下）强制1列
  @media (max-width: 992px) {
    grid-template-columns: 1fr !important;
  }

  // 间距响应式调整
  @media (max-width: 1600px) {
    gap: 20px;
  }

  @media (max-width: 1200px) {
    gap: 16px;
  }

  @media (max-width: 992px) {
    gap: 20px;
  }

  @media (max-width: 768px) {
    gap: 16px;
  }

  @media (max-width: 576px) {
    gap: 12px;
  }
}

.chart-item {
  background: #ffffff;
  border-radius: $radius-lg;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08); // 增加阴影强度
  overflow: hidden;
  transition: all 0.3s ease;
  padding: 20px;
  // 设置最小宽度，确保每个图表有足够空间
  min-width: 0; // 防止内容溢出

  &:hover {
    transform: translateY(-6px); // 增加悬停上移距离
    box-shadow: 0 12px 30px rgba(0, 0, 0, 0.15); // 增强悬停阴影

    // 移动端禁用悬停效果
    @media (max-width: 768px) {
      transform: none;
    }
  }

  // 图表类型特定样式
  :deep(.chart-header) {
    padding: 20px 24px; // 增加内边距
    background: #f8f9fa;
    border-bottom: 1px solid #e9ecef;

    h3 {
      margin: 0;
      font-size: 18px; // 增大字体
      font-weight: 600;
      color: #333;
    }
  }

  // 图表内容区域
  :deep(.chart-content) {
    padding: 20px; // 增加内边距

    @media (max-width: 992px) {
      padding: 16px;
    }
  }

  // 中屏设备优化
  @media (max-width: 992px) {
    margin-bottom: 20px;

    &:last-child {
      margin-bottom: 0;
    }
  }

  // 移动端优化
  @media (max-width: 768px) {
    margin-bottom: 16px;
  }
}

// 空状态占位符
.chart-placeholder {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 300px; // 增加最小高度
  background: #f8f9fa;
  border-radius: $radius-lg;

  @media (max-width: 992px) {
    min-height: 250px;
  }

  @media (max-width: 768px) {
    min-height: 200px;
  }
}

// 动画效果
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.5s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

// 当只有1-2个图表时的特殊布局
.charts-card-container:has(.chart-item:only-child),
.charts-card-container:has(.chart-item:first-child:nth-last-child(2)),
.charts-card-container:has(
    .chart-item:first-child:nth-last-child(2) ~ .chart-item
  ) {
  .charts-grid {
    // 在大屏幕上，当只有1-2个图表时，可以考虑更灵活的布局
    @media (min-width: 1600px) {
      &.chart-count-1 {
        grid-template-columns: repeat(1, minmax(400px, 1fr));
        justify-content: center;
      }

      &.chart-count-2 {
        grid-template-columns: repeat(2, minmax(400px, 1fr));
        justify-content: center;
      }
    }
  }
}

// 暗黑模式适配（由 html.dark 类触发）
html.dark {
  .charts-card-container {
    background-color: #1a1a1a;
  }

  .chart-item {
    background: #242424;
    box-shadow: 0 2px 12px rgba(0, 0, 0, 0.3);

    &:hover {
      box-shadow: 0 12px 30px rgba(0, 0, 0, 0.4);
    }
  }

  .chart-placeholder {
    background: #242424;
  }

  :deep(.chart-header) {
    background: #2d2d2d;
    border-bottom-color: #374151;

    h3 {
      color: #e5e7eb;
    }
  }
}
</style>
