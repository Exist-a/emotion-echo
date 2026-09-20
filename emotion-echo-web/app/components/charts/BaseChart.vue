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
    <div v-else class="ee-empty">暂无数据</div>
  </ClientOnly>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import type { EChartsOption } from 'echarts'

interface Props {
  option: EChartsOption
  title?: string
  height?: number
}

const props = withDefaults(defineProps<Props>(), {
  height: 300,
  title: '',
})

const containerRef = ref<HTMLDivElement | null>(null)
const vChartRef = ref<any>(null)

const initOption = {
  renderer: 'canvas',
  useDirtyRect: false,
}

// 检测是否处于暗黑模式
const isDark = ref(false)

const checkDarkMode = () => {
  isDark.value = document.documentElement.classList.contains('dark')
}

// 监听暗黑模式变化
onMounted(() => {
  checkDarkMode()

  // 监听 html 元素的 class 变化
  const observer = new MutationObserver((mutations) => {
    for (const mutation of mutations) {
      if (mutation.type === 'attributes' && mutation.attributeName === 'class') {
        checkDarkMode()
      }
    }
  })

  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })

  // 初始化时 resize (echarts 实例可能尚未挂载或 SSR 上下文, 静默兜底)
  if (vChartRef.value && containerRef.value && typeof vChartRef.value.resize === 'function') {
    try {
      const { width, height } = containerRef.value.getBoundingClientRect()
      vChartRef.value.resize({ width, height })
    } catch {
      /* noop: 测试环境或 SSR 下无真实 echarts 实例, 跳过 resize */
    }
  }
})

// 检查是否有数据
const hasData = computed(() => {
  if (!props.option) return false
  const opt = props.option as any

  // series 不存在或为空数组 → 无数据
  if (!opt.series || (Array.isArray(opt.series) && opt.series.length === 0)) {
    return false
  }

  // 任意一条 series 含非空 data 数组即视为有数据
  const series = Array.isArray(opt.series) ? opt.series : [opt.series]
  return series.some((s: any) => Array.isArray(s?.data) && s.data.length > 0)
})

// 直接使用原始 option，ECharts dark 主题会自动处理
const mergedOption = computed(() => {
  return props.option || {}
})

// 监听尺寸变化
watch(
  () => props.height,
  () => {
    if (vChartRef.value && containerRef.value && typeof vChartRef.value.resize === 'function') {
      try {
        const { width } = containerRef.value.getBoundingClientRect()
        vChartRef.value.resize({ width, height: props.height })
      } catch {
        /* noop: 同 onMounted */
      }
    }
  },
)

onBeforeUnmount(() => {
  // vChart 实例随父组件销毁;此处仅保留 hook 占位防止未来内存泄漏
})
</script>

<style scoped>
.chart-container {
  /* 确保容器背景透明，让父容器的主题背景生效 */
  background: transparent;
  /* 宽度不能交给父容器的 display 类型决定：
     `margin: 0 auto` 会让 **grid/flex 子项**失去 stretch，宽度塌缩到 min-content。
     E2E-14 实测：结果弹窗 `.result-content` 是 grid ⇒ 雷达图容器只剩 100px，
     画布同步缩到 100px，轴标签「尽责性」「神经质」被裁成「性」「神」。
     块级父容器（如我的空间的 .chart-item）下 width:auto 本就等于撑满，加这两行无副作用。 */
  width: 100%;
  min-width: 0;
}
</style>
