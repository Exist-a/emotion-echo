<template>
  <section class="agent-page">
    <!-- 布局的 <header> 已经给出「EMOTION ECHO / 角色」这个大标题（app/layouts/nav.vue），
         所以这里**不再重复**第二遍标题 —— 之前页面自己又来了一组
         「ROLES / 角色」，屏幕上一上一下两个同名标题。这里只留一句说明。 -->
    <p class="agent-lede">
      角色决定它怎么称呼你、用什么语气说话。对话里可以随时切换；改这里不会改写已经说过的话。
    </p>

    <div class="agent-bar">
      <p
        class="agent-quota"
        :aria-label="`已建 ${agents.length} 个角色，上限 ${AGENT_LIMITS.profilesPerUser} 个`"
      >
        <span class="agent-quota-count">{{ agents.length }} / {{ AGENT_LIMITS.profilesPerUser }}</span>
        <span class="agent-quota-meter" aria-hidden="true">
          <i
            v-for="slot in AGENT_LIMITS.profilesPerUser"
            :key="slot"
            class="agent-quota-slot"
            :class="{ 'is-used': slot <= agents.length }"
          />
        </span>
      </p>

      <!-- 编辑器现在是真的模态弹窗（遮罩挡住整页）⇒ 不必再用 v-if 把入口藏起来防误点，
           按钮保持常驻，禁用条件也回到纯粹的一条：名额没满。 -->
      <button
        type="button"
        class="primary-btn"
        :disabled="!agentStore.canCreateMore"
        :title="agentStore.canCreateMore ? '' : `每户最多 ${AGENT_LIMITS.profilesPerUser} 个角色`"
        @click="startCreate"
      >
        新建角色
      </button>
      <span v-if="!agentStore.canCreateMore" class="limit-hint">
        已达上限（{{ AGENT_LIMITS.profilesPerUser }} 个）—— 删掉一个再建
      </span>
    </div>

    <p v-if="loading && agents.length === 0" class="agent-loading quiet-pulse">正在准备角色…</p>

    <div v-if="agents.length > 0" class="agent-list">
      <AgentCard
        v-for="a in agents"
        :key="a.id"
        :agent="a"
        @edit="startEdit(a)"
        @delete="handleDelete(a)"
      />
    </div>

    <!-- 空态是"邀请动作"，不是"报告故障"：把创建入口放在伸手可及的地方。 -->
    <button
      v-else-if="!loading"
      type="button"
      class="agent-empty-card"
      :disabled="!agentStore.canCreateMore"
      @click="startCreate"
    >
      <span class="agent-empty-plus" aria-hidden="true">＋</span>
      <span class="agent-empty-title">创建第一个角色</span>
      <span class="agent-empty-body">给它起个名字，写下你希望它怎么跟你说话。</span>
    </button>

    <!-- ============ 编辑角色：模态弹窗 ============
         为什么弹窗而不是内联面板（用户 2026-10-18 反馈"出现面板的逻辑很怪"）：
         内联面板把"正在编辑"和"列表"混在同一列流里，用户看不到边界在哪，
         而且它掉在列表下方要滚动才可见。模态把这件事一次讲清楚：现在在改这一件事，
         改完或取消再回到列表。模态该有的四件事都实现了：语义、Esc、点遮罩、锁背景滚动。 -->
    <div v-if="editing" class="agent-modal" @click.self="requestClose">
      <div
        ref="editorRef"
        class="agent-modal-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="agent-editor-title"
      >
        <header class="agent-modal-head">
          <div>
            <h3 id="agent-editor-title" class="agent-editor-title">
              {{ form.id ? '编辑角色' : '新建角色' }}
            </h3>
            <p class="agent-editor-note">改的是它怎么说话；它自己的身份与安全准则不在这里。</p>
          </div>
          <button type="button" class="agent-modal-close" aria-label="关闭" @click="requestClose">
            ×
          </button>
        </header>

        <form class="agent-modal-form" @submit.prevent="handleSave">
          <div class="agent-modal-body">
            <div class="agent-editor-main">
              <div class="field">
                <span class="field-label">角色名</span>
                <input
                  ref="nameRef"
                  v-model="form.name"
                  type="text"
                  :maxlength="AGENT_LIMITS.name"
                  placeholder="例如：小E、阿哲、树洞"
                  data-testid="agent-name"
                />
                <span class="field-meta">
                  <span class="field-hint">它自称的名字，也是卡片上的字</span>
                  <span class="field-counter">{{ count(form.name) }}/{{ AGENT_LIMITS.name }}</span>
                </span>
              </div>

              <div class="field">
                <span class="field-label">性别</span>
                <div class="seg" role="group" aria-label="性别（仅用于展示）">
                  <button
                    v-for="g in GENDERS"
                    :key="g"
                    type="button"
                    class="seg-btn"
                    :class="{ active: form.gender === g }"
                    :aria-pressed="form.gender === g"
                    @click="form.gender = g"
                  >
                    {{ g }}
                  </button>
                </div>
                <span class="field-meta">
                  <span class="field-hint">只影响展示，不改变音色</span>
                </span>
              </div>

              <div class="field">
                <span class="field-label">人设</span>
                <textarea
                  v-model="form.persona"
                  rows="3"
                  :maxlength="AGENT_LIMITS.persona"
                  placeholder="例如：一个直率、说话不绕弯的朋友，会指出我回避的问题。"
                  data-testid="agent-persona"
                />
                <span class="field-meta">
                  <span class="field-hint">它是个什么样的人</span>
                  <span class="field-counter">{{ count(form.persona) }}/{{ AGENT_LIMITS.persona }}</span>
                </span>
              </div>

              <div class="field">
                <span class="field-label">语气</span>
                <input
                  v-model="form.tone"
                  type="text"
                  :maxlength="AGENT_LIMITS.tone"
                  placeholder="例如：直接、简短"
                />
                <span class="field-meta">
                  <span class="field-counter">{{ count(form.tone) }}/{{ AGENT_LIMITS.tone }}</span>
                </span>
              </div>

              <div class="field">
                <span class="field-label">不要</span>
                <div v-for="(_, i) in form.boundaries" :key="i" class="boundary-row">
                  <input
                    v-model="form.boundaries[i]"
                    type="text"
                    :maxlength="AGENT_LIMITS.boundary"
                    placeholder="例如：不评价外貌"
                    :data-testid="`agent-boundary-${i}`"
                  />
                  <button
                    type="button"
                    class="ghost-btn is-danger"
                    :aria-label="`移除第 ${i + 1} 条不要`"
                    @click="removeBoundary(i)"
                  >
                    移除
                  </button>
                </div>
                <button
                  v-if="form.boundaries.length < AGENT_LIMITS.boundariesCount"
                  type="button"
                  class="add-btn"
                  @click="addBoundary"
                >
                  ＋ 添加一条
                </button>
                <span class="field-meta">
                  <span class="field-hint">
                    明确不许它做的事，最多 {{ AGENT_LIMITS.boundariesCount }} 条（已填
                    {{ form.boundaries.length }} 条）
                  </span>
                </span>
              </div>

              <div class="field">
                <span class="field-label">音色</span>
                <!-- 自绘下拉：原生 <select> 的展开层由浏览器绘制、不在 DOM 里，CSS 改不动 -->
                <EeSelect
                  v-model="form.voice"
                  :options="voiceOptions"
                  testid="agent-voice"
                  aria-label="音色"
                />
              </div>
            </div>

            <aside class="agent-editor-aside">
              <p class="aside-label">预览</p>
              <AgentCard :agent="draftProfile" variant="preview" />
              <div class="agent-preview" data-testid="agent-preview">
                <p class="aside-label">它会这样认识自己</p>
                <pre>{{ preview }}</pre>
              </div>
            </aside>
          </div>

          <footer class="agent-modal-foot">
            <ul v-if="errors.length" class="agent-errors" data-testid="agent-errors" role="alert">
              <li v-for="e in errors" :key="e">{{ e }}</li>
            </ul>

            <!-- 关掉弹窗会丢掉正在写的字 ⇒ 先问一句，而不是静默丢弃 -->
            <div v-if="discardConfirm" class="agent-discard">
              <span class="agent-discard-text">放弃未保存的修改？</span>
              <button type="button" class="ghost-btn" @click="closeEditor">放弃修改</button>
              <button type="button" class="primary-btn" @click="discardConfirm = false">
                继续编辑
              </button>
            </div>

            <div v-else class="agent-form-actions">
              <button type="submit" class="primary-btn" :disabled="loading" data-testid="agent-save">
                {{ loading ? '保存中…' : '保存' }}
              </button>
              <button type="button" class="ghost-btn" @click="requestClose">取消</button>
            </div>
          </footer>
        </form>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
