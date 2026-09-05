import { defineStore } from 'pinia'
import { ref } from 'vue'

export const useDigitalHumanStore = defineStore('digitalHuman', () => {
  // ==================== State ====================

  const visible = ref(true)
  const position = ref({ x: 0, y: 0 })
  const voiceEnabled = ref(true)
  const currentLipShape = ref<string>('neutral')
  const volume = ref(2.0) // 保存音量

  // ==================== Actions ====================

  const setPosition = (x: number, y: number) => {
    position.value = { x, y }
  }

  const toggleVisible = () => {
    visible.value = !visible.value
  }

  const toggleVoice = () => {
    voiceEnabled.value = !voiceEnabled.value
  }

  const setVisible = (value: boolean) => {
    visible.value = value
  }

  const setLipShape = (shape: string) => {
    currentLipShape.value = shape
  }

  const setVolume = (vol: number) => {
    volume.value = vol
  }

  return {
    visible,
    position,
    voiceEnabled,
    currentLipShape,
    volume,
    setPosition,
    toggleVisible,
    toggleVoice,
    setVisible,
    setLipShape,
    setVolume
  }
})
