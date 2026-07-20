<template>
  <ReportScaffold
    title="今日的回声"
    description="你今天的对话被轻轻记录在这里。"
    :loading="isLoading"
    v-model:date="date"
    @change="fetchDailyReport"
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
import type { DailyReport } from '~/types/api'
import { get } from '~/composables/useApi'
import { getEmotionLabel } from '~/utils'

const date = ref(formatDate(new Date()))
const isLoading = ref(false)
const reportData = ref<DailyReport | null>(null)

function formatDate(d: Date): string {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

const chartData = computed<ChartItem[]>(() => {
  if (!reportData.value) return []
  const items: ChartItem[] = []
  if (reportData.value.emotionDistribution?.length > 0) {
    items.push({
      chartType: 'pie',
      title: '情绪分布',
      data: reportData.value.emotionDistribution.map((item) => ({ ...item, name: getEmotionLabel(item.name) }))
    })
  }
  return items
})

const fetchDailyReport = async () => {
  if (!date.value) return
  isLoading.value = true
  try {
    const data = await get<DailyReport>('/reports/daily', { date: date.value })
    reportData.value = data
  } catch (error: any) {
    notify('加载失败', error?.message || '日报告生成失败,请稍后重试', 'error', 3000)
    reportData.value = null
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchDailyReport)
</script>
