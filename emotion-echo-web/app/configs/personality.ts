// 人格五因素量表的展示契约（E2E-14）
//
// 与后端两处实现保持一致，改任一处必须同步改另两处：
//   - assessment-svc/internal/scoring/scorer.go  BigFiveScorer（维度→题号映射、反向题）
//   - emotion-echo-web-bff/internal/handler/ai_stream_handler.go  formatPersonalityContext（等级分档）
//
// 等级分档：每维度 6 题 × 1-5 分 ⇒ 6~30，18 为中性；≥23 高 / 14~22 中 / ≤13 低。
import type { RadarIndicator } from '~/types/charts/radarChartType'

/** 人格量表结果的 riskLevel 标记值（后端 BigFiveScorer 写入） */
export const PERSONALITY_RISK_LEVEL = 'dimension_profile'

/** 单个维度满分（6 题 × 5 分） */
export const PERSONALITY_DIMENSION_MAX = 30

/** 维度定义（顺序即雷达图顶点顺序，也是结果详情里的展示顺序） */
export const PERSONALITY_DIMENSIONS = [
  { key: 'openness', label: '开放性' },
  { key: 'conscientiousness', label: '尽责性' },
  { key: 'extraversion', label: '外向性' },
  { key: 'agreeableness', label: '宜人性' },
  { key: 'neuroticism', label: '神经质' },
] as const

/** 该结果是否来自人格量表（人格量表无"风险"概念，用标记值区分） */
export function isPersonalityResult(riskLevel: string | undefined): boolean {
  return riskLevel === PERSONALITY_RISK_LEVEL
}

/** 维度分档（与 BFF formatPersonalityContext 同规则） */
export function personalityLevel(score: number): '高' | '中' | '低' {
  if (score >= 23) return '高'
  if (score <= 13) return '低'
  return '中'
}

/** 雷达图指标 */
export function personalityIndicators(): RadarIndicator[] {
  return PERSONALITY_DIMENSIONS.map((d) => ({
    name: d.label,
    max: PERSONALITY_DIMENSION_MAX,
  }))
}

/**
 * 雷达图数据（按 PERSONALITY_DIMENSIONS 顺序取值）。
 * 缺失维度取 0 —— 雷达图必须与指标数量一致，否则渲染错位；
 * 是否展示由调用方用 hasPersonalityScores 判断。
 */
export function personalityRadarData(factorScores?: Record<string, number>): number[] {
  return PERSONALITY_DIMENSIONS.map((d) => factorScores?.[d.key] ?? 0)
}

/** 维度分是否可用（非空且含至少一个维度） */
export function hasPersonalityScores(factorScores?: Record<string, number>): boolean {
  if (!factorScores) return false
  return PERSONALITY_DIMENSIONS.some((d) => typeof factorScores[d.key] === 'number')
}
