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

  const accumulatedDeltaText = ref('')
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
    cancelAIStream()
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
      {
        message: content,
        emotion,
        conversationId,
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

    await navigateTo({
      name: 'chat-conversation-detail',
      params: { id: newSessionId }
    })

    await nextTick()

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
