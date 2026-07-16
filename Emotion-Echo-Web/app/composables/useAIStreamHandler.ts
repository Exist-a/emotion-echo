/**
 * AI 流式响应处理 Composable
 * 处理 SSE 流式响应，解析 Server-Sent Events
 */
import type { StreamChunk } from '~/types/api'

export interface AIStreamParams {
  message: string
  emotion: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
  conversationId?: string
  shouldGenerateTitle?: boolean
  voiceEmotion?: string
}

export interface AIStreamCallbacks {
  onStart?: (data: { conversationId?: string; userMessageId?: string }) => void
  onDelta?: (content: string) => void
  onFinish?: (data: { messageId: string; emotion?: string }) => void
  onError?: (error: string) => void
  onTruncated?: (data: { content: string }) => void
  onTitleUpdated?: (data: { conversationId: string; title: string }) => void
}

export interface UseAIStreamHandlerReturn {
  isStreaming: Ref<boolean>
  streamingContent: Ref<string>
  sendAIStream: (params: AIStreamParams, callbacks?: AIStreamCallbacks) => Promise<{ isOk: boolean; msg: string }>
  cancelAIStream: () => void
}

export function useAIStreamHandler(): UseAIStreamHandlerReturn {
  const isStreaming = ref(false)
  const streamingContent = ref('')

  let streamAbortController: AbortController | null = null
  let streamCancelled = ref(false)
  let parseErrorCount = 0
  const MAX_PARSE_ERRORS = 5

  const cancelAIStream = () => {
    if (streamAbortController) {
      streamCancelled.value = true
      streamAbortController.abort()
      streamAbortController = null
      isStreaming.value = false
    }
  }

  const sendAIStream = async (
    params: AIStreamParams,
    callbacks: AIStreamCallbacks = {}
  ): Promise<{ isOk: boolean; msg: string }> => {
    if (isStreaming.value) {
      return { isOk: false, msg: '正在对话中' }
    }

    isStreaming.value = true
    streamingContent.value = ''
    streamCancelled.value = false
    parseErrorCount = 0

    const runtimeConfig = useRuntimeConfig()
    const token = import.meta.client ? localStorage.getItem('access_token') : ''
    const streamUrl = `${runtimeConfig.public.API_BASE_URL || 'http://localhost:8080/api/v1'}/ai/stream`

    streamAbortController = new AbortController()

    try {
      const response = await fetch(streamUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: token ? `Bearer ${token}` : ''
        },
        body: JSON.stringify(params),
        credentials: 'include',
        signal: streamAbortController.signal
      })

      if (!response.ok) {
        let errMsg = `HTTP ${response.status}`
        try {
          const errBody = await response.json()
          errMsg = errBody.message || errMsg
        } catch {
        }
        throw new Error(`请求失败: ${errMsg}`)
      }

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('无法读取响应')
      }

      const decoder = new TextDecoder()
      let buffer = ''
      let fullContent = ''
      let aiMessageId: string | null = null

      try {
        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })

          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            const trimmed = line.trim()
            if (!trimmed) continue

            if (trimmed.startsWith('data: ')) {
              try {
                const rawData = trimmed.slice(6).trim()
                const data: StreamChunk = JSON.parse(rawData)

                switch (data.type) {
                  case 'start':
                    if (data.conversationId) {
                      callbacks.onStart?.({ conversationId: data.conversationId })
                    }
                    if (data.userMessageId) {
                      callbacks.onStart?.({ userMessageId: data.userMessageId })
                    }
                    break

                  case 'delta':
                    if (data.content) {
                      fullContent += data.content
                      streamingContent.value = fullContent
                      callbacks.onDelta?.(data.content)
                    }
                    break

                  case 'finish':
                    if (data.messageId) {
                      aiMessageId = data.messageId
                    }
                    callbacks.onFinish?.({
                      messageId: aiMessageId || '',
                      emotion: data.emotion
                    })
                    break

                  case 'error':
                    if (streamCancelled.value) {
                      return { isOk: true, msg: '已取消' }
                    }
                    const errMsg = data.error || data.message || '流式响应错误'
                    callbacks.onError?.(errMsg)
                    return { isOk: false, msg: errMsg }

                  case 'truncated':
                    callbacks.onTruncated?.({ content: data.content || fullContent })
                    return { isOk: true, msg: '对话已截断' }

                  case 'title_updated':
                    if (data.conversationId && data.title) {
                      callbacks.onTitleUpdated?.({
                        conversationId: data.conversationId,
                        title: data.title
                      })
                    }
                    break
                }
              } catch (e: any) {
                parseErrorCount++
                if (parseErrorCount >= MAX_PARSE_ERRORS) {
                  callbacks.onError?.(`数据解析错误过多，已停止`)
                  return { isOk: false, msg: '数据解析错误' }
                }
              }
            }
          }
        }
      } finally {
        reader.releaseLock()
      }

      return { isOk: true, msg: '对话完成' }
    } catch (error: any) {
      if (streamCancelled.value) {
        return { isOk: true, msg: '已取消' }
      }
      callbacks.onError?.(error.message || '对话失败')
      return { isOk: false, msg: error.message || '对话失败' }
    } finally {
      isStreaming.value = false
      streamAbortController = null
      parseErrorCount = 0
    }
  }

  return {
    isStreaming,
    streamingContent,
    sendAIStream,
    cancelAIStream
  }
}
