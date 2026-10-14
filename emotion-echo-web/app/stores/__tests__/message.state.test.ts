import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useMessageStore, canTransition, MESSAGE_STATUS_TRANSITIONS } from '~/stores/message'
import { getFailedMessages } from '~/utils/messageCache'
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
      let resolvePost: (v: unknown) => void = () => {}
      postMock.mockImplementation(
        () =>
          new Promise((resolve) => {
            resolvePost = resolve
          }),
      )

      const store = useMessageStore()
      store.currentSessionId = S
      const inflight = store.sendMessage('你好', undefined, 'cid-1')

      // 服务端还没返回：占位必须已经在列表里，且状态是 sending（而非 sent）
      expect(
        store.currentMessages.map((m) => String(m.id)),
        '发送瞬间就该看到自己那条（乐观插入）',
      ).toEqual(['cid-1'])
      expect(statusOf(store, 'cid-1'), '占位状态必须是 sending').toBe('sending')

      resolvePost(serverMsg('42', 4200, '你好'))
      await inflight

      expect(store.currentMessages.map((m) => String(m.id)), '占位被真实 id 替换').toEqual(['42'])
      expect(statusOf(store, '42')).toBe('sent')
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
