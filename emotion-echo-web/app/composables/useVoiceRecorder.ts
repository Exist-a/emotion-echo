/**
 * 语音录制 Composable
 * 管理 MediaRecorder 生命周期和状态
 */
import { post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
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
        mimeType: 'audio/webm;codecs=opus',
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

    // E2E-F-110：**不要**在这里置 isStopped —— 该标志的语义是"onstop 已处理过一次"
    // （防重复上传），只在 onstop 回调和 onUnmounted 里置位。
    // 历史 bug：此处先置 isStopped = true，而 onstop 首行是 `if (isStopped) return`
    // ⇒ 上传路径永不执行（且 ondataavailable 的 `if (!isStopped ...)` 连最后一块音频
    // 也丢掉）⇒ 用户点"录音→停止"后**完全静默、无请求**。
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

      const result = await post(API_ROUTES.voiceUpload.path, formData)

      if (result) {
        const { messageId, transcript, emotion, audioUrl } = result as any

        if (messageStore.currentSessionId !== currentConversationId) {
          await messageStore.switchSession(currentConversationId)
        }

        // 语音消息落库（E2E-16 plan §2.A.4 / 测试点 #5，2026-09-22 用户实测钉出）：
        // 此前只 push 内存行 + ai/stream skipUserMessage=true ⇒ DB 无 user 行、刷新即丢。
        // 后端能力早已就绪（SendMessageReq.ContentType/FileName）。这里真 POST：
        //   content = 音频 URL（loadMoreMessages 映射 content→audioUrl 还原气泡的依据）
        //   contentType = 'audio'（DB content_type=audio）；fileName = 原始文件名
        // 落库失败降级本地行（不吞消息），userMessageId 退回上传返回的临时 id。
        const clientMsgId = crypto.randomUUID()
        let userMessageId = messageId
        try {
          const res: any = await messageStore.sendMessage(
            audioUrl || '',
            (emotion || 'neutral') as any,
            clientMsgId,
            'audio',
            'recording.webm',
          )
          if (res?.isOk && res?.data?.id !== undefined) {
            userMessageId = String(res.data.id)
          }
        } catch {
          // 降级：本地行 + 临时 id，AI 链路照常
        }

        const displayRow: any = {
          id: userMessageId,
          conversationId: currentConversationId,
          sender: 'user',
          content: transcript || '',
          contentType: 'audio',
          audioUrl: audioUrl || '',
          audioDuration: recordedDuration,
          emotionTag: emotion,
          status: 'sent',
          sendTime: Date.now(),
          clientMsgId,
        }

        // 真 store.sendMessage 已把服务端行 push 进 currentMessages（按 clientMsgId/id 幂等），
        // 该行没有 audioUrl 字段 ⇒ 就地打补丁供气泡渲染；找不到（降级路径）则推本地行。
        const idx = messageStore.currentMessages.findIndex(
          (m: any) => m.clientMsgId === clientMsgId || m.id === userMessageId,
        )
        if (idx >= 0) {
          messageStore.currentMessages[idx] = {
            ...messageStore.currentMessages[idx],
            ...displayRow,
          }
        } else {
          messageStore.currentMessages.push(displayRow)
        }

        const conversation = conversationStore.conversationList.find(
          (c) => c.id === currentConversationId,
        )
        if (conversation) {
          conversation.lastMessage = (transcript || '').slice(0, 100)
          conversation.lastMessageTime = Date.now()
          conversation.updatedAt = new Date().toISOString()
        }

        // userMessageId = 服务端真实消息 id：ai/stream 的 messageId 用它绑定
        // （face_emotion_results.message_id / 融合按 message_id 取行，绑随机 clientMsgId 会取空）
        options.onUploadSuccess?.({ ...result, userMessageId })
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
    getConversationId,
  }
}
