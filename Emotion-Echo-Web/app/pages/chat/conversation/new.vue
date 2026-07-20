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
        <div class="actions-left">
          <button type="button" class="ee-btn ee-btn-circle action-btn" aria-label="添加附件" @click="handleAttachment">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path d="M21 11.5l-9 9a5 5 0 0 1-7-7l9-9a3.5 3.5 0 0 1 5 5l-9 9a2 2 0 0 1-3-3l8-8" />
            </svg>
          </button>
        </div>
        <div class="actions-right">
          <div
            class="voice-record-btn"
            :class="{ recording: isRecording }"
            @click="toggleRecording"
          >
            <svg v-if="!isRecording" viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <rect x="9" y="3" width="6" height="12" rx="3" />
              <path d="M5 11a7 7 0 0 0 14 0" />
              <line x1="12" y1="18" x2="12" y2="22" />
            </svg>
            <span v-else class="voice-center-dot"></span>
            <span v-if="isRecording" class="voice-ring"></span>
          </div>
          <hr class="ee-divider" />
          <button type="submit" class="ee-btn ee-btn-circle ee-btn-primary send-btn" :disabled="!message.trim()" aria-label="发送">
            <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <line x1="5" y1="12" x2="19" y2="12" />
              <polyline points="13 5 20 12 13 19" />
            </svg>
          </button>
        </div>
      </div>
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
import { post } from '~/composables/useApi'
import { notify } from '~/composables/useNotify'

const userStore = useUserStore()
const messageStore = useMessageStore()
const digitalHumanStore = useDigitalHumanStore()
const userConfig = ref(userStore.getUserConfig())
const message = ref('')
const isRecording = ref(false)
const conversationSender = useConversationSender()

const digitalHumanVisible = ref(true)

const { playText, flushRemaining, stop } = useDigitalHumanTTS({
  onLipShapeChange: (shape) => digitalHumanStore.setLipShape(shape)
})

watch(
  () => digitalHumanStore.voiceEnabled,
  (newVal) => {
    if (!newVal) stop()
  }
)

const handleSubmit = async () => {
  const value = message.value.trim()
  if (!value) return
  message.value = ''
  const newConv = await post<{ id: string }>('/conversations', { title: value.slice(0, 30) })
  navigateTo({ name: 'chat-conversation-detail', params: { id: newConv.id } })
}

const handleAttachment = () => {
  notify('', '附件功能尚未实现', 'info')
}

const toggleRecording = () => {
  isRecording.value = !isRecording.value
  if (isRecording.value) {
    notify('', '开始录音', 'info')
  } else {
    notify('', '录音已停止', 'info')
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
  padding: 10px 14px 10px 14px;
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

.ee-textarea::placeholder { color: var(--ee-text-muted); }

.sender-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 6px;
}

.actions-left, .actions-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.voice-record-btn {
  position: relative;
  display: grid;
  width: 32px;
  height: 32px;
  place-items: center;
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  border-radius: 50%;
  cursor: pointer;
  transition: transform var(--ee-transition), background var(--ee-transition);
}
.voice-record-btn:hover { background: var(--ee-primary); color: #fff; }
.voice-record-btn.recording { background: var(--ee-accent); color: #fff; }
.voice-center-dot { width: 12px; height: 12px; background: #fff; border-radius: 50%; }
.voice-ring { position: absolute; inset: 0; border: 2px solid #fff; border-radius: 50%; animation: ring-pulse 1.5s ease-out infinite; }

@keyframes ring-pulse { from { transform: scale(0.85); opacity: 0.85; } to { transform: scale(1.5); opacity: 0; } }

.ee-divider {
  width: 1px;
  height: 20px;
  margin: 0 2px;
  background: var(--ee-border);
  border: 0;
}
</style>
