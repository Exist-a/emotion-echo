/**
 * 对话发送逻辑 Composable
 * 整合 AI 流处理和消息发送
 */
import { useMessageStore } from '~/stores/message'
import { useConversationStore } from '~/stores/conversation'
import { useAIStreamHandler } from './useAIStreamHandler'
import { useTTSManager } from './useTTSManager'
import type { MessageWithStatus } from '~/types/api'

export interface UseConversationSenderOptions {
  onLipShapeChange?: (shape: string) => void
  onEmotionChange?: (emotion: string) => void
}

export const useConversationSender = (options: UseConversationSenderOptions = {}) => {
  const messageStore = useMessageStore()
  const conversationStore = useConversationStore()

  const { sendAIStream, cancelAIStream, isStreaming } = useAIStreamHandler()
  const { playText, flushRemaining, stop, setEnabled } = useTTSManager(options)

  // Sprint 108 · 跨组件实例共享 TTS 拼接缓冲 (Stage 107 架构债修复)
  // 修前 (BUG): const accumulatedDeltaText = ref('') — composable 局部 ref,
  //            A 实例累加 delta, A 被 unmount 后状态丢失, B 实例看到空字符串.
  // 修后 (FIX): useState 顶层单例 (Nuxt 3 SSR-safe), 跨 navigateTo / 跨组件实例共享.
  const accumulatedDeltaText = useState<string>('conv-sender:accumulatedDeltaText', () => '')

  // Sprint 110 · A8 修复：跨实例 SSE race guard
  // 场景: createNewConversation → await navigateTo → new.vue unmount 同步触发 onUnmounted →
  //       若未守卫, cancelAIStream 会 abort SSE → POST /api/v1/ai/stream 永远不出栈 (IAB 2026-09-17 实测)
  // 修法: useState 共享 flag, createNewConversation 入口置 true / finally 复位,
  //       onUnmounted 内 cancelAIStream 必须被 !isInCreateFlow guard 才执行
  const isInCreateFlow = useState<boolean>('conv-sender:isInCreateFlow', () => false)
  let ttsDebounceTimer: ReturnType<typeof setTimeout> | null = null

  const flushTTS = () => {
    if (ttsDebounceTimer) {
      clearTimeout(ttsDebounceTimer)
      ttsDebounceTimer = null
    }

    if (accumulatedDeltaText.value.trim().length > 0) {
      playText(accumulatedDeltaText.value)
      accumulatedDeltaText.value = ''
    }
    flushRemaining()
  }

  const stopTTS = () => {
    if (ttsDebounceTimer) {
      clearTimeout(ttsDebounceTimer)
      ttsDebounceTimer = null
    }
    accumulatedDeltaText.value = ''
    stop()
  }

  onUnmounted(() => {
    stopTTS()
    // Sprint 110 · A8 guard: createNewConversation 跨实例 SSE 流未结束前,
    // 源实例 onUnmounted 不应 abort——由目标 [id].vue 实例接管 cancel.
    if (!isInCreateFlow.value) {
      cancelAIStream()
    }
  })

  const updateConversation = (conversationId: string, content: string) => {
    const conversation = conversationStore.conversationList.find((c) => c.id === conversationId)
    if (conversation) {
      conversation.lastMessage = content.slice(0, 100)
      conversation.lastMessageTime = Date.now()
      conversation.updatedAt = new Date().toISOString()
    }
  }

  const sendToExistingConversation = async (
    conversationId: string,
    content: string,
    emotion: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral' = 'neutral',
    callbacks?: {
      onDelta?: (content: string) => void
      onFinish?: (messageId: string, aiEmotion?: string) => void
      onError?: (error: string) => void
    },
    extraParams?: {
      shouldGenerateTitle?: boolean
      voiceEmotion?: string
      skipUserMessage?: boolean
    }
  ) => {
    if (messageStore.currentSessionId !== conversationId) {
      await messageStore.switchSession(conversationId)
    }

    updateConversation(conversationId, content)

    // Stage 33 PR-18: 写库前移到 stream 调用前。
    // 客户端生成 client_msg_id (UUID) → 落库 → 触发 Kafka outbox →
    // ai-svc 情绪分析 + analytics-svc 行为事件整条链路生效。
    const clientMsgId = crypto.randomUUID()
    let userMessageId: string = clientMsgId

    if (!extraParams?.skipUserMessage) {
      const persistResult = await messageStore.sendMessage(content, emotion, clientMsgId)
      if (!persistResult.isOk || !persistResult.data) {
        callbacks?.onError?.(persistResult.msg || '消息保存失败')
        return { isOk: false, msg: persistResult.msg || '消息保存失败' }
      }
      userMessageId = String(persistResult.data.id)
    }

    const tempAiMessage: MessageWithStatus = {
      id: `temp_ai_${Date.now()}`,
      conversationId,
      sender: 'ai',
      content: '',
      contentType: 'text',
      sendTime: Date.now(),
      createdAt: Math.floor(Date.now() / 1000),
      status: 'streaming'
    } as MessageWithStatus
    messageStore.addMessage(tempAiMessage)

    accumulatedDeltaText.value = ''
    stopTTS()

    const result = await sendAIStream(
      {
        message: content,
        emotion,
        conversationId,
        messageId: userMessageId,
        clientMsgId,
        shouldGenerateTitle: extraParams?.shouldGenerateTitle,
        voiceEmotion: extraParams?.voiceEmotion
      },
      {
        onDelta: (delta) => {
          accumulatedDeltaText.value += delta
          callbacks?.onDelta?.(delta)

          if (ttsDebounceTimer) {
            clearTimeout(ttsDebounceTimer)
          }
          ttsDebounceTimer = setTimeout(() => {
            if (accumulatedDeltaText.value.trim().length > 0) {
              playText(accumulatedDeltaText.value)
              accumulatedDeltaText.value = ''
            }
          }, 500)

          messageStore.updateMessage(tempAiMessage.id, {
            content: tempAiMessage.content + delta,
            status: 'streaming'
          })
          tempAiMessage.content += delta
        },
        onFinish: (data) => {
          flushTTS()

          messageStore.updateMessage(tempAiMessage.id, {
            id: data.messageId || tempAiMessage.id,
            status: 'sent'
          })

          updateConversation(
            conversationId,
            tempAiMessage.content.slice(0, 100) || content.slice(0, 100)
          )

          callbacks?.onFinish?.(data.messageId || '', data.emotion)
        },
        onError: (error) => {
          stopTTS()
          messageStore.updateMessage(tempAiMessage.id, {
            status: 'failed',
            content: error
          })
          callbacks?.onError?.(error)
        }
      }
    )

    return result
  }

  const createNewConversation = async (
    content: string,
    options?: {
      emotion?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
      shouldGenerateTitle?: boolean
      onDelta?: (content: string) => void
      onFinish?: () => void
      onError?: (error: string) => void
    }
  ) => {
    const createResult = await conversationStore.createConversation()

    if (!createResult.isOk || !createResult.id) {
      return { isOk: false, msg: createResult.msg }
    }

    const newSessionId = createResult.id

    // Sprint 110 · A8 修复: 在触发路由切换之前先置 guard flag,
    // 这样 new.vue 的 onUnmounted 在 navigateTo 期间同步触发时,
    // 能正确跳过 cancelAIStream, 让本方法后续的 sendToExistingConversation
    // → sendAIStream → POST /api/v1/ai/stream 顺利出栈.
    // (IAB 2026-09-17 实测: 老代码 await navigateTo → unmount → cancel no-op 但 finally
    //  残留 → 下次 sendAIStream 的 isStreaming guard 卡死, SSE 永远不出栈)
    isInCreateFlow.value = true
    try {
      // 触发路由切换 (fire-and-forget, 不 await)
      // [id].vue 会在下一个 tick mount, 此时本函数已在同步等 switchSession + SSE
      navigateTo({
        name: 'chat-conversation-detail',
        params: { id: newSessionId }
      })

      // 同步把 sessionId 切到新会话 (避免 [id].vue mount 后重复请求 messages)
      await messageStore.switchSession(newSessionId)

      const result = await sendToExistingConversation(
        newSessionId,
        content,
        options?.emotion || 'neutral',
        {
          onDelta: options?.onDelta,
          onFinish: options?.onFinish,
          onError: options?.onError
        },
        {
          shouldGenerateTitle: options?.shouldGenerateTitle
        }
      )

      return { isOk: result.isOk, msg: result.msg, id: newSessionId }
    } finally {
      // 无论成功失败, 必须在 try 退出后复位 flag
      // (否则后续 [id].vue 的 onUnmounted 永远跳过 cancel → 资源泄漏)
      isInCreateFlow.value = false
    }
  }

  return {
    isStreaming,
    sendToExistingConversation,
    createNewConversation,
    cancelAIStream,
    stopTTS,
    flushTTS,
    setTTSEnabled: setEnabled
  }
}
