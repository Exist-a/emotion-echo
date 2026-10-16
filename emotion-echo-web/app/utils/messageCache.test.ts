import { describe, it, expect, beforeEach } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import {
  saveMessage,
  saveMessages,
  getLatestMessages,
  getMessagesBySession,
  getPendingMessages,
  getFailedMessages,
  updateMessageStatus,
  trimSessionMessages,
  trimOldMessages,
  exportSessionMessages,
  getCacheStats,
} from '~/utils/messageCache'
import { getDb } from '~/utils/db'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'

// CL-05 Phase 2 · T-9 / T-10（+ G8 缓存治理）：本地缓存层的**契约**测试。
//
// 现状（2026-10-14 亲核）：`utils/messageCache.ts`（267 行完整 CRUD）**全仓零调用**，
// 且它依赖的两个索引在 `db.ts` 里**根本没建** ⇒ 一旦真被调用就抛 SchemaError：
//   · `getPendingMessages` / `getFailedMessages` 用 `where({ sessionId, status })`
//     ⇒ 需要 **[sessionId+status]** 复合索引；
//   · `trimOldMessages` 用 `where('sendTime')` ⇒ 需要 **sendTime** 单列索引。
// 本钉的作用是：**先让这两个调用红**（证明它们确实会炸），再把索引补上转绿 ——
// 避免"接线了但其实一跑就抛"的半成品状态（来源计划 §1 第 7 条）。
//
// IndexedDB 由 fake-indexeddb 垫片提供（见 tests-setup.ts 首行）。
// 依据：docs/closure-roadmap/stages/cl-05-message-pipeline/plan.md §3 Phase 2。

const __dirname = dirname(fileURLToPath(import.meta.url))
const DB_SRC = readFileSync(resolve(__dirname, 'db.ts'), 'utf8')

const SESSION = 'sess-1'
const SESSION_B = 'sess-2'

function msg(id: number, sendTime: number, status?: MessageStatus) {
  return {
    id,
    sender: 'user' as const,
    sendTime,
    content: `m${id}`,
    contentType: 'text' as const,
    ...(status ? { status } : {}),
  }
}

