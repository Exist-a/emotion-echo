<template>
  <div v-if="state" class="current-emotion-bar" role="status" aria-live="polite">
    <span class="ceb-dot" :class="`ceb-dot--${state.emotion}`" aria-hidden="true" />
    <span class="ceb-text">{{ state.text }}</span>
    <span class="ceb-modality">{{ state.modalityLabel }}</span>
  </div>
</template>

<script setup lang="ts">
// CL-07 前端半（情绪回显）· Phase 3：顶部「此刻的状态」状态条。
//
// 纯展示组件：不发请求、不做判定（判定/降级在 useCurrentEmotion）。
//   - state 为 null ⇒ 整块不渲染（无融合数据时优雅隐藏，不占位）
//   - 护栏：文案由 composable 生成（陈述化、中性、无来源词），本组件不做二次措辞
//   - modalityLabel：「综合」（含 face/voice）或「文本」（纯文本，避免夸大"多模态"）
import type { CurrentEmotionState } from '~/composables/useCurrentEmotion'

defineProps<{
  state: CurrentEmotionState | null
}>()
</script>

<style scoped>
.current-emotion-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  width: min(820px, 100%);
  margin: 0 auto;
  padding: 8px 14px;
  color: var(--ee-text-muted);
  background: var(--ee-surface-muted, rgba(0, 0, 0, 0.03));
  border-bottom: 1px solid var(--ee-border);
  font-size: 13px;
  line-height: 1.4;
}

.ceb-dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ee-primary);
}

.ceb-dot--happy {
  background: #e8b23a;
}
.ceb-dot--sad {
  background: #6b8fd4;
}
.ceb-dot--angry {
  background: #d4695f;
}
.ceb-dot--anxious {
  background: #b98bdc;
}
.ceb-dot--neutral {
  background: #9aa5a0;
}

.ceb-text {
  color: var(--ee-text);
}

.ceb-modality {
  margin-left: auto;
  padding: 1px 8px;
  color: var(--ee-text-muted);
  border: 1px solid var(--ee-border);
  border-radius: 999px;
  font-size: 11px;
}
</style>
