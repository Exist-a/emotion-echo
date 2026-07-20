import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { ref, computed } from 'vue'
import { useConversationGrouper } from './useConversationGrouper'
import type { ConversationItem } from '~/types/api'

const make = (overrides: Partial<ConversationItem>): ConversationItem => ({
  id: overrides.id ?? 'c1',
  userId: 'u1',
  title: overrides.title ?? '对话',
  isTop: overrides.isTop ?? false,
  lastMessage: null,
  lastMessageTime: null,
  createdAt: overrides.createdAt ?? new Date().toISOString(),
  updatedAt: overrides.updatedAt ?? new Date().toISOString()
})

describe('useConversationGrouper', () => {
  let now: number
  beforeEach(() => {
    now = new Date('2026-07-17T12:00:00').getTime()
    vi.useFakeTimers()
    vi.setSystemTime(now)
  })
  afterEach(() => vi.useRealTimers())

  it('groups today by date only (not time)', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'morning', updatedAt: '2026-07-17T01:00:00' }),
      make({ id: 'noon', updatedAt: '2026-07-17T11:30:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    const today = groupedConversations.value.find((g) => g.label === '今天')
    expect(today?.data.map((d) => d.id)).toEqual(['morning', 'noon'])
  })

  it('separates pinned items into the 置顶 group', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'old', updatedAt: '2026-01-01T00:00:00', isTop: true }),
      make({ id: 'today', updatedAt: '2026-07-17T08:00:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    expect(groupedConversations.value[0].label).toBe('置顶')
    expect(groupedConversations.value[0].data.map((d) => d.id)).toEqual(['old'])
    expect(groupedConversations.value.find((g) => g.label === '今天')?.data.map((d) => d.id)).toEqual(['today'])
  })

  it('buckets items within 7 days as 一周内', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'a', updatedAt: '2026-07-15T12:00:00' }),
      make({ id: 'b', updatedAt: '2026-07-12T12:00:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    const within = groupedConversations.value.find((g) => g.label === '一周内')
    expect(within?.data.map((d) => d.id)).toEqual(['a', 'b'])
  })

  it('buckets items 8-30 days as 三十天内', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'mid', updatedAt: '2026-06-25T12:00:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    expect(groupedConversations.value.find((g) => g.label === '三十天内')?.data.map((d) => d.id)).toEqual(['mid'])
  })

  it('buckets items older than 30 days as 更早', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'ancient', updatedAt: '2025-01-01T00:00:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    expect(groupedConversations.value.find((g) => g.label === '更早')?.data.map((d) => d.id)).toEqual(['ancient'])
  })

  it('omits empty groups from the output', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'only-old', updatedAt: '2020-01-01T00:00:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    expect(groupedConversations.value.map((g) => g.label)).toEqual(['更早'])
  })

  it('orders groups: 置顶 → 今天 → 一周内 → 三十天内 → 更早', () => {
    const list = ref<ConversationItem[]>([
      make({ id: 'old', updatedAt: '2020-01-01T00:00:00', isTop: true }),
      make({ id: 'today', updatedAt: '2026-07-17T08:00:00' }),
      make({ id: 'mid', updatedAt: '2026-07-15T12:00:00' })
    ])
    const { groupedConversations } = useConversationGrouper(list)
    expect(groupedConversations.value.map((g) => g.label)).toEqual(['置顶', '今天', '一周内'])
  })
})
