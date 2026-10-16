import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useMessageStore, setRetryGovernor } from '~/stores/message'
import { useUserStore } from '~/stores/user'
import { saveMessage, getAllFailedMessages } from '~/utils/messageCache'
import { getDb } from '~/utils/db'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'

// CL-F-20：本地消息缓存**必须按用户隔离**。
//
// 现场（2026-10-14 D 轨核出，账本 CL-F-20）：Dexie 库名固定 `EmotionEcho`，
// 消息行只有 `sessionId`、**没有 userId**；`getAllFailedMessages()` 全库扫描。
// ⇒ 同一浏览器换账号登录时，上一账号的 `failed` 行仍在库里；若在登录完成前
//   自动补发，会**拿新账号的令牌去补旧账号的消息**（后端 `client_msg_id` 唯一索引
//   只保证"不重复入库"，**不保证"归属正确"**）。
//
// 本钉锁三件事：
//   ① 缓存行带 `userId`，查询可按用户过滤；
//   ② 补发**只补当前登录用户自己**的失败行（别的账号的行一律不碰）；
//   ③ **未登录时一律不补发**（没有可信归属，宁可不发）。
//
// 依据：docs/plans/cl-legacy-paydown-2026-10-16.md；账本 CL-F-20；来源计划 D-E / R13。

const postMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  get: vi.fn(),
  post: (...args: unknown[]) => postMock(...args),
  put: vi.fn(),
  patch: vi.fn(),
  del: vi.fn(),
}))

const S = 'sess-1'

function failedRow(id: string, content = '断网时发的') {
  return {
    id,
    sender: 'user' as const,
    sendTime: 1000,
    content,
    contentType: 'text' as const,
    status: 'failed' as MessageStatus,
    retryCount: 0,
  }
}

function serverMsg(id: string, content: string) {
  return {
    id,
    conversationId: S,
    sender: 'user',
    content,
    contentType: 'text',
    sendTime: 2000,
    createdAt: 2000,
  }
}

/** 以某用户身份登录（只用到 getId，故 userInfo.id 足够） */
function loginAs(userId: string) {
  const user = useUserStore()
  user.userInfo = { id: userId, nickname: userId } as any
  user.accessToken = 'fake-token'
}

describe('本地缓存按用户隔离（CL-F-20）', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    postMock.mockReset()
    setRetryGovernor({ now: () => Date.now(), sleep: async () => {} })
    await (await getDb()).messages.clear()
  })

  afterEach(() => {
    setRetryGovernor({
      now: () => Date.now(),
      sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
    })
  })

  describe('缓存层：查询可按 userId 过滤', () => {
    it('两个账号的失败行互不可见', async () => {
      await saveMessage(S, failedRow('a-1', 'A 用户断网消息'), 'uA')
      await saveMessage(S, failedRow('b-1', 'B 用户断网消息'), 'uB')

      const forA = await getAllFailedMessages('uA')
      const forB = await getAllFailedMessages('uB')

      expect(
        forA.map((m) => String(m.id)),
        'A 只该看到自己的',
      ).toEqual(['a-1'])
      expect(
        forB.map((m) => String(m.id)),
        'B 只该看到自己的',
      ).toEqual(['b-1'])
      expect((forA[0] as any).userId, '缓存行应落 userId 供过滤').toBe('uA')
    })

    it('不带 userId 的查询保留旧语义（全量）—— 仅用于未登录/遗留兜底', async () => {
      await saveMessage(S, failedRow('a-1'), 'uA')
      await saveMessage(S, failedRow('b-1'), 'uB')
      const all = await getAllFailedMessages()
      expect(all).toHaveLength(2)
    })
  })

  describe('补发层：只补当前登录用户自己的消息', () => {
    it('换账号后不会拿新账号令牌补旧账号的消息', async () => {
      await saveMessage(S, failedRow('b-1', 'B 用户的消息'), 'uB')

      // 现在登录的是 A —— B 那条待补发行仍在同一浏览器同一库里
      loginAs('uA')
      postMock.mockImplementation(async (_url: string, body: any) =>
        serverMsg(`srv-${body.clientMsgId}`, body.content),
      )

      const store = useMessageStore()
      store.currentSessionId = S
      const res = await store.retryFailedMessages()

      expect(postMock, '绝不能拿 A 的令牌补 B 的消息').not.toHaveBeenCalled()
      expect(res.retried).toBe(0)
      // B 的行仍留在库里等 B 自己登录后补发
      expect((await getAllFailedMessages('uB')).map((m) => String(m.id))).toEqual(['b-1'])
    })

    it('本账号自己的失败行正常补发', async () => {
      await saveMessage(S, failedRow('a-1', 'A 自己的消息'), 'uA')
      loginAs('uA')
      postMock.mockImplementation(async (_url: string, body: any) =>
        serverMsg(`srv-${body.clientMsgId}`, body.content),
      )

      const store = useMessageStore()
      store.currentSessionId = S
      const res = await store.retryFailedMessages()

      expect(res.retried).toBe(1)
      expect(res.succeeded).toBe(1)
      expect(await getAllFailedMessages('uA')).toEqual([])
    })

    it('未登录时一律不补发（无可信归属）', async () => {
      await saveMessage(S, failedRow('a-1'), 'uA')
      // 不 login
      const store = useMessageStore()
      store.currentSessionId = S
      const res = await store.retryFailedMessages()

      expect(postMock, '未登录不得发任何请求').not.toHaveBeenCalled()
      expect(res.retried).toBe(0)
      expect(await getAllFailedMessages('uA')).toHaveLength(1)
    })
  })

  describe('Dexie schema：userId 索引已声明', () => {
    it('db.ts 的 schema 串含 userId 索引（源码级契约，防"加了字段没建索引"）', async () => {
      const { readFileSync } = await import('node:fs')
      const { fileURLToPath } = await import('node:url')
      const { dirname, resolve } = await import('node:path')
      const src = readFileSync(
        resolve(dirname(fileURLToPath(import.meta.url)), '../../utils/db.ts'),
        'utf8',
      )
      expect(src, 'messages 表应向 Dexie 声明 userId 索引').toMatch(/messages:\s*'[^']*userId/)
    })
  })
})