// CL-06 · 角色管理页（侧边栏「角色」）。
//
// 边界：本页只负责"表单 ↔ payload"的映射与早提示；上限与内容黑名单由服务端强制。
// 角色是**全局**的（不属于某个会话）；"这个会话用谁"由对话页的切换器决定（写会话）。
//
// UI 轮的取舍（用户 2026-10-17 / 10-18 反馈）：
//   · 颜色 —— 全页改用项目令牌 `--ee-*`。此前满屏 `var(--primary-color, #6366f1)` 之类
//     **从未定义**的变量名，于是每次都被 var() 的兜底值接管：靛蓝按钮长在鼠尾草绿的
//     应用里，且深色模式完全不跟随。守卫见 `assets/scss/no-undefined-tokens.test.ts`。
//   · 渲染逻辑 —— 去掉与布局重复的第二个标题；编辑器从"掉在列表底下的内联面板"
//     改成**模态弹窗**（用户："出现面板的逻辑很怪，不如出现一个弹窗"）。
//   · 音色 —— 原生 <select> 换成自绘 `EeSelect`（展开层由浏览器绘制，CSS 改不动）。
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AgentProfile } from '~/types/api'
import { useAgentStore } from '~/stores/agent'
import { useNotify } from '~/composables/useNotify'
import AgentCard from '~/components/AgentCard.vue'
import EeSelect from '~/components/EeSelect.vue'
import {
  AGENT_LIMITS,
  VOICE_OPTIONS,
  buildAgentPreview,
  emptyAgentForm,
  formFromProfile,
  formToPayload,
  validateAgentForm,
  type AgentForm,
  type AgentGender,
} from '~/utils/agentForm'

