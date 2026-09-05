/**
 * TTS 管理器 Composable
 * 管理 TTS 播放和文本缓冲
 */
import { useDigitalHumanTTS } from './useDigitalHumanTTS'

export interface UseTTSManagerOptions {
  onLipShapeChange?: (shape: string) => void
  onEmotionChange?: (emotion: string) => void
}

export interface UseTTSManagerReturn {
  isPlaying: Ref<boolean>
  isEnabled: Ref<boolean>
  playText: (text: string) => void
  flushRemaining: () => void
  stop: () => void
  setEnabled: (enabled: boolean) => void
}

export function useTTSManager(options: UseTTSManagerOptions = {}): UseTTSManagerReturn {
  const isPlaying = ref(false)
  const isEnabled = ref(true)

  const accumulatedText = ref('')
  let debounceTimer: ReturnType<typeof setTimeout> | null = null

  const {
    voiceEnabled,
    playText: originalPlayText,
    flushRemaining,
    stop,
    setVoiceEnabled
  } = useDigitalHumanTTS({
    onLipShapeChange: options.onLipShapeChange,
    onEmotionChange: options.onEmotionChange
  })

  const playText = (text: string) => {
    accumulatedText.value += text

    if (debounceTimer) {
      clearTimeout(debounceTimer)
    }

    debounceTimer = setTimeout(() => {
      if (accumulatedText.value.trim().length > 0) {
        originalPlayText(accumulatedText.value)
        accumulatedText.value = ''
        isPlaying.value = true
      }
    }, 500)
  }

  const flushRemainingText = () => {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
      debounceTimer = null
    }

    if (accumulatedText.value.trim().length > 0) {
      originalPlayText(accumulatedText.value)
      accumulatedText.value = ''
      isPlaying.value = true
    }

    flushRemaining()
  }

  const stopAll = () => {
    if (debounceTimer) {
      clearTimeout(debounceTimer)
      debounceTimer = null
    }
    accumulatedText.value = ''
    stop()
    isPlaying.value = false
  }

  const setEnabled = (enabled: boolean) => {
    isEnabled.value = enabled
    setVoiceEnabled(enabled)
    if (!enabled) {
      stopAll()
    }
  }

  return {
    isPlaying,
    isEnabled,
    playText,
    flushRemaining: flushRemainingText,
    stop: stopAll,
    setEnabled
  }
}
