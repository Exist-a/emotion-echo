<template>
  <div class="voice-message">
    <div class="voice-bar" @click="togglePlay">
      <button class="play-btn">
        <span v-if="!isPlaying">▶</span>
        <span v-else>⏸</span>
      </button>
      <div class="waveform">
        <span v-for="i in 10" :key="i" class="wave" :style="{ height: waveHeights[i - 1] + '%' }"></span>
      </div>
      <span class="duration">{{ formatTime(duration) }}</span>
    </div>

    <div v-if="transcript" class="transcript">
      {{ transcript }}
    </div>

    <audio ref="audioRef" :src="audioUrl" @ended="onEnded" @timeupdate="onTimeUpdate"></audio>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue';

interface Props {
  audioUrl: string;
  duration?: number;
  transcript?: string;
}

const props = withDefaults(defineProps<Props>(), {
  duration: 0,
  transcript: '',
});

const audioRef = ref<HTMLAudioElement | null>(null);
const isPlaying = ref(false);
const currentTime = ref(0);

const waveHeights = ref<number[]>([]);

onMounted(() => {
  waveHeights.value = Array.from({ length: 10 }, () => Math.random() * 60 + 20);
});

const togglePlay = () => {
  if (!audioRef.value) return;

  if (isPlaying.value) {
    audioRef.value.pause();
    isPlaying.value = false;
  } else {
    audioRef.value.play();
    isPlaying.value = true;
  }
};

const onEnded = () => {
  isPlaying.value = false;
  currentTime.value = 0;
};

const onTimeUpdate = () => {
  if (audioRef.value) {
    currentTime.value = audioRef.value.currentTime;
  }
};

const formatTime = (seconds: number): string => {
  const mins = Math.floor(seconds / 60);
  const secs = Math.floor(seconds % 60);
  return `${mins}:${secs.toString().padStart(2, '0')}`;
};
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
