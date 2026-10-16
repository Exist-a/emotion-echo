// components/emotion/CurrentEmotionBar.test.ts
//
// CL-07 前端半 · Phase 3 契约钉：顶部状态条的展示与隐藏（含护栏与"纯文本不夸大"）。

import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import CurrentEmotionBar from './CurrentEmotionBar.vue'
import type { CurrentEmotionState } from '~/composables/useCurrentEmotion'
import { FORBIDDEN_SOURCE_WORDS } from '~/composables/useCurrentEmotion'

const base: CurrentEmotionState = {
  emotion: 'anxious',
  label: '焦虑',
  text: '此刻的状态：焦虑',
  modalityLabel: '文本',
  hasMultiModal: false,
}

describe('CurrentEmotionBar', () => {
  it('state=null → 整块不渲染（无融合数据时优雅隐藏）', () => {
    const w = mount(CurrentEmotionBar, { props: { state: null } })
    expect(w.find('.current-emotion-bar').exists()).toBe(false)
    expect(w.text()).toBe('')
  })

  it('有 state → 渲染护栏文案 + 模态标签', () => {
    const w = mount(CurrentEmotionBar, { props: { state: base } })
    expect(w.find('.current-emotion-bar').exists()).toBe(true)
    expect(w.text()).toContain('此刻的状态：焦虑')
    expect(w.text()).toContain('文本')
  })

  it('含 face/voice 的融合 → 标签显示「综合」', () => {
    const w = mount(CurrentEmotionBar, {
      props: { state: { ...base, modalityLabel: '综合', hasMultiModal: true } },
    })
    expect(w.text()).toContain('综合')
  })

  it('护栏：渲染文本不得出现来源词（识别/检测/摄像头/分析/监控）', () => {
    const w = mount(CurrentEmotionBar, { props: { state: base } })
    for (const bad of FORBIDDEN_SOURCE_WORDS) {
      expect(w.text()).not.toContain(bad)
    }
  })

  it('情绪色点按 emotion 着色（视觉区分，不影响文案）', () => {
    const w = mount(CurrentEmotionBar, { props: { state: base } })
    expect(w.find('.ceb-dot--anxious').exists()).toBe(true)
  })
})
