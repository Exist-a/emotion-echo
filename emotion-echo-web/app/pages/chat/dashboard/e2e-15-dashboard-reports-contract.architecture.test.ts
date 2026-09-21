import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// E2E-15 · 报表 Dashboard architecture regression (static-source)
//
// 代码现状调查（2026-09-21）发现 4 个 dashboard 页面（dailyReport / weeklyReport /
// monthlyReport / annualReport）共有的 1 个前端缺陷：
//
// 1. `<div class="ee-empty">暂无数据</div>` 完全没有 v-if / v-else-if 条件
//    ⇒ 即使 chartData.length > 0（有图表），「暂无数据」占位符仍然渲染，
//    与 chartCard 同屏出现 —— 与 E2E-F-14 my-space 页面空态恒渲染同型 bug。
//
// 本测试钉死上述修复，防止回归。

const DASHBOARD_FILES = [
  'dailyReport.vue',
  'weeklyReport.vue',
  'monthlyReport.vue',
  'annualReport.vue',
] as const

describe('E2E-15 · 报表 Dashboard 前端契约（防 E2E-F-14 同型残留）', () => {
  for (const file of DASHBOARD_FILES) {
    const path = `./app/pages/chat/dashboard/${file}`
    const src = readFileSync(path, 'utf8')

    describe(file, () => {
      // === 1. ee-empty 必须有 v-else-if 条件渲染（chartData.length === 0） ===
      it('ee-empty 必须有 v-else-if="chartData.length === 0" 条件（防 chartCard + 暂无数据同屏）', () => {
        expect(
          /v-else-if="chartData\.length\s*===\s*0"[\s\S]{0,120}class="ee-empty"/.test(src),
          `原 <div class="ee-empty">暂无数据</div> 无 v-if，与 <chartCard v-if="chartData.length > 0"> 同屏显示。` +
            `修复要求 v-else-if="chartData.length === 0" + class="ee-empty"。`,
        ).toBe(true)
      })

      // === 2. chartCard 必须仍用 v-if="chartData.length > 0" 守卫 ===
      it('chartCard 必须用 v-if="chartData.length > 0" 守卫', () => {
        // 模板顺序：<chartCard v-if="chartData.length > 0" :data="chartData" />
        // 正则匹配 <chartCard 后 80 字符内出现 v-if
        expect(
          /<chartCard[\s\S]{0,120}v-if="chartData\.length\s*>\s*0"/.test(src),
          `chartCard 必须用 v-if="chartData.length > 0" 条件渲染（与 ee-empty 互斥）`,
        ).toBe(true)
      })

      // === 3. ee-empty 不得有残留的 v-if 单独守卫（必须 v-else-if） ===
      it('ee-empty 不得是 v-if 单独条件（必须是 v-else-if 与 chartCard 互斥）', () => {
        // 避免未来有人写成 v-if="!showChart" + ee-empty 同样问题
        expect(
          /v-if="[\s\S]{0,40}class="ee-empty"/.test(src),
          `ee-empty 不应是 v-if 单独条件，应是 v-else-if 与 chartCard 互斥`,
        ).toBe(false)
      })
    })
  }
})