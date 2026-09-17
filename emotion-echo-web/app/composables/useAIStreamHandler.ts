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
import { API_ROUTES } from '~/lib/apiRoutes'
import { getApiBaseUrl } from '../lib/apiBaseUrl'
import { getClientAccessToken } from '~/lib/clientAccessToken'
export interface AIStreamParams {
  message: string
  emotion: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'
  conversationId?: string
  messageId?: string
  clientMsgId?: string
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
  sendAIStream: (
    params: AIStreamParams,
    callbacks?: AIStreamCallbacks,
  ) => Promise<{ isOk: boolean; msg: string }>
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
  // Sprint 108 · 跨组件实例共享状态 (Stage 107 架构债修复)
  //
  // Bug: new.vue handleSubmit 调 createNewConversation → await navigateTo 触发 new.vue unmount →
  //      composable 实例销毁 → streamAbortController / isStreaming 等局部变量丢失 →
  //      sendAIStream 的 fetch 状态不可见, SSE 流的 onDelta 回调 consumer 丢失.
  //
  // Fix: 响应式状态用 useState() (Nuxt 3 SSR-safe 跨实例 singleton);
  //      非响应式 controller 用 module-scope let (Nuxt SPA 客户端模块单例).
  const isStreaming = useState<boolean>('ai-stream:isStreaming', () => false)
  const streamingContent = useState<string>('ai-stream:streamingContent', () => '')

  // Module-scope singleton (SPA 客户端有效; SSR 每次请求独立 module 实例, 安全)
  // 跨组件实例共享 AbortController, 避免 unmount 后无法 cancel 在飞请求
  let streamAbortController: AbortController | null = null
  const streamCancelled = useState<boolean>('ai-stream:streamCancelled', () => false)
  const parseErrorCountState = useState<number>('ai-stream:parseErrorCount', () => 0)
  const finishedState = useState<boolean>('ai-stream:finished', () => false)
  // P1-R2-1: 跨 stream 调用累计"已发射给 UI 的内容"
  // 用法：SSE 重连（401 refresh）后，server 返回的 delta 与本变量前 N 字符
  //      相同则跳过（client 已有），仅 emit 真正新增部分
  const emittedContentState = useState<string>('ai-stream:emittedContent', () => '')

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
    callbacks?: AIStreamCallbacks,
  ): Promise<{ isOk: boolean; msg: string }> => {
    if (isStreaming.value) {
      return { isOk: false, msg: '正在对话中' }
    }

    isStreaming.value = true
    streamingContent.value = ''
    streamCancelled.value = false
    parseErrorCountState.value = 0
    finishedState.value = false
    // P1-R2-1: 记录"已发射内容"用于 401 重连后的去重（避免重复 emit）
    emittedContentState.value = ''

    const runtimeConfig = useRuntimeConfig()
    // Sprint 111 · R-09 修复: HttpOnly cookie 浏览器 JS 读不到, 必须用 helper
    // (userStore → cookie fallback). 之前 useCookie().value 永远空 → 401.
    const token = getClientAccessToken()
    // PR-A: 改用 fail-fast helper（决策 18 #24）；不再静默回退到 8894
    // 计算 streamUrl 时若 API_BASE_URL 漏配 → 抛错 → 进 catch 返回 isOk=false
    let streamUrl: string
    try {
      streamUrl = `${getApiBaseUrl(runtimeConfig)}${API_ROUTES.aiStream.path}`
    } catch (e: any) {
      callbacks?.onError?.(e?.message || 'API_BASE_URL 未配置')
      return { isOk: false, msg: e?.message || 'API_BASE_URL 未配置' }
    }

    streamAbortController = new AbortController()

    const triggerFinish = (extra?: { messageId?: string; emotion?: string }) => {
      if (finishedState.value) return
      finishedState.value = true
      callbacks?.onFinish?.(extra ?? {})
    }

    try {
      const response = await fetch(streamUrl, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Authorization: token ? `Bearer ${token}` : '',
        },
        body: JSON.stringify(params),
        credentials: 'include',
        signal: streamAbortController.signal,
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
        while (!finishedState.value) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })

          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          for (const line of lines) {
            if (finishedState.value) break
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
              parseErrorCountState.value++
              if (parseErrorCountState.value >= MAX_PARSE_ERRORS) {
                callbacks?.onError?.('数据解析错误过多，已停止')
                return { isOk: false, msg: '数据解析错误' }
              }
              continue
            }

            const deltaContent = payload?.choices?.[0]?.delta?.content
            if (typeof deltaContent === 'string' && deltaContent.length > 0) {
              // P1-R2-1: dedup — 401 重连后 server 整段从头推，
              // 跳过本轮已 emit 过的前缀，仅 emit 真正新增的 chunk
              let toEmit = deltaContent
              if (emittedContentState.value && deltaContent.startsWith(emittedContentState.value)) {
                toEmit = deltaContent.slice(emittedContentState.value.length)
                if (!toEmit) continue // 完全重复，跳过
              }
              fullContent += toEmit
              streamingContent.value = fullContent
              emittedContentState.value += toEmit
              callbacks?.onDelta?.(toEmit)
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
      callbacks?.onError?.(errMsg)
      return { isOk: false, msg: errMsg }
    } finally {
      isStreaming.value = false
      streamAbortController = null
      parseErrorCountState.value = 0
      // Sprint 110 · A8 修复: 显式清 streamCancelled
      // 否则上一次 cancelled=true 残留, 下次 sendAIStream catch 分支会判定为 cancelled,
      // 返回 '已取消' 但实际 fetch 已正常返回 → UI 永远看不到 AI 回复
      streamCancelled.value = false
    }
  }

  return {
    isStreaming,
    streamingContent,
    sendAIStream,
    cancelAIStream,
  }
}
