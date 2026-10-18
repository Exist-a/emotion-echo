// app/components/AgentCard.test.ts
//
// CL-06 · UI 轮：角色卡（列表态与预览态共用一个组件）。
//
// 为什么把卡片抽成组件而不是留在页面里：它同时被**列表**和**编辑时的实时预览**使用
// （同一份"角色长什么样"的表述只能有一处），而且抽出来之后就能真的 mount 起来测行为
// ——页面上那些"删除没有二次确认""卡片不显示「不要」"的问题，挂载测试一眼能抓到，
// 源码契约（readFileSync + toMatch）只能证明"写了这行字"，证不了"点下去会怎样"。
import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import AgentCard from './AgentCard.vue'
import type { AgentProfile } from '~/types/api'

function profile(over: Partial<AgentProfile> = {}): AgentProfile {
  return {
    id: 7,
    name: '小E',
    gender: '女',
    persona: '温柔、愿意倾听的陪伴者。',
    tone: '温柔、口语化',
    boundaries: ['不评价外貌'],
    voice: 'FunAudioLLM/CosyVoice2-0.5B:anna',
    isDefault: true,
    ...over,
  }
}

describe('AgentCard · 列表态', () => {
  it('渲染名字 / 人设 / 语气 / 音色（音色给中文可读名，不给模型串）', () => {
    const w = mount(AgentCard, { props: { agent: profile() } })
    expect(w.text()).toContain('小E')
    expect(w.text()).toContain('温柔、愿意倾听的陪伴者。')
    expect(w.text()).toContain('温柔、口语化')
    // 用户不该看到 `FunAudioLLM/CosyVoice2-0.5B:anna` 这种模型串
    expect(w.text()).toContain('温柔女声')
    expect(w.text()).not.toContain('FunAudioLLM')
  })

  it('把「不要」（boundaries）显示出来 —— 用户配置过却看不见等于没配', () => {
    const w = mount(AgentCard, { props: { agent: profile({ boundaries: ['不评价外貌', '不说教'] }) } })
    expect(w.text()).toContain('不评价外貌')
    expect(w.text()).toContain('不说教')
    expect(w.find('.agent-card-bounds').exists()).toBe(true)
  })

  it('没有「不要」时不渲染空的边界区（不留孤立标签）', () => {
    const w = mount(AgentCard, { props: { agent: profile({ boundaries: [] }) } })
    expect(w.find('.agent-card-bounds').exists()).toBe(false)
  })

  it('没有人设 ⇒ 给一句可读的占位，不留空白', () => {
    const w = mount(AgentCard, { props: { agent: profile({ persona: '' }) } })
    expect(w.text()).toContain('还没有人设')
  })

  it('默认角色带「默认」标记；非默认角色不带', () => {
    expect(mount(AgentCard, { props: { agent: profile() } }).text()).toContain('默认')
    expect(
      mount(AgentCard, { props: { agent: profile({ isDefault: false }) } }).text(),
    ).not.toContain('默认')
  })

  it('首字徽记用名字首字，且带上算出来的身份色调类', () => {
    const w = mount(AgentCard, { props: { agent: profile({ name: '阿哲' }) } })
    const mono = w.find('.agent-mono')
    expect(mono.text()).toBe('阿')
    expect(mono.classes().some((c) => /^tone-\d+$/.test(c))).toBe(true)
  })

  it('卡片带 data-agent-card（回归钉按它找卡片）', () => {
    expect(mount(AgentCard, { props: { agent: profile() } }).find('[data-agent-card="7"]').exists()).toBe(
      true,
    )
  })
})

describe('AgentCard · 删除需要二次确认', () => {
  it('第一次点「删除」不发出事件，只把卡片切到确认态', async () => {
    const w = mount(AgentCard, { props: { agent: profile() } })
    await w.get('[data-agent-action="delete"]').trigger('click')
    expect(w.emitted('delete')).toBeUndefined()
    expect(w.find('[data-agent-confirm]').exists()).toBe(true)
    expect(w.text()).toContain('删除「小E」')
  })

  it('确认后才 emit delete（并把确认态收起来）', async () => {
    const w = mount(AgentCard, { props: { agent: profile() } })
    await w.get('[data-agent-action="delete"]').trigger('click')
    await w.get('[data-agent-confirm="yes"]').trigger('click')
    expect(w.emitted('delete')?.length).toBe(1)
    expect(w.find('[data-agent-confirm]').exists()).toBe(false)
  })

  it('取消后回到普通态，且不 emit delete', async () => {
    const w = mount(AgentCard, { props: { agent: profile() } })
    await w.get('[data-agent-action="delete"]').trigger('click')
    await w.get('[data-agent-confirm="no"]').trigger('click')
    expect(w.emitted('delete')).toBeUndefined()
    expect(w.find('[data-agent-confirm]').exists()).toBe(false)
    expect(w.find('[data-agent-action="delete"]').exists()).toBe(true)
  })

  it('点「编辑」直接 emit（非破坏性动作不需要确认）', async () => {
    const w = mount(AgentCard, { props: { agent: profile() } })
    await w.get('[data-agent-action="edit"]').trigger('click')
    expect(w.emitted('edit')?.length).toBe(1)
  })
})

describe('AgentCard · 预览态', () => {
  it('预览态不渲染任何操作按钮（预览不是操作面）', () => {
    const w = mount(AgentCard, { props: { agent: profile(), variant: 'preview' } })
    expect(w.find('[data-agent-action="edit"]').exists()).toBe(false)
    expect(w.find('[data-agent-action="delete"]').exists()).toBe(false)
  })

  it('预览态照常显示名字/人设/音色（所见即所得）', () => {
    const w = mount(AgentCard, {
      props: { agent: profile({ name: '阿哲', persona: '直率的朋友。' }), variant: 'preview' },
    })
    expect(w.text()).toContain('阿哲')
    expect(w.text()).toContain('直率的朋友。')
  })
})
