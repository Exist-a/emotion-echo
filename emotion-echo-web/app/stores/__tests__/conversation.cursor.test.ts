import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

// CL-05 Phase 5 · T-6：会话列表「加载更多」**不得重取第一页**。
//
// 前置实证（Phase 0 基线，走网关实测）：旧实现把游标在本地用
// `lastItem.updatedAt` 推导成 RFC3339 字符串发出去，而下游**根本不认**这个参数
// ⇒ 第 2 页响应 = 第 1 页（交集 {683,684}）。且 RFC3339 只到**秒**，
// 同一秒更新的多个会话无法区分。
// 现契约：游标由**服务端**给出（不透明复合键），前端只做原样回传。
const getMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  get: (...args: unknown[]) => getMock(...args),
  post: vi.fn(),
  put: vi.fn(),
  patch: vi.fn(),
  del: vi.fn(),
}))

import { useConversationStore } from '~/stores/conversation'

const SERVER_CURSOR = '1760594460000000:9'

function conv(id: string, updatedAt: string) {
  return {
    id,
    userId: '1',
    title: `T${id}`,
    isTop: false,
    lastMessage: null,
    lastMessageTime: null,
    createdAt: updatedAt,
    updatedAt,
  }
}

describe('conversation store 游标翻页（CL-05 Phase 5 / T-6）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    getMock.mockReset()
    getMock.mockImplementation((_path: string, params: any) => {
      if (!params?.cursor) {
        return Promise.resolve({
          list: [conv('10', '2026-10-16T06:00:00+08:00'), conv('9', '2026-10-16T05:00:00+08:00')],
          hasMore: true,
          cursor: SERVER_CURSOR,
        })
      }
      return Promise.resolve({
        list: [conv('8', '2026-10-16T04:00:00+08:00')],
        hasMore: false,
        cursor: '',
      })
    })
  })

  it('第一页请求不带 cursor；游标取自**服务端**返回，而不是本地 lastItem.updatedAt', async () => {
    const store = useConversationStore()
    await store.fetchConversations()

    const params = getMock.mock.calls[0]![1] as Record<string, unknown>
    expect(params.cursor, '首页不得带游标').toBeUndefined()
    expect(store.conversationList.map((c: any) => c.id)).toEqual(['10', '9'])
  })

  it('loadMore 的第二页请求参数 ≠ 第一页（带服务端游标），且是追加、id 无交集', async () => {
    const store = useConversationStore()
    await store.fetchConversations()
    await store.loadMore()

    expect(getMock).toHaveBeenCalledTimes(2)
    const p1 = getMock.mock.calls[0]![1] as Record<string, unknown>
    const p2 = getMock.mock.calls[1]![1] as Record<string, unknown>

    expect(p2.cursor).toBe(SERVER_CURSOR)
    expect(p2, '第二页请求参数必须与第一页不同（否则等于重取第一页）').not.toEqual(p1)
    expect(
      p2.cursor,
      '游标不得再由本地 updatedAt 推导 —— 那个值服务端不认（Phase 0 基线：第2页=第1页）',
    ).not.toBe('2026-10-16T05:00:00+08:00')

    expect(store.conversationList.map((c: any) => c.id)).toEqual(['10', '9', '8'])
  })

  it('hasMore=false 之后 loadMore 不再发请求', async () => {
    const store = useConversationStore()
    await store.fetchConversations()
    await store.loadMore()
    expect(getMock).toHaveBeenCalledTimes(2)

    await store.loadMore()
    expect(getMock, 'hasMore=false 后不得再请求').toHaveBeenCalledTimes(2)
  })

  it('init() 重置游标 → 重新取第一页（替换而非追加，避免会话重复）', async () => {
    const store = useConversationStore()
    await store.fetchConversations()
    await store.loadMore()
    expect(store.conversationList).toHaveLength(3)

    await store.init()
    const p = getMock.mock.calls[2]![1] as Record<string, unknown>
    expect(p.cursor, 'init 后首页不得带游标').toBeUndefined()
    expect(store.conversationList.map((c: any) => c.id)).toEqual(['10', '9'])
  })
})
