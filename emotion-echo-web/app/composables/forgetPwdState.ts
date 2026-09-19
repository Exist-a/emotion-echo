export const useForgetPwdState = () => {
  // 流程步骤：0=未开始，1=已完成密保验证，2=已完成改密
  // 使用 localStorage 持久化，避免刷新后丢失
  const currentStep = ref<number>(0)
  const userAccount = ref<string>('')
  // E2E-07: verificationCode → securityVerified + verifiedUsername + resetToken
  const securityVerified = ref<boolean>(false)
  const verifiedUsername = ref<string>('')
  const resetToken = ref<string>('')

  // 从 localStorage 恢复（仅在客户端）
  if (import.meta.client) {
    const storedStep = localStorage.getItem('forgetPwdStep')
    const storedAccount = localStorage.getItem('forgetPwdAccount')
    const storedVerified = localStorage.getItem('forgetPwdSecurityVerified')
    const storedVerifiedUser = localStorage.getItem('forgetPwdVerifiedUsername')
    const storedResetToken = localStorage.getItem('forgetPwdResetToken')
    if (storedStep !== null) currentStep.value = parseInt(storedStep)
    if (storedAccount !== null) userAccount.value = storedAccount
    if (storedVerified === 'true') securityVerified.value = true
    if (storedVerifiedUser !== null) verifiedUsername.value = storedVerifiedUser
    if (storedResetToken !== null) resetToken.value = storedResetToken
  }

  // 更新步骤
  const updateStep = (step: number) => {
    currentStep.value = step
    if (import.meta.client) {
      localStorage.setItem('forgetPwdStep', String(step))
    }
  }

  // E2E-07: 标记密保已验证（含 resetToken）
  const markSecurityVerified = (username: string, token: string) => {
    securityVerified.value = true
    verifiedUsername.value = username
    resetToken.value = token
    if (import.meta.client) {
      localStorage.setItem('forgetPwdSecurityVerified', 'true')
      localStorage.setItem('forgetPwdVerifiedUsername', username)
      localStorage.setItem('forgetPwdResetToken', token)
    }
  }

  // 重置状态
  const resetState = () => {
    currentStep.value = 0
    userAccount.value = ''
    securityVerified.value = false
    verifiedUsername.value = ''
    resetToken.value = ''
    if (import.meta.client) {
      localStorage.removeItem('forgetPwdStep')
      localStorage.removeItem('forgetPwdAccount')
      localStorage.removeItem('forgetPwdSecurityVerified')
      localStorage.removeItem('forgetPwdVerifiedUsername')
      localStorage.removeItem('forgetPwdResetToken')
    }
  }

  // 监听并持久化账号
  watch(userAccount, (val) => {
    if (import.meta.client) {
      localStorage.setItem('forgetPwdAccount', val)
    }
  })

  return {
    currentStep,
    userAccount,
    securityVerified,
    verifiedUsername,
    resetToken,
    updateStep,
    markSecurityVerified,
    resetState,
  }
}