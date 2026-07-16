<template>
  <div class="monthly-container">
    <el-date-picker
      v-model="month"
      type="month"
      format="YYYY年MM月"
      value-format="YYYY-MM"
      placeholder="选择一个月份"
      size="large"
      :disabled-date="disabledMonth"
      @change="fetchMonthlyReport"
    />

    <el-skeleton v-if="isLoading" :rows="5" animated style="margin-top: 20px" />

    <template v-else>
      <el-card v-if="reportData" class="summary-card" shadow="hover">
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
      </el-card>

      <chartCard v-if="chartData.length > 0" :data="chartData"></chartCard>
      <el-empty v-else description="暂无数据" style="margin-top: 40px" />
    </template>
  </div>
</template>

<script setup lang="ts">
import dayjs from 'dayjs'
import type { Dayjs } from 'dayjs'
import ChartCard from '~/components/report/chartsCard.vue'
import type { ChartItem } from '~/types/charts/common'
import type { EmotionTrend } from '~/types/api'
import { get } from '~/composables/useApi'
import { getEmotionLabel } from '~/utils'

const month = ref<string>('')
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

const getCurrentDate = (): Dayjs => dayjs().startOf('day')

const chartData = computed<ChartItem[]>(() => {
  if (!reportData.value) return []
  const items: ChartItem[] = []
  if (reportData.value.dates?.length > 0 && reportData.value.series?.length > 0) {
    items.push({
      chartType: 'line',
      title: '情绪趋势',
      XData: reportData.value.dates,
      YData: [],
      seriesData: reportData.value.series
    })
  }
  if (reportData.value.emotionDistribution?.length > 0) {
    items.push({
      chartType: 'pie',
      title: '情绪分布',
      data: reportData.value.emotionDistribution.map((item) => ({
        ...item,
        name: getEmotionLabel(item.name)
      }))
    })
  }
  // 意图分布饼图
  if (reportData.value.intentDistribution && reportData.value.intentDistribution.length > 0) {
    items.push({
      chartType: 'pie',
      title: '意图分布',
      data: reportData.value.intentDistribution
    })
  }
  return items
})

const fetchMonthlyReport = async () => {
  isLoading.value = true
  try {
    const params: any = { type: 'monthly' }
    if (month.value) {
      params.start_date = dayjs(month.value).startOf('month').format('YYYY-MM-DD')
      params.end_date = dayjs(month.value).endOf('month').format('YYYY-MM-DD')
    }
    const data = await get<EmotionTrend>('/reports/trend', params)
    reportData.value = data
  } catch (error: any) {
    ElNotification({
      type: 'error',
      message: error.message || '获取月报失败'
    })
    reportData.value = null
  } finally {
    isLoading.value = false
  }
}

const disabledMonth = (date: Date): boolean => {
  const current = getCurrentDate()
  const selected = dayjs(date)
  return selected.isAfter(current, 'month')
}

const initDefaultMonth = (): void => {
  month.value = getCurrentDate().format('YYYY-MM')
}

onMounted(() => {
  initDefaultMonth()
  fetchMonthlyReport()
})
</script>

<style scoped>
.monthly-container {
  padding: 20px;
}
.summary-card {
  margin-top: 20px;
  margin-bottom: 20px;
}
.summary-text {
  font-size: 16px;
  line-height: 1.6;
  color: #303133;
  margin-bottom: 16px;
}
.stats-row {
  display: flex;
  gap: 40px;
  justify-content: center;
  border-top: 1px solid #ebeef5;
  padding-top: 16px;
}
.stat-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.stat-value {
  font-size: 24px;
  font-weight: bold;
  color: #409eff;
}
.stat-label {
  font-size: 14px;
  color: #909399;
}
</style>
