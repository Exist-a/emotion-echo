<template>
  <div class="chat-page-container" :class="$device.isMobile ? 'chat-page-container-mobile' : ''">
    <main class="main" ref="mainRef">
      <div
        class="dialog"
        :class="item.sender === 'user' ? 'dialog-user' : 'dialog-AI'"
        v-for="item in data"
        :key="item.id"
        v-memo="[item.id, item.content, item.status, item.contentType]"
      >
        <div
          v-if="item.sender === 'user' && item.contentType === 'audio'"
          class="content content-user voice-content"
        >
          <VoiceMessage
            v-if="item.audioUrl"
            :audio-url="getFullAudioUrl(item.audioUrl)"
            :duration="item.audioDuration"
            :transcript="item.content"
          />
          <span v-else class="voice-no-url">语音消息</span>
        </div>
        <div
          v-else-if="item.content"
          class="content"
          :class="item.sender === 'user' ? 'content-user' : 'content-AI'"
          :style="{ fontSize: userConfig.fontSize }"
          v-dompurify-html="getHtmlContent(item.content)"
        ></div>
        <div
          v-else-if="item.sender === 'ai' && item.status === 'streaming'"
          class="content content-AI loading-bubble"
        >
          <div class="loading-dots">
            <span></span>
            <span></span>
            <span></span>
          </div>
        </div>
      </div>
    </main>
    <div class="input-wrapper">
      <div class="sender-container">
        <el-input
          v-model="message"
          type="textarea"
          :rows="3"
          :maxlength="2000"
          resize="none"
          placeholder="请输入想倾诉的内容，按发送按钮提交..."
          class="sender-input"
          @keydown.enter.prevent="handleEnterSubmit"
        />
        <div class="sender-actions">
          <div class="actions-left">
            <el-button
              :icon="Paperclip"
              circle
              size="small"
              class="action-btn"
              @click="handleAttachment"
            />
          </div>
          <div class="actions-right">
            <div
              class="voice-record-btn"
              :class="{ recording: voiceRecorder.isRecording.value }"
              @click="toggleRecording"
            >
              <div v-if="!voiceRecorder.isRecording.value" class="microphone-icon">
                <el-icon>
                  <Microphone />
                </el-icon>
              </div>
              <div v-else class="voice-center-dot"></div>
              <div v-if="voiceRecorder.isRecording.value" class="voice-ring"></div>
            </div>
            <el-divider direction="vertical" border-style="dashed" class="vertical-divider" />
            <el-button
              v-if="!conversationSender.isStreaming.value"
              circle
              :icon="Promotion"
              type="primary"
              size="small"
              class="send-btn"
              :disabled="!message.trim()"
              @click="handleSubmit"
            />
            <el-button
              v-else
              circle
              :icon="CircleClose"
              type="danger"
              size="small"
              class="stop-btn"
              @click="handleCancel"
            />
          </div>
        </div>
      </div>
    </div>

    <div id="digital-human-wrapper" class="digital-human-wrapper">
      <DigitalHuman
        ref="digitalHumanRef"
        :visible="digitalHumanVisible"
        :draggable="true"
        :model-path="'/3d-models/digital-human.vrm'"
        @voice-toggle="handleVoiceToggle"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { Promotion, Microphone, Paperclip, CircleClose } from '@element-plus/icons-vue'
import DigitalHuman from '~/components/digital-human/DigitalHuman.vue'
import VoiceMessage from '~/components/voice/VoiceMessage.vue'
import { useConversationSender } from '~/composables/useConversationSender'
import { useVoiceRecorder } from '~/composables/useVoiceRecorder'
import { useDigitalHumanStore } from '~/stores/digitalHuman'
import { useUserStore } from '~/stores/user'
import { marked } from 'marked'

const userStore = useUserStore()
const userConfig = ref(userStore.getUserConfig())
const messageStore = useMessageStore()
const digitalHumanStore = useDigitalHumanStore()
const digitalHumanVisible = ref(true)
const digitalHumanRef = ref<InstanceType<typeof DigitalHuman> | null>(null)
const runtimeConfig = useRuntimeConfig()

