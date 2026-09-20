<template>
  <section class="user-container">
    <div class="user-info-card card">
      <img class="avatar" :src="avatarPath" alt="头像" />
      <div class="user-info">
        <span class="eyebrow">YOUR SPACE</span>
        <p ref="nicknameRef" class="nickname">{{ nickname }}</p>
        <p class="meta">{{ age }} 岁 · ID {{ id }}</p>
      </div>
      <div class="btn-group">
        <button type="button" class="ee-btn ee-btn-primary btn" @click="editInfo">修改资料</button>
        <button type="button" class="ee-btn btn" @click="navigateTo({ name: 'question' })">
          开始测验
        </button>
        <button type="button" class="ee-btn btn danger" @click="loginoutDialogVisible = true">
          退出登录
        </button>
      </div>
    </div>
    <div class="section-heading">
      <h2>和自己的相处</h2>
      <p>这些记录帮助你看见最近的节奏，不是评判。</p>
    </div>
    <div class="user-data-card card">
      <div v-for="item in chartData" :key="item.title" class="chart-item">
        <pieChart
          v-if="item.chartType === 'pie'"
          :data="item.data"
          :height="chartItemHeight"
          :title="item.title"
        />
        <lineChart
          v-if="item.chartType === 'line'"
          :height="chartItemHeight"
          :XData="item.XData"
          :YData="item.YData"
          :title="item.title"
        />
        <barChart
          v-if="item.chartType === 'bar'"
          :height="chartItemHeight"
          :XData="item.XData"
          :YData="item.YData"
          :title="item.title"
        />
      </div>
      <div v-if="isLoadingBehavior" class="ee-skeleton" />
      <div v-else-if="chartData.length === 0" class="ee-empty">暂无数据</div>
    </div>

    <!-- E2E-14：人格画像（D-02：人格量表产心理画像，驱动 AI 回复针对性） -->
    <div class="section-heading">
      <h2>我的人格画像</h2>
      <p>来自人格五因素量表，用于让对话更贴合你。</p>
    </div>
    <div class="user-data-card card personality-card">
      <div v-if="isLoadingPersonality" class="ee-skeleton" />
      <template v-else-if="personalityScores">
        <RadarChart
          :indicators="personalityIndicators()"
          :data="personalityRadarData(personalityScores)"
          :height="chartItemHeight || 300"
          title="人格维度"
        />
        <ul class="dimension-list">
          <li v-for="dim in personalityDimensions" :key="dim.key" class="dimension-row">
            <span class="dimension-name">{{ dim.label }}</span>
            <span class="dimension-score">
              {{ personalityScores[dim.key] ?? '-' }}
              <em class="dimension-level">{{ levelOf(dim.key) }}</em>
            </span>
          </li>
        </ul>
      </template>
      <div v-else class="ee-empty">
        <p>还没有人格测评结果</p>
        <button type="button" class="ee-btn ee-btn-primary" @click="navigateTo({ name: 'question' })">
          去测评
        </button>
      </div>
    </div>
  </section>

  <Teleport to="body">
    <div v-if="dialogFormVisible" class="ms-overlay" @click.self="dialogFormVisible = false">
      <div class="ms-dialog" role="dialog" aria-modal="true" aria-label="修改资料">
        <header class="ms-header">
          <h3>修改资料</h3>
          <button type="button" class="ms-close" aria-label="关闭" @click="dialogFormVisible = false">
            ✕
          </button>
        </header>
        <form class="profile-form" @submit.prevent="saveInfo">
          <label class="ee-field" data-label="头像">
            <span class="avatar-uploader" @click="triggerAvatarPick">
              <img v-if="form.avatarPath" :src="form.avatarPath" class="avatar" alt="头像预览" />
              <span class="ee-icon" aria-hidden="true">
                <svg
                  viewBox="0 0 24 24"
                  width="18"
                  height="18"
                  fill="none"
                  stroke="currentColor"
                  stroke-width="1.6"
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  aria-hidden="true"
                >
                  <path d="M12 5v14M5 12h14" />
                </svg>
              </span>
              <input
                ref="avatarInputRef"
                class="avatar-input"
                type="file"
                accept="image/*"
                @change="handleAvatarPick"
              />
            </span>
          </label>
          <label class="ee-field" data-label="昵称"
            ><input v-model="form.nickname" type="text" class="ee-input" autocomplete="off"
          /></label>
          <label class="ee-field" data-label="年龄"
            ><input v-model="form.age" type="number" class="ee-input" autocomplete="off"
          /></label>
          <div class="ms-actions">
            <button type="button" class="ee-btn" @click="dialogFormVisible = false">取消</button>
            <button type="submit" class="ee-btn ee-btn-primary">保存资料</button>
          </div>
        </form>
      </div>
    </div>
  </Teleport>

  <Teleport to="body">
    <div v-if="loginoutDialogVisible" class="ms-overlay" @click.self="loginoutDialogVisible = false">
      <div class="ms-dialog ms-dialog-sm" role="dialog" aria-modal="true" aria-label="离开这里">
        <header class="ms-header">
          <h3>离开这里</h3>
          <button
            type="button"
            class="ms-close"
            aria-label="关闭"
            @click="loginoutDialogVisible = false"
          >
            ✕
          </button>
        </header>
        <p class="ms-body">确定要退出当前账号吗？</p>
        <div class="ms-actions">
          <button type="button" class="ee-btn" @click="loginoutDialogVisible = false">留下</button>
          <button type="button" class="ee-btn ee-btn-primary" @click="handleLogout">
            确认退出
          </button>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import pieChart from '~/components/charts/pieChart.vue'
