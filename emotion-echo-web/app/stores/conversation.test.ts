import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// static-source contract: 不用 vite 运行 store 整链路(后者会触发 Nuxt alias cascade),
// 直接断言源码文案的"非空"合同即可覆盖 Bug#1 的回归。
// vitest 把 import.meta.url 暴露为 file:// URL;直接用 ROOT-relative path。
const src = readFileSync('./app/stores/conversation.ts', 'utf8')


// 抽出 togglePinConversation 块的源码
function togglePinBlock(): string {
  const start = src.indexOf('const togglePinConversation')
  // 找匹配的右括号深度跟踪
  const endIdx = src.indexOf('const deleteConversation', start)
  return src.slice(start, endIdx === -1 ? src.length : endIdx)
}
function deleteBlock(): string {
  const start = src.indexOf('const deleteConversation')
  const endIdx = src.indexOf('const setCurrentConversation', start)
  return src.slice(start, endIdx === -1 ? src.length : endIdx)
}

describe('useConversationStore · notify 文案合同(static-source)', () => {
  it('togglePinConversation 成功路径不传空 title/message', () => {
    const block = togglePinBlock()
    // 至少要有一行 notify 调用,带非空 title 与含'置顶'/'取消'语义的 message
    const notifyCalls = block.match(/notify\([^)]*\)/g) ?? []
    expect(notifyCalls.length).toBeGreaterThanOrEqual(1)
    // 任意一行 notify 调用都不能是 notify('','','success',3000) 这种双重空
    const allCalls = notifyCalls.join('\n')
    expect(allCalls).not.toMatch(/notify\(\s*['"`]\s*['"`]\s*,\s*['"`]\s*['"`]\s*,\s*['"`](?:success|info|error|warning)['"`]/)
    // success 分支内 message 应出现"置顶"或"取消置顶" 字眼
    expect(allCalls).toMatch(/置顶|取消置顶/)
  })

  it('togglePinConversation 的 title 字段不应为空字符串', () => {
    const block = togglePinBlock()
    const notifyCalls = block.match(/notify\([^)]*\)/g) ?? []
    const bad = notifyCalls.find(c => /notify\(\s*['"`]\s*['"`]\s*,/.test(c))
    expect(bad).toBeUndefined()
  })

  it('deleteConversation 必须非空 title + 含"删除" message', () => {
    const block = deleteBlock()
    const notifyCalls = block.match(/notify\([^)]*\)/g) ?? []
    expect(notifyCalls.length).toBeGreaterThanOrEqual(1)
    const allCalls = notifyCalls.join('\n')
    expect(allCalls).not.toMatch(/notify\(\s*['"`]\s*['"`]\s*,\s*['"`]\s*['"`]\s*,\s*['"`](?:success|info|error|warning)['"`]/)
    expect(allCalls).toMatch(/删除/)
  })

  it('deleteConversation 的 title 字段不应为空字符串', () => {
    const block = deleteBlock()
    const notifyCalls = block.match(/notify\([^)]*\)/g) ?? []
    const bad = notifyCalls.find(c => /notify\(\s*['"`]\s*['"`]\s*,/.test(c))
    expect(bad).toBeUndefined()
  })
})
