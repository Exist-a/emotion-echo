<template>
  <div class="chat-page">
    <main ref="mainRef" class="chat-main" aria-live="polite">
      <div class="dialog-list">
        <article
          v-for="item in data"
          :key="item.id"
          v-memo="[item.id, item.content, item.status, item.contentType]"
          class="dialog"
          :class="item.sender === 'user' ? 'dialog-user' : 'dialog-ai'"
        >
          <div
            v-if="item.sender === 'user' && item.contentType === 'audio'"
            class="bubble bubble-user voice-content"
          >
            <VoiceMessage
              v-if="item.audioUrl"
              :audio-url="getFullAudioUrl(item.audioUrl)"
              :duration="item.audioDuration"
              :transcript="item.content"
            />
            <span v-else class="voice-no-url">语音消息</span>
          </div>
          <!-- Stage 79: 文件消息（image/file/video）走 ChatFile 渲染（组件 Stage 58 已备，
               此前未挂载——stage-78 核查残余） -->
          <ChatFile
            v-else-if="item.contentType === 'image' || item.contentType === 'video' || (item.contentType === 'file' && item.content)"
            :content="item.content"
            :content-type="item.contentType as 'image' | 'file' | 'video'"
            :filename="item.fileName"
          />
          <div
            v-else-if="item.content"
            v-dompurify-html="getHtmlContent(item.content)"
            class="bubble"
            :class="item.sender === 'user' ? 'bubble-user' : 'bubble-ai'"
            :style="{ fontSize: userConfig.fontSize }"
          />
          <div v-else-if="item.sender === 'ai' && item.status === 'streaming'" class="bubble bubble-ai loading-bubble">
            <span class="quiet-pulse" aria-label="正在回复"/>
          </div>
        </article>
        <div v-if="conversationSender.isStreaming.value" class="breath-line" aria-hidden="true"><span/></div>
      </div>
    </main>

    <form class="composer" @submit.prevent="handleSubmit">
      <!-- Stage 89 PR-5：附件挂输入框（选文件即上传，随下一次发送一起走） -->
      <div v-if="pendingAttachment" class="attachment-chip">
        <span class="attachment-name">📎 {{ pendingAttachment.name }}</span>
        <span v-if="fileUpload.isUploading.value" class="attachment-status">上传中…</span>
        <button type="button" class="attachment-remove" aria-label="移除附件" @click="pendingAttachment = null">×</button>
      </div>
      <div class="composer-shell">
        <textarea
          v-model="message"
          class="composer-input"
          rows="2"
          maxlength="2000"
          placeholder="想说点什么？"
          @keydown.enter.exact.prevent="handleSubmit"
        />
        <div class="composer-actions">
          <input
            ref="fileInputRef"
            type="file"
            class="visually-hidden-input"
            aria-hidden="true"
            tabindex="-1"
            @change="onFilePicked"
          >
          <button type="button" class="icon-btn ghost" aria-label="添加附件" :disabled="fileUpload.isUploading.value" @click="handleAttachment">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
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
            <svg v-if="!faceEmotion.isCameraOn.value" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M23 7l-7 5 7 5V7z" />
              <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
            </svg>
            <svg v-else viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M23 7l-7 5 7 5V7z" />
              <rect x="1" y="5" width="15" height="14" rx="2" ry="2" />
              <circle cx="8.5" cy="12" r="1.2" fill="currentColor" />
            </svg>
          </button>
          <div class="spacer" />
          <button
            type="button"
            class="voice-btn"
            :class="{ recording: voiceRecorder.isRecording.value }"
            :aria-label="voiceRecorder.isRecording.value ? '停止录音' : '开始录音'"
            @click="toggleRecording"
          >
            <svg v-if="!voiceRecorder.isRecording.value" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <rect x="9" y="3" width="6" height="12" rx="3" />
              <path d="M5 11a7 7 0 0 0 14 0" />
              <line x1="12" y1="18" x2="12" y2="22" />
            </svg>
            <span v-else class="voice-dot" />
            <span v-if="voiceRecorder.isRecording.value" class="voice-ring" />
          </button>
          <button
            v-if="!conversationSender.isStreaming.value"
            type="submit"
            class="send-btn"
            :disabled="!message.trim()"
            aria-label="发送消息"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <line x1="5" y1="12" x2="19" y2="12" />
              <polyline points="13 5 20 12 13 19" />
            </svg>
          </button>
          <button
            v-else
            type="button"
            class="send-btn stop"
            aria-label="停止回复"
            @click="handleCancel"
          >
            <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor" aria-hidden="true">
              <rect x="6" y="6" width="12" height="12" rx="2" />
            </svg>
          </button>
        </div>
      </div>
      <video
        v-if="faceEmotion.isCameraOn.value"
        ref="cameraVideoRef"
        class="camera-preview"
        autoplay
        muted
        playsinline
      />
      <p v-if="faceEmotion.isCameraOn.value && faceEmotion.currentEmotion.value" class="camera-status">
        当前表情识别：<strong>{{ faceEmotion.currentEmotion.value.emotion }}</strong>
        <span v-if="faceEmotion.currentEmotion.value.confidence != null">
          （置信度 {{ Math.round(faceEmotion.currentEmotion.value.confidence * 100) }}%）
        </span>
      </p>
    </form>

    <div class="digital-human-wrapper">
      <ClientOnly>
        <DigitalHuman ref="digitalHumanRef" :visible="digitalHumanVisible" :draggable="true" :model-path="'/3d-models/digital-human.vrm'" @voice-toggle="handleVoiceToggle" />
      </ClientOnly>
    </div>
  </div>
