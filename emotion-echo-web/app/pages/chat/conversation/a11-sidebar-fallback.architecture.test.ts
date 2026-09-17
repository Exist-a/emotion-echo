import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// Sprint 111 · A11 architecture regression (static-source)
//
// Sprint 110 浏览器实测发现 (2026-09-17 09:35, dev mode 演示账号):
//   sidebar 会话列表 21 个 item 全部显示空标题, aria-label "对「」更多操作".
//   <span class="item-label"></span> 完全空白.
// 真根因:
//   后端 chat-svc /conversations 返回 title="" + lastMessage=null
//   (chat-svc 在 SSE 流完后异步用 AI 生成 title, 入库前 list API 读不到),
//   前端 conversationItems computed 直接用 c.title 作为 label,
//   没有 fallback → sidebar 一片空白, 用户找不到会话.
//
// 修法: title?.trim() || lastMessage (截 30 字) || `对话 #{id}`
//
// 本测试钉住:
//   1) conversationItems computed 必须有 3 级 fallback
//   2) fallback 顺序: title → lastMessage → "对话 #{id}"

const indexSrc = readFileSync('./app/pages/chat/conversation/index.vue', 'utf8')

describe('Sprint 111 · A11 sidebar 会话标题 fallback (空 title 时不能显示空)', () => {
  it('conversationItems computed 必须用 c.title (而非 c.label)', () => {
    // 原代码 label: c.title — 留着这条, 验证 fallback chain 起点正确
    expect(indexSrc).toMatch(/label:\s*c\.title/)
  })

  it('必须有 lastMessage fallback (截 30 字符)', () => {
    // Sprint 111 · A11 修复: title 空时用 lastMessage slice(0,30)
    const hasLastMessageFallback =
      /label:\s*c\.title.*\n.*\|\|\s*\(?c\.lastMessage\?.*slice\(/.test(indexSrc) ||
      /label:\s*c\.title.*\|\|\s*\(?c\.lastMessage\?.*slice\(/.test(indexSrc) ||
      /c\.lastMessage\s*\?\s*String\(c\.lastMessage\)\.slice\(/.test(indexSrc)
    expect(
      hasLastMessageFallback,
      'Sprint 111 · A11 修复要求 conversationItems computed 在 title 为空时, ' +
        'fallback 用 lastMessage.slice(0, 30). 否则 sidebar 标题永远空.',
    ).toBe(true)
  })

  it('必须有 "对话 #{id}" 兜底 (title 和 lastMessage 都空时)', () => {
    // Sprint 111 · A11 修复兜底: 后端返回 title="" + lastMessage=null 时,
    // 至少显示 "对话 #125" 让用户能找到 session.
    const hasIdFallback = /对话\s*#\$\{?c\.id\}?/.test(indexSrc)
    expect(
      hasIdFallback,
      'Sprint 111 · A11 修复兜底: title 和 lastMessage 都空时, ' +
        '显示 `对话 #{c.id}`. 否则 sidebar 全空白用户找不到任何会话.',
    ).toBe(true)
  })

  it('A11 修复前 (回归保护): 不能只有 label: c.title 一行无 fallback', () => {
    // 反向钉死: 不准再写 `label: c.title` 不带 || fallback.
    // 在 conversationItems computed 内, label 表达式必须含 || 链.
    // 抽出 label: ... 这一行, 看是否含 ||
    const labelLineMatch = indexSrc.match(/label:\s*c\.title[^\n]*/)
    const labelLine = labelLineMatch?.[0] || ''
    // 如果 label 一行里就含了 ||, 视为有 fallback (跨行用 || 起始)
    // 否则视为单行无 fallback
    const hasFallbackOnSameLine = labelLine.includes('||')
    const hasFallbackAcrossLines =
      // 多行表达式, 第一行 c.title 后换行 + 下行含 ||
      /label:\s*c\.title[^\n]*\n\s*\|\|/.test(indexSrc)
    expect(
      hasFallbackOnSameLine || hasFallbackAcrossLines,
      'Sprint 111 · A11 反向钉死: conversationItems computed 的 label 必须含 || ' +
        'fallback chain (title → lastMessage → 对话 #id). ' +
        'Sprint 110 浏览器实测 21 个会话全空白即此 bug.',
    ).toBe(true)
  })
})
