import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const CHARTS_CARD_SRC = readFileSync(resolve(__dirname, 'chartsCard.vue'), 'utf8')

// chartsCard.vue 公共合同:
//   - 接受 data: ChartItem[]
//   - data.length === 0 → 不挂任何 .chart-item
//   - data.length > 0 → 渲染对应数量的 .chart-item
//   - data 内的 chartItem → 选对应图表组件 (pie/line/bar/radar); 未知类型 → chart-placeholder
//   - gridStyle 来自响应式断点:
//       < 992px → 1 列, 992–1600px → 2 列, >= 1600px → 3 列
//   - <style> 不再使用硬编码 #fff/#333 等(必须 var(--ee-*))
//   - <style> 不滥用 !important
//   - <style> 不使用 :has() 选择器 (兼容旧浏览器)
describe('chartsCard.vue', () => {
  beforeEach(() => {
    vi.stubGlobal('useRoute', () => ({ path: '/test' }))
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  const factory = async (props: any) => {
    const mod = await import('./chartsCard.vue')
    return mount(mod.default as any, {
      props,
      attachTo: document.body,
      global: {
        stubs: {
          pieChart: { template: '<div class="pie-stub" />' },
          lineChart: { template: '<div class="line-stub" />' },
          barChart: { template: '<div class="bar-stub" />' },
          RadarChart: { template: '<div class="radar-stub" />' }
        }
      }
    })
  }

  const setInnerWidth = (value: number) => {
    const w: any = (globalThis as any).window ?? globalThis
    Object.defineProperty(w, 'innerWidth', { configurable: true, get: () => value })
  }

  it('renders nothing inside the grid when data is empty', async () => {
    const wrapper = await factory({ data: [] })
    await nextTick()
    expect(wrapper.findAll('.chart-item').length).toBe(0)
    wrapper.unmount()
  })

  it('renders one .chart-item per data entry', async () => {
    const wrapper = await factory({
      data: [
        { chartType: 'pie', title: 'A', data: [{ name: 'x', value: 1 }] },
        { chartType: 'line', title: 'B', XData: ['a'], YData: [1] },
        { chartType: 'bar', title: 'C', XData: ['a'], YData: [1] }
      ]
    })
    await nextTick()
    expect(wrapper.findAll('.chart-item').length).toBe(3)
    wrapper.unmount()
  })

  it('falls back to chart-placeholder for unknown chartType', async () => {
    const wrapper = await factory({
      data: [{ chartType: 'unknown' as any, title: 'X' }]
    })
    await nextTick()
    expect(wrapper.find('.chart-placeholder').exists()).toBe(true)
    wrapper.unmount()
  })

  it('renders 1 column on narrow viewports (< 992px)', async () => {
    setInnerWidth(800)
    const wrapper = await factory({ data: [{ chartType: 'pie', title: 'A', data: [{ name: 'x', value: 1 }] }] })
    await nextTick()
    const grid = wrapper.find('.charts-grid')
    expect(grid.attributes('style') || '').toMatch(/grid-template-columns:\s*repeat\(1,/)
    wrapper.unmount()
  })

  it('renders 3 columns on wide viewports (>= 1600px)', async () => {
    setInnerWidth(1920)
    const wrapper = await factory({ data: [{ chartType: 'pie', title: 'A', data: [{ name: 'x', value: 1 }] }] })
    await nextTick()
    const grid = wrapper.find('.charts-grid')
    expect(grid.attributes('style') || '').toMatch(/grid-template-columns:\s*repeat\(3,/)
    wrapper.unmount()
  })

  it('does not hardcode non-white hex colors in <style> (Stage 104 token alignment)', () => {
    const style = CHARTS_CARD_SRC.match(/<style[\s\S]*?<\/style>/)?.[0] ?? ''
    const hexRegex = /#[0-9a-f]{3,8}\b/gi
    const hits = style.match(hexRegex) || []
    const offToken = hits.filter((h) => h.toLowerCase() !== '#fff' && h.toLowerCase() !== '#ffffff')
    expect(offToken, `硬编码色: ${offToken.join(',')} 应改为 var(--ee-*)`).toEqual([])
  })

  it('does not use !important in <style>', () => {
    const style = CHARTS_CARD_SRC.match(/<style[\s\S]*?<\/style>/)?.[0] ?? ''
    expect(style.includes('!important')).toBe(false)
  })

  it('does not use :has() selector in <style> (compatibility guard)', () => {
    const style = CHARTS_CARD_SRC.match(/<style[\s\S]*?<\/style>/)?.[0] ?? ''
    expect(/:has\(/.test(style)).toBe(false)
  })
})