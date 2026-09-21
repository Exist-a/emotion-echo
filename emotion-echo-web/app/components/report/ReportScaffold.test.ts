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
    return mount(ReportScaffold as any, { props, slots, attachTo: document.body })
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
    expect(events![0]![0]).toBe('2026-07-02')
    wrapper.unmount()
  })

  it('emits change as a [start,end] array when both range endpoints are set', async () => {
    const wrapper = await factory({
      title: '周',
      date: ['2026-07-01', '2026-07-07'],
      pickerType: 'daterange',
    })
    const inputs = wrapper.findAll('input.date-input')
    await inputs[1]!.setValue('2026-07-14')
    const events = wrapper.emitted('change')
    expect(events).toBeTruthy()
    expect(events![0]![0]).toEqual(['2026-07-01', '2026-07-14'])
    wrapper.unmount()
  })

  it('emits update:date so v-model:date in parents persists the picked value', async () => {
    const wrapper = await factory({ title: '日', date: '2026-07-01', pickerType: 'date' })
    const input = wrapper.find('input.date-input')
    await input.setValue('2026-07-02')
    // GREEN 阶段会新增 update:date emit;RED 阶段缺少该 emit 时此断言失败
    const updateEvents = wrapper.emitted('update:date')
    expect(updateEvents).toBeTruthy()
    expect(updateEvents![0]![0]).toBe('2026-07-02')
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
      { summary: '<p class="sum">S</p>', charts: '<p class="ch">C</p>' },
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
      disableFuture: true,
    })
    const input = wrapper.find('input.date-input')
    const today = new Date()
    const maxMonth = `${today.getFullYear()}-${String(today.getMonth() + 1).padStart(2, '0')}`
    // GREEN 阶段会把 disableFuture 应用到 max;RED 阶段没有 max 时 assert 失败
    expect(input.attributes('max')).toBe(maxMonth)
    wrapper.unmount()
  })

  // ============ summary slot 排版契约（2026-09-21 用户实测反馈） ============
  //
  // 用户反馈：日报/周报的 summary 卡片「排版有点简陋，只有文字排版」。
  // 根因：`.summary-text` / `.stats-row` / `.stat-item` / `.stat-value` /
  // `.stat-label` 这些 class 在被渲染，但**全仓零 CSS 定义** ⇒ 数字与标签
  // 挤成一行（"0会话数 33消息数"）。本组测试钉住「这 5 个 class 必须有样式」。
  const readScaffoldSrc = async () => {
    const { readFileSync } = await import('node:fs')
    const { fileURLToPath } = await import('node:url')
    const { resolve, dirname } = await import('node:path')
    return readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), 'ReportScaffold.vue'), 'utf8')
  }

  it('defines styles for all summary-slot classes (回归: 零 CSS ⇒ 数字标签挤行)', async () => {
    const src = await readScaffoldSrc()
    const style = src.match(/<style[\s\S]*?<\/style>/)?.[0] ?? ''
    expect(style.length, 'ReportScaffold 必须有 <style> 块').toBeGreaterThan(0)

    for (const cls of ['summary-text', 'stats-row', 'stat-item', 'stat-value', 'stat-label']) {
      expect(
        style.includes(`.${cls}`),
        `E2E-15 FU: 必须定义 .${cls} 样式（当前缺失 ⇒ summary 卡片只有裸文字排版）`,
      ).toBe(true)
    }
  })

  it('uses only --ee-* design tokens in summary styles (不得硬编码色值)', async () => {
    const src = await readScaffoldSrc()
    const style = src.match(/<style[\s\S]*?<\/style>/)?.[0] ?? ''
    const block = style.match(/\.report-summary\s*\{[\s\S]*?\n\}/)?.[0] ?? ''
    expect(block.length, '.report-summary 块应存在').toBeGreaterThan(0)
    const hex = block.match(/#[0-9a-f]{3,8}\b/gi) || []
    expect(hex, `summary 样式硬编码色值 ${hex.join(',')} 应改为 var(--ee-*)`).toEqual([])
  })
})
