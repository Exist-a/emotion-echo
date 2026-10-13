<template>
  <ReportScaffold
    v-model:date="dateRange"
    title="这一周"
    description="把最近七天的变化，慢慢讲给你听。"
    :loading="isLoading"
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

const WEEK_SPAN_MS = 6 * 24 * 60 * 60 * 1000 // 最多跨 7 天（含首尾端点）

// 默认：最近 7 天（当天前 6 天 ~ 今天，含首尾共 7 天）
const _today = new Date()
const _start7 = new Date(_today.getTime() - WEEK_SPAN_MS)
const dateRange = ref<[string, string]>([formatDate(_start7), formatDate(_today)])
const isLoading = ref(false)
const reportData = ref<EmotionTrend | null>(null)

function formatDate(d: Date): string {
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${year}-${month}-${day}`
}

/**
 * 周报区间固定为「最多 7 天」：起点晚于终点则交换；跨度超过 7 天则把终点收回到
 * 起点 + 6 天。返回纠正后的区间（与入参不同时由调用方回写 dateRange，纠正 UI）。
 */
function clampRange(range: [string, string]): [string, string] {
  let [s, e] = range
  if (s > e) [s, e] = [e, s]
  const startMs = new Date(`${s}T00:00:00`).getTime()
  const endMs = new Date(`${e}T00:00:00`).getTime()
  if (endMs - startMs > WEEK_SPAN_MS) {
    e = formatDate(new Date(startMs + WEEK_SPAN_MS))
  }
  return [s, e]
}

const chartData = computed(() => trendToChartItems(reportData.value))

const fetchWeeklyReport = async (payload?: unknown) => {
  // ReportScaffold 的 change 事件先于 v-model:date 回流触发，故优先用事件载荷取区间
  const raw = Array.isArray(payload) ? (payload as [string, string]) : dateRange.value
  if (!raw || raw.length !== 2) return
  const range = clampRange(raw)
  // 区间被纠正（跨度过大/顺序颠倒）时回写，让日期框反映真实生效的区间
  if (range[0] !== raw[0] || range[1] !== raw[1]) {
    dateRange.value = range
  }
  isLoading.value = true
  try {
    const data = await get<EmotionTrend>(API_ROUTES.reportsTrend.path, {
      type: 'weekly',
      start: range[0],
      end: range[1],
    })
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
