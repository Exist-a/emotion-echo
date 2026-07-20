<template>
  <div class="login-page" :class="{ 'is-mobile': $device.isMobile }">
    <div class="login-card" :class="{ 'is-register': !isLogin }">
      <aside class="brand-panel">
        <span class="eyebrow">情绪回音</span>
        <h1>想说的时候，<br />有人在听。</h1>
        <p>一个安静记录情绪的地方，不用打分，也不会评判。</p>
        <div class="breath-line" aria-hidden="true"><span></span></div>
      </aside>

      <section class="form-panel">
        <header class="form-header">
          <h2>{{ isLogin ? '登录' : '注册' }}</h2>
        </header>

        <div class="form-tabs" role="tablist">
          <button type="button" role="tab" class="form-tab" :class="{ active: isLogin }" :aria-selected="isLogin" @click="isLogin = true">登录</button>
          <button type="button" role="tab" class="form-tab" :class="{ active: !isLogin }" :aria-selected="!isLogin" @click="isLogin = false">注册</button>
        </div>

        <form v-if="isLogin" class="auth-form" @submit.prevent="loginHandler">
          <label class="auth-field">
            <span class="input-icon" aria-hidden="true">@</span>
            <input v-model="loginInfo.username" class="ee-input" placeholder="邮箱" autocomplete="username" />
          </label>
          <label class="auth-field">
            <span class="input-icon" aria-hidden="true">●</span>
            <input v-model="loginInfo.password" type="password" class="ee-input" placeholder="密码" autocomplete="current-password" />
          </label>
          <div class="form-extras">
            <label class="ee-checkbox">
              <input v-model="isRemember" type="checkbox" />
              <span>记住我</span>
            </label>
            <NuxtLink to="/login/forget/verify" class="link">忘记密码</NuxtLink>
          </div>
          <button type="submit" class="ee-btn primary-btn" :disabled="isLoading">
            {{ isLoading ? '登录中…' : '登录' }}
          </button>
          <div class="divider"><span>或</span></div>
          <button type="button" class="ee-btn quick-btn" :disabled="isQuickLoading" @click="quickLogin">
            <span class="quick-icon" aria-hidden="true">↳</span>
            用演示账号快速体验
          </button>
          <p class="quick-hint">直接以已预置的 <code>demo@emotion-echo.com</code> 登录，跳过注册和验证码。</p>
        </form>

        <form v-else class="auth-form" @submit.prevent="registerHandler">
          <label class="auth-field">
            <span class="input-icon" aria-hidden="true">@</span>
            <input v-model="registerInfo.username" class="ee-input" placeholder="邮箱" autocomplete="username" />
          </label>
          <label class="auth-field">
            <span class="input-icon" aria-hidden="true">●</span>
            <input v-model="registerInfo.password" type="password" class="ee-input" placeholder="密码（6-18 位字母+数字）" autocomplete="new-password" />
          </label>
          <div class="auth-field code-field">
            <input v-model="registerInfo.verificationCode" class="ee-input" placeholder="验证码" maxlength="6" />
            <button type="button" class="ee-btn code-btn" :disabled="isGetVerificationCode" @click="getVerificationCode">
              {{ isGetVerificationCode ? `${lastSeconds}s 后重发` : '获取验证码' }}
            </button>
          </div>
          <p class="code-hint">开发模式下验证码会打印在服务端终端。</p>
          <button type="submit" class="ee-btn primary-btn" :disabled="isLoading">
            {{ isLoading ? '注册中…' : '注册并开始' }}
          </button>
        </form>

        <footer class="form-footer">
          <p>登录或注册即表示你愿意把这里当作自己的安全空间。</p>
        </footer>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { sha256 } from 'js-sha256'
import { verificationCodeCountDown } from '~/composables/verificationCodeCountDown'
import { useNotify } from '~/composables/useNotify'

definePageMeta({ layout: 'default' })

const { isMobile } = useDevice()
const isLogin = ref<boolean>(true)
const isLoading = ref(false)
const isQuickLoading = ref(false)
const isRemember = ref<boolean>(false)
const loginInfo = reactive<{ username: string; password: string }>({ username: '', password: '' })
const registerInfo = reactive<{ username: string; password: string; verificationCode: string }>({ username: '', password: '', verificationCode: '' })

onMounted(() => {
  if (!import.meta.client) return
  const stored = localStorage.getItem('remember_me')
  if (stored !== null) isRemember.value = stored === 'true'
  if (isRemember.value && loginInfo.username === '') {
    const remembered = localStorage.getItem('remember_username')
    if (remembered) loginInfo.username = remembered
  }
})

