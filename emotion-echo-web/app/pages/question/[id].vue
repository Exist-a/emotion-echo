<template>
  <section class="assessment-detail">
    <header class="detail-header">
      <button class="back-button" type="button" @click="goBackDialogVisible = true">
        ← 回到列表
      </button>
      <span class="eyebrow">SELF UNDERSTANDING</span>
    </header>

    <div v-if="isLoading" class="loading-container">
      <div class="ee-skeleton" style="height: 32px; width: 60%; margin-bottom: 16px" />
      <div class="ee-skeleton" style="height: 16px; width: 80%; margin-bottom: 32px" />
      <div class="ee-skeleton" style="height: 64px; margin-bottom: 12px" />
      <div class="ee-skeleton" style="height: 64px; margin-bottom: 12px" />
      <div class="ee-skeleton" style="height: 64px; margin-bottom: 12px" />
    </div>

    <div v-else-if="!survey.id" class="empty-state">
      <span class="empty-mark">○</span>
      <p>量表不存在或已下架</p>
    </div>

    <div v-else class="question-container card">
      <div class="question-intro">
        <h2>{{ survey.title }}</h2>
        <p v-if="survey.description">{{ survey.description }}</p>
        <div class="progress">
          <span>已完成 {{ answeredCount }} / {{ totalCount }}</span>
          <progress class="progress-bar" :value="progressPercent" max="100" />
        </div>
      </div>

      <div v-for="question in survey.questions" :key="question.id" class="question-block">
        <h3>{{ question.id }}. {{ question.title }}</h3>
        <div class="radio-row">
          <label
            v-for="option in question.options"
            :key="option.id"
            class="radio-option"
            :class="{ active: answerMap[question.id] === option.id }"
          >
            <input
              type="radio"
              :name="`q-${question.id}`"
              :value="option.id"
              :checked="answerMap[question.id] === option.id"
              @change="answerMap[question.id] = option.id"
            />
            <span>{{ option.text }}</span>
          </label>
        </div>
      </div>

      <div class="submit-btn-wrap">
        <button
          class="ee-btn ee-btn-primary ee-btn-lg"
          :disabled="!hasAnsweredAll || isSubmitting"
          @click="handleSubmit"
        >
          {{ isSubmitting ? '提交中…' : '提交这份答卷' }}
        </button>
      </div>
    </div>

    <Teleport v-if="goBackDialogVisible" to="body">
      <div class="modal-backdrop" @click.self="goBackDialogVisible = false">
        <div class="modal-card" role="dialog" aria-modal="true">
          <h3>要离开吗？</h3>
          <p>返回后，这次的作答不会被保存。</p>
          <div class="modal-actions">
            <button type="button" class="ee-btn btn-ghost" @click="goBackDialogVisible = false">
              继续作答
            </button>
            <button
              type="button"
              class="ee-btn ee-btn-primary"
              @click="navigateTo({ name: 'question' })"
            >
              确认离开
            </button>
          </div>
        </div>
      </div>
    </Teleport>

    <Teleport v-if="resultDialogVisible" to="body">
      <div class="modal-backdrop" @click.self="handleResultConfirm">
        <div class="modal-card" role="dialog" aria-modal="true">
          <h3>{{ isPersonality ? '你的人格画像' : '你给出的答案' }}</h3>
          <div v-if="submitResult" class="result-content">
            <!-- 人格量表：维度雷达图 + 逐维度分档（无"风险等级"概念） -->
            <template v-if="isPersonality">
              <RadarChart
                :indicators="personalityIndicators()"
                :data="personalityRadarData(submitResult.factorScores)"
                :height="240"
              />
              <ul class="dimension-list">
                <li v-for="dim in personalityDimensions" :key="dim.key" class="dimension-row">
                  <span class="dimension-name">{{ dim.label }}</span>
                  <span class="dimension-score">
                    {{ submitResult.factorScores?.[dim.key] ?? '-' }}
                    <em class="dimension-level">{{ levelFor(dim.key) }}</em>
                  </span>
                </li>
              </ul>
            </template>
            <!-- 症状量表：总分 + 风险等级 -->
            <template v-else>
              <div class="result-row">
                <span>总分</span><strong>{{ submitResult.totalScore }}</strong>
              </div>
              <div class="result-row">
                <span>等级</span><strong>{{ riskLevelLabel(submitResult.riskLevel) }}</strong>
              </div>
            </template>
          </div>
          <div class="modal-actions">
            <button type="button" class="ee-btn ee-btn-primary" @click="handleResultConfirm">
              回到量表列表
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>