const agentStore = useAgentStore()
const notify = useNotify()

const agents = computed(() => agentStore.agents)
const loading = computed(() => agentStore.loading)

const editing = ref(false)
const discardConfirm = ref(false)
const form = ref<AgentForm>(emptyAgentForm())
const errors = ref<string[]>([])
/** 打开时的表单快照：判断"改没改过"，用于关闭前的确认 */
const baseline = ref('')
const editorRef = ref<HTMLElement | null>(null)
const nameRef = ref<HTMLInputElement | null>(null)
/** 打开前的焦点位置：关掉弹窗后还回去（键盘用户不该被丢回页面开头） */
let restoreFocusTo: HTMLElement | null = null

const preview = computed(() => buildAgentPreview(form.value))

const GENDERS: AgentGender[] = ['女', '男', '不指定']

/** 音色选项（带分组）。数据仍只有 VOICE_OPTIONS 一处来源。 */
const voiceOptions = computed(() =>
  VOICE_OPTIONS.map((v) => ({
    value: v.value,
    label: v.label,
    group: v.gender === '女' ? '女声' : '男声',
  })),
)

/** 预览用的"假档案"：直接由表单长出来，所以右侧卡片跟输入框永远同源。 */
const draftProfile = computed<AgentProfile>(() => ({
  id: form.value.id ?? 0,
  name: form.value.name,
  gender: form.value.gender,
  persona: form.value.persona,
  tone: form.value.tone,
  boundaries: form.value.boundaries.filter((b) => b.trim().length > 0),
  voice: form.value.voice,
  isDefault: false,
}))

const dirty = computed(() => JSON.stringify(form.value) !== baseline.value)

/** 与服务端同款计长：按**字符**（码点）而不是 UTF-16 码元，emoji 才算 1 个字。 */
const count = (s: string) => Array.from(s ?? '').length

