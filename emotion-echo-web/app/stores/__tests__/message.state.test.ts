import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useMessageStore, canTransition, MESSAGE_STATUS_TRANSITIONS } from '~/stores/message'
import { getFailedMessages, getLatestMessages } from '~/utils/messageCache'
import { getDb } from '~/utils/db'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'

// CL-05 Phase 3 · T-14 / T-15：消息状态机 + 乐观插入。
//
// 现状（2026-10-14 亲核）：`sendMessage` 在**服务端返回之后**才把消息塞进列表并直接写
// `status:'sent'` ⇒ ① 用户点发送到看到自己那句话之间有网络延迟的空窗（无乐观插入）；
// ② `'sending'` 这个状态**全前端从未被赋值**（枚举白写）；③ 没有任何状态转移表/守卫。
// 本钉锁三件事：
//   · T-14a 点发送**立刻**插入一条 `status:'sending'` 的占位（temp id = clientMsgId）；
//   · T-14b 服务端返回后用**真实 id** 替换占位并置 `'sent'`（列表不重复）；
//   · T-14c 发送**失败**时占位保留为 `'failed'`（不静默丢弃）并落缓存 —— 这是 Phase 4
//          「断网重发」能工作的**前置条件**（列表里没有这条，就没有东西可重试）。
//   · T-15 转移表：`sending/streaming → sent|failed`、`failed → sending`；`sent`/`truncated`
//          为终态；其余转移一律拒绝。
//
// 依据：plan §2 T-14·T-15 / §3 Phase 3；来源计划 D-D（状态集与转移）。
// IndexedDB 由 fake-indexeddb 垫片提供。

const getMock = vi.fn()
const postMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  get: (...args: unknown[]) => getMock(...args),
  post: (...args: unknown[]) => postMock(...args),
  put: vi.fn(),
  patch: vi.fn(),
  del: vi.fn(),
}))

const S = 'c1'

