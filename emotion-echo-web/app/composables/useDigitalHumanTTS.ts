import { ref } from 'vue'
import { useTTSPlayer } from '~/composables/useTTSPlayer'
import { useDigitalHumanStore } from '~/stores/digitalHuman'
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
  const speed = ref(options.speed ?? 0.75)

  const handleLipSync: Parameters<typeof ttsPlayer.playStream>[1] = (shape, progress) => {
    if (!digitalHumanStore.voiceEnabled) return
    options.onLipShapeChange?.(shape)
  }

  const playText = async (text: string, customSpeed?: number, customVolume?: number) => {
    if (!digitalHumanStore.voiceEnabled) {
      console.log('[DigitalHumanTTS] Voice is disabled, skipping playText')
      return
    }
    const currentSpeed = customSpeed ?? speed.value
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

  const setSpeed = (newSpeed: number) => {
    speed.value = newSpeed
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
    setSpeed,
    setVolume,
  }
}