</template>

<script setup lang="ts">
import VoiceMessage from '~/components/voice/VoiceMessage.vue'
import { useConversationSender } from '~/composables/useConversationSender'
import { useVoiceRecorder } from '~/composables/useVoiceRecorder'
import { useFaceEmotion } from '~/composables/useFaceEmotion'
import { useFileUpload } from '~/composables/useFileUpload'
import { useDigitalHumanStore } from '~/stores/digitalHuman'
import { useUserStore } from '~/stores/user'
import { marked } from 'marked'

// DigitalHuman uses Three.js (WebGL) — client-only, wrapped in <ClientOnly> in template
const DigitalHuman = import.meta.client
  ? defineAsyncComponent(() => import('~/components/digital-human/DigitalHuman.client.vue'))
  : (null as any)

const userStore = useUserStore()
const userConfig = ref(userStore.getUserConfig())
const messageStore = useMessageStore()
const digitalHumanStore = useDigitalHumanStore()
const digitalHumanVisible = ref(true)
const digitalHumanRef = ref<InstanceType<typeof DigitalHuman> | null>(null)
const runtimeConfig = useRuntimeConfig()

const handleVoiceToggle = () => digitalHumanStore.toggleVoice()

watch(
  () => digitalHumanStore.voiceEnabled,
  (newVal) => {
    if (!newVal) conversationSender.stopTTS()
  }
)

const route = useRoute()
const conversationStore = useConversationStore()
const message = ref('')
const conversationIdRef = computed(() => route.params.id as string)
const cameraVideoRef = ref<HTMLVideoElement | null>(null)
const faceEmotion = useFaceEmotion({
  sessionId: conversationIdRef
})

const conversationSender = useConversationSender({
  onLipShapeChange: (shape) => digitalHumanRef.value?.setLipShape(shape),
  onEmotionChange: (emotion) => digitalHumanRef.value?.setEmotion(emotion)
})

const toggleCamera = async () => {
  try {
    if (faceEmotion.isCameraOn.value) {
      faceEmotion.stopCamera()
    } else {
      await faceEmotion.startCamera(cameraVideoRef.value!)
    }
  } catch (err: any) {
    window.alert(`摄像头开启失败：${err?.message || '请检查浏览器权限'}`)
  }
}

