/**
 * takeSentenceSegment — 流式文本切段（E2E-28 C4 / D-43 F-134 首句切段）
 *
 * 从累积的 SSE 文本里切出"可独立送 TTS 的一段"，让首声延迟从
 * "全文 LLM + 全文合成" 降到 "首句 LLM + 首句合成"（F-134）。
 *
 * 切段规则（契约见 takeSentenceSegment.test.ts）：
 *   1. 主标点（。！？!?；;…）→ 切至（含）该标点
 *   2. 无主标点但超 maxLen → 在 maxLen 内回退到最后一个逗号切
 *   3. 无任何标点且超 maxLen → maxLen 硬切（默认 80 < XTTS zh 82 上限）
 *   4. 不足一段 → null（交给 useTTSManager 的 500ms debounce 尾冲）
 */

const MAJOR_PUNCT = /[。！？!?；;…]/
const COMMA_PUNCT = /[，,]/

export interface SegmentTake {
  flush: string
  rest: string
}

function splitAt(text: string, idx: number): SegmentTake | null {
  const flush = text.slice(0, idx).trim()
  if (!flush) return null
  return { flush, rest: text.slice(idx).trimStart() }
}

export function takeSentenceSegment(
  accumulated: string,
  maxLen = 80,
): SegmentTake | null {
  if (!accumulated.trim()) return null

  const majorIdx = accumulated.search(MAJOR_PUNCT)
  if (majorIdx >= 0) {
    return splitAt(accumulated, majorIdx + 1)
  }

  if (accumulated.length > maxLen) {
    const lastComma = Math.max(
      accumulated.lastIndexOf('，', maxLen - 1),
      accumulated.lastIndexOf(',', maxLen - 1),
    )
    const cut = lastComma >= 0 ? lastComma + 1 : maxLen
    return splitAt(accumulated, cut)
  }

  return null
}
