<template>
  <ReportScaffold
    v-model:date="year"
    title="这一年"
    description="让一整年的情绪，被温柔地看见。"
    :loading="isLoading"
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
import type { EmotionTrend } from '~/types/api'
import { get } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { trendToChartItems } from '~/utils/trendReportCharts'

const year = ref(String(new Date().getFullYear()))
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

const chartData = computed(() => trendToChartItems(reportData.value))

const fetchAnnualReport = async () => {
  if (!year.value) return
  isLoading.value = true
  try {
    const data = await get<EmotionTrend>(API_ROUTES.reportsTrend.path, {
      type: 'yearly',
      year: year.value,
    })
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