const voiceRecorder = useVoiceRecorder({
  conversationId: conversationIdRef,
  onRecordingStart: () => {},
  onRecordingStop: () => {},
  onUploadSuccess: (result) => {
    const aiEmotion = result.emotion && result.emotion !== 'neutral' ? result.emotion : 'neutral'
    handleVoiceStreamResponse(result.transcript || '', aiEmotion)
  },
  onUploadError: (error) => {
    window.alert(`语音上传失败：${error}`)
  }
})

const handleVoiceStreamResponse = async (transcript: string, voiceEmotion: string) => {
  conversationSender.stopTTS()
  await conversationSender.sendToExistingConversation(
    conversationIdRef.value,
    transcript,
    'neutral',
    {
      onFinish: (messageId, aiEmotion) => {
        if (aiEmotion) digitalHumanRef.value?.setEmotion(aiEmotion)
        conversationSender.flushTTS()
      },
      onError: (error) => {
        window.alert(`AI 回复失败：${error}`)
      }
    },
    { shouldGenerateTitle: false, voiceEmotion, skipUserMessage: true }
  )
}

const handleSubmit = async () => {
  if (conversationSender.isStreaming.value) {
    conversationSender.cancelAIStream()
    conversationSender.stopTTS()
    return
  }
  const value = message.value.trim()
  // Stage 89 PR-5：文件+提问一起发——附件无文字时用默认 prompt 触发 AI 读文件
  if (pendingAttachment.value && !value) {
    if (!await sendAttachmentMessage()) return
    await conversationSender.sendToExistingConversation(
      conversationIdRef.value,
      '请帮我看看这个文件，用中文简短说明它的内容要点。',
      'neutral',
      {
        onFinish: (messageId, aiEmotion) => {
          if (aiEmotion) digitalHumanRef.value?.setEmotion(aiEmotion)
          conversationSender.flushTTS()
        },
        onError: (error) => {
          window.alert(`AI 回复失败：${error}`)
        }
      }
    )
    return
  }
  if (!value) return
  message.value = ''
  if (pendingAttachment.value && !await sendAttachmentMessage()) return
  await conversationSender.sendToExistingConversation(conversationIdRef.value, value, 'neutral', {
    onFinish: (messageId, aiEmotion) => {
      if (aiEmotion) digitalHumanRef.value?.setEmotion(aiEmotion)
      conversationSender.flushTTS()
    },
    onError: (error) => {
      window.alert(`AI 回复失败：${error}`)
    }
  })
}

const handleCancel = () => {
  conversationSender.cancelAIStream()
  conversationSender.stopTTS()
}

// ===== Stage 79: 附件上传装配（useFileUpload/ChatFile 此前已备但未接线） =====
const fileUpload = useFileUpload()
const fileInputRef = ref<HTMLInputElement | null>(null)

const handleAttachment = () => {
  fileInputRef.value?.click()
}

// Stage 89 PR-5：待发送附件（选文件即上传，随下一次发送一起走——文件+提问一次发出）
interface PendingAttachment {
  url: string
  name: string
  type: 'image' | 'file' | 'video'
}
const pendingAttachment = ref<PendingAttachment | null>(null)

const onFilePicked = async (e: Event) => {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = '' // 允许重选同一文件
  if (!file) return

  try {
    const result = await fileUpload.uploadFile(file)
    pendingAttachment.value = { url: result.url, name: file.name, type: result.type }
  } catch (err: any) {
    window.alert(`上传失败：${err?.message || '未知错误'}`)
  }
}

