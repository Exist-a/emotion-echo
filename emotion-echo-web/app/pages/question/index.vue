<template>
  <section class="assessment-page">
    <header class="page-intro">
      <span class="eyebrow">SELF UNDERSTANDING</span>
      <h2>心理测验量表</h2>
      <p>这里没有评判，只有你愿意多了解自己一些的勇气。</p>
    </header>

    <div class="action-bar">
      <button type="button" class="ee-btn btn-ghost" @click="navigateTo({ name: 'chat-user' })">
        回到我的空间
      </button>
    </div>

    <div v-if="!isLoading && tableData.length > 0" class="tab-bar" role="tablist">
      <button
        v-for="tab in TABS"
        :key="tab.key"
        type="button"
        role="tab"
        class="tab-btn"
        :class="{ active: activeTab === tab.key }"
        :aria-selected="activeTab === tab.key"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
        <span class="tab-count">{{ tabSurveys(tab.key).length }}</span>
      </button>
    </div>

    <div v-if="isLoading" class="loading-grid">
      <div v-for="i in 3" :key="i" class="card-skeleton" />
    </div>

    <div v-else-if="tableData.length === 0" class="empty-state">
      <span class="empty-mark">○</span>
      <p>暂未提供量表</p>
    </div>

    <div v-else-if="visibleSurveys.length === 0" class="empty-state">
      <span class="empty-mark">○</span>
      <p>{{ activeTabLabel }}暂未开放</p>
    </div>

    <div v-else class="assessment-grid">
      <article
        v-for="item in visibleSurveys"
        :key="item.id"
        class="assessment-card"
      >
        <header class="card-head">
          <h3>{{ item.title }}</h3>
          <span class="badge" :class="isPersonality(item) ? 'badge-personality' : ''">
            {{ categoryLabel(item.category) }}
          </span>
        </header>
        <p class="card-desc">{{ item.description }}</p>
        <dl class="card-meta">
          <div>
            <dt>题目数</dt>
            <dd>{{ item.questionNum }} 题</dd>
          </div>
        </dl>
        <footer class="card-actions">
          <button type="button" class="ee-btn ee-btn-primary" @click="doQuestion(item)">
            开始答题
          </button>
        </footer>
      </article>
    </div>

    <Teleport v-if="dialogVisible" to="body">
      <div class="modal-backdrop" @click.self="dialogVisible = false">
        <div class="modal-card" role="dialog" aria-modal="true">
          <h3>这次的结果</h3>
          <div v-if="currentResult" class="result-content">
            <div class="result-row">
              <span>总分</span><strong>{{ currentResult.totalScore }}</strong>
            </div>
            <div class="result-row">
              <span>等级</span><strong>{{ riskLevelLabel(currentResult.riskLevel) }}</strong>
            </div>
          </div>
          <div v-else>加载结果中…</div>
          <div class="modal-actions">
            <button type="button" class="ee-btn ee-btn-primary" @click="dialogVisible = false">
              收下这份结果
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import type { SurveyItem, SurveyResult } from '~/types/api'
import { get } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { useNotify } from '~/composables/useNotify'
import { PERSONALITY_RISK_LEVEL } from '~/configs/personality'

definePageMeta({ layout: 'nav' })

const { error: notifyError } = useNotify()
const dialogVisible = ref(false)
const currentResult = ref<SurveyResult | null>(null)
const isLoading = ref(true)

const tableData = ref<SurveyItem[]>([])

// D-02：人格量表与症状量表分区展示（前者产心理画像驱动 AI，后者做风险预警）
type TabKey = 'symptom' | 'personality'
const TABS: { key: TabKey; label: string }[] = [
  { key: 'symptom', label: '症状筛查' },
  { key: 'personality', label: '人格画像' },
]
const activeTab = ref<TabKey>('symptom')

const isPersonality = (item: SurveyItem) => item.category === 'personality'
const tabSurveys = (key: TabKey) =>
  key === 'personality' ? tableData.value.filter(isPersonality) : tableData.value.filter((i) => !isPersonality(i))
const visibleSurveys = computed(() => tabSurveys(activeTab.value))
const activeTabLabel = computed(
  () => TABS.find((t) => t.key === activeTab.value)?.label ?? '',
)

const categoryLabel = (category: string) => {
  const map: Record<string, string> = {
    personality: '人格',
    depression: '抑郁',
    anxiety: '焦虑',
    sleep: '睡眠',
  }
  return map[category] ?? category
}

const fetchSurveys = async () => {
  isLoading.value = true
  try {
    const data = await get<{ items: SurveyItem[] }>(API_ROUTES.surveys.path)
    tableData.value = data.items ?? []
    // 数据到位后落到有内容的一栏，避免默认停在空栏
    if (tabSurveys(activeTab.value).length === 0 && tableData.value.length > 0) {
      activeTab.value = tabSurveys('personality').length > 0 ? 'personality' : 'symptom'
    }
  } catch (err: any) {
    notifyError('获取量表列表失败', err.message)
  } finally {
    isLoading.value = false
  }
}

