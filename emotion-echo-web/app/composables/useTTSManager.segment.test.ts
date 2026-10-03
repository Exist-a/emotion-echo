/**
 * useTTSManager.segment.test.ts — F-134 首句切段接线（RED）
 *
 * 契约：攒到主标点的段**立即**发出（不再等 500ms debounce）——现状
 * debounce 被每个 SSE token 重置 ⇒ 整条回复流完才首次 TTS（首声延迟
 * = 全文 LLM + 全文合成）。切段后首声 = 首句 LLM + 首句合成。
 *
 * RED：useTTSManager 尚未接 takeSentenceSegment ⇒ 断言"立即调用"失败。
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const playTextSpy = vi.fn()
const flushSpy = vi.fn()
const stopSpy = vi.fn()
const setVoiceEnabledSpy = vi.fn()

vi.mock('./useDigitalHumanTTS', () => ({
  useDigitalHumanTTS: () => ({
    voiceEnabled: { value: true },
    playText: playTextSpy,
    flushRemaining: flushSpy,
    stop: stopSpy,
    setVoiceEnabled: setVoiceEnabledSpy,
  }),
}))

// 直接替换为不依赖切段实现的 stub？——不：本测试正是要断言切段接线，
// 故 import 真实 useTTSManager（RED 时它不切段 → 下面断言红）。
import { useTTSManager } from './useTTSManager'

beforeEach(() => {
  playTextSpy.mockClear()
  flushSpy.mockClear()
  stopSpy.mockClear()
  setVoiceEnabledSpy.mockClear()
  vi.useFakeTimers()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useTTSManager 首句切段（F-134）', () => {
  it('攒到主标点立即发出，不等 debounce', () => {
    const m = useTTSManager()
    m.playText('第一句说完了。')
    // RED：现状要等 500ms debounce（且被后续 token 重置）→ 未调用即红
    expect(playTextSpy).toHaveBeenCalledTimes(1)
    expect(playTextSpy).toHaveBeenCalledWith('第一句说完了。')
    vi.advanceTimersByTime(600)
    // 尾部无残留 → 不应二次触发
    expect(playTextSpy).toHaveBeenCalledTimes(1)
  })

  it('段后余量留到下个标点或 debounce 尾冲', () => {
    const m = useTTSManager()
    m.playText('第一句。第二句还')
    expect(playTextSpy).toHaveBeenCalledTimes(1)
    expect(playTextSpy).toHaveBeenCalledWith('第一句。')
    // 余量"第二句还"在 debounce 到期后尾冲
    vi.advanceTimersByTime(600)
    expect(playTextSpy).toHaveBeenCalledTimes(2)
    expect(playTextSpy).toHaveBeenLastCalledWith('第二句还')
  })

  it('无标点的整段走 debounce（旧行为保持）', () => {
    const m = useTTSManager()
    m.playText('没有标点的文本')
    expect(playTextSpy).not.toHaveBeenCalled()
    vi.advanceTimersByTime(600)
    expect(playTextSpy).toHaveBeenCalledTimes(1)
    expect(playTextSpy).toHaveBeenCalledWith('没有标点的文本')
  })

  it('stop 清空累积，不留脏段', () => {
    const m = useTTSManager()
    m.playText('半截句子')
    m.stop()
    vi.advanceTimersByTime(600)
    expect(playTextSpy).not.toHaveBeenCalled()
  })
})
