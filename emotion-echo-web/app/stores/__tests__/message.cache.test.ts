import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useMessageStore } from '~/stores/message'
import { saveMessage, getLatestMessages, getFailedMessages } from '~/utils/messageCache'
import { getDb } from '~/utils/db'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'

// CL-05 Phase 2b · T-11 / T-12：把本地缓存**接进真实链路**（读加速 + 写穿 + 合并策略）。
//
// 现状（2026-10-14 亲核）：`stores/message.ts` 是**纯内存 Pinia**，`utils/messageCache.ts`
// 全仓零调用 ⇒ 刷新即丢；`switchSession` 也不读本地快照。
// 本钉锁三件事：
//   · **读加速**：`switchSession` 先用本地快照渲染，再向服务端取数（用户不必等网络）；
//   · **写穿**：`loadMoreMessages` 拉到的页 / `sendMessage` 成功的行落缓存；
//   · **合并策略**：服务端覆盖同 id；**本地独有 `failed`/`sending` 不被服务端快照抹掉**
//     （否则"发失败等重发"的消息会一刷新就消失）。
// 另钉一条**降级**：缓存读失败不得挡住服务端取数（缓存是加速，不是依赖）。
//
// 依据：plan §3 Phase 2 ③④ / §2 T-11·T-12。IndexedDB 由 fake-indexeddb 垫片提供。

const getMock = vi.fn()
const postMock = vi.fn()
const getLatestMessagesMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  get: (...args: unknown[]) => getMock(...args),
  post: (...args: unknown[]) => postMock(...args),
  put: vi.fn(),
  patch: vi.fn(),
  del: vi.fn(),
}))

// messageCache 其余函数走真身，只把 getLatestMessages 包一层以便制造"缓存读失败"。
vi.mock('~/utils/messageCache', async (importOriginal) => {
  const actual = await importOriginal<typeof import('~/utils/messageCache')>()
  return {
    ...actual,
    getLatestMessages: (...args: unknown[]) =>
      getLatestMessagesMock(...args) as Promise<never>,
  }
})

const S = 'c1'

function serverMsg(id: string | number, sendTime: number, content: string) {
  return {
    id,
    conversationId: S,
    sender: 'user',
    content,
    contentType: 'text',
    sendTime,
    createdAt: sendTime,
  }
}

function cachedMsg(id: string | number, sendTime: number, content: string, status: MessageStatus) {
  return { id, sender: 'user' as const, sendTime, content, contentType: 'text' as const, status }
}

describe('message store 本地缓存接线（CL-05 Phase 2b · T-11 / T-12）', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    getMock.mockReset()
    postMock.mockReset()
    getLatestMessagesMock.mockReset()
    // 走真身（默认行为就是真实现）
    const actual = await vi.importActual<typeof import('~/utils/messageCache')>(
      '~/utils/messageCache',
    )
    getLatestMessagesMock.mockImplementation(actual.getLatestMessages)
    await (await getDb()).messages.clear()
  })

  it('T-11 读加速：切换会话先用本地快照渲染，再发服务端请求', async () => {
    await saveMessage(S, cachedMsg(1, 1000, 'local-snapshot', 'sent'))

    const store = useMessageStore()
    let lenAtRequest = -1
    getMock.mockImplementation(async () => {
      // 在"发请求的那一刻"采样：若快照没先渲染，这里会是 0
      lenAtRequest = store.currentMessages.length
      return { list: [serverMsg(1, 1000, 'server')], cursor: 0, hasMore: false }
    })

    await store.switchSession(S)

    expect(lenAtRequest, '发请求前应已用本地快照渲染（读加速）').toBe(1)
    expect(store.currentMessages).toHaveLength(1)
    expect(store.currentMessages[0]!.content, '服务端覆盖同 id').toBe('server')
  })

  it('T-12 合并：服务端覆盖同 id，本地独有 failed 不被服务端快照抹掉', async () => {
    await saveMessage(S, cachedMsg(1, 1000, 'stale', 'sent'))
    await saveMessage(S, cachedMsg(99, 1500, '本地失败·等重发', 'failed'))

    const store = useMessageStore()
    getMock.mockResolvedValue({ list: [serverMsg(1, 1000, 'fresh')], cursor: 0, hasMore: false })

    await store.switchSession(S)

    // id 在类型上是 string（api.MessageItem.id），运行时用例里传的是 number
    // ⇒ 统一 String() 归一，避免"类型说 string、跑起来是 number"的断言漂移。
    const byId = new Map(store.currentMessages.map((m) => [String(m.id), m]))
    expect([...byId.keys()].sort(), '本地 failed(99) 与服务端行(1) 都要在').toEqual(['1', '99'])
    expect(byId.get('1')!.content, '服务端同 id 覆盖本地').toBe('fresh')
    expect((byId.get('99') as any).status, '本地 failed 必须保留').toBe('failed')
  })

  it('T-12 合并后仍按 sendTime 升序（本地 failed 插在正确位置）', async () => {
    await saveMessage(S, cachedMsg(99, 500, 'early-failed', 'failed'))
    const store = useMessageStore()
    getMock.mockResolvedValue({
      list: [serverMsg(1, 1000, 'a'), serverMsg(2, 2000, 'b')],
      cursor: 0,
      hasMore: false,
    })

    await store.switchSession(S)

    expect(store.sortedMessages.map((m) => m.id)).toEqual([99, 1, 2])
  })

  it('T-11 写穿：loadMoreMessages 拉到的页写入缓存', async () => {
    const store = useMessageStore()
    store.currentSessionId = S
    getMock.mockResolvedValue({ list: [serverMsg(7, 7000, 'p1')], cursor: 0, hasMore: false })

    await store.loadMoreMessages()

    const cached = await getLatestMessages(S, 10)
    expect(cached.map((m) => m.id)).toEqual([7])
  })

  it('T-11 写穿：sendMessage 成功的行写入缓存', async () => {
    const store = useMessageStore()
    store.currentSessionId = S
    postMock.mockResolvedValue(serverMsg(11, 11000, 'mine'))

    await store.sendMessage('mine')

    const cached = await getLatestMessages(S, 10)
    expect(cached.map((m) => m.id)).toEqual([11])
  })

  it('T-11 降级：缓存读失败不得挡住服务端取数（缓存是加速，不是依赖）', async () => {
    getLatestMessagesMock.mockRejectedValueOnce(new Error('IndexedDB boom'))
    const store = useMessageStore()
    getMock.mockResolvedValue({ list: [serverMsg(3, 3000, 'from-server')], cursor: 0, hasMore: false })

    await store.switchSession(S)

    expect(store.currentMessages.map((m) => m.id)).toEqual([3])
    expect(store.currentMessages[0]!.content).toBe('from-server')
  })

  it('T-11 快照不污染：切到另一个会话时不残留上一会话的消息', async () => {
    await saveMessage(S, cachedMsg(1, 1000, 'sessA', 'sent'))
    await saveMessage('c2', cachedMsg(2, 2000, 'sessB', 'sent'))

    const store = useMessageStore()
    getMock.mockResolvedValue({ list: [], cursor: 0, hasMore: false })

    await store.switchSession(S)
    await store.switchSession('c2')

    expect(store.currentMessages.map((m) => m.id), '不应残留 c1 的快照').toEqual([2])
  })

  it('缓存写入的失败消息可被 getFailedMessages 查出（重试链路的前置条件）', async () => {
    await saveMessage(S, cachedMsg(42, 4200, 'bonk', 'failed'))
    const failed = await getFailedMessages(S)
    expect(failed.map((m) => m.id)).toEqual([42])
  })
})
