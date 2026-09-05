<template>
  <section class="assessment-page">
    <header class="page-intro">
      <span class="eyebrow">SELF UNDERSTANDING</span>
      <h2>心理测验量表</h2>
      <p>这里没有评判，只有你愿意多了解自己一些的勇气。</p>
    </header>

    <div class="action-bar">
      <button type="button" class="ee-btn btn-ghost" @click="navigateTo({ name: 'chat-user' })">回到我的空间</button>
    </div>

    <div v-if="isLoading" class="loading-grid">
      <div v-for="i in 3" :key="i" class="card-skeleton" />
    </div>

    <div v-else-if="tableData.length === 0" class="empty-state">
      <span class="empty-mark">○</span>
      <p>暂未提供量表</p>
    </div>

    <div v-else class="assessment-grid">
      <article v-for="item in tableData" :key="item.id" class="assessment-card" :class="`is-${item.status}`">
        <header class="card-head">
          <h3>{{ item.title }}</h3>
          <span class="badge" :class="`badge-${item.status}`">{{ item.statusText }}</span>
        </header>
        <dl class="card-meta">
          <div><dt>预计用时</dt><dd>{{ item.estimatedTime }}</dd></div>
          <div><dt>完成时间</dt><dd>{{ item.completedAt || '—' }}</dd></div>
        </dl>
        <footer class="card-actions">
          <button type="button" class="ee-btn ee-btn-primary" @click="doQuestion(item)">{{ item.status === 'completed' ? '再做一次' : '开始答题' }}</button>
          <button type="button" class="ee-btn" :disabled="item.status !== 'completed'" @click="checkRes(item)">查看结果</button>
        </footer>
      </article>
    </div>

    <Teleport v-if="dialogVisible" to="body">
      <div class="modal-backdrop" @click.self="dialogVisible = false">
        <div class="modal-card" role="dialog" aria-modal="true">
          <h3>这次的结果</h3>
          <div v-if="currentResult" class="result-content">
            <div class="result-row"><span>总分</span><strong>{{ currentResult.totalScore }}</strong></div>
            <div class="result-row"><span>等级</span><strong>{{ currentResult.level }}</strong></div>
            <p class="result-suggestion">{{ currentResult.suggestion }}</p>
          </div>
          <div v-else>加载结果中…</div>
          <div class="modal-actions">
            <button type="button" class="ee-btn ee-btn-primary" @click="dialogVisible = false">收下这份结果</button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import type { SurveyItem, SurveyResult } from '~/types/api'
import { get } from '~/composables/useApi'
import { useNotify } from '~/composables/useNotify'

definePageMeta({ layout: 'default' })

const { error: notifyError } = useNotify()
const dialogVisible = ref(false)
const currentResult = ref<SurveyResult | null>(null)
const isLoading = ref(true)

interface TableRow extends SurveyItem {
  statusText: string
}

const tableData = ref<TableRow[]>([])

