import type { MessageWithStatus, SendMessageParams } from '~/types/api'
import type { returnMsgType } from '~/types/commonType'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'
import { get, post } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'
import {
  getLatestMessages,
  getAllFailedMessages,
  saveMessage,
  saveMessages,
  deleteMessage,
} from '~/utils/messageCache'
import { useConversationStore } from './conversation'
import { useUserStore } from './user'

// 消息状态类型已收敛到单一来源（CL-05 Phase 1 · T-13）；
// 此处仅作转出，保留既有导入路径不破。
export type { MessageStatus } from '~/types/conversation/conversationMessagesType'

/** 进入会话时先渲染的本地快照条数上限（读加速用，不是"全部历史"） */
const SNAPSHOT_LIMIT = 50

/** 只存在于本地、**不允许被服务端快照抹掉**的状态（发失败待重发 / 在途） */
const LOCAL_ONLY_STATUSES: readonly MessageStatus[] = ['failed', 'sending']

/**
 * 当前登录用户 id（CL-F-20）。
 *
 * 本地缓存是按**浏览器**存的（Dexie 库名固定 `EmotionEcho`），不是按账号 ——
 * 写入时打上归属、查询/补发时据此过滤，才能保证换账号后"看不见也补不了"别人的消息。
 * 未登录返回空串。
 */
function currentUserId(): string {
  try {
    return useUserStore().getId || ''
  } catch {
    // store 尚未初始化（极早期调用）—— 视为未登录，宁可不写归属
    return ''
  }
}

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
 * vs `'user'|'AI'`、`contentType` 取值集不同）。这正是 **`CL-F-18` / `CL-D-12`** 的现场；
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

/**
 * 本地消息形态 = 服务端行 + **仅本地有**的队列元数据。
 * `retryCount` 由 Phase 4 引入（来源计划 D-E）；`CL-F-18` 三形态统一已由用户
 * `CL-D-13` 裁定顺延 ⇒ 此处先用交叉类型过渡，收敛时一并处理。
 */
export type StoreMessage = MessageWithStatus & { retryCount?: number }

/** 自动重试上限（来源计划 D-E：MAX=3；超限停自动重试，留手动重发） */
export const MAX_RETRY = 3

/** 退避曲线：`min(2^n × 1000ms, 30s)`，n = 已失败次数 */
export function retryDelayMs(retryCount: number): number {
  return Math.min(2 ** retryCount * 1000, 30_000)
}

/** 该行是否还应被**自动**重试（failed 且未达上限） */
export function canAutoRetry(m: { status?: MessageStatus; retryCount?: number }): boolean {
  return m.status === 'failed' && (m.retryCount ?? 0) < MAX_RETRY
}

/** 重试所需的时间/等待抽象 —— 走注入而非直接 `Date.now()`/`setTimeout`（AGENTS §3.2/§3.3） */
export interface RetryGovernor {
  now(): number
  sleep(ms: number): Promise<void>
}

let retryGovernor: RetryGovernor = {
  now: () => Date.now(),
  sleep: (ms) => new Promise((resolve) => setTimeout(resolve, ms)),
}

