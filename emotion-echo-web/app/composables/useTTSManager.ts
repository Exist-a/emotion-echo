/**
 * TTS 管理器 Composable
 * 管理 TTS 播放和文本缓冲
 *
 * E2E-28 C4 / D-43 F-134（2026-10-03）：攒到标点即切段发出 —— 旧行为
 * 500ms debounce 被每个 SSE token 重置，整条回复流完才首次 TTS（首声 =
 * 全文 LLM + 全文合成）。切段后首声 = 首句 LLM + 首句合成；余量走原
 * debounce 尾冲。切段规则见 utils/takeSentenceSegment。
 */
import { useDigitalHumanTTS } from './useDigitalHumanTTS'
import { takeSentenceSegment, type SegmentTake } from '~/utils/takeSentenceSegment'

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
    setVoiceEnabled,
  } = useDigitalHumanTTS({
    onLipShapeChange: options.onLipShapeChange,
    onEmotionChange: options.onEmotionChange,
  })

  const playText = (text: string) => {
    accumulatedText.value += text

    // F-134：攒到标点立即切段发出（while —— 一次 token 可能补完多段）。
    // 段进入 playText 的下游队列（F-129 队列化 + 预取）自然衔接。
    let seg: SegmentTake | null
    while ((seg = takeSentenceSegment(accumulatedText.value)) !== null) {
      originalPlayText(seg.flush)
      accumulatedText.value = seg.rest
      isPlaying.value = true
    }

    if (debounceTimer) {
      clearTimeout(debounceTimer)
    }

    // 尾冲：无标点余量（流末尾/短语）仍按 500ms 静默发出
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
    setEnabled,
  }
}