onMounted(() => {
  agentStore.fetchAgents()
})

const openEditor = async (next: AgentForm) => {
  form.value = next
  baseline.value = JSON.stringify(next)
  errors.value = []
  discardConfirm.value = false
  editing.value = true
  restoreFocusTo = (document.activeElement as HTMLElement) ?? null
  await nextTick()
  nameRef.value?.focus()
}

const startCreate = () => openEditor(emptyAgentForm())

const startEdit = (a: AgentProfile) => openEditor(formFromProfile(a))

/** 无条件关闭（保存成功 / 用户确认放弃 / 正在编辑的角色被删掉） */
const closeEditor = () => {
  editing.value = false
  discardConfirm.value = false
  errors.value = []
  restoreFocusTo?.focus?.()
  restoreFocusTo = null
}

/** 带"改动确认"的关闭：Esc / × / 点遮罩 / 取消 都走这里 */
const requestClose = () => {
  if (loading.value) return // 保存进行中不要关
  if (dirty.value) {
    discardConfirm.value = true
    return
  }
  closeEditor()
}

/**
 * 模态的三件配套：Esc 关闭、锁背景滚动、（关闭时还原）。
 * Esc 用 document 级监听而不是面板上的 @keydown.esc —— 用户点了遮罩后焦点就不在面板里了，
 * 只挂面板会在那种情况下失灵。
 */
const onKeydown = (e: KeyboardEvent) => {
  if (e.key === 'Escape') requestClose()
}

watch(editing, (isOpen) => {
  if (typeof document === 'undefined') return
  if (isOpen) {
    document.addEventListener('keydown', onKeydown)
    document.body.style.overflow = 'hidden'
  } else {
    document.removeEventListener('keydown', onKeydown)
    document.body.style.overflow = ''
  }
})

onBeforeUnmount(() => {
  if (typeof document === 'undefined') return
  document.removeEventListener('keydown', onKeydown)
  document.body.style.overflow = ''
})

const addBoundary = () => {
  if (form.value.boundaries.length < AGENT_LIMITS.boundariesCount) {
    form.value.boundaries.push('')
  }
}

const removeBoundary = (i: number) => {
  form.value.boundaries.splice(i, 1)
}

const handleSave = async () => {
  const errs = validateAgentForm(form.value)
  errors.value = errs
  if (errs.length > 0) return

  const isEdit = Boolean(form.value.id)
  const saved = await agentStore.saveAgent(formToPayload(form.value))
  if (saved) {
    closeEditor()
    notify.success(isEdit ? '角色已更新' : '角色已创建')
  } else {
    // 服务端的内容黑名单错误（形如 `field=persona category=jailbreak …`）也走这里，
    // 用户能看到具体是哪一类问题。
    errors.value = [agentStore.error || '保存失败']
  }
}

const handleDelete = async (a: AgentProfile) => {
  const ok = await agentStore.removeAgent(a.id)
  if (ok) {
    notify.success(`已删除「${a.name}」`)
    // 删掉的正是编辑器里在改的那个 ⇒ 直接关掉（表单对着已不存在的档案没有意义，
    // 这时不该再问"要放弃修改吗"）。
    if (editing.value && form.value.id === a.id) closeEditor()
  } else {
    notify.error(agentStore.error || '删除失败')
  }
}
</script>

<style scoped lang="scss">
.agent-page {
  /* 不写 padding：容器内边距由 app/layouts/nav.vue 的 .page-content 提供。
     之前这里加了一层自己的 padding: 24px，导致本页内容比上方标题右移 24px。
     也不居中：左边缘要与布局标题左边缘对齐。 */
  max-width: 900px;
}

.agent-lede {
  margin: 0 0 20px;
  color: var(--ee-text-muted);
  font-size: 14px;
}

/* ---- 计数 + 主入口 ---- */
.agent-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 18px;
}

.agent-quota {
  display: flex;
  align-items: center;
  gap: 9px;
  margin: 0;
}

.agent-quota-count {
  color: var(--ee-text-muted);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.02em;
}

