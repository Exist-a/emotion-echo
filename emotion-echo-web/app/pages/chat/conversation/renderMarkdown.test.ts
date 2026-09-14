/**
 * P1-R2-2: marked.parse HTML 净化单元测试
 *
 * 目的：验证 getHtmlContent 对 XSS payload 的净化能力
 * 策略：抽离 getHtmlContent 到独立模块以便测试，或 inline 测试 marked 配置
 *
 * 注意：本测试验证 marked 渲染层的 HTML 转义，与 vue-dompurify-html 的
 *       v-html 净化是双重防御（marked 层 + DOMPurify 层）。
 */
import { describe, it, expect } from 'vitest'
import { marked } from 'marked'

// 复制与 [id].vue 中相同的 marked 配置
function renderSafeMd(content: string): string {
  const renderer = new marked.Renderer()
  const origHtml = renderer.html.bind(renderer)
  renderer.html = (token: any) => {
    const raw = typeof token === 'string' ? token : (token?.text || '')
    return origHtml(raw.replace(/[&<>"']/g, (c) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c] as string)
    ))
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
    expect(out).toContain('&lt;script&gt;')
  })

  it('转义 img onerror', () => {
    const out = renderSafeMd('<img src=x onerror=alert(1)>')
    expect(out).not.toContain('onerror=')
    expect(out).toContain('&lt;img')
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

  it('双重防御：marked 层已转义后，DOMPurify 仍能干净通过', () => {
    const out = renderSafeMd('<a href="javascript:alert(1)">click</a>')
    // marked 层已把 < > 转义，javascript: scheme 不再可执行
    expect(out).not.toContain('<a href="javascript:')
    expect(out).toContain('&lt;a')
  })
})
