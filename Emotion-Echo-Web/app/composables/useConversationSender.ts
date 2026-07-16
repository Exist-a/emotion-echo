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

    const tempUserMessage: MessageWithStatus | null = extraParams?.skipUserMessage
      ? null
      : ({
          id: `temp_user_${Date.now()}`,
          conversationId,
          sender: 'user',
          content,
          contentType: 'text',
          emotionTag: emotion,
          sendTime: Date.now(),
          createdAt: Math.floor(Date.now() / 1000),
          status: 'sent'
        } as MessageWithStatus)

    if (tempUserMessage) {
      messageStore.addMessage(tempUserMessage)
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
        shouldGenerateTitle: extraParams?.shouldGenerateTitle,
        voiceEmotion: extraParams?.voiceEmotion
      },
      {
        onStart: (data) => {
          if (data.conversationId && data.userMessageId && tempUserMessage) {
            messageStore.updateMessage(tempUserMessage.id, { id: data.userMessageId } as any)
          }
        },
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
            id: data.messageId,
            status: 'sent'
          })

          updateConversation(
            conversationId,
            tempAiMessage.content.slice(0, 100) || content.slice(0, 100)
          )

          if (data.emotion) {
            callbacks?.onFinish?.(data.messageId, data.emotion)
          } else {
            callbacks?.onFinish?.(data.messageId)
          }
        },
        onError: (error) => {
          stopTTS()
          messageStore.updateMessage(tempAiMessage.id, {
            status: 'failed',
            content: error
          })
          callbacks?.onError?.(error)
        },
        onTruncated: (data) => {
          flushTTS()
          messageStore.updateMessage(tempAiMessage.id, {
            status: 'truncated',
            content: data.content
          })
          updateConversation(conversationId, data.content.slice(0, 100))
        },
        onTitleUpdated: (data) => {
          const conv = conversationStore.conversationList.find((c) => c.id === data.conversationId)
          if (conv) {
            conv.title = data.title
            conv.updatedAt = new Date().toISOString()
          }
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
