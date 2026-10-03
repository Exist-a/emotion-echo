/**
 * takeSentenceSegment.test.ts — E2E-28 C4 / D-43 F-134 首句切段（RED）
 *
 * 背景（账本 E2E-F-134 + useTTSManager 代码回读）：
 *   现状 playText 用 500ms debounce 攒全量文本 —— SSE token 间隔 <500ms，
 *   debounce 被不断重置 ⇒ **整条 AI 回复流完 + 500ms 才发起第一次 TTS**
 *   ⇒ 首声延迟 = 全文 LLM 时间 + 全文合成时间（#10 实测 40 字热 13~30s，
 *   长回复更长）。
 *   F-134（D-43 裁定"全做"之一）：攒到标点即切段发出，首段（首句）即可
 *   独立合成 → 首声延迟 = 首句 LLM + 首句合成，其余段流式跟上。
 *
 * 切段契约（takeSentenceSegment）：
 *   1. 主标点（。！？!?；;…）出现 → 切至（含）该标点
 *   2. 无主标点但超 maxLen → 在 maxLen 内回退到最后一个逗号（，,）切
 *   3. 无任何标点且超 maxLen → maxLen 硬切（防 XTTS 82 字符上限截断告警）
 *   4. 不足一段 → null（留给 debounce 尾冲）
 *   5. flush 左右两侧 trim；flush 为空 → null
 *
 * RED：模块不存在 → import 失败。
 */
import { describe, it, expect } from 'vitest'
import { takeSentenceSegment } from './takeSentenceSegment'

describe('takeSentenceSegment', () => {
  it('主标点出现即切（含标点）', () => {
    expect(takeSentenceSegment('你好。')).toEqual({ flush: '你好。', rest: '' })
  })

  it('只切第一段，剩余留给下次', () => {
    expect(takeSentenceSegment('你好，世界。明')).toEqual({
      flush: '你好，世界。',
      rest: '明',
    })
  })

  it('无主标点且超 maxLen → 回退到 maxLen 内最后逗号', () => {
    const head = '一'.repeat(70) + '，' + '二'.repeat(70)
    const r = takeSentenceSegment(head, 80)!
    expect(r.flush.endsWith('，')).toBe(true)
    expect(r.flush.length).toBeLessThanOrEqual(81)
    expect(r.rest.length).toBeGreaterThan(0)
  })

  it('无任何标点且超 maxLen → 硬切 maxLen（防 XTTS 82 上限）', () => {
    const s = '零'.repeat(100)
    const r = takeSentenceSegment(s, 80)!
    expect(r.flush.length).toBe(80)
    expect(r.rest.length).toBe(20)
  })

  it('不足一段 → null（交给 debounce 尾冲）', () => {
    expect(takeSentenceSegment('你好')).toBeNull()
    expect(takeSentenceSegment('')).toBeNull()
    expect(takeSentenceSegment('   ')).toBeNull()
  })

  it('分号/省略号也是主标点', () => {
    expect(takeSentenceSegment('想一想；做一做')?.flush).toBe('想一想；')
    expect(takeSentenceSegment('这很难…然后呢')?.flush).toBe('这很难…')
  })

  it('flush 结果不带前后空白', () => {
    const r = takeSentenceSegment('  你好。  明天')!
    expect(r.flush).toBe('你好。')
  })
})