function serverMsg(id: string, sendTime: number, content: string) {
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

function statusOf(store: ReturnType<typeof useMessageStore>, id: string): MessageStatus | undefined {
  return store.currentMessages.find((m) => String(m.id) === id)?.status as MessageStatus | undefined
}

describe('消息状态机 + 乐观插入（CL-05 Phase 3 · T-14 / T-15）', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    getMock.mockReset()
    postMock.mockReset()
    await (await getDb()).messages.clear()
  })

  describe('T-15 转移表（纯函数）', () => {
    it('合法边通过', () => {
      expect(canTransition('sending', 'sent')).toBe(true)
      expect(canTransition('sending', 'failed')).toBe(true)
      expect(canTransition('streaming', 'sent')).toBe(true)
      expect(canTransition('streaming', 'failed')).toBe(true)
      expect(canTransition('failed', 'sending')).toBe(true)
    })

    it('终态无出边（sent / truncated）', () => {
      expect(MESSAGE_STATUS_TRANSITIONS.sent).toEqual([])
      expect(MESSAGE_STATUS_TRANSITIONS.truncated).toEqual([])
      expect(canTransition('sent', 'sending')).toBe(false)
      expect(canTransition('sent', 'failed')).toBe(false)
      expect(canTransition('truncated', 'sending')).toBe(false)
    })

    it('非法转移一律拒绝（自转移 / failed 直跳 sent / sending 直跳 streaming）', () => {
      expect(canTransition('sending', 'sending')).toBe(false)
      expect(canTransition('failed', 'sent')).toBe(false)
      expect(canTransition('sending', 'streaming')).toBe(false)
    })
  })

  describe('T-14 乐观插入', () => {
    it('点发送立刻出现 sending 占位；服务端返回后换成真实 id 并置 sent（不重复）', async () => {
      // 在 "post 被调用的那一刻" 采样（而不是自己造 deferred + 猜时机：
      // 乐观插入后还 await 了一次缓存写入，手动 resolve 会与调用点竞争 ⇒ 死等到超时）
      let idsAtPost: string[] = []
      let statusAtPost: string | undefined
      postMock.mockImplementation(async () => {
        idsAtPost = store.currentMessages.map((m) => String(m.id))
        statusAtPost = statusOf(store, 'cid-1')
        return serverMsg('42', 4200, '你好')
      })

      const store = useMessageStore()
      store.currentSessionId = S
      await store.sendMessage('你好', undefined, 'cid-1')

      expect(idsAtPost, 'post 发出时占位必须已在列表里（乐观插入）').toEqual(['cid-1'])
      expect(statusAtPost, '占位状态必须是 sending（而非 sent）').toBe('sending')

      expect(store.currentMessages.map((m) => String(m.id)), '占位被真实 id 替换').toEqual(['42'])
      expect(statusOf(store, '42')).toBe('sent')
    })

    it('成功发送后缓存里不得残留临时占位行（幽灵 sending 会被重发扫描反复重发）', async () => {
      // 🩸 钉住 2026-10-14 Phase 3 落地后由全量回归抓出的真缺陷：占位行的 temp id
      // 与服务端真身 id 不同，只在内存里替换、不删缓存 ⇒ 缓存里留一条永远 sending
      // 的幽灵行（刷新后可见 + Phase 4 待重发扫描会把它当在途消息反复重发）。
      postMock.mockResolvedValue(serverMsg('42', 4200, 'hi'))
      const store = useMessageStore()
      store.currentSessionId = S

      await store.sendMessage('hi', undefined, 'cid-1')

      const cached = await getLatestMessages(S, 10)
      expect(cached.map((m) => String(m.id)), '缓存里只应有服务端真身行').toEqual(['42'])
    })

    it('未传 clientMsgId 时也乐观插入（自动临时 id），返回后替换为真实 id', async () => {
      postMock.mockResolvedValue(serverMsg('7', 700, 'auto'))
      const store = useMessageStore()
      store.currentSessionId = S

      const inflight = store.sendMessage('auto')
      const placeholder = store.currentMessages.find((m) => m.status === 'sending')
      expect(placeholder, '应有一个 sending 占位（临时 id 自动生成）').toBeTruthy()
      expect(String(placeholder!.id), '临时 id 不应等于服务端真身 id').not.toBe('7')

      await inflight
      expect(store.currentMessages.map((m) => String(m.id))).toEqual(['7'])
    })

    it('发送失败：占位保留为 failed（不静默丢弃）且落缓存，供 Phase 4 重发', async () => {
      postMock.mockRejectedValue(new Error('network down'))
      const store = useMessageStore()
      store.currentSessionId = S

      const res = await store.sendMessage('发不出去', undefined, 'cid-9')

      expect(res.isOk).toBe(false)
      expect(
        store.currentMessages.map((m) => String(m.id)),
        '失败的行必须留在列表里（否则没有东西可以重试）',
      ).toEqual(['cid-9'])
      expect(statusOf(store, 'cid-9')).toBe('failed')

      const failed = await getFailedMessages(S)
      expect(failed.map((m) => String(m.id)), '失败行要落缓存（刷新后仍可重发）').toEqual(['cid-9'])
    })
  })

  describe('T-15 store 层转移守卫', () => {
    it('已 sent 的消息不允许再转回 sending（终态保护）', async () => {
      postMock.mockResolvedValue(serverMsg('5', 500, 'x'))
      const store = useMessageStore()
      store.currentSessionId = S
      await store.sendMessage('x', undefined, 'cid-5')

      expect(store.transitMessage('5', 'sending'), 'sent → sending 必须被拒').toBe(false)
      expect(statusOf(store, '5')).toBe('sent')
    })

    it('failed 可以转回 sending（重试路径）', async () => {
      postMock.mockRejectedValue(new Error('down'))
      const store = useMessageStore()
      store.currentSessionId = S
      await store.sendMessage('retry-me', undefined, 'cid-11')
      expect(statusOf(store, 'cid-11')).toBe('failed')

      expect(store.transitMessage('cid-11', 'sending'), 'failed → sending 应被允许').toBe(true)
      expect(statusOf(store, 'cid-11')).toBe('sending')
    })
  })
})
