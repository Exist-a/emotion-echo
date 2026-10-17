<template>
  <div class="agent-switcher">
    <button
      ref="triggerRef"
      type="button"
      class="agent-trigger"
      :class="{ active: open }"
      :aria-expanded="open ? 'true' : 'false'"
      aria-haspopup="listbox"
      :aria-label="`当前角色：${activeAgent?.name ?? '未设置'}，点击切换`"
      @click="toggle"
    >
      <span class="agent-avatar" aria-hidden="true">{{ initial }}</span>
      <span class="agent-name">{{ activeAgent?.name ?? '角色' }}</span>
    </button>

    <!-- 向上展开（composer 在页面底部，向下展开会被视口截断） -->
    <div v-if="open" class="agent-menu" role="listbox" aria-label="选择角色">
      <p class="agent-menu-title">切换到</p>
      <button
        v-for="a in agents"
        :key="a.id"
        type="button"
        class="agent-option"
        :class="{ current: a.id === activeAgent?.id }"
        role="option"
        :aria-selected="a.id === activeAgent?.id ? 'true' : 'false'"
        :data-agent-option="a.id"
        @click="select(a)"
      >
        <span class="agent-avatar" aria-hidden="true">{{ a.name.slice(0, 1) }}</span>
        <span class="agent-option-text">
          <span class="agent-option-name">{{ a.name }}</span>
          <span v-if="a.isDefault" class="agent-badge">默认</span>
        </span>
      </button>
      <p v-if="agents.length === 0" class="agent-empty">还没有角色</p>

      <p v-if="error" class="agent-error">{{ error }}</p>

      <NuxtLink to="/chat/agents" class="agent-manage" @click="open = false">管理角色</NuxtLink>
    </div>
  </div>
</template>

<script setup lang="ts">
// CL-06：对话页 composer 的**角色切换器**（紧邻语音按钮左侧，向上展开，中途可随时切）。
//
// 与其他控件的关系（用户 2026-10-17 口径）：
//   · 它是**本会话**的控制器 —— 选中即 PATCH 本会话的 agentId，之后的回复用新角色；
//   · 历史消息不回溯改写（每条 AI 消息带写库时的角色快照，见 T-19 标注）；
//   · 切换**不打断输入**：本组件不碰 textarea，也不动草稿。
import { computed, onMounted, ref } from 'vue'
import { useAgentStore } from '~/stores/agent'
import { useConversationStore } from '~/stores/conversation'
import type { AgentProfile } from '~/types/api'

const props = defineProps<{
  /** 本会话 id（切换即写它） */
  conversationId: string
}>()

const agentStore = useAgentStore()
const conversationStore = useConversationStore()

const open = ref(false)
const triggerRef = ref<HTMLButtonElement | null>(null)

const agents = computed(() => agentStore.agents)
const error = computed(() => agentStore.error)

/**
 * 当前生效角色：以**本会话的 agentId** 为准（服务端是事实源），
 * 会话没有该值/值已失效 ⇒ 回落默认角色（与 BFF 同款回落链）。
 */
const conversationAgentId = computed(() => {
  const conv = conversationStore.conversationList.find((c) => c.id === props.conversationId)
  return Number(conv?.agentId ?? 0)
})
const activeAgent = computed<AgentProfile | null>(() =>
  agentStore.resolveActiveAgent(conversationAgentId.value),
)
const initial = computed(() => (activeAgent.value?.name ?? '角').slice(0, 1))

onMounted(async () => {
  if (agents.value.length === 0) {
    await agentStore.fetchAgents()
  }
})

const toggle = () => {
  open.value = !open.value
  if (open.value) agentStore.clearError()
}

const select = async (a: AgentProfile) => {
  if (a.id === activeAgent.value?.id) {
    open.value = false
    return
  }
  agentStore.clearError()
  const res = await conversationStore.updateConversationAgent(props.conversationId, a.id)
  if (res.isOk) {
    open.value = false
  } else {
    // 失败：保留原选中 + 在菜单内显示原因（不静默改本地态）
    agentStore.error = res.msg
  }
}
</script>

<style scoped>
.agent-switcher {
  position: relative;
  display: inline-flex;
  align-items: center;
}

.agent-trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px 4px 6px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 999px;
  background: transparent;
  color: inherit;
  font-size: 12px;
  line-height: 1.4;
  cursor: pointer;
  max-width: 120px;
}

.agent-trigger:hover,
.agent-trigger.active {
  border-color: var(--primary-color, #6366f1);
  color: var(--primary-color, #6366f1);
}

.agent-avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  background: var(--primary-color, #6366f1);
  color: #fff;
  font-size: 10px;
  flex: 0 0 auto;
}

.agent-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 向上展开：菜单贴住触发器的上边缘（bottom: 100%） */
.agent-menu {
  position: absolute;
  bottom: 100%;
  left: 0;
  margin-bottom: 8px;
  min-width: 180px;
  max-height: 260px;
  overflow-y: auto;
  padding: 6px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 10px;
  background: var(--surface-color, #fff);
  box-shadow: 0 8px 24px rgb(0 0 0 / 12%);
  z-index: 30;
}

.agent-menu-title {
  margin: 2px 6px 6px;
  font-size: 11px;
  color: var(--text-muted, #9ca3af);
}

.agent-option {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 8px;
  border: none;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  text-align: left;
  font-size: 13px;
  cursor: pointer;
}

.agent-option:hover {
  background: var(--hover-bg, #f3f4f6);
}

.agent-option.current {
  color: var(--primary-color, #6366f1);
}

.agent-option-text {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.agent-option-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-badge {
  font-size: 10px;
  padding: 0 4px;
  border-radius: 4px;
  background: var(--hover-bg, #f3f4f6);
  color: var(--text-muted, #9ca3af);
}

.agent-empty,
.agent-error {
  margin: 4px 6px;
  font-size: 12px;
}

.agent-error {
  color: var(--danger-color, #ef4444);
}

.agent-manage {
  display: block;
  margin-top: 6px;
  padding: 6px 8px;
  border-top: 1px solid var(--border-color, #e5e7eb);
  font-size: 12px;
  color: var(--text-muted, #6b7280);
  text-decoration: none;
}

.agent-manage:hover {
  color: var(--primary-color, #6366f1);
}
</style>
