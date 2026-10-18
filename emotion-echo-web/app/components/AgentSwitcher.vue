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
          <span v-if="a.isDefault" class="agent-seal">默认</span>
        </span>
      </button>
      <p v-if="agents.length === 0" class="agent-empty">还没有角色</p>

      <p v-if="error" class="agent-error" role="alert">{{ error }}</p>

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
import { agentInitial } from '~/utils/agentIdentity'
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
/** 与角色卡同款首字（emoji 名字不会被劈成半个字符） */
const initial = computed(() => agentInitial(activeAgent.value?.name))

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

<style scoped lang="scss">
/* 全部走 --ee-* 令牌。此前这里写的是 `var(--primary-color, #6366f1)` 一类**从未定义**
   的变量名 ⇒ 每次都被兜底值接管（靛蓝 + Tailwind 灰），既不随主题也不随深色模式。
   机器守卫：app/assets/scss/no-undefined-tokens.test.ts */
.agent-switcher {
  position: relative;
  display: inline-flex;
  align-items: center;
  /* composer 在页面底部、菜单向上展开 ⇒ 必然叠在消息区上方。
     这一层是给**整块控件**建层叠上下文：只给 .agent-menu 写高 z-index 是不够的，
     若外层参与别的层叠上下文，菜单的值只在那一层内部比较，整块仍会被压住。 */
  z-index: 100;
}

.agent-trigger {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 130px;
  padding: 4px 10px 4px 5px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 1px solid var(--ee-border);
  border-radius: 999px;
  font-size: 12px;
  line-height: 1.4;
  transition:
    color var(--ee-transition),
    border-color var(--ee-transition);
}

.agent-trigger:hover,
.agent-trigger.active {
  color: var(--ee-primary);
  border-color: color-mix(in srgb, var(--ee-primary) 55%, var(--ee-border));
}

.agent-avatar {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  border-radius: 50%;
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
}

.agent-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 向上展开：菜单贴住触发器的上边缘（bottom: 100%）。
   锚点取**右**边缘（right: 0）而不是左边缘 —— 触发器贴着 composer 右侧，
   若从它的左边缘向右展开（190px 宽），菜单会顶出祖先 `conversation-page` 的
   右边缘（该祖先 overflow: hidden），**右侧被裁掉约 25px**，被裁处露出的是
   `div.page-content`（用户 2026-10-18 实测指出"选择框在另一个元素下面"）。
   贴右边缘向左展开 ⇒ 菜单整体落在祖先盒子内，不会越界也裁不到。 */
.agent-menu {
  position: absolute;
  bottom: 100%;
  right: 0;
  /* 必须压过页面上其余内容（用户实测"被压在下面"）。这一层比 .agent-switcher 更高，
     所以菜单还压得住同一行里的其他控件。 */
  z-index: 1000;
  min-width: 190px;
  max-height: 260px;
  overflow-y: auto;
  padding: 6px;
  margin-bottom: 8px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  box-shadow: var(--ee-shadow-soft);
}

.agent-menu-title {
  margin: 2px 6px 6px;
  color: var(--ee-text-muted);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.agent-option {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 6px 8px;
  color: inherit;
  background: transparent;
  border: 0;
  border-radius: var(--ee-radius-sm);
  font-size: 13px;
  text-align: left;
  transition: background var(--ee-transition);
}

.agent-option:hover {
  background: var(--ee-surface-muted);
}

.agent-option.current {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
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

.agent-seal {
  flex: 0 0 auto;
  padding: 0 5px;
  color: var(--ee-primary);
  border: 1px solid color-mix(in srgb, var(--ee-primary) 38%, transparent);
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
}

.agent-empty,
.agent-error {
  margin: 4px 6px;
  font-size: 12px;
}

.agent-empty {
  color: var(--ee-text-muted);
}

.agent-error {
  color: var(--ee-danger);
}

.agent-manage {
  display: block;
  padding: 7px 8px;
  margin-top: 6px;
  color: var(--ee-text-muted);
  border-top: 1px solid var(--ee-border);
  font-size: 12px;
}

.agent-manage:hover {
  color: var(--ee-primary);
}
</style>
