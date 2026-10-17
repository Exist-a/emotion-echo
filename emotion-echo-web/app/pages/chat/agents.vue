<template>
  <section class="agent-page">
    <header class="page-intro">
      <span class="eyebrow">ROLES</span>
      <h2>角色</h2>
      <p>给每个角色一个名字和相处方式；对话里可以随时切换。</p>
    </header>

    <!-- 列表 -->
    <div class="agent-list">
      <article v-for="a in agents" :key="a.id" class="agent-card" :data-agent-card="a.id">
        <div class="agent-card-head">
          <span class="agent-avatar" aria-hidden="true">{{ a.name.slice(0, 1) }}</span>
          <h3 class="agent-card-name">{{ a.name }}</h3>
          <span v-if="a.isDefault" class="agent-badge">默认</span>
        </div>
        <p class="agent-card-persona">{{ a.persona || '（还没有人设）' }}</p>
        <p class="agent-card-meta">
          <span v-if="a.tone">语气：{{ a.tone }}</span>
          <span class="agent-card-voice">{{ voiceLabel(a.voice) }}</span>
        </p>
        <div class="agent-card-actions">
          <button type="button" class="link-btn" @click="startEdit(a)">编辑</button>
          <button type="button" class="link-btn danger" @click="handleDelete(a)">删除</button>
        </div>
      </article>
    </div>

    <p v-if="agents.length === 0 && !loading" class="agent-empty">
      还没有角色。点下面「新建角色」创建一个，或刷新页面让系统准备默认角色。
    </p>

    <div class="agent-toolbar">
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

    <!-- 编辑表单 -->
    <form v-if="editing" class="agent-form" @submit.prevent="handleSave">
      <h3>{{ form.id ? '编辑角色' : '新建角色' }}</h3>

      <label class="field">
        <span class="field-label">角色名</span>
        <input
          v-model="form.name"
          type="text"
          :maxlength="AGENT_LIMITS.name"
          placeholder="例如：小E、阿哲、树洞"
          data-testid="agent-name"
        />
        <span class="field-counter">{{ form.name.length }}/{{ AGENT_LIMITS.name }}</span>
      </label>

      <label class="field">
        <span class="field-label">性别（仅用于展示）</span>
        <select v-model="form.gender">
          <option value="女">女</option>
          <option value="男">男</option>
          <option value="不指定">不指定</option>
        </select>
      </label>

      <label class="field">
        <span class="field-label">人设（它是个什么样的人）</span>
        <textarea
          v-model="form.persona"
          rows="3"
          :maxlength="AGENT_LIMITS.persona"
          placeholder="例如：一个直率、说话不绕弯的朋友，会指出我回避的问题。"
          data-testid="agent-persona"
        />
        <span class="field-counter">{{ form.persona.length }}/{{ AGENT_LIMITS.persona }}</span>
      </label>

      <label class="field">
        <span class="field-label">语气</span>
        <input
          v-model="form.tone"
          type="text"
          :maxlength="AGENT_LIMITS.tone"
          placeholder="例如：直接、简短"
        />
      </label>

      <div class="field">
        <span class="field-label">不要（最多 {{ AGENT_LIMITS.boundariesCount }} 条）</span>
        <div v-for="(_, i) in form.boundaries" :key="i" class="boundary-row">
          <input
            v-model="form.boundaries[i]"
            type="text"
            :maxlength="AGENT_LIMITS.boundary"
            placeholder="例如：不评价外貌"
            :data-testid="`agent-boundary-${i}`"
          />
          <button type="button" class="link-btn danger" @click="removeBoundary(i)">移除</button>
        </div>
        <button
          v-if="form.boundaries.length < AGENT_LIMITS.boundariesCount"
          type="button"
          class="link-btn"
          @click="addBoundary"
        >
          + 添加一条
        </button>
      </div>

      <label class="field">
        <span class="field-label">音色</span>
        <select v-model="form.voice" data-testid="agent-voice">
          <option v-for="v in VOICE_OPTIONS" :key="v.value" :value="v.value">
            {{ v.label }}
          </option>
        </select>
      </label>

      <!-- 预览：让用户看到"这个角色会怎么自我介绍"（T-15） -->
      <div class="agent-preview" data-testid="agent-preview">
        <span class="field-label">预览</span>
        <pre>{{ preview }}</pre>
      </div>

      <ul v-if="errors.length" class="agent-errors" data-testid="agent-errors">
        <li v-for="e in errors" :key="e">{{ e }}</li>
      </ul>

      <div class="agent-form-actions">
        <button type="submit" class="primary-btn" :disabled="loading" data-testid="agent-save">
          {{ loading ? '保存中…' : '保存' }}
        </button>
        <button type="button" class="link-btn" @click="cancelEdit">取消</button>
      </div>
    </form>
  </section>
