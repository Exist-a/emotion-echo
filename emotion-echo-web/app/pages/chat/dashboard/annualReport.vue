<template>
  <ReportScaffold
    title="这一年"
    description="让一整年的情绪，被温柔地看见。"
    :loading="isLoading"
    v-model:date="year"
    picker-type="year"
    @change="fetchAnnualReport"
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
import { API_ROUTES } from '~/lib/apiRoutes'
import { getEmotionLabel, getIntentLabel } from '~/utils'

const year = ref(String(new Date().getFullYear()))
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

const chartData = computed<ChartItem[]>(() => {
  if (!reportData.value) return []
  const items: ChartItem[] = []
  if (reportData.value.series?.length) {
    items.push({
      chartType: 'line',
      title: '每月趋势',
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
  // Stage 85：意图分布饼图（字段缺失隐藏，旧报表兼容）
  if (reportData.value.intentDistribution?.length) {
    items.push({
      chartType: 'pie',
      title: '意图分布',
      data: reportData.value.intentDistribution.map((item) => ({ name: getIntentLabel(item.intent), value: item.count }))
    })
  }
  return items
})

const fetchAnnualReport = async () => {
  if (!year.value) return
  isLoading.value = true
  try {
    const data = await get<EmotionTrend>(API_ROUTES.reportsTrend.path, { type: 'yearly', year: year.value })
    reportData.value = data
  } catch (error: any) {
    notify('加载失败', error?.message || '年度报告生成失败,请稍后重试', 'error', 3000)
    reportData.value = null
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchAnnualReport)
</script>
