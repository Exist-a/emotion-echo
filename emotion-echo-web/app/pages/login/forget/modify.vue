<template>
  <article class="forget-card">
    <header>
      <span class="eyebrow">STEP 2</span>
      <h2>为账户设一个新密码</h2>
      <p>6-18 位字母与数字组合，请记牢它。</p>
    </header>
    <form class="form">
      <label class="ee-field" data-label="新密码">
        <input
          v-model="formInfo.newPassword"
          type="password"
          class="ee-input input"
          placeholder="至少 6 位"
          autocomplete="new-password"
        />
      </label>
      <label class="ee-field" data-label="再次输入">
        <input
          v-model="formInfo.confirmNewPassword"
          type="password"
          class="ee-input input"
          placeholder="再输入一次"
          autocomplete="new-password"
        />
      </label>
    </form>
    <button type="button" class="ee-btn primary-btn ee-btn-primary" @click="gotoSuccess">
      保存新密码
    </button>
  </article>
</template>

<script setup lang="ts">
import { useForgetPwdState } from '~/composables/forgetPwdState'
import { post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'

definePageMeta({ middleware: 'forget-pwd' })
const emits = defineEmits(['changeActive'])
const { updateStep, securityVerified, resetToken } = useForgetPwdState()

const formInfo = ref({ newPassword: '', confirmNewPassword: '' })

const gotoSuccess = async () => {
  if (!formInfo.value.newPassword || formInfo.value.newPassword.length < 6) {
    notify('密码不符合要求', '密码需为 6-18 位，包含字母和数字', 'error', 3000)
    return
  }
  if (formInfo.value.newPassword !== formInfo.value.confirmNewPassword) {
    notify('两次输入的密码不一致', '', 'error', 3000)
    return
  }
  if (!securityVerified.value || !resetToken.value) {
    notify('密码修改失败', '请返回上一步完成密保验证', 'error', 3000)
    return
  }
  try {
    await post(API_ROUTES.authResetPassword.path, {
      resetToken: resetToken.value,
      newPassword: formInfo.value.newPassword,
    })
    notify('密码已更新', '请用新密码登录', 'success', 3000)
    updateStep(2)
    emits('changeActive')
  } catch (error: any) {
    notify('密码修改失败', '', 'error', 3000)
  }
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