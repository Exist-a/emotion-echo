import { useTTSPlayer } from '~/composables/useTTSPlayer'
import { useDigitalHumanStore } from '~/stores/digitalHuman'
import { useUserStore } from '~/stores/user'
import { TTS_SPEED_TO_VALUE } from '~/types/userConfig/userConfigType'
import type { LipShape } from '~/composables/useTTSPlayer'

export interface DigitalHumanTTSOptions {
  onLipShapeChange?: (shape: LipShape) => void
  onEmotionChange?: (emotion: string) => void
  voiceEnabled?: boolean
  speed?: number
  volume?: number
}

export function useDigitalHumanTTS(options: DigitalHumanTTSOptions = {}) {
  const ttsPlayer = useTTSPlayer()
  const digitalHumanStore = useDigitalHumanStore()
  const userStore = useUserStore()

  const handleLipSync: Parameters<typeof ttsPlayer.playStream>[1] = (shape, progress) => {
    if (!digitalHumanStore.voiceEnabled) return
    options.onLipShapeChange?.(shape)
  }

  // 语速解析（F-199 后续，2026-10-07 用户反馈 0.75 偏慢）：
  // customSpeed > options.speed > userConfig.ttsSpeed（设置页三档持久化，缺省 normal=1.0）。
  // 每次 playText 现读配置 —— 设置页改档后下一句即生效，无需刷新。
  // 旧实现是局部 ref(0.75) 且无任何入口可改 ⇒ 用户永远听 0.75 倍速。
  const resolveSpeed = (customSpeed?: number) =>
    customSpeed ??
    options.speed ??
    TTS_SPEED_TO_VALUE[userStore.getUserConfig().ttsSpeed ?? 'normal']

  const playText = async (text: string, customSpeed?: number, customVolume?: number) => {
    if (!digitalHumanStore.voiceEnabled) {
      console.log('[DigitalHumanTTS] Voice is disabled, skipping playText')
      return
    }
    const currentSpeed = resolveSpeed(customSpeed)
    const currentVolume = customVolume ?? digitalHumanStore.volume
    await ttsPlayer.playStream(text, handleLipSync, currentSpeed, currentVolume)
  }

  const flushRemaining = async () => {
    if (!digitalHumanStore.voiceEnabled) return
    await ttsPlayer.flushBuffer(handleLipSync)
  }

  const stop = () => {
    console.log('[DigitalHumanTTS] Stopping TTS')
    ttsPlayer.stop()
  }

  const setVoiceEnabled = (enabled: boolean) => {
    digitalHumanStore.voiceEnabled = enabled
    if (!enabled) {
      stop() // 静音直接停止播放
    }
  }

  const setVolume = (newVolume: number) => {
    digitalHumanStore.volume = newVolume
    ttsPlayer.setVolume(newVolume)
  }

  return {
    ttsPlayer,
    voiceEnabled: digitalHumanStore.voiceEnabled,
    playText,
    flushRemaining,
    stop,
    setVoiceEnabled,
    setVolume,
  }
}
