<template>
  <article class="forget-card">
    <header>
      <span class="eyebrow">STEP 2</span>
      <h2>为账户设一个新密码</h2>
      <p>6-18 位字母与数字组合，请记牢它。</p>
    </header>
    <form class="form">
      <label class="ee-field" data-label="新密码">
        <input type="password" class="ee-input input" placeholder="至少 6 位" autocomplete="new-password" v-model="formInfo.newPassword" :show-password="true">
      </label>
      <label class="ee-field" data-label="再次输入">
        <input type="password" class="ee-input input" placeholder="再输入一次" autocomplete="new-password" v-model="formInfo.confirmNewPassword" :show-password="true">
      </label>
    </form>
    <button type="button" class="ee-btn primary-btn ee-btn-primary" @click="gotoSuccess">保存新密码</button>
  </article>
</template>

<script setup lang="ts">
import { useForgetPwdState } from '~/composables/forgetPwdState'
import { post } from '~/composables/useApi'
import { sha256 } from 'js-sha256'

definePageMeta({ middleware: 'forget-pwd' })
const emits = defineEmits(['changeActive'])
const formRef = ref()
const { updateStep, userAccount, verificationCode } = useForgetPwdState()
const passwordReg = /^(?=.*[a-zA-Z])(?=.*\d).{6,18}$/

const formInfo = ref({ newPassword: '', confirmNewPassword: '' })
const rules = ref({
  newPassword: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { pattern: passwordReg, message: '密码需为 6-18 位，包含字母和数字', trigger: 'blur' }
  ],
  confirmNewPassword: [
    { required: true, message: '请再次输入密码', trigger: 'blur' },
    {
      validator: (_: any, value: string, callback: (err?: Error) => void) => {
        if (value !== formInfo.value.newPassword) callback(new Error('两次输入的密码不一致'))
        else callback()
      },
      trigger: 'blur'
    }
  ]
})

const gotoSuccess = async () => {
  const valid = await formRef.value.validate().catch(() => false)
  if (!valid) return
  if (!userAccount.value || !verificationCode.value) {
    notify('密码修改失败', '请返回上一步重新验证', 'error', 3000)
    return
  }
  try {
    await post('/auth/reset-password', { username: userAccount.value, verificationCode: verificationCode.value, newPassword: sha256(formInfo.value.newPassword) })
    notify('密码已更新', '请用新密码登录', 'success', 3000)
    updateStep(2)
    emits('changeActive')
  } catch (error: any) {
    notify('密码修改失败', '', 'error', 3000)
  }
}
</script>

<style scoped lang="scss">
.forget-card { display: grid; gap: 18px; padding: clamp(20px, 3vw, 28px); background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); }
.forget-card header { display: grid; gap: 6px; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: .16em; }
.forget-card h2 { font-size: clamp(20px, 2.5vw, 24px); letter-spacing: -.03em; }
.forget-card p { color: var(--ee-text-muted); font-size: 13px; }
.input { height: 42px; }
.primary-btn { width: 100%; height: 42px; border-radius: var(--ee-radius-md); }
</style>
