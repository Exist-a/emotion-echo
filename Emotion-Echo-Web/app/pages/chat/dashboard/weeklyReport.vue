<template>
  <div class="weekly-container">
    <el-date-picker
      v-model="dateRange"
      type="daterange"
      format="YYYY年MM月DD日"
      value-format="YYYY-MM-DD"
      size="large"
      :disabled-date="disabledDate"
      @change="handleFixed7Days"
    />
    <p class="description">请选择7天</p>

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

// 获取今天的开始时间（0点0分0秒）
const getToday = (): Dayjs => dayjs().startOf('day')

// 日期范围
const dateRange = ref<[string, string] | []>([])
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)
let fetchPromise: Promise<void> | null = null

const chartData = computed<ChartItem[]>(() => {
  if (!reportData.value) {
    console.log('[DEBUG] reportData is null')
    return []
  }

  console.log('[DEBUG] reportData:', JSON.stringify(reportData.value, null, 2))

  const items: ChartItem[] = []
  // 折线图
  console.log('[DEBUG] dates:', reportData.value.dates, 'series:', reportData.value.series)
  if (reportData.value.dates?.length > 0 && reportData.value.series?.length > 0) {
    console.log('[DEBUG] 添加折线图，数据条数:', reportData.value.dates.length)
    items.push({
      chartType: 'line',
      title: '情绪趋势',
      XData: reportData.value.dates,
      YData: [], // 多 series 时使用 seriesData
      seriesData: reportData.value.series
    })
  } else {
    console.log('[DEBUG] 折线图条件不满足: dates有值?', reportData.value.dates?.length > 0, 'series有值?', reportData.value.series?.length > 0)
  }
  // 饼图
  console.log('[DEBUG] emotionDistribution:', reportData.value.emotionDistribution)
  if (reportData.value.emotionDistribution?.length > 0) {
    console.log('[DEBUG] 添加饼图，数据条数:', reportData.value.emotionDistribution.length)
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
  console.log('[DEBUG] intentDistribution:', reportData.value.intentDistribution)
  if (reportData.value.intentDistribution && reportData.value.intentDistribution.length > 0) {
    items.push({
      chartType: 'pie',
      title: '意图分布',
      data: reportData.value.intentDistribution
    })
  }
  console.log('[DEBUG] 最终 chartData items:', items.length, items)
  return items
})

const fetchWeeklyReport = async () => {
  // 防止并发请求
  if (fetchPromise) {
    return fetchPromise
  }

  isLoading.value = true
  fetchPromise = (async () => {
    try {
      const params: any = { type: 'weekly' }
      if (dateRange.value && dateRange.value.length === 2) {
        params.start_date = dateRange.value[0]
        params.end_date = dateRange.value[1]
      }
      const data = await get<EmotionTrend>('/reports/trend', params)
      reportData.value = data
    } catch (error: any) {
      ElNotification({
        type: 'error',
        message: error.message || '获取周报失败'
      })
      reportData.value = null
    } finally {
      isLoading.value = false
      fetchPromise = null
    }
  })()

  return fetchPromise
}

// 禁用日期函数（优化版）
const disabledDate = (time: Date): boolean => {
  const currentDate = dayjs(time).startOf('day')
  const today = getToday()
  return currentDate.isAfter(today)
}

// 处理固定7天选择
const handleFixed7Days = (val: [string, string] | null): void => {
  if (!val || val.length !== 2) {
    dateRange.value = []
    return
  }
  const [startDateStr] = val
  const today = getToday()
  const startDate = dayjs(startDateStr).startOf('day')
  if (startDate.isAfter(today)) {
    dateRange.value = []
    return
  }
  let endDate = startDate.add(6, 'day')
  if (endDate.isAfter(today)) {
    endDate = today
  }
  if (endDate.isBefore(startDate)) {
    endDate = startDate
  }
  dateRange.value = [startDate.format('YYYY-MM-DD'), endDate.format('YYYY-MM-DD')]
  // 刷新周报数据
  fetchWeeklyReport()
}

// 初始化默认选中本周（周一至今天）
const initDefaultWeek = (): void => {
  const today = getToday()
  let startOfWeek = today.startOf('week').add(1, 'day')
  if (today.day() === 0) {
    startOfWeek = today.subtract(6, 'day').startOf('day')
  }
  if (startOfWeek.isAfter(today)) {
    startOfWeek = today
  }
  const endOfWeek = today
  dateRange.value = [startOfWeek.format('YYYY-MM-DD'), endOfWeek.format('YYYY-MM-DD')]
}

onMounted(() => {
  initDefaultWeek()
  fetchWeeklyReport()
})
</script>

<style scoped>
.weekly-container {
  padding: 20px;
}
.description {
  margin: 10px 0;
  font-size: 14px;
  color: #666;
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