/* 5 个格子 = 5 个名额。它编码的是真实上限，不是装饰。 */
.agent-quota-meter {
  display: inline-flex;
  gap: 3px;
}

.agent-quota-slot {
  width: 12px;
  height: 4px;
  background: var(--ee-surface-muted);
  border-radius: 999px;
  transition: background var(--ee-transition);
}

.agent-quota-slot.is-used {
  background: var(--ee-primary);
}

.agent-bar .primary-btn {
  margin-left: auto;
}

.limit-hint {
  color: var(--ee-text-muted);
  font-size: 12px;
}

/* ---- 卡片网格 ---- */
.agent-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
  gap: 12px;
}

.agent-loading {
  margin: 8px 0;
  color: var(--ee-text-muted);
  font-size: 13px;
}

.agent-empty-card {
  display: grid;
  justify-items: start;
  gap: 4px;
  width: 100%;
  padding: 28px 26px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 1px dashed color-mix(in srgb, var(--ee-primary) 35%, var(--ee-border));
  border-radius: var(--ee-radius-lg);
  text-align: left;
  transition:
    border-color var(--ee-transition),
    background var(--ee-transition);
}

.agent-empty-card:hover:not(:disabled) {
  background: var(--ee-primary-soft);
  border-color: var(--ee-primary);
}

.agent-empty-card:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.agent-empty-plus {
  color: var(--ee-primary);
  font-size: 20px;
  line-height: 1;
}

.agent-empty-title {
  color: var(--ee-text);
  font-size: 14px;
  font-weight: 600;
}

.agent-empty-body {
  font-size: 12.5px;
}

/* ---- 模态弹窗 ---- */
.agent-modal {
  position: fixed;
  inset: 0;
  z-index: 400;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background: color-mix(in srgb, var(--ee-text) 42%, transparent);
  animation: ee-fade-in var(--ee-transition);
}

.agent-modal-panel {
  display: flex;
  flex-direction: column;
  width: min(860px, 100%);
  max-height: min(84vh, 700px);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: var(--ee-shadow-soft);
}

.agent-modal-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 18px 22px 14px;
  border-bottom: 1px solid var(--ee-border);
}

.agent-editor-title {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
  letter-spacing: -0.01em;
}

.agent-editor-note {
  margin: 3px 0 0;
  color: var(--ee-text-muted);
  font-size: 12px;
}

.agent-modal-close {
  flex: 0 0 auto;
  width: 30px;
  height: 30px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 0;
  border-radius: var(--ee-radius-sm);
  font-size: 18px;
  line-height: 1;
  transition:
    color var(--ee-transition),
    background var(--ee-transition);
}

.agent-modal-close:hover {
  color: var(--ee-text);
  background: var(--ee-surface-muted);
}

.agent-modal-form {
  display: flex;
  min-height: 0;
  flex-direction: column;
}

/* 只有中段滚动：标题与按钮始终可见（内联面板时代做不到这一点） */
.agent-modal-body {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 300px;
  gap: 24px;
  align-items: start;
  min-height: 0;
  padding: 20px 22px;
  overflow-y: auto;
}

.agent-editor-aside {
  display: grid;
  gap: 10px;
}