// Stage 89 PR-5：文件消息落库（content = MinIO URL，fileName = 原始名）
const sendAttachmentMessage = async (): Promise<boolean> => {
  const att = pendingAttachment.value
  if (!att) return true
  const persist = await messageStore.sendMessage(
    att.url,
    undefined,
    typeof crypto !== 'undefined' && crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-f`,
    att.type,
    att.name
  )
  if (!persist.isOk) {
    window.alert(`文件消息发送失败：${persist.msg}`)
    return false
  }
  pendingAttachment.value = null
  return true
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
    // P1-R2-2: marked.parse 默认允许 HTML（mermaid/script 可直接嵌入），
    // 依赖 vue-dompurify-html 在 v-html 渲染前净化。但 marked 输出仍是
    // 未净化 HTML，在 SSR / 非 vue 上下文有 XSS 风险。显式禁用 HTML 解析：
    const result = marked.parse(content, {
      gfm: true,
      breaks: true,
      async: false,
      // 关键：禁用 raw HTML 透传（marked v9+ 选项名为 mangle/headerIds 等无关）
      // 老版本 marked 没有专门"sanitize"开关，需要通过自定义 renderer 过滤
      renderer: (() => {
        const renderer = new marked.Renderer()
        const origHtml = renderer.html.bind(renderer)
        // html token → 转义后输出（不是真的 HTML）
        renderer.html = (token: any) => {
          const raw = typeof token === 'string' ? token : (token?.text || '')
          const escapeMap: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }
          return origHtml(raw.replace(/[&<>"']/g, (c: string) => escapeMap[c] ?? c))
        }
        return renderer
      })(),
    })
    return typeof result === 'string' ? result : String(result)
  } catch (e) {
    return content
  }
}

const data = computed(() => messageStore.sortedMessages)
const mainRef = ref<HTMLElement>()

const scrollToBottom = () => {
  nextTick(() => {
    if (mainRef.value) mainRef.value.scrollTop = mainRef.value.scrollHeight
  })
}

watch(() => messageStore.currentMessages.length, () => scrollToBottom(), { flush: 'post' })

onMounted(async () => {
  if (!route.params.id) {
    window.alert('获取会话错误')
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
.chat-page {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  position: relative;
  background: var(--ee-bg);
}

.chat-main {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.dialog-list {
  display: flex;
  flex-direction: column;
  width: min(820px, 100%);
  margin: 0 auto;
  padding: 24px 16px 8px;
}

.dialog {
  display: flex;
  align-items: flex-end;
  max-width: 100%;
  padding: 6px 0;
  animation: ee-fade-in 220ms var(--ease-quiet, cubic-bezier(0.22, 1, 0.36, 1));
}

.dialog-user { justify-content: flex-end; }
.dialog-ai { justify-content: flex-start; }

.bubble {
  display: inline-block;
  max-width: 78%;
  padding: 10px 14px;
  color: var(--ee-text);
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: var(--ee-radius-lg);
  line-height: 1.7;
  word-wrap: break-word;
  word-break: break-word;
  box-sizing: border-box;
}

.bubble p { margin: 0.5em 0; }
.bubble p:first-child { margin-top: 0; }
.bubble p:last-child { margin-bottom: 0; }
.bubble strong { font-weight: 600; }
.bubble code {
  padding: 0.15em 0.4em;
  background: var(--ee-surface-muted);
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.9em;
}
.bubble pre {
  margin: 0.6em 0;
  padding: 0.7em 0.9em;
  overflow-x: auto;
  background: var(--ee-surface-muted);
  border-radius: 8px;
}
.bubble pre code { padding: 0; background: transparent; }
.bubble ul, .bubble ol { margin: 0.5em 0; padding-left: 1.5em; }
.bubble li { margin: 0.25em 0; }
.bubble blockquote {
  margin: 0.5em 0;
  padding-left: 0.8em;
  color: var(--ee-text-muted);
  border-left: 3px solid var(--ee-primary);
}
.bubble h1, .bubble h2, .bubble h3, .bubble h4, .bubble h5, .bubble h6 { margin: 0.6em 0 0.3em; font-weight: 600; }
.bubble a { color: var(--ee-primary); text-decoration: underline; }

.bubble-user {
  color: #fff;
  background: var(--ee-primary);
  border-color: var(--ee-primary);
  border-bottom-right-radius: 4px;
}

.bubble-ai {
  border-bottom-left-radius: 4px;
}

.voice-content {
  min-width: 200px;
  padding: 8px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.voice-no-url { color: rgba(255, 255, 255, 0.85); font-size: 12px; text-align: center; }

.loading-bubble {
  min-width: 70px;
  padding: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.quiet-pulse {
  display: block;
  width: 28px;
  height: 8px;
  background: var(--ee-primary);
  border-radius: 999px;
  animation: ee-quiet-pulse 1.6s ease-in-out infinite;
}

.breath-line { display: flex; justify-content: flex-start; padding: 4px 0 8px 22px; }
.breath-line span {
  display: block;
  width: 48px;
  height: 2px;
  background: var(--ee-primary);
  border-radius: 999px;
  animation: ee-quiet-pulse 1.8s ease-in-out infinite;
}

/* 输入区 */
.attachment-chip {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  margin-bottom: 6px;
  border-radius: 8px;
  background: var(--color-surface-muted, rgba(0, 0, 0, 0.04));
  font-size: 13px;
}
.attachment-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.attachment-status { color: var(--color-text-secondary, #888); }
.attachment-remove {
  border: none; background: none; cursor: pointer;
  font-size: 16px; line-height: 1; color: var(--color-text-secondary, #888);
}

.composer {
  display: flex;
  justify-content: center;
  padding: 12px 16px 18px;
  background: linear-gradient(to top, var(--ee-bg) 60%, transparent);
}

.composer-shell {
  display: flex;
  flex-direction: column;
  width: min(820px, 100%);
  padding: 10px 12px 10px 14px;
  background: var(--ee-surface);
  border: 1px solid var(--ee-border);
  border-radius: 20px;
  box-shadow: 0 4px 16px rgba(32, 37, 34, 0.06);
}

.composer-input {
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
}

.composer-input::placeholder { color: var(--ee-text-muted); }

.composer-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 6px;
}

.spacer { flex: 1; }

.visually-hidden-input {
  position: absolute;
  width: 1px;
  height: 1px;
  opacity: 0;
  pointer-events: none;
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
  transition: color var(--ee-transition), border-color var(--ee-transition), background var(--ee-transition);
}

.icon-btn:hover {
  color: var(--ee-primary);
  border-color: var(--ee-primary);
  background: var(--ee-primary-soft);
}

.icon-btn.active {
  color: var(--ee-primary);
  border-color: var(--ee-primary);
  background: var(--ee-primary-soft);
}

.voice-btn {
  // Sprint 111 · A10 fix-2: 补 border-radius: 50% (Sprint 110 漏了, 父 .icon-btn 的
  // border-radius: 50% 被 .voice-btn 自身的 background 重置覆盖 → 方形按钮).
  // background 加 fallback 防 --ee-primary-soft 未定义时回退深绿.
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: var(--ee-primary);
  background: var(--ee-primary-soft, rgba(95, 143, 123, 0.12));
  border-color: transparent;
  border-radius: 50%;
}
.voice-btn:hover {
  color: #fff;
  background: var(--ee-primary);
  border-color: var(--ee-primary);
}

.voice-dot {
  width: 12px;
  height: 12px;
  background: var(--ee-accent);
  border-radius: 50%;
}

.voice-ring {
  position: absolute;
  inset: 0;
  border: 2px solid var(--ee-accent);
  border-radius: 50%;
  animation: ring-pulse 1.5s ease-out infinite;
}

@keyframes ring-pulse {
  from { transform: scale(0.85); opacity: 0.85; }
  to { transform: scale(1.5); opacity: 0; }
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
  transition: background var(--ee-transition), transform var(--ee-transition);
}

.send-btn:hover:not(:disabled) { background: var(--ee-primary-hover); transform: translateY(-1px); }
.send-btn:disabled { background: var(--ee-border); cursor: not-allowed; }
.send-btn.stop { background: var(--ee-accent); }
.send-btn.stop:hover { background: var(--ee-accent); }

.digital-human-wrapper {
  position: fixed;
  right: 18px;
  bottom: 96px;
  z-index: 6;
  width: 60px;
  height: 40px;
  cursor: move;
}

@media (max-width: 768px) {
  .dialog-list { padding: 14px 10px 8px; }
  .bubble { max-width: 86%; }
  .composer { padding: 8px 10px 12px; }
  .digital-human-wrapper { right: 10px; bottom: 84px; transform: scale(0.8); transform-origin: bottom right; }
}
</style>