const fetchSurveys = async () => {
  isLoading.value = true
  try {
    const data = await get<{ list: SurveyItem[] }>('/surveys')
    tableData.value = data.list.map((item) => ({
      ...item,
      statusText: item.status === 'completed' ? '已完成' : '未开始'
    }))
  } catch (err: any) {
    notifyError('获取量表列表失败', err.message)
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchSurveys)

const doQuestion = (data: TableRow) => navigateTo({ name: 'question-detail', params: { id: data.id } })

const checkRes = async (data: TableRow) => {
  if (!data.resultId) {
    notifyError('还没有结果', '请先完成量表')
    return
  }
  dialogVisible.value = true
  currentResult.value = null
  try {
    const result = await get<SurveyResult>(`/surveys/result/${data.resultId}`)
    currentResult.value = result
  } catch (err: any) {
    notifyError('获取结果失败', err.message)
  }
}
</script>

<style scoped lang="scss">
.assessment-page { width: min(960px, 100%); margin: 0 auto; }
.page-intro { margin-bottom: 16px; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: 0.16em; }
.page-intro h2 { margin: 6px 0 0; font-size: clamp(22px, 2.6vw, 30px); font-weight: 600; letter-spacing: -0.02em; }
.page-intro p { margin: 6px 0 0; color: var(--ee-text-muted); font-size: 14px; }

.action-bar { display: flex; justify-content: flex-end; margin: 0 0 16px; }

.ee-btn { display: inline-flex; align-items: center; justify-content: center; height: 38px; padding: 0 18px; background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-md); color: var(--ee-text); cursor: pointer; font-size: 13px; font-weight: 600; transition: background var(--ee-transition), color var(--ee-transition), border-color var(--ee-transition); }
.ee-btn:hover:not(:disabled) { background: var(--ee-surface-muted); }
.ee-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.ee-btn-primary { background: var(--ee-primary); color: #fff; border-color: var(--ee-primary); }
.ee-btn-primary:hover:not(:disabled) { background: var(--ee-primary-hover); border-color: var(--ee-primary-hover); }
.btn-ghost { background: transparent; color: var(--ee-text-muted); }

.loading-grid { display: grid; gap: 14px; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
.card-skeleton { height: 180px; background: linear-gradient(90deg, var(--ee-surface-muted) 25%, color-mix(in srgb, var(--ee-surface-muted) 50%, var(--ee-surface)) 50%, var(--ee-surface-muted) 75%); background-size: 200% 100%; animation: skeleton-shimmer 1.4s ease-in-out infinite; border-radius: var(--ee-radius-lg); }
@keyframes skeleton-shimmer { from { background-position: 200% 0; } to { background-position: -200% 0; } }

.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 64px 8px; color: var(--ee-text-muted); }
.empty-mark { font-size: 32px; opacity: 0.6; }
.empty-state p { margin: 0; font-size: 13px; }

.assessment-grid { display: grid; gap: 14px; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
.assessment-card { display: grid; gap: 14px; padding: 18px 20px; background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); transition: transform var(--ee-transition), border-color var(--ee-transition); }
.assessment-card:hover { transform: translateY(-2px); border-color: color-mix(in srgb, var(--ee-primary) 45%, var(--ee-border)); }
.card-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.card-head h3 { margin: 0; font-size: 16px; font-weight: 600; }
.badge { padding: 2px 9px; border-radius: 999px; font-size: 11px; }
.badge-completed { color: var(--ee-primary); background: var(--ee-primary-soft); }
.badge-not_started { color: var(--ee-text-muted); background: var(--ee-surface-muted); }
.card-meta { display: grid; gap: 6px; margin: 0; }
.card-meta div { display: flex; justify-content: space-between; font-size: 13px; color: var(--ee-text-muted); }
.card-meta dd { margin: 0; color: var(--ee-text); }
.card-actions { display: flex; gap: 8px; }

.modal-backdrop { position: fixed; inset: 0; z-index: 80; display: flex; align-items: center; justify-content: center; padding: 16px; background: rgba(20, 27, 23, 0.45); backdrop-filter: blur(2px); }
.modal-card { width: min(520px, 100%); padding: 20px; background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); box-shadow: 0 12px 36px rgba(32, 37, 34, 0.15); }
.modal-card h3 { margin: 0 0 12px; font-size: 16px; font-weight: 600; }
.result-content { display: grid; gap: 10px; }
.result-row { display: flex; justify-content: space-between; align-items: baseline; padding-bottom: 8px; border-bottom: 1px solid var(--ee-border); }
.result-row strong { font-size: 22px; color: var(--ee-primary); }
.result-suggestion { margin: 0; color: var(--ee-text-muted); line-height: 1.7; }
.modal-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 16px; }

@media (max-width: 600px) {
  .action-bar { justify-content: stretch; }
  .action-bar .ee-btn { width: 100%; }
}
</style>
