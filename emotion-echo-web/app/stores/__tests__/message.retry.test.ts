import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import {
  useMessageStore,
  retryDelayMs,
  canAutoRetry,
  setRetryGovernor,
  MAX_RETRY,
} from '~/stores/message'
import { saveMessage, getAllFailedMessages, getLatestMessages } from '~/utils/messageCache'
import { getDb } from '~/utils/db'
import type { MessageStatus } from '~/types/conversation/conversationMessagesType'

// CL-05 Phase 4 · T-16 / T-17 / T-18：重试计数 + 退避 + 断网重连重扫。
//
// 现状（2026-10-14 亲核）：`retryCount` 只是个**恒为 0 的字段**（无自增/无上限）；
// `plugins/init.ts` 的 `online` 监听**只有 console.log + TODO**，`retryFailedMessages` 全仓无定义。
// 本钉锁三件事：
//   · T-16 每次失败 `retryCount` 自增；达 `MAX_RETRY` 后**不再被自动重试选中**（转手动重发）；
//   · T-17 退避曲线 = `min(2^n × 1000ms, 30s)`（n = 已失败次数）；
//   · T-18 联网后把**缓存里全部会话**的 `failed` 行按退避补发，**复用原 id 作 clientMsgId**
//          （后端 `client_msg_id` 唯一索引兜幂等 ⇒ 不会重复入库），成功后置 `sent` 并从失败集移除。
//
// 时间/等待走**可注入的 governor**（AGENTS §3.2「时钟/随机数必须通过接口暴露」+ §3.3
// 「永不睡眠」）：默认真 `Date.now()`/`setTimeout`，测试注入立即返回的实现，避免单测真睡 2/4/8 秒。
//
// 依据：plan §2 T-16·T-17·T-18 / §3 Phase 4；来源计划 D-E（MAX=3 / 指数退避 / 复用 clientMsgId）。

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

function failedRow(id: string, retryCount: number, content = '待补发') {
  return {
    id,
    sender: 'user' as const,
    sendTime: 1000,
    content,
    contentType: 'text' as const,
    status: 'failed' as MessageStatus,
    retryCount,
  }
}

let sleepCalls: number[] = []

describe('重试 + 断网重扫（CL-05 Phase 4 · T-16 / T-17 / T-18）', () => {
  beforeEach(async () => {
    setActivePinia(createPinia())
    getMock.mockReset()
    postMock.mockReset()
    sleepCalls = []
    // 注入"立即返回"的等待实现，并记录退避时长（用于断言曲线真的被用上）
    setRetryGovernor({
      now: () => Date.now(),
      sleep: async (ms: number) => {
        sleepCalls.push(ms)
      },
    })
    await (await getDb()).messages.clear()
  })

  afterEach(() => {
    // 还原默认（真等待），避免污染其它测试文件
    setRetryGovernor({
      now: () => Date.now(),
      sleep: (ms) => new Promise((r) => setTimeout(r, ms)),
    })
  })

  describe('T-17 退避曲线（纯函数）', () => {
    it('min(2^n × 1000ms, 30s)', () => {
      expect(retryDelayMs(1)).toBe(2000)
      expect(retryDelayMs(2)).toBe(4000)
      expect(retryDelayMs(3)).toBe(8000)
      expect(retryDelayMs(4)).toBe(16000)
      // 封顶 30s：超过就压到 30_000（2^5×1000=32000 已被封顶）
      expect(retryDelayMs(5)).toBe(30000)
      expect(retryDelayMs(12)).toBe(30000)
    })
  })

  describe('T-16 重试计数与上限', () => {
    it('失败一次后 retryCount 自增为 1', async () => {
      postMock.mockRejectedValue(new Error('network down'))
      const store = useMessageStore()
      store.currentSessionId = S

      await store.sendMessage('发不出去', undefined, 'cid-1')

      const failed = await getAllFailedMessages()
      expect(failed).toHaveLength(1)
      expect((failed[0] as any).retryCount, '首次失败后应为 1').toBe(1)
    })

    it('已达上限的行不再被自动重试选中（转手动重发）', async () => {
      expect(canAutoRetry(failedRow('at-max', MAX_RETRY) as any), '达上限应停自动重试').toBe(false)
      expect(canAutoRetry(failedRow('under-max', MAX_RETRY - 1) as any)).toBe(true)
    })

    it('retryFailedMessages 跳过已达上限的行（不发请求）', async () => {
      await saveMessage(S, failedRow('cid-max', MAX_RETRY))
      const store = useMessageStore()
      store.currentSessionId = S

      const res = await store.retryFailedMessages()

      expect(postMock, '达上限的行不得再自动发').not.toHaveBeenCalled()
      expect(res.retried).toBe(0)
    })
  })

  describe('T-18 断网重连重扫', () => {
    it('联网后补发缓存里全部会话的 failed 行，复用原 id 作 clientMsgId，成功后转 sent', async () => {
      // 模拟"刷新后仍在"的离线队列：直接落缓存（不经过内存列表）
      await saveMessage(S, failedRow('cid-9', 1, '断网时发的'))
      await saveMessage('c2', failedRow('cid-other', 1, '另一会话断网时发的'))

      postMock.mockImplementation(async (_url: string, body: any) =>
        serverMsg(`srv-${body.clientMsgId}`, 2000, body.content),
      )

      const store = useMessageStore()
      store.currentSessionId = S
      const res = await store.retryFailedMessages()

      expect(res.retried, '两个会话的失败行都要被扫到（离线队列与会话无关）').toBe(2)
      expect(res.succeeded).toBe(2)

      // 复用原 id 作幂等键（后端 client_msg_id 唯一索引兜底，不会重复入库）
      const sentKeys = postMock.mock.calls.map((c) => (c[1] as any).clientMsgId).sort()
      expect(sentKeys).toEqual(['cid-9', 'cid-other'])

      // 退避真的被用上（每个目标一次）
      expect(sleepCalls.sort((a, b) => a - b)).toEqual([2000, 2000])

      // 失败集清空；当前会话的内存里出现真身行
      expect(await getAllFailedMessages()).toEqual([])
      const cached = await getLatestMessages(S, 10)
      expect(cached.map((m) => String(m.id))).toEqual(['srv-cid-9'])
      expect(cached[0]!.status).toBe('sent')

      // 幽灵行不得残留（同 Phase 3 的教训）
      expect(cached.some((m) => String(m.id) === 'cid-9'), '临时 id 行必须被清掉').toBe(false)
    })

    it('补发再次失败：retryCount 再 +1 且仍留在失败集（等待下次联网）', async () => {
      await saveMessage(S, failedRow('cid-fail', 1))
      postMock.mockRejectedValue(new Error('still down'))

      const store = useMessageStore()
      store.currentSessionId = S
      const res = await store.retryFailedMessages()

      expect(res.succeeded).toBe(0)
      const still = await getAllFailedMessages()
      expect(still).toHaveLength(1)
      expect((still[0] as any).retryCount, '再次失败应再 +1').toBe(2)
    })

    it('没有待补发消息时是空操作（不发请求、不睡）', async () => {
      const store = useMessageStore()
      store.currentSessionId = S
      const res = await store.retryFailedMessages()

      expect(res.retried).toBe(0)
      expect(postMock).not.toHaveBeenCalled()
      expect(sleepCalls).toEqual([])
    })
  })
})
