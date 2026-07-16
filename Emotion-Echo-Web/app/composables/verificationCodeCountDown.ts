import { ref } from 'vue'

export const verificationCodeCountDown = () => {
  const isGetVerificationCode = ref(false)
  const lastSeconds = ref(60)
  const verificationCodeText = '秒后重新获取'
  let interval: ReturnType<typeof setInterval> | null = null

  // 启动倒计时
  const startCountdown = () => {
    isGetVerificationCode.value = true
    lastSeconds.value = 60

    // 清除旧定时器，避免叠加
    if (interval) clearInterval(interval)

    interval = setInterval(() => {
      lastSeconds.value--
      if (lastSeconds.value <= 0) {
        clearInterval(interval!)
        interval = null
        isGetVerificationCode.value = false
      }
    }, 1000)
  }

  // 停止倒计时
  const stopCountdown = () => {
    isGetVerificationCode.value = false
    if (interval) {
      clearInterval(interval)
      interval = null
    }
  }

  return {
    isGetVerificationCode,
    lastSeconds,
    verificationCodeText,
    startCountdown,
    stopCountdown
  }
}
