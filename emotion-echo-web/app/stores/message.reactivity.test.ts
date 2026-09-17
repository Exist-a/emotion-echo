import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// Sprint 110 · A8 修复: messageStore.addMessage / sendMessage 内
// 必须用 spread 赋值 (currentMessages.value = [...currentMessages.value, x])
// 而非 .push(), 以确保 Vue 3 reactive proxy 触发响应式更新
//
// IAB Playwright 2026-09-17 实测: .push() 在 Nuxt 3 + pinia + 跨组件实例场景下
// 偶发不触发响应式, 导致 [id].vue 的 v-for="item in data" 不更新
// → AI 回复不渲染 (Sprint 109b stage 已确认)
//
// 本测试钉住:
const messageStoreSrc = readFileSync('./app/stores/message.ts', 'utf8')

describe('messageStore · A8 响应式修复 (Sprint 110)', () => {
  it('addMessage MUST use spread assignment NOT .push() (Vue 3 reactive 响应式保险)', () => {
    // 抽 addMessage 函数体
    const start = messageStoreSrc.indexOf('const addMessage =')
    expect(start, 'addMessage 函数必须存在').toBeGreaterThan(-1)
    const end = messageStoreSrc.indexOf('\n  const ', start + 1)
    const block = messageStoreSrc.slice(start, end === -1 ? messageStoreSrc.length : end)

    const usesSpread = /currentMessages\.value\s*=\s*\[[\s\S]*?\.\.\.currentMessages\.value/.test(block)
    const usesPush = /currentMessages\.value\.push\(/.test(block)
    expect(
      usesSpread && !usesPush,
      'addMessage 必须用 spread 赋值 (currentMessages.value = [...currentMessages.value, msg]) ' +
        '而不是 .push() —— 后者在 Nuxt 3 + pinia + 跨组件实例下偶发不触发响应式, 导致 AI 回复不渲染.',
    ).toBe(true)
  })

  it('sendMessage MUST use spread assignment NOT .push() (Vue 3 reactive 响应式保险)', () => {
    // 抽 sendMessage 函数体
    const start = messageStoreSrc.indexOf('const sendMessage =')
    expect(start, 'sendMessage 函数必须存在').toBeGreaterThan(-1)
    const end = messageStoreSrc.indexOf('\n  const ', start + 1)
    const block = messageStoreSrc.slice(start, end === -1 ? messageStoreSrc.length : end)

    const usesSpread = /currentMessages\.value\s*=\s*\[[\s\S]*?\.\.\.currentMessages\.value/.test(block)
    const usesPush = /currentMessages\.value\.push\(/.test(block)
    expect(
      usesSpread && !usesPush,
      'sendMessage 内 push user 消息也必须改 spread 赋值, 避免 .push() 不触发响应式导致 UI 不更新.',
    ).toBe(true)
  })
})
