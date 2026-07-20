<template>
  <ReportScaffold
    title="这个月"
    description="把过去三十天的节奏轻轻展开。"
    :loading="isLoading"
    v-model:date="month"
    picker-type="month"
    @change="fetchMonthlyReport"
  >
    <template v-if="reportData" #summary>
      <p class="summary-text">{{ reportData.summary }}</p>
      <div class="stats-row">
        <div class="stat-item">
          <span class="stat-value">{{ reportData.conversationCount }}</span>
          <span class="stat-label">会话数</span>
        </div>
        <div class="stat-item">
          <span class="stat-value">{{ reportData.messageCount }}</span>
          <span class="stat-label">消息数</span>
        </div>
        <div class="stat-item">
          <span class="stat-value">{{ reportData.wordCount }}</span>
          <span class="stat-label">总字数</span>
        </div>
      </div>
    </template>
    <template #charts>
      <chartCard v-if="chartData.length > 0" :data="chartData" />
      <div class="ee-empty">暂无数据</div>
    </template>
  </ReportScaffold>
</template>

<script setup lang="ts">
import ChartCard from '~/components/report/chartsCard.vue'
import type { ChartItem } from '~/types/charts/common'
import type { EmotionTrend } from '~/types/api'
import { get } from '~/composables/useApi'
import { getEmotionLabel } from '~/utils'

const month = ref(formatMonth(new Date()))
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

function formatMonth(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const chartData = computed<ChartItem[]>(() => {
  if (!reportData.value) return []
  const items: ChartItem[] = []
  if (reportData.value.series?.length) {
    items.push({
      chartType: 'line',
      title: '每日趋势',
      XData: reportData.value.dates,
      YData: reportData.value.series.flatMap((s) => s.data)
    })
  }
  if (reportData.value.emotionDistribution?.length > 0) {
    items.push({
      chartType: 'pie',
      title: '情绪分布',
      data: reportData.value.emotionDistribution.map((item) => ({ ...item, name: getEmotionLabel(item.name) }))
    })
  }
  return items
})

const fetchMonthlyReport = async () => {
  if (!month.value) return
  isLoading.value = true
  try {
    const data = await get<EmotionTrend>('/reports/trend', { type: 'monthly', month: month.value })
    reportData.value = data
  } catch (error: any) {
    notify('加载失败', error?.message || '月报告生成失败,请稍后重试', 'error', 3000)
    reportData.value = null
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchMonthlyReport)
</script>
