<template>
  <div class="voice-message">
    <div class="voice-bar" @click="togglePlay">
      <button class="play-btn">
        <span v-if="!isPlaying">▶</span>
        <span v-else>⏸</span>
      </button>
      <div class="waveform">
        <span v-for="i in 10" :key="i" class="wave" :style="{ height: waveHeights[i - 1] + '%' }" />
      </div>
      <span class="duration">{{ formatTime(actualDuration) }}</span>
    </div>

    <div v-if="showTranscript" class="transcript">
      {{ transcript }}
    </div>

    <audio ref="audioRef" :src="audioUrl" @ended="onEnded" @timeupdate="onTimeUpdate" @loadedmetadata="onLoadedMetadata" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'

interface Props {
  audioUrl: string
  duration?: number
  transcript?: string
}

const props = withDefaults(defineProps<Props>(), {
  duration: 0,
  transcript: '',
})

const audioRef = ref<HTMLAudioElement | null>(null)
const isPlaying = ref(false)
const currentTime = ref(0)
// F-117：audioDuration 字段不在 SendMessageReq（DB 无该列），加载行 prop.duration=0/undefined
// ⇒ 必须从 <audio>.loadedmetadata 事件恢复真实时长（prop 只是兜底，metadata 优先）
const actualDuration = ref(props.duration)

const waveHeights = ref<number[]>([])

onMounted(() => {
  waveHeights.value = Array.from({ length: 10 }, () => Math.random() * 60 + 20)
})

// F-117：后端落库 content=音频 URL，刷新后 transcript 槽被错误填充为 URL。
// 内部识别 URL 前缀（含 http/https/api path），是 URL 则不渲染 transcript 槽。
const isUrlTranscript = computed(() => {
  const t = (props.transcript || '').trim()
  if (!t) return false
  return t.startsWith('http://') || t.startsWith('https://') || t.startsWith('/api/')
})

const showTranscript = computed(() => {
  return Boolean(props.transcript) && !isUrlTranscript.value
})

const togglePlay = () => {
  if (!audioRef.value) return

  if (isPlaying.value) {
    audioRef.value.pause()
    isPlaying.value = false
  } else {
    audioRef.value.play()
    isPlaying.value = true
  }
}

const onEnded = () => {
  isPlaying.value = false
  currentTime.value = 0
}

const onTimeUpdate = () => {
  if (audioRef.value) {
    currentTime.value = audioRef.value.currentTime
  }
}

// F-117：从 audio metadata 恢复真实时长（prop.duration=0/undefined 时触发）。
// 一旦 audio.duration 可用即覆盖 actualDuration；浏览器在 loadedmetadata 后必填。
const onLoadedMetadata = () => {
  const d = audioRef.value?.duration
  if (typeof d === 'number' && isFinite(d) && d > 0) {
    actualDuration.value = d
  }
}

const formatTime = (seconds: number): string => {
  const mins = Math.floor(seconds / 60)
  const secs = Math.floor(seconds % 60)
  return `${mins}:${secs.toString().padStart(2, '0')}`
}
</script>

<style scoped lang="scss">
.voice-message {
  width: 100%;
}

.voice-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 16px;
  background-color: #f5f7fa;
  border-radius: 12px;
  cursor: pointer;
  transition: background-color 0.2s;

  &:hover {
    background-color: #e8eaf0;
  }

  html.dark & {
    background-color: #2d2d2d;

    &:hover {
      background-color: #3d3d3d;
    }
  }
}

.play-btn {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  border: none;
  background-color: #409eff;
  color: white;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  flex-shrink: 0;

  &:hover {
    background-color: #66b1ff;
  }
}

.waveform {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  height: 24px;
}

.wave {
  width: 3px;
  background-color: #409eff;
  border-radius: 2px;
  transition: height 0.1s ease;

  html.dark & {
    background-color: #66b1ff;
  }
}

.duration {
  font-size: 13px;
  color: #606266;
  flex-shrink: 0;
  font-family: monospace;

  html.dark & {
    color: #a0a0a0;
  }
}

.transcript {
  margin-top: 8px;
  padding: 0 16px;
  font-size: 14px;
  color: #606266;
  line-height: 1.5;

  html.dark & {
    color: #a0a0a0;
  }
}
</style>