import lineChart from '~/components/charts/lineChart.vue'
import barChart from '~/components/charts/barChart.vue'
import RadarChart from '~/components/charts/RadarChart.vue'
import type { ChartItem } from '~/types/charts/common'
import {
  PERSONALITY_DIMENSIONS,
  hasPersonalityScores,
  isPersonalityResult,
  personalityIndicators,
  personalityLevel,
  personalityRadarData,
} from '~/configs/personality'
import { ref, onMounted } from 'vue'
import { get, post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { notify } from '~/composables/useNotify'

const userStore = useUserStore()
// E2E-11：必须包 computed —— Pinia 的 defineStore(setup) 返回值经 store 代理后
// ref 会被解包，`const nickname = userStore.getNickname` 拿到的是**普通字符串快照**
// （setup 时 userInfo 仍为 null ⇒ 冻结在兜底值 "用户"/18/空 ID，永不更新），
// 且 `nickname.value` 为 undefined（导致编辑弹框回填为空）。
const nickname = computed(() => userStore.getNickname)
const avatarPath = computed(() => userStore.getAvatarPath)
const age = computed(() => userStore.getAge)
const id = computed(() => userStore.getId)
const dialogFormVisible = ref(false)
const chartItemHeight = ref(0)
const loginoutDialogVisible = ref(false)
const avatarInputRef = ref<HTMLInputElement | null>(null)

const form = ref<{
  nickname: string
  avatarPath: string
  age: number
}>({
  nickname: '',
  avatarPath: '',
  age: 18,
})

const validateInfo = () => {
  form.value.nickname = form.value.nickname.trim()
  if (!nicknameReg.test(form.value.nickname)) {
    notify(
      '',
      '昵称格式错误，长度为2-12个字符，包含中英文（含繁体）、数字（全角/半角）、下划线、横线、中文间隔号',
      'warning',
      3000,
    )
    return false
  }
  form.value.age = +form.value.age
  if (form.value.age < 0 || form.value.age > 130) {
    notify('', '年龄格式错误，应在0-130之间', 'warning', 3000)
    return false
  }
  return true
}

/**
 * 触发原生文件选择（E2E-11：原 <el-upload> 未解析，改用原生 input[type=file]）
 */
const triggerAvatarPick = () => {
  avatarInputRef.value?.click()
}

/**
 * 处理选中的头像文件：先校验大小，再上传。
 *
 * 上传链路：BFF POST /api/v1/user/avatar → MinIO → user-svc 落库。
 */
const handleAvatarPick = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  // 允许重复选择同一文件（否则第二次不触发 change）
  input.value = ''
  if (!file) return
  if (!beforeAvatarUpload(file)) return

  // 先本地预览，上传成功后替换为服务端返回的公开 URL
  form.value.avatarPath = URL.createObjectURL(file)
  try {
    const formData = new FormData()
    formData.append('avatar', file)
    const res = await post<{ avatar: string }>(API_ROUTES.userAvatar.path, formData)
    form.value.avatarPath = res.avatar
    // 同步更新 store（页面顶部头像）
    if (userStore.userInfo) {
      userStore.userInfo.avatar = res.avatar
    }
    notify('', '头像上传成功', 'success', 3000)
  } catch (error: any) {
    notify('', error?.message || '头像上传失败，请重试', 'error', 3000)
  }
}