describe('messageCache 契约（CL-05 Phase 2）', () => {
  beforeEach(async () => {
    const db = await getDb()
    // fake-indexeddb 的库在同一进程内共享 ⇒ 每个用例先清干净，
    // 否则用例间互相污染（"单跑绿、全跑红"的经典来源）。
    await db.messages.clear()
  })

  it('Dexie schema 必须逐项声明缓存在用的索引（源码级契约）', () => {
    // 精确到"逗号分隔的索引条目"，否则 [sessionId+sendTime] 会让裸 'sendTime' 的
    // 子串断言永真（假绿）。
    // ⚠️ 取**最后一份**声明：Dexie 每个 version 都要重述完整 schema，
    // 生效的是最高版本那份（v1 那份是升级起点，不是最终索引集）。
    const specs = [...DB_SRC.matchAll(/messages:\s*'([^']+)'/g)].map((m) => m[1] ?? '')
    expect(specs.length, 'db.ts 应至少声明一份 messages 规格').toBeGreaterThan(0)
    const indexes = specs[specs.length - 1]!.split(',').map((s) => s.trim())
    expect(indexes, `实际索引：${indexes.join(' | ')}`).toEqual([
      'id',
      'sessionId',
      // CL-F-20：归属用户 + [userId+status] 复合索引（按用户隔离查询/补发）
      'userId',
      'sendTime',
      '[sessionId+sendTime]',
      '[sessionId+status]',
      '[userId+status]',
    ])
  })

  it('schema 以**增量** version 升级（保留旧声明 ⇒ Dexie 自动迁移，不丢旧缓存）', async () => {
    const db = await getDb()
    expect(db.verno, 'Dexie 版本应为 3（CL-F-20 引入 userId）').toBe(3)
    expect(DB_SRC, 'v1 声明必须保留（加法式升级，不是重写）').toMatch(/version\(1\)\.stores\(/)
    expect(DB_SRC, 'v2 声明应保留').toMatch(/version\(2\)\.stores\(/)
    expect(DB_SRC, 'v3 声明应存在').toMatch(/version\(3\)\.stores\(/)
  })

  describe('T-9 索引可用性（修前必红）', () => {
    it('getFailedMessages 不应抛错，且只返回本会话的 failed 行', async () => {
      await saveMessage(SESSION, msg(1, 1000, 'sent'))
      await saveMessage(SESSION, msg(2, 2000, 'failed'))
      await saveMessage(SESSION_B, msg(3, 3000, 'failed'))

      const failed = await getFailedMessages(SESSION)
      expect(failed.map((m) => m.id)).toEqual([2])
    })

    it('getPendingMessages 不应抛错，且只返回 sending 行', async () => {
      await saveMessage(SESSION, msg(1, 1000, 'sending'))
      await saveMessage(SESSION, msg(2, 2000, 'sent'))

      const pending = await getPendingMessages(SESSION)
      expect(pending.map((m) => m.id)).toEqual([1])
    })

    it('trimOldMessages 不应抛错，且只删"足够旧 + 已发送"的行', async () => {
      const now = Date.now()
      const old = now - 40 * 24 * 60 * 60 * 1000 // 40 天前
      await saveMessage(SESSION, msg(1, old, 'sent')) // 该删
      await saveMessage(SESSION, msg(2, old, 'failed')) // 保留：未发送成功
      await saveMessage(SESSION, msg(3, now, 'sent')) // 保留：够新

      const removed = await trimOldMessages(30)
      expect(removed).toBe(1)

      const left = await getLatestMessages(SESSION, 10)
      expect(left.map((m) => m.id).sort()).toEqual([2, 3])
    })

    it('exportSessionMessages 不应抛错，且按 sendTime 升序导出', async () => {
      await saveMessages(SESSION, [msg(3, 3000), msg(1, 1000), msg(2, 2000)])

      const rows = await exportSessionMessages(SESSION)
      expect(rows.map((m) => m.id)).toEqual([1, 2, 3])
    })
  })

  describe('T-10 缓存往返与分页', () => {
    it('saveMessage → getLatestMessages 往返一致（含 status/retryCount 落库）', async () => {
      await saveMessage(SESSION, msg(1, 1000, 'sent'))
      const rows = await getLatestMessages(SESSION, 10)
      expect(rows).toHaveLength(1)
      expect(rows[0]).toMatchObject({ id: 1, sessionId: SESSION, content: 'm1', status: 'sent' })
      expect(rows[0]?.retryCount).toBe(0)
    })

    it('saveMessages 批量落库 + updateMessageStatus 改状态', async () => {
      await saveMessages(SESSION, [msg(1, 1000), msg(2, 2000)])
      await updateMessageStatus(1, 'failed')

      const failed = await getFailedMessages(SESSION)
      expect(failed.map((m) => m.id)).toEqual([1])
    })

    it('getMessagesBySession 游标分页：两页不重不漏（最新在前）', async () => {
      await saveMessages(
        SESSION,
        [1, 2, 3, 4, 5].map((i) => msg(i, i * 1000)),
      )

      const page1 = await getMessagesBySession(SESSION, 2)
      expect(page1.map((m) => m.id)).toEqual([5, 4])

      const cursor = page1[page1.length - 1]!.sendTime
      const page2 = await getMessagesBySession(SESSION, 2, cursor)
      expect(page2.map((m) => m.id)).toEqual([3, 2])

      const ids = [...page1, ...page2].map((m) => m.id)
      expect(new Set(ids).size, '两页不得重复').toBe(ids.length)
    })
  })

  describe('缓存治理（G8 / 来源计划 D-F；非 T-16）', () => {
    it('trimSessionMessages 只保留最新 maxCount 条', async () => {
      await saveMessages(
        SESSION,
        [1, 2, 3, 4, 5].map((i) => msg(i, i * 1000)),
      )
      const removed = await trimSessionMessages(SESSION, 3)
      expect(removed).toBe(2)

      const rows = await getLatestMessages(SESSION, 10)
      expect(rows.map((m) => m.id).sort()).toEqual([3, 4, 5])
    })

    it('getCacheStats 统计 sending / failed 计数', async () => {
      await saveMessage(SESSION, msg(1, 1000, 'sending'))
      await saveMessage(SESSION, msg(2, 2000, 'failed'))
      await saveMessage(SESSION, msg(3, 3000, 'sent'))

      const stats = await getCacheStats()
      expect(stats).toMatchObject({
        totalMessages: 3,
        totalSessions: 1,
        sendingCount: 1,
        failedCount: 1,
      })
    })
  })
})
