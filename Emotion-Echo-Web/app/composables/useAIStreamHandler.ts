/**
 * AI 流式响应处理 Composable
 * 解析 OpenAI 兼容 SSE 格式：
 *   data: {"choices":[{"delta":{"content":"..."}}]}\n\n
 *   data: [DONE]\n\n
 *
 * 历史：Stage 33 之前按 data.type === 'start'|'delta'|'finish' 解析，
 *       与 BFF 实际 OpenAI 输出格式不匹配，导致用户看到永远"streaming"。
 *       PR-17 改为 OpenAI 兼容解析。
 */
export interface AIStreamParams {
  message: string
  emotion: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
  conversationId?: string
  shouldGenerateTitle?: boolean
  voiceEmotion?: string
}

export interface AIStreamCallbacks {
  onDelta?: (content: string) => void
  onFinish?: (data: { messageId?: string; emotion?: string }) => void
  onError?: (error: string) => void
}

export interface UseAIStreamHandlerReturn {
  isStreaming: Ref<boolean>
  streamingContent: Ref<string>
  sendAIStream: (params: AIStreamParams, callbacks?: AIStreamCallbacks) => Promise<{ isOk: boolean; msg: string }>
  cancelAIStream: () => void
}

const SSE_DONE_SENTINEL = '[DONE]'
const MAX_PARSE_ERRORS = 5

interface OpenAIDeltaPayload {
  choices?: Array<{
    delta?: {
      content?: string
    }
  }>
}

export function useAIStreamHandler(): UseAIStreamHandlerReturn {
  const isStreaming = ref(false)
  const streamingContent = ref('')

  let streamAbortController: AbortController | null = null
  let streamCancelled = ref(false)
  let parseErrorCount = 0
  let finished = false

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
    finished = false

    const runtimeConfig = useRuntimeConfig()
    const token = import.meta.client ? localStorage.getItem('access_token') : ''
    const streamUrl = `${runtimeConfig.public.API_BASE_URL || 'http://localhost:8894/api/v1'}/ai/stream`

    streamAbortController = new AbortController()

    const triggerFinish = (extra?: { messageId?: string; emotion?: string }) => {
      if (finished) return
      finished = true
      callbacks.onFinish?.(extra ?? {})
    }

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
          /* ignore non-JSON error body */
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

      try {
        while (!finished) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })

          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (finished) break
            const trimmed = line.trim()
            if (!trimmed) continue

            if (!trimmed.startsWith('data:')) continue
            const rawData = trimmed.slice(5).trim()
            if (!rawData) continue

            if (rawData === SSE_DONE_SENTINEL) {
              triggerFinish()
              continue
            }

            let payload: OpenAIDeltaPayload
            try {
              payload = JSON.parse(rawData) as OpenAIDeltaPayload
            } catch {
              parseErrorCount++
              if (parseErrorCount >= MAX_PARSE_ERRORS) {
                callbacks.onError?.('数据解析错误过多，已停止')
                return { isOk: false, msg: '数据解析错误' }
              }
              continue
            }

            const deltaContent = payload?.choices?.[0]?.delta?.content
            if (typeof deltaContent === 'string' && deltaContent.length > 0) {
              fullContent += deltaContent
              streamingContent.value = fullContent
              callbacks.onDelta?.(deltaContent)
            }
          }
        }
      } finally {
        reader.releaseLock()
      }

      triggerFinish()
      return { isOk: true, msg: '对话完成' }
    } catch (error: any) {
      if (streamCancelled.value || error?.name === 'AbortError') {
        return { isOk: true, msg: '已取消' }
      }
      const errMsg = error?.message || '对话失败'
      callbacks.onError?.(errMsg)
      return { isOk: false, msg: errMsg }
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
