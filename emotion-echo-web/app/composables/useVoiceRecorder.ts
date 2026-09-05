/**
 * 语音录制 Composable
 * 管理 MediaRecorder 生命周期和状态
 */
import { post } from '~/composables/useApi'
import { useMessageStore } from '~/stores/message'
import { useConversationStore } from '~/stores/conversation'

export interface UseVoiceRecorderOptions {
  conversationId?: Ref<string> | string
  onRecordingStart?: () => void
  onRecordingStop?: (blob: Blob, duration: number) => void
  onError?: (error: string) => void
  onUploadStart?: () => void
  onUploadSuccess?: (result: any) => void
  onUploadError?: (error: string) => void
}

export interface UseVoiceRecorderReturn {
  isRecording: Ref<boolean>
  isUploading: Ref<boolean>
  duration: Ref<number>
  startRecording: () => Promise<void>
  stopRecording: () => void
  getConversationId: () => string
}

export function useVoiceRecorder(options: UseVoiceRecorderOptions = {}): UseVoiceRecorderReturn {
  const isRecording = ref(false)
  const isUploading = ref(false)
  const duration = ref(0)

  const conversationIdRef = isRef(options.conversationId)
    ? options.conversationId
    : ref(options.conversationId || '')

  let mediaRecorder: MediaRecorder | null = null
  let audioChunks: Blob[] = []
  let timer: number | null = null
  let startTime = 0
  let isStopped = false

  const messageStore = useMessageStore()
  const conversationStore = useConversationStore()

  const getConversationId = () => {
    return isRef(conversationIdRef) ? conversationIdRef.value : conversationIdRef
  }

  const handleError = (error: string) => {
    console.error('[VoiceRecorder]', error)
    options.onError?.(error)
  }

  const clearTimer = () => {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  const stopMediaRecorder = () => {
    if (mediaRecorder && mediaRecorder.state !== 'inactive') {
      mediaRecorder.stop()
    }
    mediaRecorder = null
  }

  const startRecording = async () => {
    if (isRecording.value) return

    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true })

      audioChunks = []
      isStopped = false
      mediaRecorder = new MediaRecorder(stream, {
        mimeType: 'audio/webm;codecs=opus'
      })

      mediaRecorder.ondataavailable = (event) => {
        if (!isStopped && event.data.size > 0) {
          audioChunks.push(event.data)
        }
      }

      mediaRecorder.onstop = async () => {
        if (isStopped) return
        isStopped = true

        const audioBlob = new Blob(audioChunks, { type: 'audio/webm' })
        const recordedDuration = Math.round((Date.now() - startTime) / 1000)

        stream.getTracks().forEach((track) => track.stop())

        options.onRecordingStop?.(audioBlob, recordedDuration)
        await handleUpload(audioBlob, recordedDuration)
      }

      mediaRecorder.onerror = () => {
        handleError('录音失败')
        stopRecording()
      }

      mediaRecorder.start()
      isRecording.value = true
      startTime = Date.now()
      duration.value = 0

      timer = window.setInterval(() => {
        duration.value = Math.round((Date.now() - startTime) / 1000)
      }, 1000)

      options.onRecordingStart?.()
    } catch {
      handleError('无法访问麦克风，请检查权限设置')
    }
  }

  const stopRecording = () => {
    if (!isRecording.value) return

    isStopped = true
    isRecording.value = false

    clearTimer()
    stopMediaRecorder()
  }

  const handleUpload = async (blob: Blob, recordedDuration: number) => {
    const currentConversationId = getConversationId()
    if (!currentConversationId) {
      handleError('会话ID无效')
      return
    }

    isUploading.value = true
    options.onUploadStart?.()

    try {
      const formData = new FormData()
      formData.append('conversationId', currentConversationId)
      formData.append('file', blob, 'recording.webm')

      const result = await post('/voice/upload', formData)

      if (result) {
        const { messageId, transcript, emotion, audioUrl } = result as any

        const userMessage: any = {
          id: messageId,
          conversationId: currentConversationId,
          sender: 'user',
          content: transcript || '',
          contentType: 'audio',
          audioUrl: audioUrl || '',
          audioDuration: recordedDuration,
          emotionTag: emotion,
          status: 'sent',
          sendTime: Date.now()
        }

        if (messageStore.currentSessionId !== currentConversationId) {
          await messageStore.switchSession(currentConversationId)
        }

        messageStore.currentMessages.push(userMessage)

        const conversation = conversationStore.conversationList.find((c) => c.id === currentConversationId)
        if (conversation) {
          conversation.lastMessage = (transcript || '').slice(0, 100)
          conversation.lastMessageTime = Date.now()
          conversation.updatedAt = new Date().toISOString()
        }

        options.onUploadSuccess?.(result)
      }
    } catch (error: any) {
      handleError(error.message || '上传失败')
      options.onUploadError?.(error.message || '上传失败')
    } finally {
      isUploading.value = false
    }
  }

  onUnmounted(() => {
    isStopped = true

    if (mediaRecorder && mediaRecorder.state !== 'inactive') {
      mediaRecorder.ondataavailable = null
      mediaRecorder.onstop = null
      mediaRecorder.onerror = null
      mediaRecorder.stop()
    }
    mediaRecorder = null

    clearTimer()

    audioChunks = []
    duration.value = 0
  })

  return {
    isRecording,
    isUploading,
    duration,
    startRecording,
    stopRecording,
    getConversationId
  }
}
