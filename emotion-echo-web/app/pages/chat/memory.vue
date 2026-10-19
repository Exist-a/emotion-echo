<template>
  <section class="memory-page">
    <!-- 布局的 <header> 已经渲染 h1（app/layouts/nav.vue）⇒ 页面不再重复标题 -->
    <p class="memory-lede">
      这里只记录你自己的事：讲过的经历、在意的人、长期的状态。写完就会出现在右边的关系图里。
    </p>

    <div class="memory-grid">
      <div class="memory-graph-card">
        <BaseChart v-if="hasGraph" :option="graphOption" :height="graphHeight" />
        <div v-else class="memory-empty">
          <p class="memory-empty-title">{{ loading ? '正在读取记忆…' : '还没有记忆' }}</p>
          <p v-if="!loading" class="memory-empty-body">
            在下面写下你想让它记住的事，抽取完成后会出现在这张图上。
          </p>
        </div>
      </div>

      <div class="memory-side">
        <div class="memory-stream" aria-live="polite">
          <p v-for="(ack, i) in acks" :key="i" class="memory-ack">{{ ack }}</p>
        </div>

        <form class="memory-composer" @submit.prevent="submit">
          <label class="sr-only" for="memory-input">写下一件想被记住的事</label>
          <textarea
            id="memory-input"
            v-model="draft"
            class="memory-input"
            rows="4"
            maxlength="4000"
            placeholder="例如：前年我一个人去了西藏，那之后我对独处这件事没那么害怕了。"
            :disabled="submitting"
            @keydown.enter.exact.prevent="submit"
          />
          <div class="memory-composer-bar">
            <span class="memory-counter">{{ count }}/4000</span>
            <button class="primary-btn" type="submit" :disabled="!canSubmit">
              {{ submitting ? '记录中…' : '记录' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import BaseChart from '~/components/charts/BaseChart.vue'
import { get, post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { buildMemoryGraphOption, hasMemoryGraph } from '~/utils/memoryGraph'
import type { MemoryGraph, MemoryNoteResult } from '~/types/memory'

const graphHeight = 460

const graph = ref<MemoryGraph>({ nodes: [], edges: [] })
const draft = ref('')
const acks = ref<string[]>([])
const submitting = ref(false)
const loading = ref(true)

const count = computed(() => draft.value.length)
const canSubmit = computed(() => !submitting.value && draft.value.trim().length > 0)
const graphOption = computed(() => buildMemoryGraphOption(graph.value))
const hasGraph = computed(() => hasMemoryGraph(graph.value))

async function loadGraph() {
  try {
    graph.value = await get<MemoryGraph>(API_ROUTES.memoryGraph.path)
  } catch {
    // useApi 已统一提示（401 会跳登录）；这里保持空图，不让页头崩
  } finally {
    loading.value = false
  }
}

async function submit() {
  const content = draft.value.trim()
  if (!content || submitting.value) return
  submitting.value = true
  try {
    const res = await post<MemoryNoteResult>(API_ROUTES.memoryNotes.path, { content })
    acks.value.push(res?.ackText || '记忆已更新')
    draft.value = ''
    // 抽取是异步的：延迟刷新两次图（不做无限轮询；用户也可重新进入页面）
    window.setTimeout(loadGraph, 1200)
    window.setTimeout(loadGraph, 3500)
  } catch {
    // useApi 已统一处理 401；这里补一条用户可见的失败回执（否则按钮恢复但页面毫无反馈）
    acks.value.push('这次没记录成功，请稍后再试。')
  } finally {
    submitting.value = false
  }
}

onMounted(loadGraph)
</script>

<style scoped lang="scss">
.memory-page {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 16px;
  padding: 4px 0 12px;
}

.memory-lede {
  margin: 0;
  color: var(--ee-text-muted);
  font-size: 14px;
  line-height: 1.7;
}

.memory-grid {
  display: grid;
  min-height: 0;
  flex: 1;
  gap: 18px;
  grid-template-columns: minmax(0, 1fr) minmax(300px, 380px);
}

.memory-graph-card,
.memory-side {
  min-height: 0;
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg, 18px);
  background: var(--ee-surface);
}

.memory-graph-card {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 12px;
  overflow: hidden;
}

.memory-side {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 14px;
}

.memory-stream {
  display: flex;
  min-height: 0;
  flex: 1;
  flex-direction: column;
  gap: 8px;
  overflow-y: auto;
}

.memory-ack {
  align-self: flex-start;
  max-width: 100%;
  margin: 0;
  padding: 8px 12px;
  border-radius: 12px;
  background: color-mix(in srgb, var(--ee-primary) 16%, transparent);
  color: var(--ee-text);
  font-size: 13px;
}

.memory-composer {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.memory-input {
  width: 100%;
  resize: vertical;
  border: 1px solid var(--ee-border);
  border-radius: 12px;
  background: var(--ee-bg);
  color: var(--ee-text);
  font-family: inherit;
  font-size: 14px;
  line-height: 1.6;
  padding: 10px 12px;
}

.memory-input:focus-visible {
  outline: 2px solid var(--ee-primary);
  outline-offset: 1px;
}

.memory-composer-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.memory-counter {
  color: var(--ee-text-muted);
  font-size: 12px;
}

.memory-empty {
  padding: 32px;
  text-align: center;
}

.memory-empty-title {
  margin: 0 0 6px;
  color: var(--ee-text);
  font-size: 15px;
}

.memory-empty-body {
  margin: 0;
  color: var(--ee-text-muted);
  font-size: 13px;
  line-height: 1.7;
}

.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 900px) {
  .memory-grid {
    grid-template-columns: minmax(0, 1fr);
  }
  .memory-graph-card {
    min-height: 360px;
  }
}
</style>