const beforeAvatarUpload = (rawFile: File) => {
  if (rawFile.size / 1024 / 1024 > 2) {
    notify('', '头像不能超过 2MB', 'error', 3000)
    return false
  }
  return true
}

const editInfo = () => {
  // 打开时用当前值回填，避免上次未保存的编辑残留
  form.value.nickname = nickname.value as string
  form.value.avatarPath = avatarPath.value as string
  form.value.age = age.value as number
  dialogFormVisible.value = true
}

const saveInfo = async () => {
  if (!validateInfo()) return

  const [nickRes, ageRes] = await Promise.all([
    userStore.editNickname(form.value.nickname),
    userStore.editAge(form.value.age),
  ])

  if (!nickRes.isOk || !ageRes.isOk) {
    notify('', nickRes.msg || ageRes.msg || '保存失败，请重试', 'error', 3000)
    return
  }

  notify('', '信息修改成功！', 'success', 3000)
  dialogFormVisible.value = false
}

// 用户行为数据
const behaviorData = ref({
  dayNight: null as any,
  depth: null as any,
  frequency: null as any,
})
const isLoadingBehavior = ref(false)

// 获取用户行为数据
const fetchBehaviorData = async () => {
  isLoadingBehavior.value = true
  try {
    const [dayNight, depth, frequency] = await Promise.all([
      get(API_ROUTES.userBehaviorDayNight.path),
      get(API_ROUTES.userBehaviorDepth.path),
      get(API_ROUTES.userBehaviorFrequency.path),
    ])
    behaviorData.value.dayNight = dayNight
    behaviorData.value.depth = depth
    behaviorData.value.frequency = frequency
  } catch (error: any) {
    notify('', error?.message || '获取行为数据失败', 'warning', 3000)
  } finally {
    isLoadingBehavior.value = false
  }
}

// ==================== E2E-14：人格画像 ====================

const personalityDimensions = PERSONALITY_DIMENSIONS
const personalityScores = ref<Record<string, number> | null>(null)
const isLoadingPersonality = ref(false)

const levelOf = (key: string) => {
  const score = personalityScores.value?.[key]
  return typeof score === 'number' ? personalityLevel(score) : '-'
}

/**
 * 取最新人格量表结果的维度分。
 *
 * 列表按 submittedAt 倒序（assessment-svc `ORDER BY submitted_at DESC`），
 * 取第一条带维度分的人格结果即可；无结果不是错误 —— 页面展示引导去测评。
 */
const fetchPersonality = async () => {
  isLoadingPersonality.value = true
  try {
    const data = await get<{ items?: Array<Record<string, any>> }>(API_ROUTES.surveyResults.path)
    const latest = (data?.items ?? []).find(
      (it) => isPersonalityResult(it.riskLevel) && hasPersonalityScores(it.factorScores),
    )
    personalityScores.value = latest?.factorScores ?? null
  } catch (error: any) {
    notify('', error?.message || '获取人格画像失败', 'warning', 3000)
  } finally {
    isLoadingPersonality.value = false
  }
}

