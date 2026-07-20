<template>
  <article class="success-card">
    <span class="quiet-pulse success-mark" aria-hidden="true"></span>
    <span class="eyebrow">DONE</span>
    <h2>密码已更新</h2>
    <p>用新密码回到登录页，继续与情绪对话。</p>
    <button type="button" class="ee-btn ee-btn-primary" @click="gotoLogin">去登录</button>
  </article>
</template>

<script setup lang="ts">
import { useForgetPwdState } from '~/composables/forgetPwdState'

definePageMeta({ middleware: 'forget-pwd' })

const gotoLogin = () => {
  const { resetState } = useForgetPwdState()
  resetState()
  navigateTo('/login')
}
</script>

<style scoped lang="scss">
.success-card { display: grid; gap: 14px; padding: clamp(28px, 4vw, 40px); text-align: center; background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); }
.success-mark { display: block; width: 56px; height: 6px; margin: 0 auto 10px; background: var(--ee-primary); border-radius: 999px; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: .16em; }
.success-card h2 { font-size: clamp(22px, 3vw, 28px); letter-spacing: -.03em; }
.success-card p { color: var(--ee-text-muted); font-size: 14px; }
.success-card :deep(.el-button) { width: min(260px, 100%); margin: 6px auto 0; height: 42px; border-radius: var(--ee-radius-md); }
</style>
