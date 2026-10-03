/**
 * useConversationSender.f134Segment.test.ts — F-134 上游第二层 debounce 红线（RED）
 *
 * 背景（E2E-28 #13 运行时实测 2026-10-03）：
 *   manager 层（useTTSManager）已按标点切段，但 **sender 的 onDelta 自带
 *   第二个 500ms debounce**：SSE token 间隔 <500ms ⇒ debounce 被持续重置
 *   ⇒ 流中永不触发，全部积压到 onFinish flushTTS —— 实测
 *   `ttsFirstMs == sseFinishMs == 6785`（同刻！）F-134 的"首声提前"收益
 *   被上游完全挡住，manager 切段只剩"结束时把全文拆多段"的排队收益。
 *
 * 契约（静态源码范式，同 createFlow.test.ts 先例）：
 *   1. sender 必须 import takeSentenceSegment
 *   2. onDelta 内必须在 debounce 之前判"已攒出可切段"→ 立即
 *      playText + 清空 accumulated（不再等 500ms）
 *   3. 500ms debounce 尾冲**保留**（无标点余量/流尾仍靠它）
 *
 * RED：当前 sender 源码无 takeSentenceSegment ⇒ 断言 1/2 失败。
 */
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

const senderSrc = readFileSync('./app/composables/useConversationSender.ts', 'utf8')

describe('useConversationSender · F-134 流中标点即发（第二层 debounce 修复）', () => {
  it('必须 import takeSentenceSegment', () => {
    expect(
      /import\s*\{[^}]*takeSentenceSegment[^}]*\}\s*from\s*['"][^'"]*takeSentenceSegment['"]/.test(
        senderSrc,
      ),
      'sender 未引入 takeSentenceSegment —— onDelta 无法在流中判"已攒出整段"',
    ).toBe(true)
  })

  it('onDelta 内必须有"可切段即立即 playText + 清空"分支（在 500ms debounce 之外）', () => {
    const idx = senderSrc.indexOf('onDelta:')
    expect(idx, 'sender 必须有 onDelta 回调').toBeGreaterThan(-1)
    // 取 onDelta 回调体（到下一个顶层 onDelta/onFinish 之前的窗口）
    const finIdx = senderSrc.indexOf('onFinish:', idx)
    const block = senderSrc.slice(idx, finIdx > idx ? finIdx : idx + 1500)

    expect(
      block.includes('takeSentenceSegment('),
      'onDelta 体内未调用 takeSentenceSegment —— #13 实测 ttsFirst==sseFinish（流中永不发）',
    ).toBe(true)
    expect(
      /takeSentenceSegment\([^)]*\)[\s\S]{0,200}?playText\(/.test(block),
      '判出可切段后必须立即 playText（不得等 debounce）',
    ).toBe(true)
    expect(
      /playText\(accumulatedDeltaText\.value\)\s*\n\s*accumulatedDeltaText\.value = ''/.test(block),
      '立即发出后必须清空 accumulatedDeltaText（否则 manager 收到重复累积文本）',
    ).toBe(true)
  })

  it('500ms debounce 尾冲必须保留（无标点余量/流尾）', () => {
    expect(
      // 窗口 400：GREEN 后 debounce 分支缩进加深（else 嵌套），200 会顶爆
      /ttsDebounceTimer\s*=\s*setTimeout\([\s\S]{0,400}?,\s*500\)/.test(senderSrc),
      'debounce 尾冲被删 —— 无标点文本将永远不发 TTS',
    ).toBe(true)
  })

  it('onFinish 的 flushTTS 兜底必须保留（流尾余量 + flushRemaining 清队）', () => {
    // 防"只做了 onDelta 分支、把流末兜底删了"的半边改动：流在标点中间
    // 断掉（异常/中断）时，残留文本只能靠 onFinish flushTTS 发出。
    const finIdx = senderSrc.indexOf('onFinish:')
    expect(finIdx, 'onFinish 回调必须存在').toBeGreaterThan(-1)
    const finBlock = senderSrc.slice(finIdx, finIdx + 400)
    expect(
      /flushTTS\(\)/.test(finBlock),
      'onFinish 必须调用 flushTTS（残留 accumulated + flushRemaining 兜底）',
    ).toBe(true)
    expect(
      /playText\(accumulatedDeltaText\.value\)/.test(senderSrc.slice(0, senderSrc.indexOf('onDelta:'))),
      'flushTTS 定义内必须保留 playText 调用',
    ).toBe(true)
  })
})
