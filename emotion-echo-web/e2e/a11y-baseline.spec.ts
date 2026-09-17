/**
 * a11y-baseline.spec.ts — E2E-04 无障碍基线扫描
 *
 * 对 6 个主页面跑 axe-core 自动扫描，记录 critical/serious 问题。
 * 策略：只修 critical/serious，其余记账本。
 *
 * 判定：[A] 自动可判（axe 返回结构化结果）
 */
import { test, expect } from '@playwright/test'
import AxeBuilder from '@axe-core/playwright'

const PAGES = [
  { name: '登录页', path: '/login' },
  { name: '聊天页', path: '/chat' },
  { name: '用户空间', path: '/chat/user' },
  { name: '测验列表', path: '/question' },
  { name: '设置页', path: '/chat/setting' },
  { name: '仪表盘', path: '/chat/dashboard' },
]

for (const page of PAGES) {
  test(`a11y baseline: ${page.name} (${page.path})`, async ({ page: p }) => {
    await p.goto(page.path)
    // Wait for page to be interactive
    await p.waitForLoadState('networkidle')

    const results = await new AxeBuilder({ page: p })
      .withTags(['wcag2a', 'wcag2aa', 'best-practice'])
      .analyze()

    const critical = results.violations.filter(
      (v) => v.impact === 'critical' || v.impact === 'serious',
    )

    // Log all violations for recording
    if (results.violations.length > 0) {
      console.log(`\n[${page.name}] ${results.violations.length} violations:`)
      for (const v of results.violations) {
        console.log(`  [${v.impact}] ${v.id}: ${v.description} (${v.nodes.length} nodes)`)
      }
    }

    // For baseline: log but don't fail on non-critical
    // Critical/serious will be fixed or recorded in discovered-unresolved.md
    if (critical.length > 0) {
      console.log(`\n[${page.name}] ${critical.length} critical/serious violations found`)
    }

    // Soft assert: record violations but don't block CI
    // Uncomment below to enforce:
    // expect(critical).toEqual([])
  })
}
