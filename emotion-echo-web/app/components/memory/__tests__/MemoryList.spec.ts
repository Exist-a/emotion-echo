// CL-08d：MemoryList 组件契约（两段式确认 / 事件载荷 / busy 门）。
import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'

import MemoryList from '../MemoryList.vue'
import type { MemoryItem } from '~/types/memory'

const items: MemoryItem[] = [
  { kind: 'fact', id: 1, text: '用户居住地杭州', createdAt: '2026-10-25' },
  { kind: 'note', id: 2, text: '我在学萨克斯', createdAt: '2026-10-25' },
]

describe('MemoryList', () => {
  it('renders items with kind badges', () => {
    const w = mount(MemoryList, { props: { items } })
    expect(w.text()).toContain('用户居住地杭州')
    expect(w.text()).toContain('我在学萨克斯')
    expect(w.findAll('[data-kind="note"]')).toHaveLength(1)
  })

  it('arms confirm on first click, emits on second, resets on outside click', async () => {
    const w = mount(MemoryList, { props: { items }, attachTo: document.body })
    const btn = w.find('[data-testid="delete-item"]')
    await btn.trigger('click')
    expect(w.find('[data-testid="confirm-item"]').exists()).toBe(true)

    await w.find('[data-testid="confirm-item"]').trigger('click')
    expect(w.emitted('delete')).toEqual([['fact', 1]])
    expect(w.find('[data-testid="confirm-item"]').exists()).toBe(false)
    w.unmount()
  })

  it('delete-all uses its own armed key', async () => {
    const w = mount(MemoryList, { props: { items } })
    await w.find('[data-testid="delete-all"]').trigger('click')
    await w.find('[data-testid="confirm-all"]').trigger('click')
    expect(w.emitted('delete')).toEqual([['all', 0]])
    w.unmount()
  })

  it('first click does NOT emit (两段式)', async () => {
    const w = mount(MemoryList, { props: { items } })
    await w.find('[data-testid="delete-item"]').trigger('click')
    expect(w.emitted('delete')).toBeUndefined()
    w.unmount()
  })

  it('busy blocks arming and emitting', async () => {
    const w = mount(MemoryList, { props: { items, busy: true } })
    await w.find('[data-testid="delete-item"]').trigger('click')
    await w.find('[data-testid="delete-item"]').trigger('click')
    expect(w.emitted('delete')).toBeUndefined()
    w.unmount()
  })

  it('empty state text', () => {
    const w = mount(MemoryList, { props: { items: [] } })
    expect(w.text()).toContain('还没有可管理的记忆。')
  })
})
