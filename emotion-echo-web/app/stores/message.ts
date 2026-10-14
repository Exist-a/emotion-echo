import type { MessageWithStatus, SendMessageParams } from '~/types/api'
import type { returnMsgType } from '~/types/commonType'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'
import { get, post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import { getLatestMessages, saveMessage, saveMessages, deleteMessage } from '~/utils/messageCache'
import { useConversationStore } from './conversation'

// 消息状态类型已收敛到单一来源（CL-05 Phase 1 · T-13）；
// 此处仅作转出，保留既有导入路径不破。
export type { MessageStatus } from '~/types/conversation/conversationMessagesType'

/** 进入会话时先渲染的本地快照条数上限（读加速用，不是"全部历史"） */
const SNAPSHOT_LIMIT = 50

/** 只存在于本地、**不允许被服务端快照抹掉**的状态（发失败待重发 / 在途） */
const LOCAL_ONLY_STATUSES: readonly MessageStatus[] = ['failed', 'sending']

/**
 * 合并策略 A —— 服务端**快照替换**（`switchSession` 用）：服务端行覆盖同 id；
 * 本地独有的 `failed`/`sending` 保留，否则"发失败等重发"的消息一刷新就消失（T-12）。
 */
export function mergeServerMessages(
  local: MessageWithStatus[],
  server: MessageWithStatus[],
): MessageWithStatus[] {
  const serverIds = new Set(server.map((m) => m.id))
  const preserved = local.filter(
    (m) => !serverIds.has(m.id) && LOCAL_ONLY_STATUSES.includes(m.status as MessageStatus),
  )
  return [...preserved, ...server]
}

/**
 * 合并策略 B —— 分页**追加**（`loadMoreMessages` 用）：按 id 去重后追加，同 id 以服务端为准。
 * ⚠️ 不能复用 A：第 2 页里没有第 1 页的行，用 A 会把已加载的旧页整片丢掉。
 */
export function appendServerPage(
  existing: MessageWithStatus[],
  page: MessageWithStatus[],
): MessageWithStatus[] {
  const byId = new Map(existing.map((m) => [m.id, m]))
  for (const m of page) byId.set(m.id, m)
  return [...byId.values()]
}

/**
 * 缓存写入适配：`messageCache` 入参类型是 `conversationMessageType`，而服务端行是
 * `types/api.MessageWithStatus` —— 两者字段名与取值集本就不一致（`sender:'user'|'ai'`
 * vs `'user'|'AI'`、`contentType` 取值集不同）。这正是 **`CL-F-13` / `CL-D-12`** 的现场；
 * 用户已裁定**顺延**（`CL-D-13` 到 Phase 3、`CL-D-12` 只登记不动）⇒ 此处先用断言过渡。
 */
function toCacheRow(m: MessageWithStatus) {
  return m as unknown as Parameters<typeof saveMessage>[1]
}

/**
 * 消息状态**转移表**（CL-05 Phase 3 · T-15，单一来源）。
 *
 * - `sending`（已发出待确认）→ `sent` | `failed`
 * - `streaming`（AI 流式进行中）→ `sent` | `failed`
 * - `failed`（可重试）→ `sending`（重试触发）
 * - `sent` / `truncated` = **终态**，无出边 —— 禁止"已发出的消息被回退成 sending"
 *   这类脏状态（曾让状态机名不副实）。
 */
export const MESSAGE_STATUS_TRANSITIONS: Record<MessageStatus, readonly MessageStatus[]> = {
  sending: ['sent', 'failed'],
  streaming: ['sent', 'failed'],
  failed: ['sending'],
  sent: [],
  truncated: [],
}

/** 该状态转移是否被允许（终态/非法转移返回 false） */
export function canTransition(from: MessageStatus, to: MessageStatus): boolean {
  return (MESSAGE_STATUS_TRANSITIONS[from] ?? []).includes(to)
}

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

    // 读加速（T-11）：先用本地快照把界面渲染出来，再向服务端取数。
    // 缓存是**加速而非依赖** ⇒ 读失败只告警，绝不能挡住服务端取数。
    try {
      const snapshot = (await getLatestMessages(
        sessionId,
        SNAPSHOT_LIMIT,
      )) as unknown as MessageWithStatus[]
      // await 期间会话可能已被切走 ⇒ 别把旧会话的快照写进新会话
      if (currentSessionId.value !== sessionId) return
      if (snapshot.length > 0) {
        currentMessages.value = mergeServerMessages([], snapshot)
      }
    } catch (e) {
      console.warn('[MESSAGE_STORE] 本地快照读取失败（降级为纯服务端）:', e)
    }

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
      }>(API_ROUTES.messagesByConv.path.replace(':id', currentSessionId.value || ''), params)

      if (data.list.length > 0) {
        const newMessages = data.list
          // 语音消息落库映射（E2E-16 测试点 #5，2026-09-22 用户实测钉出）：
          // 后端行 content=音频URL、无 audioUrl 字段，而气泡渲染条件是 item.audioUrl
          // （[id].vue v-if）—— 不映射的话即使落库成功，刷新后语音气泡照样消失。
          .map((m) => {
            const row = m as any
            return row.contentType === 'audio' && !row.audioUrl && row.content
              ? { ...m, audioUrl: row.content }
              : m
          })
        // 合并策略 B（分页追加）：按 id 去重，同 id 以服务端为准
        currentMessages.value = appendServerPage(currentMessages.value, newMessages)
        messageCursor.value = data.cursor
        hasMore.value = data.hasMore

        // 写穿（T-11）：拉到的页落缓存。缓存失败不得影响本次加载。
        try {
          await saveMessages(
            currentSessionId.value,
            newMessages.map(toCacheRow) as Parameters<typeof saveMessages>[1],
          )
        } catch (e) {
          console.warn('[MESSAGE_STORE] 缓存写入失败（不影响加载）:', e)
        }
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

  /** 写穿（T-11）：发送成功的消息落缓存；缓存失败不得影响发送结果与返回体 */
  const writeThrough = async (m: MessageWithStatus) => {
    try {
      await saveMessage(currentSessionId.value as string, toCacheRow(m))
    } catch (e) {
      console.warn('[MESSAGE_STORE] 缓存写入失败（不影响发送）:', e)
    }
  }

  /**
   * 状态转移（T-15）：**非法转移一律拒绝**并告警，返回是否真的发生了转移。
   * 与 `MESSAGE_STATUS_TRANSITIONS` 是同一套规则，避免散落的 `status = '...'` 赋值绕过守卫。
   */
  const transitMessage = (messageId: string, to: MessageStatus): boolean => {
    const idx = currentMessages.value.findIndex((m) => String(m.id) === String(messageId))
    if (idx === -1) return false
    const current = currentMessages.value[idx]!
    // 缺省当作 sent：消息一旦落过服务端就有终态语义，不该被随意回退
    const from = (current.status ?? 'sent') as MessageStatus
    if (!canTransition(from, to)) {
      console.warn(`[MESSAGE_STORE] 非法状态转移被拒：${from} → ${to}（id=${messageId}）`)
      return false
    }
    currentMessages.value[idx] = { ...current, status: to } as MessageWithStatus
    return true
  }

  const sendMessage = async (
    content: string,
    emotionTag?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral',
    clientMsgId?: string,
    // Stage 79: 文件消息（image/file/video）透传；缺省 text 保持向后兼容
    contentType?: SendMessageParams['contentType'],
    // Stage 89 PR-5: 文件消息原始文件名（contentType=file 时透传 chat-svc file_name）
    fileName?: string,
  ): Promise<returnMsgType> => {
    if (!currentSessionId.value || isSending.value) {
      return { isOk: false, msg: '无法发送消息' }
    }

    const sessionId = currentSessionId.value as string
    isSending.value = true

    // 乐观插入（T-14a）：先插一条 `sending` 占位，让用户**立刻**看到自己那句话，
    // 不必等网络往返。临时 id 优先用 `clientMsgId` —— 它同时是后端的幂等键，
    // 同值便于"服务端返回后按 id 就地替换"。
    const tempId = clientMsgId || `local-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
    const optimistic = {
      id: tempId,
      conversationId: sessionId,
      sender: 'user',
      content,
      contentType: contentType || 'text',
      sendTime: Date.now(),
      createdAt: Date.now(),
      status: 'sending',
      ...(emotionTag ? { emotionTag } : {}),
      ...(fileName ? { fileName } : {}),
    } as unknown as MessageWithStatus
    currentMessages.value = [...currentMessages.value, optimistic]
    await writeThrough(optimistic)

    try {
      const params: SendMessageParams = {
        content,
        contentType: contentType || 'text',
      }
      if (emotionTag) params.emotionTag = emotionTag
      if (clientMsgId) params.clientMsgId = clientMsgId
      if (fileName) params.fileName = fileName

      const message = await post<MessageWithStatus>(
        API_ROUTES.sendMessage.path.replace(':id', sessionId),
        params,
      )

      // 服务端真身**就地替换**乐观占位：换真实 id、置终态 sent。
      // （原"幂等命中则 update 而非 push"的分支已被此路径覆盖：占位 id 就是 clientMsgId。）
      const landed = { ...message, status: 'sent' } as MessageWithStatus
      const idx = currentMessages.value.findIndex((m) => String(m.id) === String(tempId))
      if (idx >= 0) {
        currentMessages.value[idx] = landed
      } else {
        currentMessages.value = [...currentMessages.value, landed]
      }
      // 临时占位行必须**从缓存里删掉**：否则刷新后会看到一条永远 sending 的幽灵行，
      // 且 Phase 4 的"待重发扫描"（找 sending/failed）会把它当成在途消息反复重发。
      // 🩸 这条是 2026-10-14 Phase 3 落地后由**全量回归**（Phase 2b 的写穿钉）抓出来的。
      if (String(tempId) !== String(landed.id)) {
        try {
          await deleteMessage(tempId)
        } catch (e) {
          console.warn('[MESSAGE_STORE] 清理乐观占位缓存失败（不影响发送）:', e)
        }
      }
      await writeThrough(landed)

      const conversationStore = useConversationStore()
      const conversation = conversationStore.conversationList.find((c) => c.id === sessionId)
      if (conversation) {
        conversation.lastMessage = content.slice(0, 100)
        conversation.lastMessageTime = Date.now()
        conversation.updatedAt = new Date().toISOString()
      }

      return { isOk: true, msg: '发送成功', data: message }
    } catch (error: any) {
      // 失败：占位**保留**并置 failed，**不静默丢弃** —— 列表里没有这条，
      // Phase 4 的「断网重发」就无从重试（这是重试链路的前置条件）。
      // 走守卫而非直接赋值：若占位已被别处置为终态，则保持原状。
      const idx = currentMessages.value.findIndex((m) => String(m.id) === String(tempId))
      if (idx >= 0) {
        const current = currentMessages.value[idx]!
        if (canTransition((current.status ?? 'sending') as MessageStatus, 'failed')) {
          const failed = { ...current, status: 'failed' } as MessageWithStatus
          currentMessages.value[idx] = failed
          await writeThrough(failed)
        }
      }
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
      const existing = currentMessages.value[idx]
      if (existing) {
        currentMessages.value[idx] = { ...existing, ...updates }
      }
    }
  }

  const addMessage = (message: MessageWithStatus) => {
    // Sprint 110 · A8 修复: 用 spread 赋值触发 ref 引用变化 (保险),
    // 比 .push() 在某些 Vue 3 reactive proxy 配置下更稳定触发响应式
    currentMessages.value = [...currentMessages.value, message]
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
    transitMessage,
    reset,
    updateMessage,
    addMessage,
    removeMessage,
  }
})