<script setup lang="ts">
import type { SurveyDetail, SurveyResult } from '~/types/api'
import { get, post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import RadarChart from '~/components/charts/RadarChart.vue'
import {
  PERSONALITY_DIMENSIONS,
  isPersonalityResult,
  personalityIndicators,
  personalityLevel,
  personalityRadarData,
} from '~/configs/personality'

definePageMeta({ layout: 'default' })

const route = useRoute()
const isLoading = ref(true)
const isSubmitting = ref(false)
const survey = ref<SurveyDetail>({
  id: 0,
  title: '',
  description: '',
  estimatedTime: '',
  questions: [],
})
const goBackDialogVisible = ref(false)
const resultDialogVisible = ref(false)
const submitResult = ref<SurveyResult | null>(null)
const answerMap = ref<Record<string, number>>({})

const getQuestionDetail = async (id: string | number) => {
  isLoading.value = true
  try {
    const data = await get<SurveyDetail>(API_ROUTES.surveyById.path.replace(':id', String(id)))
    survey.value = data
    answerMap.value = {}
    data.questions.forEach((q) => (answerMap.value[q.id] = 0))
  } catch (err: any) {
    window.alert(err.message || '获取题目失败')
  } finally {
    isLoading.value = false
  }
}

const hasAnsweredAll = computed(() => {
  const questions = survey.value.questions
  if (questions.length === 0) return false
  return questions.every((q) => (answerMap.value[q.id] || 0) > 0)
})

const answeredCount = computed(
  () => survey.value.questions.filter((q) => (answerMap.value[q.id] || 0) > 0).length,
)
const totalCount = computed(() => survey.value.questions.length)
const progressPercent = computed(() =>
  totalCount.value === 0 ? 0 : Math.round((answeredCount.value / totalCount.value) * 100),
)

const handleSubmit = async () => {
  if (!hasAnsweredAll.value) return
  // 提交 **option.score**，不是 option.id（E2E-F-97）。
  //
  // 后端 `SubmitSurveyReq.Answers` 的契约是 `map[questionId]score`，scorer 直接按
  // score 求和/校验。原实现提交的是选项 id —— 只有当种子数据恰好 `id == score`
  // 时才正确（BIG5 是这样），而 PHQ-9/GAD-7 的选项是 `id 1..4 / score 0..3`
  // （id = score + 1），于是：全选"完全没有"被算成 9 分/mild（假阳性），
  // 选"几乎每天"(id=4) 直接 HTTP 400（4 超出 score 值域 0-3）。
  const answers: Record<string, number> = {}
  survey.value.questions.forEach((q) => {
    const selectedId = answerMap.value[q.id]
    if (!selectedId) return
    const opt = q.options.find((o) => o.id === selectedId)
    if (opt) answers[q.id] = opt.score
  })
  isSubmitting.value = true
  try {
    const result = await post<SurveyResult>(
      API_ROUTES.submitSurvey.path.replace(':id', String(survey.value.id)),
      { answers },
    )
    submitResult.value = result
    resultDialogVisible.value = true
  } catch (err: any) {
    window.alert(err.message || '提交失败')
  } finally {
    isSubmitting.value = false
  }
}

const riskLevelLabel = (level: string) => {
  const map: Record<string, string> = {
    none: '正常', mild: '轻度', moderate: '中度', severe: '重度', extreme: '极重度',
  }
  return map[level] ?? level
}

// E2E-14：人格量表结果按画像渲染（雷达图 + 维度分档），症状量表仍走总分/等级
const personalityDimensions = PERSONALITY_DIMENSIONS
const isPersonality = computed(
  () => !!submitResult.value && isPersonalityResult(submitResult.value.riskLevel),
)
const levelFor = (key: string) => {
  const score = submitResult.value?.factorScores?.[key]
  return typeof score === 'number' ? personalityLevel(score) : '-'
}

const handleResultConfirm = () => {
  resultDialogVisible.value = false
  navigateTo({ name: 'question' })
}

onMounted(() => {
  const questionId = route.params.id as string | number
  if (questionId) getQuestionDetail(questionId)
  else {
    window.alert('缺少题目ID')
    isLoading.value = false
  }
})
</script>

<style scoped lang="scss">
.assessment-detail {
  width: min(880px, 100%);
  margin: 0 auto;
}
.detail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 4px 18px;
}
.back-button {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  cursor: pointer;
  font-size: 13px;
}
.back-button:hover {
  color: var(--ee-primary);
  border-color: var(--ee-primary);
}
.eyebrow {
  color: var(--ee-primary);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.16em;
}