const handleVoiceToggle = () => {
  digitalHumanStore.toggleVoice()
}

watch(
  () => digitalHumanStore.voiceEnabled,
  (newVal) => {
    if (!newVal) {
      conversationSender.stopTTS()
    }
  }
)

const route = useRoute()
const conversationStore = useConversationStore()
const message = ref('')
const conversationIdRef = computed(() => route.params.id as string)

const conversationSender = useConversationSender({
  onLipShapeChange: (shape) => {
    digitalHumanRef.value?.setLipShape(shape)
  },
  onEmotionChange: (emotion) => {
    digitalHumanRef.value?.setEmotion(emotion)
  }
})

const voiceRecorder = useVoiceRecorder({
  conversationId: conversationIdRef,
  onRecordingStart: () => {},
  onRecordingStop: () => {},
  onUploadSuccess: (result) => {
    const aiEmotion = result.emotion && result.emotion !== 'neutral' ? result.emotion : 'neutral'
    handleVoiceStreamResponse(result.transcript || '', aiEmotion)
  },
  onUploadError: (error) => {
    ElNotification({
      title: '语音上传失败',
      message: error,
      type: 'error'
    })
  }
})

const handleVoiceStreamResponse = async (transcript: string, voiceEmotion: string) => {
  conversationSender.stopTTS()

  await conversationSender.sendToExistingConversation(
    conversationIdRef.value,
    transcript,
    'neutral',
    {
      onDelta: () => {},
      onFinish: (messageId, aiEmotion) => {
        if (aiEmotion) {
          digitalHumanRef.value?.setEmotion(aiEmotion)
        }
        conversationSender.flushTTS()
      },
      onError: (error) => {
        ElNotification({
          title: 'AI 回复失败',
          message: error,
          type: 'error'
        })
      }
    },
    {
      shouldGenerateTitle: false,
      voiceEmotion,
      skipUserMessage: true
    }
  )
}

const handleSubmit = async () => {
  if (conversationSender.isStreaming.value) {
    conversationSender.cancelAIStream()
    conversationSender.stopTTS()
    return
  }

  const value = message.value.trim()
  if (!value) return

  message.value = ''

  await conversationSender.sendToExistingConversation(conversationIdRef.value, value, 'neutral', {
    onFinish: (messageId, aiEmotion) => {
      if (aiEmotion) {
        digitalHumanRef.value?.setEmotion(aiEmotion)
      }
      conversationSender.flushTTS()
    },
    onError: (error) => {
      ElNotification({
        title: 'AI 回复失败',
        message: error,
        type: 'error'
      })
    }
  })
}

const handleCancel = () => {
  conversationSender.cancelAIStream()
  conversationSender.stopTTS()
}

const handleEnterSubmit = (e: KeyboardEvent) => {
  if (!e.shiftKey) {
    handleSubmit()
  }
}

const handleAttachment = () => {
  console.log('打开附件上传')
}

const toggleRecording = async () => {
  if (voiceRecorder.isRecording.value) {
    voiceRecorder.stopRecording()
  } else {
    await voiceRecorder.startRecording()
  }
}

const getFullAudioUrl = (url: string) => {
  if (!url) return url
  if (url.startsWith('http')) return url
  const apiBase = runtimeConfig.public.API_BASE_URL as string
  const baseUrl = apiBase.replace('/api/v1', '')
  return baseUrl + url
}

const getHtmlContent = (content: string) => {
  if (!content || typeof content !== 'string') return ''
  try {
    const result = marked.parse(content)
    return typeof result === 'string' ? result : String(result)
  } catch (e) {
    return content
  }
}

const data = computed(() => messageStore.sortedMessages)
const mainRef = ref<HTMLElement>()

const scrollToBottom = () => {
  nextTick(() => {
    if (mainRef.value) {
      mainRef.value.scrollTop = mainRef.value.scrollHeight
    }
  })
}

