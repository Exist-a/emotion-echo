// E2E-14：人格量表展示契约测试
//
// 这些常量/函数被三处消费：/question 列表分 tab、结果弹窗、我的空间雷达图。
// 分档边界与后端 BFF formatPersonalityContext 必须一致 —— 否则用户看到的等级
// 与注入 AI 的画像等级会不同（同一份数据两套说法）。
import { describe, it, expect } from 'vitest'
import {
  PERSONALITY_DIMENSIONS,
  PERSONALITY_DIMENSION_MAX,
  PERSONALITY_RISK_LEVEL,
  hasPersonalityScores,
  isPersonalityResult,
  personalityIndicators,
  personalityLevel,
  personalityRadarData,
} from './personality'

describe('personality 展示契约', () => {
  it('riskLevel 标记值锁定为 dimension_profile（与 Go BigFiveScorer 一致）', () => {
    expect(PERSONALITY_RISK_LEVEL).toBe('dimension_profile')
  })

  it('五个维度齐全且 label 与后端画像文本一致', () => {
    expect(PERSONALITY_DIMENSIONS.map((d) => d.key)).toEqual([
      'openness',
      'conscientiousness',
      'extraversion',
      'agreeableness',
      'neuroticism',
    ])
    expect(PERSONALITY_DIMENSIONS.map((d) => d.label)).toEqual([
      '开放性',
      '尽责性',
      '外向性',
      '宜人性',
      '神经质',
    ])
  })

  it('单维度满分 = 6 题 × 5 分 = 30', () => {
    expect(PERSONALITY_DIMENSION_MAX).toBe(30)
  })

  describe('isPersonalityResult', () => {
    it('标记值判定为人格结果', () => {
      expect(isPersonalityResult('dimension_profile')).toBe(true)
    })

    it('症状量表的 none/mild/moderate/severe/extreme 均不为人格结果', () => {
      for (const lvl of ['none', 'mild', 'moderate', 'severe', 'extreme']) {
        expect(isPersonalityResult(lvl)).toBe(false)
      }
    })

    it('空值/undefined 不为人格结果', () => {
      expect(isPersonalityResult('')).toBe(false)
      expect(isPersonalityResult(undefined)).toBe(false)
    })
  })

  describe('personalityLevel 分档（6~30，18 中性）', () => {
    it('>=23 为高', () => {
      expect(personalityLevel(23)).toBe('高')
      expect(personalityLevel(30)).toBe('高')
    })

    it('14~22 为中', () => {
      expect(personalityLevel(14)).toBe('中')
      expect(personalityLevel(18)).toBe('中')
      expect(personalityLevel(22)).toBe('中')
    })

    it('<=13 为低', () => {
      expect(personalityLevel(13)).toBe('低')
      expect(personalityLevel(6)).toBe('低')
    })
  })

  describe('personalityIndicators', () => {
    it('五个指标，每个 max=30，name 用中文标签', () => {
      const indicators = personalityIndicators()
      expect(indicators).toHaveLength(5)
      for (const ind of indicators) {
        expect(ind.max).toBe(30)
        expect(ind.name.length).toBeGreaterThan(0)
      }
      expect(indicators.map((i) => i.name)).toEqual([
        '开放性',
        '尽责性',
        '外向性',
        '宜人性',
        '神经质',
      ])
    })
  })

  describe('personalityRadarData', () => {
    it('按维度定义顺序取值', () => {
      const data = personalityRadarData({
        openness: 26,
        conscientiousness: 18,
        extraversion: 24,
        agreeableness: 19,
        neuroticism: 10,
      })
      expect(data).toEqual([26, 18, 24, 19, 10])
    })

    it('缺失维度补 0（雷达图长度必须与指标一致，否则渲染错位）', () => {
      const data = personalityRadarData({ openness: 20 })
      expect(data).toHaveLength(5)
      expect(data[0]).toBe(20)
      expect(data.slice(1)).toEqual([0, 0, 0, 0])
    })

    it('undefined 返回全 0 数组而非抛错', () => {
      expect(personalityRadarData(undefined)).toEqual([0, 0, 0, 0, 0])
    })
  })

  describe('hasPersonalityScores', () => {
    it('含任一维度分即为 true', () => {
      expect(hasPersonalityScores({ openness: 20 })).toBe(true)
    })

    it('空对象/undefined 为 false', () => {
      expect(hasPersonalityScores({})).toBe(false)
      expect(hasPersonalityScores(undefined)).toBe(false)
    })
  })
})
