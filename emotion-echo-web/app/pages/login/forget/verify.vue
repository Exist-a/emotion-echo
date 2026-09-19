<template>
  <article class="forget-card">
    <header>
      <span class="eyebrow">STEP 1</span>
      <h2>先确认一下这是你的账户</h2>
      <p>回答你注册时设置的密保问题。</p>
    </header>
    <form class="form">
      <label class="ee-field" data-label="用户名">
        <input
          v-model="formInfo.username"
          type="text"
          class="ee-input input"
          placeholder="用户名"
          autocomplete="username"
        />
      </label>
      <div v-if="questions.length > 0" class="security-questions">
        <label
          v-for="(q, index) in questions"
          :key="q.questionOrder"
          class="ee-field"
          :data-label="`密保问题 ${index + 1}`"
        >
          <span class="question-text">{{ q.question }}</span>
          <input
            v-model="formInfo.answers[q.questionOrder]"
            type="text"
            class="ee-input input"
            placeholder="请输入答案"
          />
        </label>
      </div>
    </form>
    <button
      v-if="questions.length === 0"
      type="button"
      class="ee-btn primary-btn ee-btn-primary"
      @click="fetchQuestions"
    >
      获取密保问题
    </button>
    <button
      v-else
      type="button"
      class="ee-btn primary-btn ee-btn-primary"
      @click="verifyAnswers"
    >
      验证
    </button>
  </article>
</template>

<script setup lang="ts">
import { useForgetPwdState } from '~/composables/forgetPwdState'

const emits = defineEmits(['changeActive'])
const formInfo = ref({
  username: '',
  answers: {} as Record<number, string>,
})
const questions = ref<Array<{ questionOrder: number; question: string }>>([])

const userStore = useUserStore()
const { updateStep, userAccount, markSecurityVerified } = useForgetPwdState()

const fetchQuestions = async () => {
  if (!formInfo.value.username) {
    notify('请输入用户名', '', 'error', 3000)
    return
  }
  const res = await userStore.getSecurityQuestions(formInfo.value.username)
  if (!res.isOk || !res.questions) {
    notify('获取密保问题失败', res.msg || '', 'error', 3000)
    return
  }
  if (res.questions.length === 0) {
    notify('该用户未设置密保问题', '无法找回密码', 'error', 3000)
    return
  }
  questions.value = res.questions
}

const verifyAnswers = async () => {
  for (const q of questions.value) {
    const answer = formInfo.value.answers[q.questionOrder]
    if (!answer || answer.trim() === '') {
      notify('请填写所有密保答案', '', 'error', 3000)
      return
    }
  }
  let resetToken = ''
  for (const q of questions.value) {
    const res = await userStore.verifySecurityAnswer({
      username: formInfo.value.username,
      questionOrder: q.questionOrder,
      answer: formInfo.value.answers[q.questionOrder]?.trim() || '',
    })
    if (!res.isOk) {
      notify('密保答案错误', '请重新确认', 'error', 3000)
      return
    }
    if (res.resetToken) {
      resetToken = res.resetToken
    }
  }
  userAccount.value = formInfo.value.username
  markSecurityVerified(formInfo.value.username, resetToken)
  updateStep(1)
  emits('changeActive')
}
</script>

<style scoped lang="scss">
.forget-card {
  display: grid;
  gap: 18px;
  padding: clamp(20px, 3vw, 28px);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
}
.forget-card header {
  display: grid;
  gap: 6px;
}
.eyebrow {
  color: var(--ee-primary);
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.16em;
}
.forget-card h2 {
  font-size: clamp(20px, 2.5vw, 24px);
  letter-spacing: -0.03em;
}
.forget-card p {
  color: var(--ee-text-muted);
  font-size: 13px;
}
.security-questions {
  display: grid;
  gap: 12px;
}
.question-text {
  font-size: 14px;
  color: var(--ee-text);
  margin-bottom: 4px;
}
.input {
  height: 44px;
  padding: 0 14px;
  border: 2px solid var(--ee-border);
  border-radius: 10px;
  font-size: 14px;
  transition: border-color 0.2s, box-shadow 0.2s;
  background: var(--ee-surface);

  &:focus {
    outline: none;
    border-color: var(--ee-primary);
    box-shadow: 0 0 0 3px rgba(var(--ee-primary-rgb, 99, 102, 241), 0.15);
  }

  &::placeholder {
    color: var(--ee-text-muted);
    opacity: 0.7;
  }
}
.primary-btn {
  width: 100%;
  height: 44px;
  border-radius: 10px;
  font-weight: 600;
}
</style>