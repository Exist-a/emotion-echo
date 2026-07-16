<template>
  <div class="daily-container">
    <el-date-picker
      v-model="date"
      type="date"
      format="YYYY年MM月DD日"
      value-format="YYYY-MM-DD"
      placeholder="选择一个日期"
      size="large"
      :disabled-date="disabledDate"
      @change="fetchDailyReport"
    />

    <el-skeleton v-if="isLoading" :rows="5" animated style="margin-top: 20px" />

    <template v-else>
      <!-- 摘要 -->
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

      <!-- 图表 -->
      <chartCard v-if="chartData.length > 0" :data="chartData"></chartCard>
      <el-empty v-else description="暂无数据" style="margin-top: 40px" />
    </template>
  </div>
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
      data: reportData.value.emotionDistribution.map((item) => ({
        ...item,
        name: getEmotionLabel(item.name)
      }))
    })
  }
  if (reportData.value.intentDistribution && reportData.value.intentDistribution.length > 0) {
    items.push({
      chartType: 'pie',
      title: '意图分布',
      data: reportData.value.intentDistribution
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
    ElNotification({
      type: 'error',
      message: error.message || '获取日报失败'
    })
    reportData.value = null
  } finally {
    isLoading.value = false
  }
}

const disabledDate = (time: Date) => {
  return time.getTime() > Date.now()
}

onMounted(() => {
  fetchDailyReport()
})
</script>

<style scoped>
.daily-container {
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
