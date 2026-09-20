import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// E2E-F-97 前端契约钉：答题页必须提交 **option.score**，不能提交 option.id。
//
// 为什么用静态源断言而不是挂载测试：`[id].vue` 依赖 Nuxt 自动导入 + 路由 +
// 真实网络调用，happy-dom 下挂载会失真（见 memory: tdd-contract-vs-behavioral-tests）。
// 真实行为由 Playwright `e2e/survey-scoring.spec.ts` 驱动 UI 守（含精确取值与 400 场景）。

const detailSrc = readFileSync('./app/pages/question/[id].vue', 'utf8')

describe('E2E-F-97 · 答题提交必须按 score 而非 id', () => {
  it('提交构建逻辑必须取 option.score', () => {
    expect(
      /answers\[q\.id\]\s*=\s*opt\.score|answers\[q\.id\]\s*=\s*opt\?\.score/.test(detailSrc),
      'E2E-F-97: 提交值必须来自 option.score —— 后端 answers 的契约是 map[questionId]score。' +
        '原实现提交 option.id，因 PHQ-9/GAD-7 种子数据 id = score + 1（1..4 vs 0..3）而系统性虚高 +1/题，' +
        '选最后一档(id=4)还会被 scorer 判为超出 score 值域直接 400。',
    ).toBe(true)
  })

  it('不得把 answerMap 的选项 id 直接当作提交值', () => {
    // 原写法：Object.entries(answerMap.value).forEach(([qId, optId]) => { answers[qId] = optId })
    expect(
      /answers\[\s*qId\s*\]\s*=\s*optId/.test(detailSrc),
      'E2E-F-97: 不得再把 option.id（optId）直接写进 answers —— 那是 id 不是 score。',
    ).toBe(false)
  })

  it('必须通过 id → option 反查拿到 score（而非假定二者相等）', () => {
    expect(
      /find\(\s*\(?o\)?\s*=>\s*o\.id\s*===\s*selectedId\s*\)/.test(detailSrc),
      'E2E-F-97: 需显式反查选中项（answerMap 存 id 用于 UI 高亮，提交时映射为 score），' +
        '不能在两个语义之间做隐式假设。',
    ).toBe(true)
  })
})
