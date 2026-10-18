<template>
  <article class="agent-card" :data-agent-card="agent.id">
    <div class="agent-card-head">
      <span class="agent-mono" :class="`tone-${toneIndex}`" aria-hidden="true">{{ initial }}</span>
      <div class="agent-card-ident">
        <h3 class="agent-card-name">{{ displayName }}</h3>
        <p class="agent-card-sub">
          <span class="agent-card-voice">{{ voiceLabel }}</span>
          <span v-if="agent.isDefault" class="agent-seal">默认</span>
        </p>
      </div>
    </div>

    <p class="agent-card-persona" :class="{ 'is-empty': !personaText }">
      {{ personaText || '还没有人设' }}
    </p>

    <p v-if="agent.tone" class="agent-card-tone">
      <span class="agent-card-tone-label">语气</span>{{ agent.tone }}
    </p>

    <!-- 「不要」是用户在编辑器里逐条写下的约束，必须回到卡片上可见 ——
         只在表单里可见、回到列表就消失，等于让用户以为自己没配过。 -->
    <ul v-if="bounds.length" class="agent-card-bounds">
      <li class="agent-card-bounds-label">不要</li>
      <li v-for="b in bounds" :key="b" class="agent-card-bound">{{ b }}</li>
    </ul>

    <!-- 预览态不是操作面（编辑时的右侧实时预览不该长出按钮） -->
    <div v-if="variant === 'list'" class="agent-card-actions">
      <template v-if="!confirming">
        <button type="button" class="ghost-btn" data-agent-action="edit" @click="emit('edit')">
          编辑
        </button>
        <button
          type="button"
          class="ghost-btn is-danger"
          data-agent-action="delete"
          @click="confirming = true"
        >
          删除
        </button>
      </template>

      <!-- 删除不可撤销（服务端是软删除，但用户的角色确实从列表里消失了），
           所以就地进入一次确认，而不是点了就没了。
           确认键叫「确认删除」而不是「删除」：同一张卡上两个同名按钮会让
           辅助技术（和 Playwright 的 strict locator）分不清点的是哪一个。 -->
      <template v-else>
        <span class="confirm-question">删除「{{ displayName }}」？</span>
        <button type="button" class="danger-btn" data-agent-confirm="yes" @click="confirmDelete">
          确认删除
        </button>
        <button type="button" class="ghost-btn" data-agent-confirm="no" @click="confirming = false">
          取消
        </button>
      </template>
    </div>
  </article>
</template>

<script setup lang="ts">
// CL-06 · UI 轮：角色卡（**列表态**与**编辑预览态**共用同一个组件）。
//
// 一处表述：角色的"长相"只有这一个来源。管理页的列表用它，编辑器右侧的实时预览也用它
// —— 用户在输入框里打的每个字，都在同一张卡上长出来（所见即所得），而不是两套结构互相漂移。
//
// 纯展示组件：不碰 store、不发请求，只 emit `edit` / `delete`。
import { computed, ref, watch } from 'vue'
import type { AgentProfile } from '~/types/api'
import { agentInitial, agentToneIndex } from '~/utils/agentIdentity'
import { voiceLabelOf } from '~/utils/agentForm'

const props = withDefaults(
  defineProps<{
    agent: AgentProfile
    /** list = 卡片带操作；preview = 只读（编辑时的实时预览） */
    variant?: 'list' | 'preview'
  }>(),
  { variant: 'list' },
)

const emit = defineEmits<{
  (e: 'edit'): void
  (e: 'delete'): void
}>()

const confirming = ref(false)

// 换了角色（同一张卡片被复用到另一个 agent）就把确认态收起来，
// 免得"删小E"的确认框挂到"阿哲"头上。
watch(
  () => props.agent.id,
  () => {
    confirming.value = false
  },
)

const displayName = computed(() => props.agent.name?.trim() || '未命名角色')
const personaText = computed(() => props.agent.persona?.trim() ?? '')
const voiceLabel = computed(() => voiceLabelOf(props.agent.voice))
const initial = computed(() => agentInitial(props.agent.name))
const toneIndex = computed(() => agentToneIndex(props.agent.name))
const bounds = computed(() => (props.agent.boundaries ?? []).filter((b) => b && b.trim().length > 0))

const confirmDelete = () => {
  confirming.value = false
  emit('delete')
}
</script>

