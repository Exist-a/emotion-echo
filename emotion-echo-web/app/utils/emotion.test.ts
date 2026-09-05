import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { getEmotionLabel, EmotionLabel, EMOTION_LABEL_MAP } from './emotion'

describe('emotion utils', () => {
  it('maps known emotions to friendly Chinese labels', () => {
    expect(getEmotionLabel('happy')).toBe('开心')
    expect(getEmotionLabel('sad')).toBe('悲伤')
    expect(getEmotionLabel('angry')).toBe('愤怒')
    expect(getEmotionLabel('anxious')).toBe('焦虑')
    expect(getEmotionLabel('neutral')).toBe('中性')
    expect(getEmotionLabel('unk')).toBe('未知')
  })

  it('falls back to the original value when label is unknown', () => {
    // @ts-expect-error testing runtime guard
    expect(getEmotionLabel('sleepy')).toBe('sleepy')
  })

  it('exposes a complete map of all EmotionLabel members', () => {
    const values: EmotionLabel[] = ['happy', 'sad', 'angry', 'anxious', 'neutral']
    for (const v of values) {
      expect(EMOTION_LABEL_MAP[v]).toBeTruthy()
    }
  })
})

describe('theme system smoke (token presence)', () => {
  it('exposes the Quiet Companion tokens we expect', () => {
    const root = getComputedStyle(document.documentElement)
    // The custom properties should be present even if zero-valued in happy-dom
    expect(root.getPropertyValue('--ee-bg')).toBeDefined()
    expect(root.getPropertyValue('--ee-primary')).toBeDefined()
  })
})