// 动态构建图表数据
const chartData = computed<ChartItem[]>(() => {
  const items: ChartItem[] = []

  // 1. 昼夜使用模式（饼图）
  if (behaviorData.value.dayNight?.periods?.length > 0) {
    items.push({
      chartType: 'pie',
      title: '昼夜使用模式',
      data: behaviorData.value.dayNight.periods.map((p: any) => ({
        name: `${p.label} (${p.hours})`,
        value: p.value,
      })),
    })
  }

  // 2. 对话频次趋势（折线图）
  if (behaviorData.value.frequency?.dates?.length > 0) {
    items.push({
      chartType: 'line',
      title: '近30天对话频次',
      XData: behaviorData.value.frequency.dates,
      YData: behaviorData.value.frequency.messageCount,
    })
  }

  // 3. 互动深度（柱状图）
  if (behaviorData.value.depth) {
    items.push({
      chartType: 'bar',
      title: '互动深度指标',
      XData: ['平均轮数', '最长连续(天)', '总会话', '总消息', '日均消息'],
      YData: [
        Math.round(behaviorData.value.depth.avgSessionRounds || 0),
        behaviorData.value.depth.maxConsecutiveDays || 0,
        behaviorData.value.depth.totalConversations || 0,
        behaviorData.value.depth.totalMessages || 0,
        Math.round(behaviorData.value.depth.avgMessagesPerDay || 0),
      ],
    })
  }

  return items
})

//确认退出登录
const handleLogout = async () => {
  try {
    await userStore.logout()

    notify('', '已退出登录', 'success', 3000)

    navigateTo('/login')
  } catch (error) {
    notify('', '退出登录失败', 'error', 3000)
  }
  loginoutDialogVisible.value = false
}
onMounted(async () => {
  chartItemHeight.value = vhToPx(40)
  // auth.global.ts 约定：userInfo 是页面元数据，由各页面 onMounted 自取。
  // 不取会用 store 兜底值（"用户"/18 岁/空 ID），编辑弹框也会回填错误数据。
  await userStore.fetchUserInfo().catch(() => {})
  fetchBehaviorData()
  fetchPersonality()
})
</script>

<style scoped lang="scss">
.user-container {
  width: min(1000px, 100%);
  margin: 0 auto;
}
.card {
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
}
.user-info-card {
  display: flex;
  align-items: center;
  gap: 24px;
  padding: clamp(20px, 4vw, 38px);
}
.avatar {
  width: 88px;
  height: 88px;
  border: 3px solid var(--ee-primary-soft);
}
.user-info {
  flex: 1;
  min-width: 0;
}
.eyebrow {
  color: var(--ee-primary);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.16em;
}
.nickname {
  margin-top: 5px;
  font-size: clamp(22px, 3vw, 30px);
  font-weight: 600;
  letter-spacing: -0.04em;
}
.meta {
  margin-top: 5px;
  color: var(--ee-text-muted);
  font-size: 13px;
}
.btn-group {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
}
.btn {
  min-height: 38px;
  border-radius: var(--ee-radius-md);
}
.btn.danger {
  color: var(--ee-accent);
  border-color: color-mix(in srgb, var(--ee-accent) 35%, var(--ee-border));
}
.section-heading {
  margin: 34px 0 14px;
}
.section-heading h2 {
  font-size: 20px;
  letter-spacing: -0.03em;
}
.section-heading p {
  margin-top: 4px;
  color: var(--ee-text-muted);
  font-size: 13px;
}
.user-data-card {
  display: grid;
  min-height: 240px;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 14px;
  padding: 16px;
}
.chart-item {
  min-height: 180px;
  padding: 10px;
  background: var(--ee-surface-muted);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
}
/* E2E-11：.ee-field / .ee-input 全仓无样式定义（global.scss 只定义 .ee-btn），
   data-label 不会渲染成标签 ⇒ 这里补本地样式，让字段标签可见 */