.aside-label {
  margin: 0;
  color: var(--ee-text-muted);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.agent-preview pre {
  margin: 0;
  padding: 12px 14px;
  background: var(--ee-surface-muted);
  border-left: 2px solid color-mix(in srgb, var(--ee-primary) 45%, transparent);
  border-radius: var(--ee-radius-sm);
  color: var(--ee-text-muted);
  font-family: inherit;
  font-size: 12px;
  line-height: 1.75;
  white-space: pre-wrap;
  word-break: break-word;
}

.agent-modal-foot {
  padding: 14px 22px 18px;
  border-top: 1px solid var(--ee-border);
}

.agent-form-actions,
.agent-discard {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-form-actions {
  justify-content: flex-end;
}

.agent-discard-text {
  margin-right: auto;
  font-size: 13px;
  font-weight: 600;
}

/* ---- 字段 ---- */
.field {
  display: block;
  margin-bottom: 16px;
}

.field-label {
  display: block;
  margin-bottom: 5px;
  font-size: 12px;
  font-weight: 600;
}

.field-meta {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-top: 4px;
  min-height: 15px;
}

.field-hint,
.field-counter {
  color: var(--ee-text-muted);
  font-size: 11px;
}

.field-counter {
  margin-left: auto;
  font-variant-numeric: tabular-nums;
}

.field input,
.field textarea {
  width: 100%;
  padding: 9px 11px;
  background: var(--ee-bg);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font-family: inherit;
  font-size: 13.5px;
  transition:
    border-color var(--ee-transition),
    background var(--ee-transition);
}

.field textarea {
  resize: vertical;
  line-height: 1.7;
}

.field input:hover,
.field textarea:hover {
  border-color: color-mix(in srgb, var(--ee-primary) 40%, var(--ee-border));
}

.field input:focus,
.field textarea:focus {
  background: var(--ee-surface);
  border-color: var(--ee-primary);
  outline: none;
}

.field input::placeholder,
.field textarea::placeholder {
  color: color-mix(in srgb, var(--ee-text-muted) 75%, transparent);
}

/* ---- 性别三分段（3 个短选项用分段控件比下拉更省一次点击） ---- */
.seg {
  display: inline-flex;
  gap: 3px;
  padding: 3px;
  background: var(--ee-surface-muted);
  border-radius: 10px;
}

.seg-btn {
  min-width: 62px;
  height: 28px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 0;
  border-radius: 7px;
  font-size: 12.5px;
  font-weight: 600;
  transition:
    color var(--ee-transition),
    background var(--ee-transition);
}

.seg-btn:hover {
  color: var(--ee-text);
}

.seg-btn.active {
  color: var(--ee-text);
  background: var(--ee-surface);
  box-shadow: 0 1px 2px rgb(32 37 34 / 6%);
}

/* ---- 「不要」逐条 ---- */
.boundary-row {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
}

.boundary-row input {
  flex: 1;
}

.add-btn {
  padding: 6px 0;
  color: var(--ee-primary);
  background: none;
  border: 0;
  font-size: 12.5px;
  font-weight: 600;
}

.add-btn:hover {
  color: var(--ee-primary-hover);
}

/* ---- 按钮 ---- */
.primary-btn {
  height: 36px;
  padding: 0 16px;
  /* 令牌而不是 #fff：深色模式下 --ee-primary 是浅绿，白字会读不出来 */
  color: var(--ee-on-primary);
  background: var(--ee-primary);
  border: 1px solid var(--ee-primary);
  border-radius: var(--ee-radius-md);
  font-size: 13px;
  font-weight: 600;
  transition:
    background var(--ee-transition),
    border-color var(--ee-transition);
}

.primary-btn:hover:not(:disabled) {
  background: var(--ee-primary-hover);
  border-color: var(--ee-primary-hover);
}

.primary-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.ghost-btn {
  height: 36px;
  padding: 0 14px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font-size: 13px;
  font-weight: 600;
  transition:
    color var(--ee-transition),
    border-color var(--ee-transition);
}

.ghost-btn:hover {
  color: var(--ee-text);
  border-color: color-mix(in srgb, var(--ee-text-muted) 45%, var(--ee-border));
}

.ghost-btn.is-danger {
  height: 36px;
  color: var(--ee-danger);
  border-color: transparent;
}

.ghost-btn.is-danger:hover {
  background: var(--ee-danger-soft);
}

.agent-errors {
  margin: 0 0 12px;
  padding: 10px 12px 10px 30px;
  color: var(--ee-danger);
  background: var(--ee-danger-soft);
  border-radius: var(--ee-radius-md);
  font-size: 12.5px;
  list-style: disc;
}

/* ---- 窄屏：弹窗内的两栏收成一栏 ---- */
@media (max-width: 760px) {
  .agent-modal {
    padding: 12px;
  }

  .agent-modal-body {
    grid-template-columns: minmax(0, 1fr);
  }

  .agent-modal-panel {
    max-height: 92vh;
  }
}
</style>
