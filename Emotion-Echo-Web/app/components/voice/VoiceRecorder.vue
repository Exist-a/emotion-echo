<template>
  <div class="voice-recorder">
    <span class="ee-tooltip">
      <button
        class="record-btn"
        :class="{ recording: isRecording }"
        @click="toggleRecording"
      >
        <span v-if="!isRecording" class="record-icon">🎤</span>
        <span v-else class="stop-icon">⏹</span>
      </button>
    </span>

    <div v-if="isRecording" class="recording-status">
      <span class="recording-dot"></span>
      <span class="recording-time">{{ formatTime(recordingTime) }}</span>
    </div>

    <div v-if="isUploading" class="uploading-status">
      <span class="uploading-text">上传中...</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onUnmounted } from 'vue';

interface Props {
  disabled?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
});

const emit = defineEmits<{
  (e: 'recorded', data: { audioBlob: Blob; duration: number }): void;
  (e: 'error', error: string): void;
}>();

const isRecording = ref(false);
const isUploading = ref(false);
const recordingTime = ref(0);

let mediaRecorder: MediaRecorder | null = null;
let audioChunks: Blob[] = [];
let timer: number | null = null;
let startTime = 0;

const toggleRecording = async () => {
  if (props.disabled || isUploading.value) return;

  if (isRecording.value) {
    stopRecording();
  } else {
    await startRecording();
  }
};

const startRecording = async () => {
  try {
    const stream = await navigator.mediaDevices.getUserMedia({ audio: true });

    audioChunks = [];
    mediaRecorder = new MediaRecorder(stream, {
      mimeType: 'audio/webm;codecs=opus',
    });

    mediaRecorder.ondataavailable = (event) => {
      if (event.data.size > 0) {
        audioChunks.push(event.data);
      }
    };

    mediaRecorder.onstop = async () => {
      const audioBlob = new Blob(audioChunks, { type: 'audio/webm' });
      const duration = Math.round((Date.now() - startTime) / 1000);

      stream.getTracks().forEach((track) => track.stop());

      emit('recorded', { audioBlob, duration });
    };

    mediaRecorder.onerror = (event) => {
      console.error('[VoiceRecorder] MediaRecorder error:', event);
      emit('error', '录音失败，请重试');
      stopRecording();
    };

    mediaRecorder.start();
    isRecording.value = true;
    startTime = Date.now();
    recordingTime.value = 0;

    timer = window.setInterval(() => {
      recordingTime.value = Math.round((Date.now() - startTime) / 1000);
    }, 1000);
  } catch (error) {
    console.error('[VoiceRecorder] Failed to start recording:', error);
    emit('error', '无法访问麦克风，请检查权限设置');
  }
};

const stopRecording = () => {
  if (mediaRecorder && mediaRecorder.state !== 'inactive') {
    mediaRecorder.stop();
  }

  if (timer) {
    clearInterval(timer);
    timer = null;
  }

  isRecording.value = false;
};

const formatTime = (seconds: number): string => {
  const mins = Math.floor(seconds / 60);
  const secs = seconds % 60;
  return `${mins.toString().padStart(2, '0')}:${secs.toString().padStart(2, '0')}`;
};

onUnmounted(() => {
  if (isRecording.value) {
    stopRecording();
  }
  if (timer) {
    clearInterval(timer);
  }
});
</script>

<style scoped lang="scss">
.voice-recorder {
  display: flex;
  align-items: center;
  gap: 8px;
}

.record-btn {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  border: none;
  background-color: #f5f5f5;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  transition: all 0.3s ease;

  &:hover {
    background-color: #e8e8e8;
  }

  &.recording {
    background-color: #f56c6c;
    animation: pulse 1s infinite;

    .record-icon,
    .stop-icon {
      color: white;
    }
  }

  &:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
}

@keyframes pulse {
  0% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.05);
  }
  100% {
    transform: scale(1);
  }
}

.recording-status {
  display: flex;
  align-items: center;
  gap: 6px;
  color: #f56c6c;
  font-size: 14px;
}

.recording-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background-color: #f56c6c;
  animation: blink 1s infinite;
}

@keyframes blink {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.3;
  }
}

.recording-time {
  font-family: monospace;
  font-weight: 500;
}

.uploading-status {
  color: #909399;
  font-size: 14px;
}
</style>