.profile-form .ee-field {
  display: block;
  margin-bottom: 14px;
}
.profile-form .ee-field::before {
  display: block;
  margin-bottom: 6px;
  color: var(--ee-text-muted);
  content: attr(data-label);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.profile-form .ee-input {
  width: 100%;
  padding: 9px 12px;
  color: var(--ee-text);
  background: var(--ee-surface-muted);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font-size: 14px;
}
.profile-form .ee-input:focus {
  border-color: var(--ee-primary);
  outline: none;
}
.profile-form {
  padding: 10px 20px;
}
.avatar-uploader {
  position: relative;
  display: block;
  width: 88px;
  height: 88px;
  border: 1px dashed var(--ee-border);
  border-radius: var(--ee-radius-md);
  cursor: pointer;
  transition: border-color 0.15s ease;
}
.avatar-uploader:hover {
  border-color: var(--ee-primary);
}
.avatar-uploader .avatar {
  display: block;
  width: 86px;
  height: 86px;
  object-fit: cover;
  border-radius: calc(var(--ee-radius-md) - 1px);
}
.avatar-uploader .ee-icon {
  position: absolute;
  right: -6px;
  bottom: -6px;
  display: grid;
  width: 26px;
  height: 26px;
  place-items: center;
  color: var(--ee-on-primary, #fff);
  background: var(--ee-primary);
  border: 2px solid var(--ee-surface);
  border-radius: 50%;
}
.avatar-input {
  display: none;
}

/* E2E-11：原生弹框（替代未解析的 <el-dialog>，对齐 SecurityQuestionDialog.vue） */
.ms-overlay {
  position: fixed;
  z-index: 2000;
  display: grid;
  background: rgb(0 0 0 / 45%);
  inset: 0;
  place-items: center;
  padding: 16px;
}
.ms-dialog {
  width: min(500px, calc(100vw - 32px));
  max-height: calc(100vh - 32px);
  overflow-y: auto;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: 0 18px 48px rgb(0 0 0 / 22%);
}
.ms-dialog-sm {
  width: min(420px, calc(100vw - 32px));
}
.ms-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 20px 8px;
}
.ms-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.ms-close {
  padding: 4px 8px;
  color: var(--ee-text-muted);
  background: none;
  border: none;
  border-radius: var(--ee-radius-sm);
  cursor: pointer;
  font-size: 14px;
}
.ms-close:hover {
  color: var(--ee-text);
  background: var(--ee-surface-muted);
}
.ms-body {
  padding: 4px 20px 12px;
  color: var(--ee-text-muted);
  font-size: 14px;
}
.ms-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding: 12px 20px 18px;
}
.avatar-uploader-icon {
  display: grid;
  width: 88px;
  height: 88px;
  place-items: center;
  color: var(--ee-text-muted);
  border: 1px dashed var(--ee-border);
  border-radius: var(--ee-radius-md);
  font-size: 22px;
}
.dimension-list {
  display: grid;
  gap: 8px;
  margin: 16px 0 0;
  padding: 0;
  list-style: none;
}
.dimension-row {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  padding-bottom: 8px;
  border-bottom: 1px solid var(--ee-border);
  font-size: 13px;
}
.dimension-name {
  color: var(--ee-text-muted);
}
.dimension-score {
  display: inline-flex;
  align-items: baseline;
  gap: 8px;
  color: var(--ee-text);
  font-weight: 600;
}
.dimension-level {
  color: var(--ee-primary);
  font-size: 12px;
  font-style: normal;
  font-weight: 600;
}
.personality-card {
  display: block;
}

@media (max-width: 700px) {
  .user-info-card {
    align-items: flex-start;
    flex-wrap: wrap;
  }
  .user-info {
    flex-basis: calc(100% - 120px);
  }
  .btn-group {
    width: 100%;
    justify-content: flex-start;
  }
  .btn {
    flex: 1;
  }
  .user-data-card {
    grid-template-columns: 1fr;
  }
}
</style>
