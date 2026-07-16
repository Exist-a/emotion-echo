import type { MessageWithStatus, SendMessageParams } from '~/types/api'
import type { returnMsgType } from '~/types/commonType'
import { get, post } from '~/composables/useApi'
import { useConversationStore } from './conversation'

export type MessageStatus = 'sending' | 'sent' | 'failed' | 'streaming' | 'truncated'

export const useMessageStore = defineStore('message', () => {
  const currentSessionId = ref<string | null>(null)
  const currentMessages = ref<MessageWithStatus[]>([])
  const isLoading = ref(false)
  const hasMore = ref(true)
  const isSending = ref(false)
  const messageCursor = ref<number>(0)

  const sortedMessages = computed(() => {
    return [...currentMessages.value].sort((a, b) => a.sendTime - b.sendTime)
  })

  const switchSession = async (sessionId: string) => {
    if (currentSessionId.value === sessionId) {
      return
    }

    currentSessionId.value = sessionId
    currentMessages.value = []
    hasMore.value = true
    messageCursor.value = 0

    try {
      await loadMoreMessages()
    } catch (e) {
      console.warn('[MESSAGE_STORE] loadMoreMessages failed:', e)
    }
  }

  const loadMoreMessages = async (limit: number = 20): Promise<returnMsgType> => {
    if (!currentSessionId.value || isLoading.value || !hasMore.value) {
      return { isOk: true, msg: '无需加载' }
    }

    isLoading.value = true
    try {
      const params: any = { limit }
      if (messageCursor.value > 0) {
        params.cursor = messageCursor.value
      }

      const data = await get<{
        list: MessageWithStatus[]
        cursor: number
        hasMore: boolean
      }>(`/conversations/${currentSessionId.value}/messages`, params)

      if (data.list.length > 0) {
        const existingIds = new Set(currentMessages.value.map((m) => m.id))
        const newMessages = data.list.filter((m) => !existingIds.has(m.id))
        currentMessages.value = [...currentMessages.value, ...newMessages]
        messageCursor.value = data.cursor
        hasMore.value = data.hasMore
      } else {
        hasMore.value = false
      }

      return { isOk: true, msg: '加载成功' }
    } catch (error: any) {
      return { isOk: false, msg: error.message || '加载失败' }
    } finally {
      isLoading.value = false
    }
  }

  const sendMessage = async (
    content: string,
    emotionTag?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
  ): Promise<returnMsgType> => {
    if (!currentSessionId.value || isSending.value) {
      return { isOk: false, msg: '无法发送消息' }
    }

    isSending.value = true

    try {
      const params: SendMessageParams = {
        content,
        contentType: 'text'
      }
      if (emotionTag) params.emotionTag = emotionTag

      const message = await post<MessageWithStatus>(
        `/conversations/${currentSessionId.value}/messages`,
        params
      )

      currentMessages.value.push({
        ...message,
        status: 'sent'
      })

      const conversationStore = useConversationStore()
      const conversation = conversationStore.conversationList.find(
        (c) => c.id === currentSessionId.value
      )
      if (conversation) {
        conversation.lastMessage = content.slice(0, 100)
        conversation.lastMessageTime = Date.now()
        conversation.updatedAt = new Date().toISOString()
      }

      return { isOk: true, msg: '发送成功' }
    } catch (error: any) {
      return { isOk: false, msg: error.message || '发送失败' }
    } finally {
      isSending.value = false
    }
  }

  const reset = () => {
    currentSessionId.value = null
    currentMessages.value = []
    isLoading.value = false
    hasMore.value = true
    isSending.value = false
    messageCursor.value = 0
  }

  const updateMessage = (messageId: string, updates: Partial<MessageWithStatus>) => {
    const idx = currentMessages.value.findIndex((m) => m.id === messageId)
    if (idx !== -1) {
      currentMessages.value[idx] = { ...currentMessages.value[idx], ...updates }
    }
  }

  const addMessage = (message: MessageWithStatus) => {
    currentMessages.value.push(message)
  }

  const removeMessage = (messageId: string) => {
    const idx = currentMessages.value.findIndex((m) => m.id === messageId)
    if (idx !== -1) {
      currentMessages.value.splice(idx, 1)
    }
  }

  return {
    currentSessionId,
    currentMessages,
    isLoading,
    hasMore,
    isSending,
    sortedMessages,
    switchSession,
    loadMoreMessages,
    sendMessage,
    reset,
    updateMessage,
    addMessage,
    removeMessage
  }
})