<style scoped lang="scss">
.agent-card {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 18px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  transition:
    border-color var(--ee-transition),
    transform var(--ee-transition);

  /* 身份色调的两枚变量，默认一套（.tone-N 覆盖）；全部取自项目调色板 + 主题令牌，
     所以深色模式自动跟随，不需要第二份暗色定义。 */
  --mono-ink: var(--ee-primary);
  --mono-wash: var(--ee-primary-soft);
}

.agent-card:hover {
  border-color: color-mix(in srgb, var(--ee-primary) 45%, var(--ee-border));
  transform: translateY(-1px);
}

.agent-card-head {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.agent-mono {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 38px;
  height: 38px;
  color: var(--mono-ink);
  background: var(--mono-wash);
  border-radius: 50%;
  font-size: 17px;
  font-weight: 600;
  line-height: 1;
}

/* 4 枚色调：彼此可辨，但都还在鼠尾草/陶土这套语言里 —— 是一套安静的子集，不是彩虹板。 */
.tone-0 {
  --mono-ink: var(--ee-primary);
  --mono-wash: var(--ee-primary-soft);
}
.tone-1 {
  --mono-ink: var(--ee-accent);
  --mono-wash: color-mix(in srgb, var(--ee-accent) 18%, var(--ee-surface));
}
.tone-2 {
  --mono-ink: var(--ee-text-muted);
  --mono-wash: var(--ee-surface-muted);
}
.tone-3 {
  --mono-ink: color-mix(in srgb, var(--ee-primary) 62%, var(--ee-accent));
  --mono-wash: color-mix(in srgb, var(--ee-primary) 14%, var(--ee-surface));
}

.agent-card-ident {
  min-width: 0;
}

.agent-card-name {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.01em;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-card-sub {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 2px 0 0;
  color: var(--ee-text-muted);
  font-size: 12px;
}

.agent-card-voice {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.agent-seal {
  flex: 0 0 auto;
  padding: 0 6px;
  color: var(--ee-primary);
  border: 1px solid color-mix(in srgb, var(--ee-primary) 38%, transparent);
  border-radius: 999px;
  font-size: 10px;
  font-weight: 600;
  letter-spacing: 0.04em;
}

.agent-card-persona {
  display: -webkit-box;
  overflow: hidden;
  margin: 0;
  font-size: 13.5px;
  line-height: 1.65;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 3;
}

.agent-card-persona.is-empty {
  color: var(--ee-text-muted);
  font-style: italic;
}

.agent-card-tone {
  display: flex;
  align-items: baseline;
  gap: 7px;
  margin: 0;
  font-size: 12.5px;
}

.agent-card-tone-label {
  flex: 0 0 auto;
  color: var(--ee-text-muted);
  font-size: 11px;
  letter-spacing: 0.08em;
}

.agent-card-bounds {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin: 0;
}

.agent-card-bounds-label {
  color: var(--ee-text-muted);
  font-size: 11px;
  letter-spacing: 0.08em;
  line-height: 20px;
}

.agent-card-bound {
  padding: 0 8px;
  color: var(--ee-text-muted);
  background: var(--ee-surface-muted);
  border-radius: 999px;
  font-size: 11.5px;
  line-height: 20px;
}

.agent-card-actions {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: auto;
  padding-top: 4px;
}

.ghost-btn,
.danger-btn {
  padding: 4px 10px;
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--ee-radius-sm);
  font-size: 12.5px;
  font-weight: 600;
  transition:
    color var(--ee-transition),
    background var(--ee-transition),
    border-color var(--ee-transition);
}

.ghost-btn {
  color: var(--ee-text-muted);
}

.ghost-btn:hover {
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
}

.ghost-btn.is-danger {
  color: var(--ee-danger);
}

.ghost-btn.is-danger:hover {
  color: var(--ee-danger);
  background: var(--ee-danger-soft);
}

.confirm-question {
  margin-right: auto;
  font-size: 12.5px;
  font-weight: 600;
}

.danger-btn {
  color: var(--ee-danger);
  background: var(--ee-danger-soft);
  border-color: color-mix(in srgb, var(--ee-danger) 30%, transparent);
}

.danger-btn:hover {
  color: var(--ee-surface);
  background: var(--ee-danger);
  border-color: var(--ee-danger);
}

@media (prefers-reduced-motion: reduce) {
  .agent-card:hover {
    transform: none;
  }
}
</style>
