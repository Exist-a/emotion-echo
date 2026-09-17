/**
 * P1-R2-2: marked.parse HTML 净化单元测试
 *
 * 目的：验证 getHtmlContent 对 XSS payload 的净化能力
 * 策略：抽离 getHtmlContent 到独立模块以便测试，或 inline 测试 marked 配置
 *
 * 注意：本测试验证 marked 渲染层的 HTML 转义，与 vue-dompurify-html 的
 *       v-html 净化是双重防御（marked 层 + DOMPurify 层）。
 *
 * Stage 101 follow-up：marked v18 的 renderer.html(token) 接收 Tokens.HTML
 * 对象（不是 string），token.text 字段是原始 HTML 字符串。本测试的 inline
 * renderer 改用 token.text 转义后再 wrap，避免 v18 之前与之后 API 签名差异。
 */
import { describe, it, expect } from 'vitest'
import { marked, type Tokens } from 'marked'

// 复制与 [id].vue 中相同的 marked 配置 + 适配 marked v18 token API
function renderSafeMd(content: string): string {
  const renderer = new marked.Renderer()
  // marked v18：renderer.html 接收 Tokens.HTML 对象，text 字段是 HTML 字符串
  // v18 把 < > 等特殊字符 escape 一遍后直接拼回 output；为防御 XSS，在这一层
  // 再次 escape token.text（双重 escape 等价于"任何 < 都被转义"）。
  renderer.html = (token: Tokens.HTML | Tokens.Tag) => {
    const raw = (token as any)?.text ?? ''
    const escapeMap: Record<string, string> = { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }
    return raw.replace(/[&<>"']/g, (c: string) => escapeMap[c] ?? c)
  }
  const result = marked.parse(content, {
    gfm: true,
    breaks: true,
    async: false,
    renderer,
  } as any)
  return typeof result === 'string' ? result : String(result)
}

describe('P1-R2-2 marked HTML sanitization', () => {
  it('转义 script 标签（不再作为 HTML 解析）', () => {
    const out = renderSafeMd('hello <script>alert(1)</script>')
    expect(out).not.toContain('<script>')
    // 至少一处 < 被转义（&lt; 存在）
    expect(out).toMatch(/&lt;/)
  })

  it('转义 img onerror', () => {
    const out = renderSafeMd('<img src=x onerror=alert(1)>')
    // 关键：浏览器不再解析为 <img 元素，onerror 不会触发
    expect(out).not.toContain('<img')
    expect(out).toMatch(/&lt;/)
  })

  it('保留 markdown 强调语法（粗体/斜体/代码）', () => {
    const out = renderSafeMd('**bold** and *italic* and `code`')
    expect(out).toContain('<strong>bold</strong>')
    expect(out).toContain('<em>italic</em>')
    expect(out).toContain('<code>code</code>')
  })

  it('保留 GFM 表格与换行', () => {
    const out = renderSafeMd('a|b\n--|--\n1|2')
    expect(out).toContain('<table>')
    expect(out).toContain('<th>a</th>')
  })

  it('攻击者注入 mermaid script 不被执行', () => {
    const payload = '<script src="http://evil/x.js"></script>正常文本'
    const out = renderSafeMd(payload)
    expect(out).not.toMatch(/<script[^>]*src/i)
    expect(out).toContain('正常文本')
  })

  it('双重防御：marked 层已转义后，javascript: scheme 不再可执行', () => {
    const out = renderSafeMd('<a href="javascript:alert(1)">click</a>')
    // marked 层已把 < > 转义，javascript: scheme 不再可执行
    expect(out).not.toContain('<a href="javascript:')
    expect(out).toMatch(/&lt;/)
  })
})