const { startCountdown, isGetVerificationCode, lastSeconds } = verificationCodeCountDown()
const userStore = useUserStore()
const { success, error } = useNotify()

const loginHandler = () => {
  if (!loginInfo.username.trim() || !loginInfo.password.trim()) {
    error('登录失败', '请输入邮箱和密码')
    return
  }
  if (import.meta.client) {
    localStorage.setItem('remember_me', String(isRemember.value))
    if (isRemember.value) localStorage.setItem('remember_username', loginInfo.username.trim())
    else localStorage.removeItem('remember_username')
  }
  handleLogin(loginInfo.username.trim(), loginInfo.password.trim())
}

const handleLogin = async (username: string, password: string) => {
  isLoading.value = true
  try {
    const result = await userStore.login({ username, password: sha256(password), rememberMe: isRemember.value })
    if (result.isOk) {
      await userStore.fetchUserInfo().catch(() => {})
      success('欢迎回来')
      await navigateTo('/chat/conversation')
    } else {
      error('登录失败', result.msg)
    }
  } finally {
    isLoading.value = false
  }
}

const registerHandler = () => {
  if (!registerInfo.username.trim() || !registerInfo.password.trim() || !registerInfo.verificationCode.trim()) {
    error('注册失败', '请填完所有字段')
    return
  }
  handleRegister(registerInfo.username.trim(), registerInfo.password.trim(), registerInfo.verificationCode.trim())
}

const handleRegister = async (username: string, password: string, verificationCode: string) => {
  isLoading.value = true
  try {
    const result = await userStore.register({ username, password: sha256(password), verificationCode })
    if (result.isOk) {
      success('已为你准备好', '欢迎，开始聊吧')
      await navigateTo('/chat/conversation')
    } else {
      error('注册失败', result.msg)
    }
  } finally {
    isLoading.value = false
  }
}

const getVerificationCode = async () => {
  if (!/^[\w.+-]+@[\w-]+\.[\w.-]+$/.test(registerInfo.username)) {
    error('无法获取验证码', '请填写正确的邮箱')
    return
  }
  const result = await userStore.sendVerificationCode({ username: registerInfo.username, type: 'register' })
  if (result.isOk) {
    success('验证码已发送', '请到服务端终端查看')
    startCountdown()
  } else {
    error('发送失败', result.msg)
  }
}

const quickLogin = async () => {
  if (isQuickLoading.value) return
  isQuickLoading.value = true
  try {
    const result = await userStore.login({
      username: 'demo@emotion-echo.com',
      password: sha256('Demo12345'),
      rememberMe: true
    })
    if (result.isOk) {
      await userStore.fetchUserInfo().catch(() => {})
      success('已进入体验模式')
      await navigateTo('/chat/conversation')
    } else {
      error('快速体验暂不可用', result.msg || '请用真实邮箱注册')
    }
  } finally {
    isQuickLoading.value = false
  }
}
</script>

<style scoped lang="scss">
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: clamp(20px, 4vw, 48px);
  background: radial-gradient(circle at 12% 18%, color-mix(in srgb, var(--ee-primary-soft) 90%, var(--ee-bg)), var(--ee-bg) 60%);
}

.login-card {
  display: grid;
  width: min(960px, 100%);
  min-height: 580px;
  grid-template-columns: 1fr 1fr;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-xl);
  box-shadow: 0 12px 36px rgba(32, 37, 34, 0.06);
  overflow: hidden;
}

.brand-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: clamp(28px, 4vw, 48px);
  color: var(--ee-text);
  background: linear-gradient(160deg, var(--ee-primary-soft) 0%, color-mix(in srgb, var(--ee-primary-soft) 40%, var(--ee-surface)) 100%);
}
.eyebrow { color: var(--ee-primary); font-size: 13px; font-weight: 600; letter-spacing: 0.08em; }
.brand-panel h1 { margin: 4px 0 0; font-size: clamp(26px, 3vw, 34px); font-weight: 600; letter-spacing: -0.02em; line-height: 1.35; }
.brand-panel p { color: var(--ee-text-muted); font-size: 14px; line-height: 1.7; max-width: 32ch; }
.breath-line { margin-top: auto; }
.breath-line span { display: block; width: 56px; height: 3px; background: var(--ee-primary); border-radius: 999px; animation: ee-quiet-pulse 2.4s ease-in-out infinite; }

