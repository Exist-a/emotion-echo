<template>
  <div class="new-conversation">
    <h1 class="title">你好啊，让我们开始聊天吧</h1>
    <form class="sender-container" @submit.prevent="handleSubmit">
      <textarea
        v-model="message"
        class="ee-input ee-textarea sender-input"
        placeholder="请输入想倾诉的内容，按发送按钮提交..."
        :rows="3"
        :maxlength="2000"
        @keydown.enter.exact.prevent="handleSubmit"
      />
      <div class="sender-actions">
        <button
          type="button"
          class="icon-btn ghost"
          aria-label="添加附件"
          @click="handleAttachment"
        >
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M21 11.5l-9 9a5 5 0 0 1-7-7l9-9a3.5 3.5 0 0 1 5 5l-9 9a2 2 0 0 1-3-3l8-8" />
          </svg>
        </button>
        <button
          type="button"
          class="icon-btn ghost camera-btn"
          :class="{ active: faceEmotion.isCameraOn.value }"
          :aria-label="faceEmotion.isCameraOn.value ? '关闭摄像头' : '开启摄像头'"
          @click="toggleCamera"
        >
          <svg
            v-if="!faceEmotion.isCameraOn.value"
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M23 7l-7 5 7 5V7z" />
            <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
          </svg>
          <svg
            v-else
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M23 7l-7 5 7 5V7z" />
            <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
            <circle cx="8.5" cy="12" r="1.2" fill="currentColor" />
          </svg>
        </button>
        <div class="spacer" />
        <div class="voice-record-btn" :class="{ recording: voiceRecorder.isRecording.value }" @click="toggleRecording">
          <svg
            v-if="!voiceRecorder.isRecording.value"
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="1.6"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <rect x="9" y="3" width="6" height="12" rx="3" />
            <path d="M5 11a7 7 0 0 0 14 0" />
            <line x1="12" y1="18" x2="12" y2="22" />
          </svg>
          <span v-else class="voice-center-dot" />
          <span v-if="voiceRecorder.isRecording.value" class="voice-ring" />
        </div>
        <button type="submit" class="send-btn" :disabled="!message.trim()" aria-label="发送">
          <svg
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <line x1="5" y1="12" x2="19" y2="12" />
            <polyline points="13 5  20 12 13 19" />
          </svg>
        </button>
      </div>
      <video
        v-if="faceEmotion.isCameraOn.value"
        ref="cameraVideoRef"
        class="camera-preview"
        autoplay
        muted
        playsinline
      />
      <p
        v-if="faceEmotion.isCameraOn.value && faceEmotion.currentEmotion.value"
        class="camera-status"
      >
        当前表情识别：<strong>{{ faceEmotion.currentEmotion.value.emotion }}</strong>
        <span v-if="faceEmotion.currentEmotion.value.confidence != null">
          （置信度 {{ Math.round(faceEmotion.currentEmotion.value.confidence * 100) }}%）
        </span>
      </p>
    </form>
  </div>
</template>

