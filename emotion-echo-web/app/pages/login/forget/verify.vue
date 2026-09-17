<template>
  <article class="forget-card">
    <header>
      <span class="eyebrow">STEP 1</span>
      <h2>先确认一下这是你的账户</h2>
      <p>输入你注册时使用的用户名，我们会发一封验证码到这里。</p>
    </header>
    <form class="form">
      <label class="ee-field" data-label="用户名">
        <input type="text" class="ee-input input" placeholder="用户名" autocomplete="username" v-model="formInfo.username">
      </label>
      <label class="ee-field" data-label="验证码">
        <div class="code-row">
          <input type="text" class="ee-input input" placeholder="6 位数字验证码" maxlength="6" v-model="formInfo.verificationCode">
          <button type="button" class="ee-btn code-btn ee-btn-primary" @click="getVerificationCode">
            {{ isGetVerificationCode ? `${lastSeconds}s` : '获取验证码' }}
          </button>
        </div>
      </label>
    </form>
    <button type="button" class="ee-btn primary-btn ee-btn-primary" @click="gotoModify">继续</button>
  </article>
</template>

<script setup lang="ts">
import { useForgetPwdState } from '~/composables/forgetPwdState'
import { verificationCodeCountDown } from '~/composables/verificationCodeCountDown'

// Sprint 112：项目登录、注册、重置全用 username；UI 文案与现状对齐，不再误称"手机号或邮箱"。
const emits = defineEmits(['changeActive'])
const formRef = ref()
const formInfo = ref({ username: '', verificationCode: '' })
const rules = ref({
  username: [
    { required: true, message: '请输入用户名', trigger: 'blur' },
    // 用户名规则：字母/数字/下划线/点/中划线，2-32 字符（与 user-svc 入库 schema 对齐）
    { pattern: /^[a-zA-Z0-9._-]{2,32}$/, message: '请输入 2-32 位的字母、数字、点、下划线或中划线', trigger: 'blur' }
  ],
  verificationCode: [
    { required: true, message: '请输入验证码', trigger: 'blur' },
    { pattern: /^\d{6}$/, message: '验证码为 6 位数字', trigger: 'blur' }
  ]
})

const { startCountdown, isGetVerificationCode, lastSeconds } = verificationCodeCountDown()
const userStore = useUserStore()
const { updateStep, userAccount, verificationCode } = useForgetPwdState()

const getVerificationCode = () => {
  formRef.value?.validateField('username', async (isValid: boolean) => {
    if (!isValid) {
      notify('无法获取验证码', '请填写正确的用户名', 'error', 3000)
      return
    }
    const res = await userStore.sendVerificationCode({ username: formInfo.value.username, type: 'reset' })
    if (!res.isOk) {
      notify('验证码发送失败', '', 'error', 3000)
      return
    }
    notify('验证码已发送', '请注意查收', 'success', 3000)
    startCountdown()
  })
}

const gotoModify = () => {
  formRef.value.validate((valid: boolean) => {
    if (!valid) return
    userAccount.value = formInfo.value.username
    verificationCode.value = formInfo.value.verificationCode
    updateStep(1)
    emits('changeActive')
  })
}
</script>

<style scoped lang="scss">
.forget-card { display: grid; gap: 18px; padding: clamp(20px, 3vw, 28px); background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); }
.forget-card header { display: grid; gap: 6px; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: .16em; }
.forget-card h2 { font-size: clamp(20px, 2.5vw, 24px); letter-spacing: -.03em; }
.forget-card p { color: var(--ee-text-muted); font-size: 13px; }
.code-row { display: flex; gap: 8px; }
.code-row .input { flex: 1; }
.code-btn { white-space: nowrap; }
.primary-btn { width: 100%; height: 42px; border-radius: var(--ee-radius-md); }
@media (max-width: 480px) { .code-row { flex-direction: column; } .code-btn { width: 100%; } }
</style>