onMounted(fetchSurveys)

const doQuestion = (data: SurveyItem) =>
  navigateTo({ name: 'question-detail', params: { id: data.id } })

const riskLevelLabel = (level: string) => {
  const map: Record<string, string> = {
    none: '正常', mild: '轻度', moderate: '中度', severe: '重度', extreme: '极重度',
  }
  return map[level] ?? level
}
</script>

<style scoped lang="scss">
.assessment-page {
  width: min(960px, 100%);
  margin: 0 auto;
}
.page-intro {
  margin-bottom: 16px;
}
.eyebrow {
  color: var(--ee-primary);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.16em;
}
.page-intro h2 {
  margin: 6px 0 0;
  font-size: clamp(22px, 2.6vw, 30px);
  font-weight: 600;
  letter-spacing: -0.02em;
}
.page-intro p {
  margin: 6px 0 0;
  color: var(--ee-text-muted);
  font-size: 14px;
}

.action-bar {
  display: flex;
  justify-content: flex-end;
  margin: 0 0 16px;
}

.ee-btn:hover:not(:disabled) {
  background: var(--ee-surface-muted);
}
.ee-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-ghost {
  background: transparent;
  color: var(--ee-text-muted);
}

.tab-bar {
  display: flex;
  gap: 8px;
  margin: 0 0 16px;
  border-bottom: 1px solid var(--ee-border);
}
.tab-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 0;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  font-size: 14px;
  transition: color var(--ee-transition), border-color var(--ee-transition);
}
.tab-btn:hover {
  color: var(--ee-text);
}
.tab-btn.active {
  color: var(--ee-primary);
  border-bottom-color: var(--ee-primary);
  font-weight: 600;
}
.tab-count {
  padding: 1px 7px;
  color: var(--ee-text-muted);
  background: var(--ee-surface-muted);
  border-radius: 999px;
  font-size: 11px;
  font-weight: 500;
}
.tab-btn.active .tab-count {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
}

.loading-grid {
  display: grid;
  gap: 14px;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
}
.card-skeleton {
  height: 180px;
  background: linear-gradient(
    90deg,
    var(--ee-surface-muted) 25%,
    color-mix(in srgb, var(--ee-surface-muted) 50%, var(--ee-surface)) 50%,
    var(--ee-surface-muted) 75%
  );
  background-size: 200% 100%;
  animation: skeleton-shimmer 1.4s ease-in-out infinite;
  border-radius: var(--ee-radius-lg);
}
@keyframes skeleton-shimmer {
  from {
    background-position: 200% 0;
  }
  to {
    background-position: -200% 0;
  }
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 64px 8px;
  color: var(--ee-text-muted);
}
.empty-mark {
  font-size: 32px;
  opacity: 0.6;
}
.empty-state p {
  margin: 0;
  font-size: 13px;
}

.assessment-grid {
  display: grid;
  gap: 14px;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
}
.assessment-card {
  display: grid;
  gap: 14px;
  padding: 18px 20px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  transition:
    transform var(--ee-transition),
    border-color var(--ee-transition);
}
.assessment-card:hover {
  transform: translateY(-2px);
  border-color: color-mix(in srgb, var(--ee-primary) 45%, var(--ee-border));
}
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.card-head h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.badge {
  padding: 2px 9px;
  border-radius: 999px;
  font-size: 11px;
  color: var(--ee-text-muted);
  background: var(--ee-surface-muted);
}
.badge-personality {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
}
.badge-completed {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
}
.badge-not_started {
  color: var(--ee-text-muted);
  background: var(--ee-surface-muted);
}
.card-meta {
  display: grid;
  gap: 6px;
  margin: 0;
}
.card-meta div {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
  color: var(--ee-text-muted);
}
.card-meta dd {
  margin: 0;
  color: var(--ee-text);
}
.card-actions {
  display: flex;
  gap: 8px;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  background: rgba(20, 27, 23, 0.45);
  backdrop-filter: blur(2px);
}
.modal-card {
  width: min(520px, 100%);
  padding: 20px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: 0 12px 36px rgba(32, 37, 34, 0.15);
}
.modal-card h3 {
  margin: 0 0 12px;
  font-size: 16px;
  font-weight: 600;
}
.result-content {
  display: grid;
  gap: 10px;
}
.result-row {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--ee-border);
}
.result-row strong {
  font-size: 22px;
  color: var(--ee-primary);
}
.result-suggestion {
  margin: 0;
  color: var(--ee-text-muted);
  line-height: 1.7;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

@media (max-width: 600px) {
  .action-bar {
    justify-content: stretch;
  }
  .action-bar .ee-btn {
    width: 100%;
  }
}
</style>