<script setup lang="ts">
import { watch } from 'vue'
import { useDigitalHumanTTS } from '~/composables/useDigitalHumanTTS'
import { useDigitalHumanStore } from '~/stores/digitalHuman'
import { useUserStore } from '~/stores/user'
import { useMessageStore } from '~/stores/message'
import { useConversationSender } from '~/composables/useConversationSender'
import { useFaceEmotion } from '~/composables/useFaceEmotion'
import { useVoiceRecorder } from '~/composables/useVoiceRecorder'
import { post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { notify } from '~/composables/useNotify'

const userStore = useUserStore()
const messageStore = useMessageStore()
const digitalHumanStore = useDigitalHumanStore()
const userConfig = ref(userStore.getUserConfig())
const message = ref('')
const conversationSender = useConversationSender()

const digitalHumanVisible = ref(true)
const cameraVideoRef = ref<HTMLVideoElement | null>(null)
const faceEmotion = useFaceEmotion()

// D-13（E2E-16 plan §A.5）：/new 页 voice-record-btn 接真实 useVoiceRecorder，
// 与 /conversation/[id].vue 同模式 —— 录音→BFF /voice/upload→落库→触发 AI 回复。
// 旧实现只 toggle isRecording 状态+toast，未真实录音上传（用户实测：录音后无任何反馈）。
const voiceRecorder = useVoiceRecorder({
  onUploadSuccess: (result) => {
    const aiEmotion = result.emotion && result.emotion !== 'neutral' ? result.emotion : 'neutral'
    const transcript = result.transcript || ''
    // 把语音文本作为新会话首条消息（与 handleSubmit 一致路径：createNewConversation）
    conversationSender.createNewConversation(transcript).then((r) => {
      if (!r.isOk) notify('发送失败', r.msg, 'error')
    }).catch((err) => notify('发送失败', err?.message || '', 'error'))
  },
  onUploadError: (err: string) => {
    notify('语音上传失败', err || '语音上传失败', 'error')
  },
})

const { playText, flushRemaining, stop } = useDigitalHumanTTS({
  onLipShapeChange: (shape) => digitalHumanStore.setLipShape(shape),
})

watch(
  () => digitalHumanStore.voiceEnabled,
  (newVal) => {
    if (!newVal) stop()
  },
)

const handleSubmit = async () => {
  const value = message.value.trim()
  if (!value) return
  message.value = ''
  // D-14（E2E-16）：face emotion 上下文（3 秒有效窗口，摄像头未开则 undefined 不污染 prompt）
  const recentFace = faceEmotion.getRecentEmotion()
  // Stage 105 fix: 走 conversationSender.createNewConversation,
  // 内部按顺序: createConversation → navigateTo → switchSession → sendMessage + sendAIStream。
  // 旧实现只建会话不发言, sendMessage / SSE stream 链路被跳过, 用户在 /new 看不到 AI 回复。
  const result = await conversationSender.createNewConversation(value, {
    faceEmotion: recentFace?.emotion,
    faceConfidence: recentFace?.confidence,
  })
  if (!result.isOk) {
    notify('发送失败', result.msg, 'error')
  }
}

const handleAttachment = () => {
  notify('', '附件功能尚未实现', 'info')
}

const toggleRecording = async () => {
  if (voiceRecorder.isRecording.value) {
    voiceRecorder.stopRecording()
  } else {
    try {
      await voiceRecorder.startRecording()
    } catch (err: any) {
      notify('录音启动失败', err?.message || '请检查麦克风权限', 'error')
    }
  }
}

const toggleCamera = async () => {
  try {
    if (faceEmotion.isCameraOn.value) {
      faceEmotion.stopCamera()
      notify('', '摄像头已关闭', 'info')
    } else {
      await faceEmotion.startCamera(cameraVideoRef.value!)
      notify('', '摄像头已开启，正在分析表情', 'success')
    }
  } catch (err: any) {
    notify('摄像头开启失败', err?.message || '请检查浏览器权限', 'error')
  }
}
</script>

<style scoped lang="scss">
.new-conversation {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  width: min(820px, 100%);
  margin: 0 auto;
  padding: 80px 16px 24px;
  text-align: center;
}

.title {
  margin: 0 0 32px;
  color: var(--ee-text);
  font-size: clamp(22px, 2.6vw, 30px);
  font-weight: 600;
  letter-spacing: -0.02em;
}

.sender-container {
  display: flex;
  flex-direction: column;
  width: 100%;
  padding: 10px 14px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: 20px;
  box-shadow: 0 4px 16px rgba(32, 37, 34, 0.06);
}

.ee-textarea {
  display: block;
  width: 100%;
  padding: 4px 2px;
  color: var(--ee-text);
  background: transparent;
  border: 0;
  outline: 0;
  resize: none;
  font: inherit;
  line-height: 1.6;
  min-height: 64px;
}

.ee-textarea::placeholder {
  color: var(--ee-text-muted);
}

.sender-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 6px;
}

.spacer {
  flex: 1;
}

.icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: var(--ee-text-muted);
  background: transparent;
  border: 1px solid var(--ee-border);
  border-radius: 50%;
  cursor: pointer;
  transition:
    color var(--ee-transition),
    border-color var(--ee-transition),
    background var(--ee-transition);
}

.icon-btn:hover,
.icon-btn.active {
  color: var(--ee-primary);
  border-color: var(--ee-primary);
  background: var(--ee-primary-soft);
}

.voice-record-btn {
  position: relative;
  display: inline-grid;
  width: 32px;
  height: 32px;
  place-items: center;
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  border-radius: 50%;
  cursor: pointer;
  transition:
    transform var(--ee-transition),
    background var(--ee-transition);
}
.voice-record-btn:hover {
  background: var(--ee-primary);
  color: #fff;
}
.voice-record-btn.recording {
  background: var(--ee-accent);
  color: #fff;
}
.voice-center-dot {
  width: 12px;
  height: 12px;
  background: #fff;
  border-radius: 50%;
}
.voice-ring {
  position: absolute;
  inset: 0;
  border: 2px solid #fff;
  border-radius: 50%;
  animation: ring-pulse 1.5s ease-out infinite;
}

@keyframes ring-pulse {
  from {
    transform: scale(0.85);
    opacity: 0.85;
  }
  to {
    transform: scale(1.5);
    opacity: 0;
  }
}

.send-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: #fff;
  background: var(--ee-primary);
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  transition:
    background var(--ee-transition),
    transform var(--ee-transition);
}
.send-btn:hover:not(:disabled) {
  background: var(--ee-primary-hover);
  transform: translateY(-1px);
}
.send-btn:disabled {
  background: var(--ee-border);
  cursor: not-allowed;
}

.camera-preview {
  width: 240px;
  height: 180px;
  margin: 12px auto 0;
  border-radius: 12px;
  background: #000;
  object-fit: cover;
  display: block;
}

.camera-status {
  margin: 8px 0 0;
  font-size: 12px;
  color: var(--ee-text-muted);
  text-align: center;
}
.camera-status strong {
  color: var(--ee-primary);
  margin: 0 4px;
}
</style>
