import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// E2E-14 · 人格画像前端契约（static-source）
//
// 覆盖 D-02 的三个前端落点：
//   1. /question 列表按 category 分区（症状筛查 / 人格画像）
//   2. /question/[id] 结果弹窗对人格量表渲染雷达图而非"风险等级"
//   3. /chat/user 我的空间展示人格维度雷达图（读 /surveys/results）
//
// 用静态契约而非挂载测试：这三处都依赖 Nuxt 自动导入 + ECharts canvas，
// happy-dom 下挂载失真（见 memory: tdd-contract-vs-behavioral-tests）。

const listSrc = readFileSync('./app/pages/question/index.vue', 'utf8')
const detailSrc = readFileSync('./app/pages/question/[id].vue', 'utf8')
const userSrc = readFileSync('./app/pages/chat/user/index.vue', 'utf8')
const apiTypesSrc = readFileSync('./app/types/api.ts', 'utf8')
const apiRoutesSrc = readFileSync('./app/lib/apiRoutes.ts', 'utf8')

describe('E2E-14 · /question 列表按 category 分区', () => {
  it('存在两个 tab：症状筛查 + 人格画像', () => {
    expect(listSrc, '缺少"症状筛查" tab').toContain('症状筛查')
    expect(listSrc, '缺少"人格画像" tab').toContain('人格画像')
  })

  it('按 category === "personality" 分流，而非把两类混在一起展示', () => {
    // 原实现 `v-for="item in tableData"` 直接渲染全部 → 两类量表混排
    expect(
      /item\.category\s*===\s*['"]personality['"]/.test(listSrc),
      'E2E-14: 列表必须按 category === "personality" 区分两类量表（D-02 决议）',
    ).toBe(true)
    expect(
      listSrc.includes('v-for="item in tableData"'),
      'E2E-14: 不得直接遍历 tableData 渲染（会绕过 tab 筛选）',
    ).toBe(false)
  })

  it('category 显示为中文标签（后端返回的是 depression/anxiety/personality 等英文）', () => {
    expect(
      /personality:\s*['"]人格['"]/.test(listSrc),
      'E2E-14: badge 需把 category 英文值映射为中文可读标签',
    ).toBe(true)
  })
})

describe('E2E-14 · /question/[id] 结果弹窗按量表类型分流', () => {
  it('引入 RadarChart 组件', () => {
    expect(
      /import\s+RadarChart\s+from\s+['"]~\/components\/charts\/RadarChart\.vue['"]/.test(detailSrc),
      'E2E-14: 人格结果弹窗需要雷达图',
    ).toBe(true)
  })

  it('用 isPersonalityResult 判定，而非硬编码 riskLevel 字面量', () => {
    expect(
      detailSrc.includes('isPersonalityResult'),
      'E2E-14: 判定必须走 configs/personality 的 isPersonalityResult（单一事实源）',
    ).toBe(true)
    // 防漂移：页面内不得再出现裸的 dimension_profile 字面量比较
    expect(
      /riskLevel\s*===\s*['"]dimension_profile['"]/.test(detailSrc),
      'E2E-14: 不得在页面里硬编码 dimension_profile（应走 isPersonalityResult）',
    ).toBe(false)
  })

  it('人格量表不再展示"等级"（人格无风险等级概念）', () => {
    expect(
      detailSrc.includes('personalityDimensions'),
      'E2E-14: 人格结果需展示五维度明细',
    ).toBe(true)
    expect(
      detailSrc.includes('personalityRadarData'),
      'E2E-14: 人格结果需按维度定义顺序喂给雷达图',
    ).toBe(true)
  })
})

describe('E2E-14 · 我的空间人格画像', () => {
  it('引入 RadarChart 与人格配置', () => {
    expect(
      /import\s+RadarChart\s+from\s+['"]~\/components\/charts\/RadarChart\.vue['"]/.test(userSrc),
      'E2E-14: 我的空间需展示人格维度雷达图',
    ).toBe(true)
    expect(userSrc).toContain('personalityIndicators')
    expect(userSrc).toContain('personalityRadarData')
  })

  it('从 /surveys/results 取数（复用既有端点，不新造接口）', () => {
    expect(
      userSrc.includes('API_ROUTES.surveyResults.path'),
      'E2E-14: 人格画像取数走 API_ROUTES.surveyResults（GET /surveys/results）',
    ).toBe(true)
  })

  it('onMounted 必须调用取数函数（否则雷达图永远是空态）', () => {
    expect(
      /onMounted\([\s\S]*fetchPersonality\(\)/.test(userSrc),
      'E2E-14: fetchPersonality 必须在 onMounted 中被调用',
    ).toBe(true)
  })

  it('无人格结果时给出"去测评"引导而非静默空白', () => {
    expect(userSrc).toContain('去测评')
  })
})

describe('E2E-14 · 类型与路由契约', () => {
  it('SurveyResult 带 factorScores（维度分数）', () => {
    expect(
      /interface SurveyResult\s*\{[\s\S]*?factorScores\?:\s*Record<string,\s*number>/.test(apiTypesSrc),
      'E2E-14: SurveyResult.factorScores 必须声明（后端 SubmitSurveyResp/GetSurveyResultResp 已返回）',
    ).toBe(true)
  })

  it('apiRoutes 定义 surveyResults → GET /surveys/results', () => {
    expect(
      /surveyResults:\s*\{\s*method:\s*['"]GET['"],\s*path:\s*['"]\/surveys\/results['"]\s*\}/.test(
        apiRoutesSrc,
      ),
      'E2E-14: 需登记 surveyResults 路由（与 BFF survey_handler 的 /surveys/results 对齐）',
    ).toBe(true)
  })
})