watch(
  () => messageStore.currentMessages.length,
  () => scrollToBottom(),
  { flush: 'post' }
)

onMounted(async () => {
  if (!route.params.id) {
    ElNotification({
      type: 'error',
      message: '获取会话错误'
    })
    return
  }
  const id = route.params.id as string
  if (messageStore.currentSessionId !== id) {
    await messageStore.switchSession(id)
  }
  scrollToBottom()
})

onUnmounted(() => {
  conversationSender.cancelAIStream()
  conversationSender.stopTTS()
  if (voiceRecorder.isRecording.value) {
    voiceRecorder.stopRecording()
  }
})
</script>

<style scoped lang="scss">
.chat-page-container {
  width: 100%;
  height: calc(100vh - 25px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  padding: 0 10px;
  box-sizing: border-box;
}
.chat-page-container-mobile {
  height: calc(100vh - 25px - 65px);
}
.main {
  flex: 1;
  width: 80%;
  margin: 0 auto;
  overflow-y: auto;
  padding: 10px 0 20px;
  &::-webkit-scrollbar {
    width: 6px;
  }
  &::-webkit-scrollbar-thumb {
    background-color: #e5e7eb;
    border-radius: 3px;
  }

  .dialog {
    padding: 10px 0;
    display: flex;
    max-width: 100%;
    height: auto;
    align-items: flex-end;

    .content {
      display: inline-block;
      background-color: #efefefaa;
      border-radius: $radius-lg;
      padding: 8px 12px;
      line-height: 1.6;
      max-width: 70%;
      word-wrap: break-word;
      word-break: break-word;
      box-sizing: border-box;

      p {
        margin: 0.5em 0;
        &:first-child {
          margin-top: 0;
        }
        &:last-child {
          margin-bottom: 0;
        }
      }

      strong {
        font-weight: 600;
      }
      em {
        font-style: italic;
      }

      code {
        background-color: rgba(0, 0, 0, 0.08);
        padding: 0.2em 0.4em;
        border-radius: 4px;
        font-family: monospace;
        font-size: 0.9em;
      }

      pre {
        background-color: rgba(0, 0, 0, 0.08);
        padding: 0.8em 1em;
        border-radius: 6px;
        overflow-x: auto;
        margin: 0.8em 0;
        code {
          background-color: transparent;
          padding: 0;
          font-size: 0.9em;
        }
      }

      ul,
      ol {
        margin: 0.5em 0;
        padding-left: 1.5em;
      }

      li {
        margin: 0.25em 0;
      }

      blockquote {
        border-left: 3px solid #ccc;
        padding-left: 1em;
        margin: 0.5em 0;
        color: #666;
      }

      h1,
      h2,
      h3,
      h4,
      h5,
      h6 {
        margin: 0.6em 0 0.3em;
        font-weight: 600;
      }
      h1 {
        font-size: 1.5em;
      }
      h2 {
        font-size: 1.3em;
      }
      h3 {
        font-size: 1.15em;
      }
      h4 {
        font-size: 1.05em;
      }
      h5 {
        font-size: 1em;
      }
      h6 {
        font-size: 0.95em;
      }

      a {
        color: #409eff;
        text-decoration: underline;
        &:hover {
          color: #66b1ff;
        }
      }
    }

    .voice-content {
      display: flex;
      flex-direction: column;
      gap: 6px;
      padding: 6px 10px;
      min-width: 200px;
      .voice-no-url {
        color: #999;
        font-size: 12px;
        text-align: center;
      }
    }
  }

  .dialog-user {
    justify-content: flex-end;
    .content-user {
      background-color: #07c160;
      color: #fff;
      border-top-left-radius: 12px;
      border-top-right-radius: 12px;
      border-bottom-left-radius: 12px;
      border-bottom-right-radius: 4px;
    }
  }

  .dialog-AI {
    justify-content: flex-start;
    .content-AI {
      background-color: #f0f0f0;
      color: #333;
      border-top-left-radius: 12px;
      border-top-right-radius: 12px;
      border-bottom-right-radius: 12px;
      border-bottom-left-radius: 4px;
    }
  }
}

.input-wrapper {
  width: 100%;
  padding: 10px 0;
  display: flex;
  justify-content: center;
  align-items: center;
  box-sizing: border-box;
  overflow: hidden;
  margin-bottom: 30px;
}

.sender-container {
  width: clamp(300px, 80%, 1200px);
  max-width: 100%;
  min-width: 0;
  flex-shrink: 1;
  border: 1px solid #e5e7eb;
  border-radius: $radius-xxl;
  background-color: transparent;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;

  .sender-input {
    :deep(.el-textarea__inner) {
      border: none;
      box-shadow: none;
      background-color: transparent;
      resize: none;
      padding: 8px 4px;
      &::placeholder {
        color: #9ca3af;
        font-size: 14px;
      }
    }
  }
}

.loading-bubble {
  min-width: 60px;
  padding: 8px 12px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.loading-dots {
  display: flex;
  gap: 4px;
  align-items: center;
  span {
    width: 8px;
    height: 8px;
    background-color: #999;
    border-radius: 50%;
    animation: bounce 1.4s infinite ease-in-out both;
    &:nth-child(1) {
      animation-delay: -0.32s;
    }
    &:nth-child(2) {
      animation-delay: -0.16s;
    }
  }
}

@keyframes bounce {
  0%,
  80%,
  100% {
    transform: scale(0.6);
    opacity: 0.5;
  }
  40% {
    transform: scale(1);
    opacity: 1;
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

.sender-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 4px 0;

  .actions-left {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .actions-right {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .action-btn {
    color: #6b7280;
    border-color: #e5e7eb;
    &:hover {
      color: #409eff;
      border-color: #c6e2ff;
      background-color: #f0f9ff;
    }
  }

  .voice-record-btn {
    width: 36px;
    height: 36px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    position: relative;
    background-color: #f5f5f5;
    transition: all 0.3s;

    &:hover {
      background-color: #e9d5ff;
    }

    .microphone-icon {
      display: flex;
      align-items: center;
      justify-content: center;
      color: #3388ea;
      font-size: 18px;
    }

    .voice-center-dot {
      width: 16px;
      height: 16px;
      border-radius: 50%;
      background-color: #f56c6c;
      z-index: 2;
      transition: all 0.3s;
    }

    .voice-ring {
      position: absolute;
      border-radius: 50%;
      border: 2px solid #f56c6c;
      animation: ring-pulse 1.5s infinite ease-out;
      z-index: 1;
      width: 36px;
      height: 36px;
    }

    &.recording {
      background-color: #fff;
      .voice-center-dot {
        transform: scale(1.2);
      }
    }
  }

  @keyframes ring-pulse {
    0% {
      transform: scale(0.8);
      opacity: 1;
    }
    100% {
      transform: scale(1.6);
      opacity: 0;
    }
  }

  .vertical-divider {
    height: 24px !important;
    margin: 0 6px;
    border-color: #e5e7eb;
  }

  .send-btn {
    background-color: #409eff;
    border-color: #409eff;
    width: 36px;
    height: 36px;
    &:hover {
      background-color: #66b1ff;
      border-color: #66b1ff;
    }
    &:disabled {
      background-color: #a0cfff;
      border-color: #a0cfff;
      cursor: not-allowed;
    }
  }

  .stop-btn {
    background-color: #f56c6c;
    border-color: #f56c6c;
    width: 36px;
    height: 36px;
    &:hover {
      background-color: #f78985;
      border-color: #f78985;
    }
  }
}

#digital-human-wrapper {
  width: 60px;
  height: 40px;
  position: fixed;
  right: 20px;
  top: 20px;
  z-index: 10000;
  cursor: move;
}

@media (max-width: 768px) {
  .sender-container {
    width: 95%;
  }
}

@media (max-width: 768px) {
  .main {
    width: 95%;
    padding-bottom: 15px;
  }
  .chat-page-container {
    padding: 0 5px;
  }
  .dialog .content {
    max-width: 80%;
  }
}
</style>