</template>

<script setup lang="ts">
// CL-06 · 角色管理页（侧边栏「角色」）。
//
// 边界：本页只负责"表单 ↔ payload"的映射与早提示；上限与内容黑名单由服务端强制。
// 角色是**全局**的（不属于某个会话）；"这个会话用谁"由对话页的切换器决定（写会话）。
import { computed, onMounted, ref } from 'vue'
import type { AgentProfile } from '~/types/api'
import { useAgentStore } from '~/stores/agent'
import { useNotify } from '~/composables/useNotify'
import {
  AGENT_LIMITS,
  VOICE_OPTIONS,
  buildAgentPreview,
  emptyAgentForm,
  formFromProfile,
  formToPayload,
  validateAgentForm,
  type AgentForm,
} from '~/utils/agentForm'

const agentStore = useAgentStore()
const notify = useNotify()

const agents = computed(() => agentStore.agents)
const loading = computed(() => agentStore.loading)

const editing = ref(false)
const form = ref<AgentForm>(emptyAgentForm())
const errors = ref<string[]>([])

const preview = computed(() => buildAgentPreview(form.value))

onMounted(() => {
  agentStore.fetchAgents()
})

const voiceLabel = (value: string) =>
  VOICE_OPTIONS.find((v) => v.value === value)?.label ?? (value || '默认音色')

const startCreate = () => {
  form.value = emptyAgentForm()
  errors.value = []
  editing.value = true
}

const startEdit = (a: AgentProfile) => {
  form.value = formFromProfile(a)
  errors.value = []
  editing.value = true
}

const cancelEdit = () => {
  editing.value = false
  errors.value = []
}

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

  const saved = await agentStore.saveAgent(formToPayload(form.value))
  if (saved) {
    editing.value = false
    notify.success('角色已保存')
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
  } else {
    notify.error(agentStore.error || '删除失败')
  }
}
</script>

<style scoped>
.agent-page {
  padding: 24px;
}

.agent-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 12px;
  margin: 16px 0;
}

.agent-card {
  padding: 12px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 10px;
}

.agent-card-head {
  display: flex;
  align-items: center;
  gap: 8px;
}

.agent-card-name {
  margin: 0;
  font-size: 15px;
}

.agent-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--primary-color, #6366f1);
  color: #fff;
  font-size: 12px;
}

.agent-badge {
  font-size: 10px;
  padding: 0 4px;
  border-radius: 4px;
  background: var(--hover-bg, #f3f4f6);
  color: var(--text-muted, #9ca3af);
}

.agent-card-persona {
  margin: 8px 0 4px;
  font-size: 13px;
  color: var(--text-muted, #6b7280);
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.agent-card-meta {
  display: flex;
  gap: 8px;
  font-size: 12px;
  color: var(--text-muted, #9ca3af);
  margin: 0;
}

.agent-card-actions,
.agent-form-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}

.agent-toolbar {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}

.limit-hint {
  font-size: 12px;
  color: var(--text-muted, #9ca3af);
}

.primary-btn {
  padding: 8px 14px;
  border: none;
  border-radius: 8px;
  background: var(--primary-color, #6366f1);
  color: #fff;
  cursor: pointer;
}

.primary-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.link-btn {
  border: none;
  background: none;
  color: var(--primary-color, #6366f1);
  cursor: pointer;
  font-size: 13px;
  padding: 4px 0;
}

.link-btn.danger {
  color: var(--danger-color, #ef4444);
}

.agent-form {
  max-width: 560px;
  padding: 16px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
}

.field {
  display: block;
  margin-bottom: 12px;
}

.field-label {
  display: block;
  font-size: 12px;
  color: var(--text-muted, #6b7280);
  margin-bottom: 4px;
}

.field input,
.field textarea,
.field select {
  width: 100%;
  padding: 8px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 8px;
  font: inherit;
}

.field-counter {
  float: right;
  font-size: 11px;
  color: var(--text-muted, #9ca3af);
}

.boundary-row {
  display: flex;
  gap: 8px;
  margin-bottom: 6px;
}

.agent-preview pre {
  margin: 0;
  padding: 10px;
  border-radius: 8px;
  background: var(--hover-bg, #f9fafb);
  font-size: 12px;
  white-space: pre-wrap;
  word-break: break-word;
}

.agent-errors {
  margin: 8px 0;
  padding-left: 18px;
  color: var(--danger-color, #ef4444);
  font-size: 13px;
}
</style>
