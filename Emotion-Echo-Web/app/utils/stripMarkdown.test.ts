import { describe, it, expect } from 'vitest'
import { stripMarkdown, isMostlyCode, extractReadableText } from './stripMarkdown'

describe('stripMarkdown', () => {
  it('returns empty string for falsy input', () => {
    expect(stripMarkdown('')).toBe('')
    // @ts-expect-error testing runtime guard
    expect(stripMarkdown(null)).toBe('')
  })

  it('removes fenced code blocks by default', () => {
    const text = 'before\n```js\nconst x = 1\n```\nafter'
    expect(stripMarkdown(text)).toBe('before\n\nafter')
  })

  it('keeps code blocks when option is disabled', () => {
    const text = '```js\nconst x = 1\n```'
    expect(stripMarkdown(text, { removeCodeBlocks: false })).toContain('const x = 1')
  })

  it('removes inline code', () => {
    expect(stripMarkdown('use `npm install` to install')).toBe('use to install')
  })

  it('strips markdown links but keeps the label', () => {
    expect(stripMarkdown('see [the docs](https://example.com)')).toBe('see the docs')
  })

  it('removes plain URLs', () => {
    expect(stripMarkdown('visit https://example.com today')).toBe('visit today')
  })

  it('strips headings, lists, and blockquotes', () => {
    expect(stripMarkdown('# Title\n- one\n- two\n> quote')).toBe('Title\none\ntwo\nquote')
  })

  it('removes images but keeps the alt text', () => {
    expect(stripMarkdown('![a happy dog](https://example.com/dog.png)')).toBe('a happy dog')
  })

  it('collapses repeated whitespace and trims', () => {
    expect(stripMarkdown('   lots   of   space   ')).toBe('lots of space')
  })
})

describe('isMostlyCode', () => {
  it('returns true for code-shaped text', () => {
    expect(isMostlyCode('function foo() { return 1 }')).toBe(true)
  })
  it('returns false for plain prose', () => {
    expect(isMostlyCode('我今天感觉很平静')).toBe(false)
  })
})

describe('extractReadableText', () => {
  it('returns empty when mostly code', () => {
    const text = '```js\nconst answer = 42\n```'
    expect(extractReadableText(text)).toBe('')
  })
  it('returns cleaned prose', () => {
    const text = '今天 **很棒** 的一天。'
    expect(extractReadableText(text)).toBe('今天 很棒 的一天。')
  })
})