.card {
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  padding: 28px clamp(20px, 4vw, 36px);
}
.question-container {
  display: grid;
  gap: 22px;
}
.question-intro h2 {
  font-size: clamp(20px, 2.4vw, 26px);
  font-weight: 600;
  letter-spacing: -0.02em;
  margin: 0;
}
.question-intro p {
  margin: 8px 0 0;
  color: var(--ee-text-muted);
  font-size: 14px;
}
.progress {
  display: grid;
  gap: 6px;
  margin-top: 18px;
  color: var(--ee-text-muted);
  font-size: 12px;
  letter-spacing: 0.04em;
}
.progress-bar {
  appearance: none;
  width: 100%;
  height: 6px;
  background: var(--ee-surface-muted);
  border-radius: 999px;
  overflow: hidden;
  border: 0;
}
.progress-bar::-webkit-progress-bar {
  background: var(--ee-surface-muted);
  border-radius: 999px;
}
.progress-bar::-webkit-progress-value {
  background: var(--ee-primary);
  border-radius: 999px;
  transition: width var(--ee-transition);
}
.progress-bar::-moz-progress-bar {
  background: var(--ee-primary);
}

.question-block {
  display: grid;
  gap: 12px;
  padding: 18px 0;
  border-top: 1px solid var(--ee-border);
}
.question-block h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.radio-row {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}
.radio-option {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 40px;
  padding: 0 14px;
  color: var(--ee-text);
  background: var(--ee-surface-muted);
  border: 1px solid transparent;
  border-radius: var(--ee-radius-md);
  cursor: pointer;
  font-size: 14px;
  transition:
    background var(--ee-transition),
    color var(--ee-transition),
    border-color var(--ee-transition);
}
.radio-option:hover {
  background: var(--ee-primary-soft);
}
.radio-option.active {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  border-color: var(--ee-primary);
  font-weight: 600;
}
.radio-option input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
}

.submit-btn-wrap {
  display: flex;
  justify-content: center;
}

.ee-btn:hover {
  background: var(--ee-surface-muted);
}
.ee-btn:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}
.ee-btn-lg {
  height: 44px;
  padding: 0 28px;
  font-size: 14px;
}
.btn-ghost {
  background: transparent;
  color: var(--ee-text-muted);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 80px 8px;
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

.ee-skeleton {
  background: linear-gradient(
    90deg,
    var(--ee-surface-muted) 25%,
    color-mix(in srgb, var(--ee-surface-muted) 50%, var(--ee-surface)) 50%,
    var(--ee-surface-muted) 75%
  );
  background-size: 200% 100%;
  animation: skeleton-shimmer 1.4s ease-in-out infinite;
  border-radius: var(--ee-radius-sm, 6px);
}
@keyframes skeleton-shimmer {
  from {
    background-position: 200% 0;
  }
  to {
    background-position: -200% 0;
  }
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
.modal-card p {
  margin: 0 0 8px;
  color: var(--ee-text-muted);
  font-size: 13px;
  line-height: 1.6;
}
.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

.result-content {
  display: grid;
  gap: 10px;
  margin-top: 8px;
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
  margin-top: 8px;
}

.dimension-list {
  margin: 12px 0 0;
  padding: 0;
  list-style: none;
  display: grid;
  gap: 6px;
}
.dimension-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--ee-border);
  font-size: 13px;
}
.dimension-name {
  color: var(--ee-text-muted);
}
.dimension-score {
  display: inline-flex;
  align-items: baseline;
  gap: 6px;
  color: var(--ee-text);
  font-weight: 600;
}
.dimension-level {
  color: var(--ee-primary);
  font-size: 12px;
  font-style: normal;
  font-weight: 600;
}

@media (max-width: 600px) {
  .detail-header {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }
  .radio-row {
    flex-direction: column;
    align-items: stretch;
  }
  .radio-option {
    width: 100%;
  }
}
</style>
