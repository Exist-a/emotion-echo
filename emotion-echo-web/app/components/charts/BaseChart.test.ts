import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { nextTick } from 'vue'

// BaseChart.vue 公共合同:
//   - title 为字符串时,渲染成 <h3> 标题
//   - option.series 为空/缺失 → 渲染 "暂无数据" 占位 (不再静默显示空 div)
//   - option.series 有数据 → 渲染 VChartFull 容器 (300px 默认高度)
//   - 容器高度跟随 props.height (透传给 style)
//   - 不向 console 打印调试日志 (Stage 103 收口: 清掉 [BaseChart] 系列 console.log)
describe('BaseChart.vue', () => {
  let logSpy: ReturnType<typeof vi.spyOn>
  let warnSpy: ReturnType<typeof vi.spyOn>

  beforeEach(() => {
    logSpy = vi.spyOn(console, 'log').mockImplementation(() => {})
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
  })

  afterEach(() => {
    logSpy.mockRestore()
    warnSpy.mockRestore()
    vi.unstubAllGlobals()
    document.documentElement.classList.remove('dark')
  })

  const factory = async (props: any) => {
    const { default: BaseChart } = await import('./BaseChart.vue')
    return mount(BaseChart as any, { props, attachTo: document.body })
  }

  it('renders title as an h3 centered above the chart', async () => {
    const wrapper = await factory({
      option: { series: [{ type: 'pie', data: [{ name: 'A', value: 1 }] }] },
      title: '情绪分布',
      height: 280,
    })
    await nextTick()
    const h3 = wrapper.find('h3')
    expect(h3.exists()).toBe(true)
    expect(h3.text()).toBe('情绪分布')
    expect(h3.attributes('style') || '').toMatch(/text-align:\s*center/)
    wrapper.unmount()
  })

  it('shows the empty-state placeholder when option has no series', async () => {
    const wrapper = await factory({ option: { series: [] }, title: '空数据', height: 280 })
    await nextTick()
    expect(wrapper.text()).toContain('暂无数据')
    // 关键: 不再渲染 VChartFull 容器 (空态应彻底不挂图表)
    expect(wrapper.find('.chart-container').exists()).toBe(false)
    wrapper.unmount()
  })

  it('shows the empty-state placeholder when option is undefined', async () => {
    const wrapper = await factory({ title: '空', height: 280 })
    await nextTick()
    expect(wrapper.text()).toContain('暂无数据')
    wrapper.unmount()
  })

  it('renders the chart container with the requested height when series has data', async () => {
    const wrapper = await factory({
      option: { series: [{ type: 'pie', data: [{ name: 'A', value: 1 }] }] },
      title: 'ok',
      height: 320,
    })
    await nextTick()
    const container = wrapper.find('.chart-container')
    expect(container.exists()).toBe(true)
    expect(container.attributes('style') || '').toMatch(/height:\s*320px/)
    wrapper.unmount()
  })

  it('does not log debug messages from hasData / mergedOption (Stage 103 cleanup)', async () => {
    await factory({
      option: { series: [{ type: 'pie', data: [{ name: 'A', value: 1 }] }] },
      title: '静默',
      height: 300,
    })
    await nextTick()
    const calls = logSpy.mock.calls.flat().map(String)
    // 不允许再出现 [BaseChart] 调试前缀
    expect(calls.some((m) => m.includes('[BaseChart]'))).toBe(false)
  })

  it('toggles to dark theme when html.dark is present at mount', async () => {
    document.documentElement.classList.add('dark')
    const wrapper = await factory({
      option: { series: [{ type: 'pie', data: [{ name: 'A', value: 1 }] }] },
      title: 'dark',
      height: 300,
    })
    await nextTick()
    // VChartFull 接收 theme="dark" (这里只验证无报错; 真实主题生效是 echarts 行为)
    expect(wrapper.exists()).toBe(true)
    wrapper.unmount()
  })
})
