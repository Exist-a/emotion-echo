// app/components/EeSelect.test.ts
//
// CL-06 · UI 轮：**自绘下拉**（替代原生 <select>）。
//
// 为什么必须自绘：原生 <select> 展开后的那一层是**操作系统/浏览器绘制的**，
// 不属于页面 DOM ⇒ 无论怎么写 CSS 都改不了它的样子（用户实测："展开的样式似乎
// 是原生浏览器样式，你没修改"）。想要成套的主题色/深色模式/分组，只能自己画。
//
// 自绘的代价是键盘与关闭行为都要自己负责 —— 所以这个文件钉的正是那些"自己负责的部分"：
// 展开、选中、Esc、点外面、方向键，以及**不被滚动容器裁切**（面板 Teleport 到 body）。
//
// 因为面板挂到了 body，本文件对面板/选项一律从 `document` 查询；触发器仍在组件内。
import { describe, it, expect, afterEach } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import EeSelect from './EeSelect.vue'

const OPTIONS = [
  { value: 'FunAudioLLM/CosyVoice2-0.5B:anna', label: 'anna（温柔女声）', group: '女声' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:bella', label: 'bella（明亮女声）', group: '女声' },
  { value: 'FunAudioLLM/CosyVoice2-0.5B:alex', label: 'alex（清朗男声）', group: '男声' },
]

let wrapper: VueWrapper | null = null
afterEach(() => {
  wrapper?.unmount()
  wrapper = null
})

function mountSelect(props: Record<string, unknown> = {}) {
  wrapper = mount(EeSelect, {
    props: { modelValue: OPTIONS[0]!.value, options: OPTIONS, testid: 'agent-voice', ...props },
    attachTo: document.body,
  })
  return wrapper
}

const panel = () => document.querySelector('.ee-select-panel')
const panelOptions = () => [...document.querySelectorAll('[data-ee-option]')]
const groupLabels = () =>
  [...document.querySelectorAll('.ee-select-group-label')].map((n) => n.textContent!.trim())
const click = (el: Element | null) => {
  if (!el) throw new Error('目标元素不存在')
  el.dispatchEvent(new MouseEvent('click', { bubbles: true }))
}
const press = (el: Element | null, key: string) => {
  if (!el) throw new Error('目标元素不存在')
  el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true }))
}
const tick = () => new Promise((r) => setTimeout(r, 0))

describe('EeSelect · 收起态', () => {
  it('触发器显示当前选中项的**可读名**（不显示模型串）', () => {
    const w = mountSelect()
    expect(w.find('.ee-select-trigger').text()).toContain('anna（温柔女声）')
    expect(w.find('.ee-select-trigger').text()).not.toContain('FunAudioLLM')
  })

  it('值不在选项里 ⇒ 显示 placeholder（老数据/音色下线时不显示空白按钮）', () => {
    const w = mountSelect({ modelValue: 'gone', placeholder: '默认音色' })
    expect(w.find('.ee-select-trigger').text()).toContain('默认音色')
  })

  it('收起时没有列表；且带 aria-haspopup 与 aria-expanded=false', () => {
    const w = mountSelect()
    expect(panel()).toBeNull()
    const t = w.find('.ee-select-trigger')
    expect(t.attributes('aria-haspopup')).toBe('listbox')
    expect(t.attributes('aria-expanded')).toBe('false')
  })

  it('支持把 data-testid 透传到触发器（页面契约靠它找控件）', () => {
    expect(mountSelect().find('[data-testid="agent-voice"]').exists()).toBe(true)
  })
})

describe('EeSelect · 展开与选择', () => {
  it('点触发器展开列表，aria-expanded 变 true', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    expect(panel()).not.toBeNull()
    expect(w.find('.ee-select-trigger').attributes('aria-expanded')).toBe('true')
  })

  // 回归钉：面板 Teleport 到 body。留在原地就会被最近的 overflow 容器裁掉
  // （实测：放进弹窗里时面板被切断，只剩一个分组标题露出来）。
  it('面板挂在 body 上（不在组件内部 ⇒ 不会被滚动容器裁切）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    expect(panel()!.parentElement).toBe(document.body)
    expect(w.find('[role="listbox"]').exists()).toBe(false)
  })

  it('按分组渲染标题（8 个音色分女声/男声，靠分组才扫得动）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    expect(groupLabels()).toEqual(['女声', '男声'])
  })

  it('选项标 aria-selected（当前值的那条为 true）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    const opts = panelOptions()
    expect(opts).toHaveLength(3)
    expect(opts[0]!.getAttribute('aria-selected')).toBe('true')
    expect(opts[1]!.getAttribute('aria-selected')).toBe('false')
  })

  it('点某个选项 ⇒ emit update:modelValue 并收起', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    click(document.querySelector('[data-ee-option="FunAudioLLM/CosyVoice2-0.5B:alex"]'))
    await tick()
    expect(w.emitted('update:modelValue')?.[0]).toEqual([OPTIONS[2]!.value])
    expect(panel()).toBeNull()
  })

  it('Esc 收起，且不改变选中值（取消不是选择）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    press(panel(), 'Escape')
    await tick()
    expect(panel()).toBeNull()
    expect(w.emitted('update:modelValue')).toBeUndefined()
  })

  it('方向键移动高亮 + Enter 选中（键盘可用，不只鼠标）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    press(panel(), 'ArrowDown')
    await tick()
    press(panel(), 'Enter')
    await tick()
    expect(w.emitted('update:modelValue')?.[0]).toEqual([OPTIONS[1]!.value])
  })

  it('方向键高亮停在边界不外溢（最后一项再按下不越界）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    for (let i = 0; i < 6; i++) {
      press(panel(), 'ArrowDown')
      await tick()
    }
    press(panel(), 'Enter')
    await tick()
    expect(w.emitted('update:modelValue')?.[0]).toEqual([OPTIONS[2]!.value])
  })
})

describe('EeSelect · 点外面关闭', () => {
  it('点到组件外面 ⇒ 收起（否则面板会一直挂在页面上）', async () => {
    const w = mountSelect()
    await w.find('.ee-select-trigger').trigger('click')
    expect(panel()).not.toBeNull()

    document.body.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await tick()

    expect(panel()).toBeNull()
  })
})
