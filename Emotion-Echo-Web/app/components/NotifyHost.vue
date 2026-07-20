<template>
  <Teleport to="body">
    <div v-if="toasts.length" class="notify-stack" aria-live="polite">
      <div
        v-for="t in toasts"
        :key="t.id"
        class="notify-card"
        :class="`is-${t.type}`"
        role="status"
      >
        <span class="notify-mark" aria-hidden="true">{{ markFor(t.type) }}</span>
        <div class="notify-body">
          <strong v-if="t.title">{{ t.title }}</strong>
          <p v-if="t.message">{{ t.message }}</p>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { useNotify } from '~/composables/useNotify'
const { toasts } = useNotify()
function markFor(t: string) {
  return t === 'success' ? '✓' : t === 'error' ? '!' : t === 'warning' ? '!' : 'i'
}
</script>

<style scoped>
.notify-stack {
  position: fixed;
  top: 16px;
  left: 50%;
  z-index: 200;
  display: flex;
  flex-direction: column;
  gap: 8px;
  transform: translateX(-50%);
  pointer-events: none;
}

.notify-card {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  min-width: 240px;
  max-width: min(90vw, 380px);
  padding: 10px 14px;
  color: var(--ee-text);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  box-shadow: 0 8px 24px rgba(32, 37, 34, 0.12);
  pointer-events: auto;
  animation: notify-in 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

.notify-card strong { display: block; font-size: 13px; font-weight: 600; }
.notify-card p { margin: 2px 0 0; color: var(--ee-text-muted); font-size: 12px; line-height: 1.5; }

.notify-mark {
  display: grid;
  width: 22px;
  height: 22px;
  place-items: center;
  flex-shrink: 0;
  border-radius: 50%;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
}

.notify-card.is-success .notify-mark { background: var(--ee-primary); }
.notify-card.is-error .notify-mark { background: var(--ee-accent); }
.notify-card.is-warning .notify-mark { background: #d9a042; }
.notify-card.is-info .notify-mark { background: var(--ee-text-muted); }

@keyframes notify-in {
  from { opacity: 0; transform: translateY(-8px); }
  to { opacity: 1; transform: translateY(0); }
}
</style>
