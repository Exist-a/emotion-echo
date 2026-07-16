export const useForgetPwdState = () => {
  // 流程步骤：0=未开始，1=已完成确认账号，2=已完成验证码验证
  // 使用 localStorage 持久化，避免刷新后丢失
  const currentStep = ref<number>(0)
  const userAccount = ref<string>('')
  const verificationCode = ref<string>('')

  // 从 localStorage 恢复（仅在客户端）
  if (import.meta.client) {
    const storedStep = localStorage.getItem('forgetPwdStep')
    const storedAccount = localStorage.getItem('forgetPwdAccount')
    const storedCode = localStorage.getItem('forgetPwdCode')
    if (storedStep !== null) currentStep.value = parseInt(storedStep)
    if (storedAccount !== null) userAccount.value = storedAccount
    if (storedCode !== null) verificationCode.value = storedCode
  }

  // 更新步骤
  const updateStep = (step: number) => {
    currentStep.value = step
    if (import.meta.client) {
      localStorage.setItem('forgetPwdStep', String(step))
    }
  }

  // 重置状态
  const resetState = () => {
    currentStep.value = 0
    userAccount.value = ''
    verificationCode.value = ''
    if (import.meta.client) {
      localStorage.removeItem('forgetPwdStep')
      localStorage.removeItem('forgetPwdAccount')
      localStorage.removeItem('forgetPwdCode')
    }
  }

  // 监听并持久化账号和验证码
  watch(userAccount, (val) => {
    if (import.meta.client) {
      localStorage.setItem('forgetPwdAccount', val)
    }
  })

  watch(verificationCode, (val) => {
    if (import.meta.client) {
      localStorage.setItem('forgetPwdCode', val)
    }
  })

  return {
    currentStep,
    userAccount,
    verificationCode,
    updateStep,
    resetState
  }
}