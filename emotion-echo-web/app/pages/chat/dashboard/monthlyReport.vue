<template>
  <ReportScaffold
    v-model:date="month"
    title="这个月"
    description="把过去三十天的节奏轻轻展开。"
    :loading="isLoading"
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
      </div>
    </template>
    <template #charts>
      <chartCard v-if="chartData.length > 0" :data="chartData" />
      <div v-else-if="chartData.length === 0" class="ee-empty">暂无数据</div>
    </template>
  </ReportScaffold>
</template>

<script setup lang="ts">
import ChartCard from '~/components/report/chartsCard.vue'
import type { EmotionTrend } from '~/types/api'
import { get } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { trendToChartItems } from '~/utils/trendReportCharts'

const month = ref(formatMonth(new Date()))
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

function formatMonth(d: Date): string {
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`
}

const chartData = computed(() => trendToChartItems(reportData.value))

const fetchMonthlyReport = async () => {
  if (!month.value) return
  isLoading.value = true
  try {
    const data = await get<EmotionTrend>(API_ROUTES.reportsTrend.path, {
      type: 'monthly',
      month: month.value,
    })
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
