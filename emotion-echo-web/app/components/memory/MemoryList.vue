<template>
  <div class="mem-list" data-testid="memory-list">
    <div class="mem-list-head">
      <span class="mem-list-title">记忆列表</span>
      <button
        v-if="items.length > 0"
        class="mem-del danger"
        type="button"
        :data-testid="armed === 'all' ? 'confirm-all' : 'delete-all'"
        @click="onClick('all', 0)"
      >
        {{ armed === 'all' ? '确认删除全部？' : '删除全部' }}
      </button>
    </div>

    <p v-if="items.length === 0" class="mem-list-empty">还没有可管理的记忆。</p>

    <ul v-else class="mem-items">
      <li v-for="it in items" :key="it.kind + it.id" class="mem-item">
        <span class="mem-item-kind" :data-kind="it.kind">{{ it.kind === 'note' ? '自述' : '记忆' }}</span>
        <span class="mem-item-text" :title="it.text">{{ it.text }}</span>
        <span v-if="it.createdAt" class="mem-item-date">{{ it.createdAt }}</span>
        <button
          class="mem-del"
          type="button"
          :aria-label="`删除：${it.text.slice(0, 20)}`"
          :data-testid="armed === it.kind + it.id ? 'confirm-item' : 'delete-item'"
          @click="onClick(it.kind, it.id)"
        >
          {{ armed === it.kind + it.id ? '确认？' : '删除' }}
        </button>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { onUnmounted, ref } from 'vue'
import type { MemoryItem } from '~/types/memory'

// CL-08d（T-21 默认文案，待用户裁定）：两段式确认 —— 第一次点 = 预备（按钮变
// "确认？"），第二次点 = 真删；点别处/再次点击别项自动复位。不用 window.confirm
// （E2E/自动化对原生对话框不友好），也不用弹窗组件（页面保持轻）。
const props = defineProps<{ items: MemoryItem[]; busy?: boolean }>()
const emit = defineEmits<{
  (e: 'delete', kind: 'fact' | 'note' | 'all', id: number): void
}>()

const armed = ref<string>('')

function onClick(kind: 'fact' | 'note' | 'all', id: number) {
  if (props.busy) return
  const key = kind === 'all' ? 'all' : kind + id
  if (armed.value !== key) {
    armed.value = key
    return
  }
  armed.value = ''
  emit('delete', kind, id)
}

function disarm() {
  armed.value = ''
}
defineExpose({ disarm })

// 全局点击（自身按钮以外）复位预备态
if (typeof document !== 'undefined') {
  document.addEventListener('click', onDocClick)
  onUnmounted(() => document.removeEventListener('click', onDocClick))
}
function onDocClick(e: Event) {
  const target = e.target as HTMLElement | null
  if (target && target.closest('.mem-del')) return
  disarm()
}
</script>

<style scoped lang="scss">
.mem-list {
  display: flex;
  min-height: 0;
  flex-direction: column;
  gap: 8px;
}

.mem-list-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.mem-list-title {
  color: var(--ee-text);
  font-size: 13px;
  font-weight: 600;
}

.mem-list-empty {
  margin: 0;
  color: var(--ee-text-muted);
  font-size: 12px;
}

.mem-items {
  display: flex;
  max-height: 260px;
  margin: 0;
  padding: 0;
  overflow-y: auto;
  list-style: none;
  flex-direction: column;
  gap: 6px;
}

.mem-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  border: 1px solid var(--ee-border);
  border-radius: 10px;
  font-size: 12px;
}

.mem-item-kind {
  flex-shrink: 0;
  padding: 2px 6px;
  border-radius: 6px;
  background: color-mix(in srgb, var(--ee-primary) 14%, transparent);
  color: var(--ee-text);
}

.mem-item-text {
  min-width: 0;
  overflow: hidden;
  flex: 1;
  color: var(--ee-text);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mem-item-date {
  flex-shrink: 0;
  color: var(--ee-text-muted);
  font-size: 11px;
}

.mem-del {
  flex-shrink: 0;
  padding: 2px 8px;
  border: 1px solid var(--ee-border);
  border-radius: 8px;
  background: transparent;
  color: var(--ee-text-muted);
  cursor: pointer;
  font-size: 11px;
}

.mem-del.danger,
.mem-del[data-testid='confirm-item'],
.mem-del[data-testid='confirm-all'] {
  border-color: color-mix(in srgb, #b4552d 45%, transparent);
  color: #b4552d;
}

.mem-del:hover {
  color: var(--ee-text);
}
</style>