.form-panel { display: flex; flex-direction: column; gap: 18px; padding: clamp(28px, 4vw, 48px); background: var(--ee-surface); }
.form-header h2 { font-size: clamp(20px, 2.4vw, 26px); font-weight: 600; letter-spacing: -0.02em; margin: 0; }
.form-tabs { display: inline-flex; gap: 4px; padding: 4px; background: var(--ee-surface-muted); border-radius: var(--ee-radius-md); align-self: flex-start; }
.form-tab { padding: 6px 16px; color: var(--ee-text-muted); background: transparent; border: 0; border-radius: 6px; cursor: pointer; font-size: 13px; font-weight: 600; }
.form-tab.active { color: var(--ee-text); background: var(--ee-surface); box-shadow: 0 1px 2px rgba(32, 37, 34, 0.06); }

.auth-form { display: grid; gap: 12px; }

.auth-field {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 0 12px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-md);
  transition: border-color var(--ee-transition), box-shadow var(--ee-transition);
}
.auth-field:focus-within { border-color: var(--ee-primary); box-shadow: 0 0 0 3px color-mix(in srgb, var(--ee-primary) 25%, transparent); }
.auth-field .input-icon { color: var(--ee-text-muted); font-size: 14px; font-weight: 700; }

.ee-input {
  flex: 1;
  min-width: 0;
  padding: 10px 2px;
  color: var(--ee-text);
  background: transparent;
  border: 0;
  outline: 0;
  font: inherit;
}
.ee-input::placeholder { color: var(--ee-text-muted); }

.form-extras { display: flex; align-items: center; justify-content: space-between; margin: 4px 0 8px; font-size: 12px; }
.ee-checkbox { display: inline-flex; align-items: center; gap: 6px; color: var(--ee-text-muted); cursor: pointer; }
.ee-checkbox input { accent-color: var(--ee-primary); }

.link { color: var(--ee-primary); text-decoration: none; }
.link:hover { text-decoration: underline; }

.ee-btn { display: inline-flex; align-items: center; justify-content: center; height: 38px; padding: 0 18px; background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-md); color: var(--ee-text); cursor: pointer; font-size: 13px; font-weight: 600; transition: background var(--ee-transition), color var(--ee-transition), border-color var(--ee-transition); }
.ee-btn:disabled { cursor: not-allowed; opacity: 0.6; }
.ee-btn-primary { background: var(--ee-primary); color: #fff; border-color: var(--ee-primary); }
.ee-btn-primary:hover:not(:disabled) { background: var(--ee-primary-hover); border-color: var(--ee-primary-hover); }
.primary-btn { height: 44px; }

.divider { display: flex; align-items: center; gap: 12px; margin: 6px 0; color: var(--ee-text-muted); font-size: 11px; letter-spacing: 0.16em; }
.divider::before, .divider::after { content: ""; flex: 1; height: 1px; background: var(--ee-border); }

.quick-btn { height: 44px; color: var(--ee-primary); background: var(--ee-primary-soft); border: 1px dashed color-mix(in srgb, var(--ee-primary) 45%, transparent); border-radius: var(--ee-radius-md); font-weight: 600; }
.quick-btn:hover:not(:disabled) { background: color-mix(in srgb, var(--ee-primary-soft) 60%, var(--ee-primary)); }
.quick-icon { margin-right: 6px; }
.quick-hint, .code-hint { margin: 0; color: var(--ee-text-muted); font-size: 11px; line-height: 1.6; }
.quick-hint code { background: var(--ee-surface-muted); padding: 1px 4px; border-radius: 3px; font-size: 10px; }

.code-field { padding-right: 4px; gap: 4px; }
.code-field .ee-input { padding: 10px 2px; }
.code-btn { white-space: nowrap; height: 32px; padding: 0 12px; background: var(--ee-primary-soft); color: var(--ee-primary); border: 1px solid color-mix(in srgb, var(--ee-primary) 30%, transparent); border-radius: var(--ee-radius-md); font-weight: 600; }
.code-btn:hover:not(:disabled) { background: color-mix(in srgb, var(--ee-primary-soft) 60%, var(--ee-primary)); }

.form-footer { margin-top: auto; color: var(--ee-text-muted); font-size: 11px; text-align: center; }

@media (max-width: 760px) {
  .login-card { grid-template-columns: 1fr; min-height: auto; }
  .brand-panel { padding: 24px; gap: 8px; }
  .brand-panel h1 { font-size: 22px; }
  .breath-line { display: none; }
  .form-panel { padding: 24px; gap: 14px; }
}
</style>
