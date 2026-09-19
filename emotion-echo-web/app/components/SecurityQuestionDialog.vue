<template>
  <Teleport to="body">
    <div v-if="modelValue" class="sq-overlay" @click.self="handleClose">
      <div class="sq-dialog" role="dialog" aria-modal="true" aria-label="设置密保问题">
        <header class="sq-header">
          <h3>设置密保问题</h3>
          <button type="button" class="sq-close" aria-label="关闭" @click="handleClose">
            ✕
          </button>
        </header>

        <p class="sq-hint">此密保用于找回密码，请认真填写。</p>

        <form class="sq-form" @submit.prevent="handleSubmit">
          <div v-for="(q, idx) in questions" :key="idx" class="sq-field">
            <label class="sq-label">问题 {{ idx + 1 }}：{{ q }}</label>
            <input
              v-model="answers[idx]"
              class="ee-input sq-input"
              :placeholder="'请输入答案'"
              autocomplete="off"
            />
            <p v-if="submitted && !answers[idx]?.trim()" class="sq-error">
              答案不能为空
            </p>
          </div>

          <div class="sq-actions">
            <button type="submit" class="ee-btn primary-btn sq-submit">
              确认注册
            </button>
          </div>
        </form>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
const props = defineProps<{
  modelValue: boolean
  questions: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  submit: [data: { questions: string[]; answers: string[] }]
}>()

const answers = ref<string[]>([])
const submitted = ref(false)

// 重置答案当 questions 变化
watch(
  () => props.questions,
  (qs) => {
    answers.value = qs.map(() => '')
    submitted.value = false
  },
  { immediate: true },
)

const handleClose = () => {
  emit('update:modelValue', false)
}

const handleSubmit = () => {
  submitted.value = true
  const trimmed = answers.value.map((a) => a?.trim() ?? '')
  const allFilled = trimmed.every((a) => a.length > 0)
  if (!allFilled) return

  emit('submit', {
    questions: props.questions,
    answers: trimmed,
  })
}
</script>

<style scoped lang="scss">
.sq-overlay {
  position: fixed;
  inset: 0;
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.4);
  backdrop-filter: blur(2px);
}

.sq-dialog {
  width: min(440px, 90vw);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-xl);
  box-shadow: 0 16px 48px rgba(32, 37, 34, 0.12);
  padding: 28px;
}

.sq-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;

  h3 {
    margin: 0;
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }
}

.sq-close {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: transparent;
  border: 0;
  border-radius: 50%;
  color: var(--ee-text-muted);
  font-size: 16px;
  cursor: pointer;
  transition: background var(--ee-transition);

  &:hover {
    background: var(--ee-surface-muted);
  }
}

.sq-hint {
  margin: 0 0 20px;
  color: var(--ee-text-muted);
  font-size: 13px;
  line-height: 1.6;
}

.sq-form {
  display: grid;
  gap: 16px;
}

.sq-field {
  display: grid;
  gap: 6px;
}

.sq-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--ee-text);
}

.sq-input {
  padding: 10px 12px;
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  font: inherit;
  font-size: 14px;
  color: var(--ee-text);
  background: var(--ee-surface);
  transition:
    border-color var(--ee-transition),
    box-shadow var(--ee-transition);

  &:focus {
    border-color: var(--ee-primary);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--ee-primary) 25%, transparent);
    outline: 0;
  }
}

.sq-error {
  margin: 0;
  color: #e74c3c;
  font-size: 12px;
}

.sq-actions {
  margin-top: 4px;
}

.sq-submit {
  width: 100%;
  height: 44px;
}
</style>