/** 注入重试调度实现（**仅测试使用**：让单测不真睡 2/4/8 秒） */
export function setRetryGovernor(governor: RetryGovernor) {
  retryGovernor = governor
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
            currentUserId(),
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
      await saveMessage(currentSessionId.value as string, toCacheRow(m), currentUserId())
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

  /**
   * 投递一条**已在本地占位**的消息（`sendMessage` 与断网补发共用）：
   * POST → 就地替换为服务端真身（真实 id + 终态 `sent`）→ 清掉临时 id 的缓存行 → 真身写穿。
   *
   * ⚠️ 清临时行这步不能省：占位 temp id 与服务端真身 id **不同**，只在内存替换的话
   * 缓存里会留一条永远 `sending` 的**幽灵行**（刷新后可见；且待重发扫描会把它当在途消息
   * **反复重发**）。🩸 该缺陷由 Phase 3 落地后的**全量回归**抓出。
   * 内存里找不到占位（例如补发的是**别的会话**的行）时不触碰内存列表。
   */
  const deliver = async (
    tempId: string,
    payload: SendMessageParams,
    sessionId: string,
  ): Promise<MessageWithStatus> => {
    const message = await post<MessageWithStatus>(
      API_ROUTES.sendMessage.path.replace(':id', sessionId),
      payload,
    )
    const landed = { ...message, status: 'sent' } as MessageWithStatus
    const idx = currentMessages.value.findIndex((m) => String(m.id) === String(tempId))
    if (idx >= 0) currentMessages.value[idx] = landed
    if (String(tempId) !== String(landed.id)) {
      try {
        await deleteMessage(tempId)
      } catch (e) {
        console.warn('[MESSAGE_STORE] 清理乐观占位缓存失败（不影响发送）:', e)
      }
    }
    await saveMessage(sessionId, toCacheRow(landed), currentUserId())
    return message
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

      // 服务端真身**就地替换**乐观占位（换真实 id、置终态 sent、清临时缓存行）
      const message = await deliver(tempId, params, sessionId)

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
          const failed = {
            ...current,
            status: 'failed',
            // T-16：失败即自增 —— 退避与"达上限停自动重试"都依赖它
            retryCount: ((current as StoreMessage).retryCount ?? 0) + 1,
          } as unknown as StoreMessage
          currentMessages.value[idx] = failed
          await writeThrough(failed)
        }
      }
      return { isOk: false, msg: error.message || '发送失败' }
    } finally {
      isSending.value = false
    }
  }

  /**
   * 断网重连重扫（T-18）：把缓存里**全部会话**的 `failed` 行按退避补发。
   *
   * 三个要点：
   * ① 扫**全部会话**而非当前会话 —— 离线队列是会话无关的（用户可能在 A 会话断网、
   *    切到 B 才联网，只扫当前会话会漏发）；
   * ② **复用原行 id 作 `clientMsgId`** ⇒ 命中后端 `client_msg_id` 唯一索引，
   *    重复补发不会重复入库（幂等复用，不重造）；
   * ③ 达 `MAX_RETRY` 的行直接跳过（转手动重发），不退化成"无限重试"。
   *
   * **④ CL-F-20（按用户隔离）**：只扫**当前登录用户自己**的失败行；未登录一律不补发。
   * 同一浏览器换账号后，上一账号留在库里的 `failed` 行必须**看不见**——否则会拿新账号
   * 的令牌去补旧账号的消息（后端唯一索引只保证不重复入库，不保证归属正确）。
   */
  const retryFailedMessages = async (): Promise<{ retried: number; succeeded: number }> => {
    const userId = currentUserId()
    if (!userId) {
      // 没有可信归属 ⇒ 宁可一条都不发，等用户登录后 online 事件/手动触发再补
      console.warn('[MESSAGE_STORE] 未登录，跳过离线补发（无法判定消息归属）')
      return { retried: 0, succeeded: 0 }
    }

    let pending: StoreMessage[] = []
    try {
      pending = (await getAllFailedMessages(userId)) as unknown as StoreMessage[]
    } catch (e) {
      console.warn('[MESSAGE_STORE] 读取待重发队列失败（不影响后续）:', e)
      return { retried: 0, succeeded: 0 }
    }

    const targets = pending.filter((m) => canAutoRetry(m))
    let succeeded = 0

    for (const row of targets) {
      const rowSessionId = (row as { sessionId?: string }).sessionId
      if (!rowSessionId) continue
      const tempId = String(row.id)
      const attempts = (row.retryCount ?? 0) + 1

      // 退避：第 n 次尝试前先等 min(2^n × 1000ms, 30s)
      await retryGovernor.sleep(retryDelayMs(attempts))

      const inCurrentSession = rowSessionId === currentSessionId.value
      if (inCurrentSession) transitMessage(tempId, 'sending')

      try {
        await deliver(
          tempId,
          {
            content: row.content ?? '',
            contentType: row.contentType as SendMessageParams['contentType'],
            clientMsgId: tempId,
          },
          rowSessionId,
        )
        succeeded += 1
      } catch (error: any) {
        const bumped = {
          ...row,
          status: 'failed',
          retryCount: attempts,
        } as unknown as StoreMessage
        try {
          // 保留原归属（CL-F-20）：补发失败后写回时不能让 userId 丢失，
          // 否则这条行下次就"无主"、永远不会被本用户补发。
          const owner = (row as { userId?: string }).userId ?? userId
          await saveMessage(rowSessionId, toCacheRow(bumped), owner)
        } catch (e) {
          console.warn('[MESSAGE_STORE] 重试失败后写缓存失败:', e)
        }
        if (inCurrentSession) {
          const idx = currentMessages.value.findIndex((m) => String(m.id) === tempId)
          if (idx >= 0) currentMessages.value[idx] = bumped
        }
        if (attempts >= MAX_RETRY) {
          console.warn(
            `[MESSAGE_STORE] 已达重试上限（${MAX_RETRY}），停止自动重试（留手动重发）：${tempId}`,
          )
        } else {
          console.warn(
            `[MESSAGE_STORE] 补发失败（第 ${attempts} 次），留待下次联网：${tempId}`,
            error,
          )
        }
      }
    }

    return { retried: targets.length, succeeded }
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
    retryFailedMessages,
    reset,
    updateMessage,
    addMessage,
    removeMessage,
  }
})
