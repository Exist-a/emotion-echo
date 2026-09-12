// Stage 82 PR-3b：意图标签映射测试
import { describe, it, expect } from 'vitest'
import { getIntentLabel, INTENT_LABEL_MAP } from '~/utils/intent'

describe('getIntentLabel', () => {
  it('6 类意图全部有中文名', () => {
    const intents = ['emotional_support', 'study_help', 'tech_help', 'career_help', 'lifestyle', 'other']
    for (const intent of intents) {
      const label = getIntentLabel(intent)
      expect(label).toBeTruthy()
      expect(label).not.toBe('未分类')
    }
  })

  it('未知意图兜底"未分类"', () => {
    expect(getIntentLabel('')).toBe('未分类')
    expect(getIntentLabel('unknown_xyz')).toBe('未分类')
  })

  it('与后端白名单键一致（漂移防护）', () => {
    // chat-svc allowedIntents / llm intent.INTENTS 同键集
    expect(Object.keys(INTENT_LABEL_MAP).sort()).toEqual(
      ['career_help', 'emotional_support', 'lifestyle', 'other', 'study_help', 'tech_help', 'unk']
    )
  })
})
