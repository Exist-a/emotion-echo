<template>
  <ReportScaffold
    title="这一周"
    description="把最近七天的变化，慢慢讲给你听。"
    :loading="isLoading"
    :date="dateRange"
    picker-type="daterange"
    @change="fetchWeeklyReport"
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
import type { DailyReport, EmotionTrend } from '~/types/api'
import { get } from '~/composables/useApi'
import { getEmotionLabel } from '~/utils'

const dateRange = ref<[string, string]>([formatDate(new Date()), formatDate(new Date())])
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

function formatDate(d: Date): string {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
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

const fetchWeeklyReport = async () => {
  if (!dateRange.value || dateRange.value.length !== 2) return
  isLoading.value = true
  try {
    const data = await get<EmotionTrend>('/reports/trend', { type: 'weekly', start: dateRange.value[0], end: dateRange.value[1] })
    reportData.value = data
  } catch (error: any) {
    notify('加载失败', error?.message || '周报告生成失败,请稍后重试', 'error', 3000)
    reportData.value = null
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchWeeklyReport)
</script>
