<template>
  <ClientOnly>
    <h3 v-if="title" style="text-align: center">{{ title }}</h3>
    <div
      v-if="hasData"
      ref="containerRef"
      class="chart-container"
      :style="{ margin: '0 auto', height: `${height}px` }"
    >
      <VChartFull
        ref="vChartRef"
        :option="mergedOption"
        :init-option="initOption"
        :theme="isDark ? 'dark' : undefined"
        style="width: 100%; height: 100%"
      />
    </div>
    <div class="ee-empty">暂无数据</div>
  </ClientOnly>
</template>

<script setup lang="ts">
import type { EChartsOption } from "echarts";

interface Props {
  option: EChartsOption;
  title?: string;
  height?: number;
}

const props = withDefaults(defineProps<Props>(), {
  height: 300,
  title: "",
});

const containerRef = ref<HTMLDivElement | null>(null);
const vChartRef = ref<any>(null);

const initOption = {
  renderer: "canvas",
  useDirtyRect: false,
};

// 检测是否处于暗黑模式
const isDark = ref(false);

const checkDarkMode = () => {
  isDark.value = document.documentElement.classList.contains("dark");
};

// 监听暗黑模式变化
onMounted(() => {
  checkDarkMode();

  // 监听 html 元素的 class 变化
  const observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.type === "attributes" && mutation.attributeName === "class") {
        checkDarkMode();
      }
    }
  });

  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class"],
  });

  // 初始化时 resize
  if (vChartRef.value && containerRef.value) {
    const { width, height } = containerRef.value.getBoundingClientRect();
    vChartRef.value.resize({ width, height });
  }
});

// 检查是否有数据
const hasData = computed(() => {
  console.log('[BaseChart] props.option:', JSON.stringify(props.option, null, 2))
  if (!props.option) {
    console.log('[BaseChart] option is null/undefined')
    return false
  }
  const opt = props.option as any;

  // 如果 series 不存在或为空数组，视为无数据
  if (!opt.series || (Array.isArray(opt.series) && opt.series.length === 0)) {
    console.log('[BaseChart] series is empty or not exist')
    return false;
  }

  // 检查 series 是否有数据
  const series = Array.isArray(opt.series) ? opt.series : [opt.series];
  console.log('[BaseChart] series length:', series.length)
  const result = series.some((s: any) => {
    if (s.data && Array.isArray(s.data)) {
      console.log('[BaseChart] series item data length:', s.data.length)
      return s.data.length > 0;
    }
    return false;
  });
  console.log('[BaseChart] hasData result:', result)
  return result;
});

// 直接使用原始 option，ECharts dark 主题会自动处理
const mergedOption = computed(() => {
  return props.option || {};
});

// 监听尺寸变化
watch(
  () => props.height,
  () => {
    if (vChartRef.value && containerRef.value) {
      const { width } = containerRef.value.getBoundingClientRect();
      vChartRef.value.resize({ width, height: props.height });
    }
  }
);
</script>

<style scoped>
.chart-container {
  /* 确保容器背景透明，让父容器的主题背景生效 */
  background: transparent;
}
</style>
