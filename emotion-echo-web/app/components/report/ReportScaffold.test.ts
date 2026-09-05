import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'

// ReportScaffold.vue 公共合同:
//   - pickerType ∈ 'date' | 'daterange' | 'month' | 'year'
//   - change input 后 emit 'change'(单 picker emit string;daterange emit [start,end])
//   - 渲染 default summary / charts slots,空给 empty-state
//   - loading=true 显示 skeleton
//
// RED-only 阶段:任何偏离以上合同即 FAIL。
// ReportScaffold 当前 EMIT 仅 'change',不 emit 'update:date',
// 这意味着父页面 v-model:date 不会生效。GREEN 阶段会补 update:date。
describe('ReportScaffold.vue', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const factory = async (props: Record<string, any>, slots: Record<string, string> = {}) => {
    const { default: ReportScaffold } = await import('./ReportScaffold.vue')
    return mount(ReportScaffold, { props, slots, attachTo: document.body })
  }

  it('renders title and description in the header', async () => {
    const wrapper = await factory({ title: '日报', description: '看看今天的心情' })
    expect(wrapper.text()).toContain('日报')
    expect(wrapper.text()).toContain('看看今天的心情')
    wrapper.unmount()
  })

  it('emits change with the new ISO string for single date picker', async () => {
    const wrapper = await factory({ title: '日', date: '2026-07-01', pickerType: 'date' })
    const input = wrapper.find('input.date-input')
    await input.setValue('2026-07-02')
    const events = wrapper.emitted('change')
    expect(events).toBeTruthy()
    expect(events![0][0]).toBe('2026-07-02')
    wrapper.unmount()
  })

  it('emits change as a [start,end] array when both range endpoints are set', async () => {
    const wrapper = await factory({
      title: '周',
      date: ['2026-07-01', '2026-07-07'],
      pickerType: 'daterange'
    })
    const inputs = wrapper.findAll('input.date-input')
    await inputs[1].setValue('2026-07-14')
    const events = wrapper.emitted('change')
    expect(events).toBeTruthy()
    expect(events![0][0]).toEqual(['2026-07-01', '2026-07-14'])
    wrapper.unmount()
  })

  it('emits update:date so v-model:date in parents persists the picked value', async () => {
    const wrapper = await factory({ title: '日', date: '2026-07-01', pickerType: 'date' })
    const input = wrapper.find('input.date-input')
    await input.setValue('2026-07-02')
    // GREEN 阶段会新增 update:date emit;RED 阶段缺少该 emit 时此断言失败
    const updateEvents = wrapper.emitted('update:date')
    expect(updateEvents).toBeTruthy()
    expect(updateEvents![0][0]).toBe('2026-07-02')
    wrapper.unmount()
  })

  it('switches to skeleton state when loading is true', async () => {
    const wrapper = await factory({ title: '日', date: '2026-07-01', loading: true })
    expect(wrapper.findAll('.ee-skeleton').length).toBeGreaterThan(0)
    expect(wrapper.find('.empty-state').exists()).toBe(false)
    wrapper.unmount()
  })

  it('renders default empty-state when no charts slot provided', async () => {
    const wrapper = await factory({ title: '日', date: '2026-07-01' })
    expect(wrapper.find('.empty-state').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders summary / charts slot content when provided', async () => {
    const wrapper = await factory(
      { title: '日', date: '2026-07-01' },
      { summary: '<p class="sum">S</p>', charts: '<p class="ch">C</p>' }
    )
    expect(wrapper.find('.report-summary').html()).toContain('S')
    expect(wrapper.find('.report-charts').html()).toContain('C')
    wrapper.unmount()
  })

  it('caps future months / years on month / year pickers when disableFuture=true', async () => {
    const wrapper = await factory({
      title: '月',
      date: '2026-01',
      pickerType: 'month',
      disableFuture: true
    })
    const input = wrapper.find('input.date-input')
    const today = new Date()
    const maxMonth = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}`
    // GREEN 阶段会把 disableFuture 应用到 max;RED 阶段没有 max 时 assert 失败
    expect(input.attributes('max')).toBe(maxMonth)
    wrapper.unmount()
  })
})
