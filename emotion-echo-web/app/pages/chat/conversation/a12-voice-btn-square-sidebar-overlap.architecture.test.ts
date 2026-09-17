import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// Sprint 111 · A12 architecture regression (static-source)
//
// 用户 2026-09-17 反馈 (内嵌浏览器实测):
// 1) voice-btn 是方形 (不是圆形)
// 2) sidebar 折叠按钮 (‹) 和展开按钮 (›) 同时渲染, 位置重叠
// 3) sidebar 加号 (+) 按钮不是中轴对齐
// 4) 字符 (+/›/‹) 视觉重心偏下 — 字体 ascent 偏移, CSS 修不到完美
//
// 真根因 (修 1-3):
// 1) voice-btn 覆盖了 .icon-btn 的 border-radius: 50% 但没补回 → 方形
// 2) fold-btn 和 sidebar-expand 都没有 v-if 条件渲染 → sidebar 展开时两个都在
// 3) .sidebar-expand 用 var(--ee-radius-md) (矩形), 不是 50% 圆形
//
// 真根因 (修 4):
// 4) 字符 (+/›/‹) 字身框几何中心 ≠ 字符视觉重心 (Inter 字体 ascent 大),
//    translateY 微调在不同字体/分辨率下不稳定. 最稳: 改用 SVG 图标.
//    SVG path 数据几何中心 = 视觉中心.
//
// 本测试钉死:
//   1) voice-btn 块必须有 border-radius: 50%
//   2) fold-btn 必须 v-if=!isMenuFolded, sidebar-expand 必须 v-if=isMenuFolded
//   3) .sidebar-header .icon-btn + .sidebar-expand 都必须 border-radius: 50%
//   4) fold-btn / new-btn / sidebar-expand 内必须用 SVG 图标 (不能是文字字符)

const indexSrc = readFileSync(
  './app/pages/chat/conversation/index.vue',
  'utf8'
)
const idSrc = readFileSync(
  './app/pages/chat/conversation/[id].vue',
  'utf8'
)

describe('Sprint 111 · A12 voice-btn / sidebar 重叠 / icon-btn 居中修复', () => {
  // === 1. voice-btn border-radius ===
  it('voice-btn 必须显式声明 border-radius: 50% (覆盖 .icon-btn 圆形)', () => {
    const voiceBtnMatch = idSrc.match(/\.voice-btn\s*\{[\s\S]*?\}/)
    const block = voiceBtnMatch?.[0] || ''
    expect(
      /border-radius:\s*50%/.test(block),
      'Sprint 111 · A12 修复要求 .voice-btn 必须显式 border-radius: 50%, ' +
      '否则 .icon-btn 父类的 50% 被自身 background 重置覆盖 → 方形按钮. ' +
      '用户 2026-09-17 反馈: "语音按钮都是方的你修个啥了". ' +
      'voice-btn 在 [id].vue 不在 index.vue.'
    ).toBe(true)
  })

  // === 2. fold-btn v-if ===
  it('fold-btn 必须 v-if="!isMenuFolded" (展开时才显示)', () => {
    // 用更精确的 regex 抓 fold-btn: 从 v-if="..." 之后到 class="icon-btn fold-btn"
    const foldBtnBlock = indexSrc.match(/v-if="[^"]*isMenuFolded[^"]*"[\s\S]*?class="icon-btn fold-btn"[\s\S]*?>/)?.[0] || ''
    expect(
      foldBtnBlock.length > 0 && /v-if="[^"]*!\s*isMenuFolded/.test(foldBtnBlock),
      'Sprint 111 · A12 修复要求 fold-btn 必须 v-if="!isMenuFolded". ' +
      '用户反馈: "侧边收起那按钮你他妈能不能正好了" — 折叠后 fold-btn 还渲染导致重叠.'
    ).toBe(true)
  })

  // === 3. sidebar-expand v-if ===
  it('sidebar-expand 必须 v-if="isMenuFolded" (折叠时才显示)', () => {
    // 找 "class=\"sidebar-expand\"" 之前最近的 v-if 表达式
    const expandClassIdx = indexSrc.indexOf('class="sidebar-expand"')
    if (expandClassIdx < 0) {
      expect.fail('Sprint 111 · A12 测试失败: 源码中找不到 class="sidebar-expand" 按钮')
      return
    }
    // 取按钮上方 200 字符, 抓最近的 v-if
    const before = indexSrc.substring(Math.max(0, expandClassIdx - 200), expandClassIdx)
    const vIfMatch = before.match(/v-if="([^"]+)"/g)
    const lastVIf = vIfMatch?.[vIfMatch.length - 1] || ''
    expect(
      lastVIf.includes('isMenuFolded') && !lastVIf.includes('!isMenuFolded') && !lastVIf.includes('! isMenuFolded'),
      'Sprint 111 · A12 修复要求 sidebar-expand 必须 v-if="...isMenuFolded..." ' +
      '(不能 !isMenuFolded). 否则 sidebar 展开时两个按钮同时渲染 → 重叠. ' +
      `当前 sidebar-expand 最近的 v-if: "${lastVIf}"`
    ).toBe(true)
  })

  // === 4. sidebar-header .icon-btn 圆形 override ===
  it('sidebar-header .icon-btn 必须 border-radius: 50%', () => {
    const sidebarHeaderBlock = indexSrc.match(/\.sidebar-header\s+\.icon-btn\s*\{[\s\S]*?\n\}/)?.[0] || ''
    expect(
      /border-radius:\s*50%/.test(sidebarHeaderBlock),
      'Sprint 111 · A12 修复要求 .sidebar-header .icon-btn 必须 border-radius: 50%, ' +
      '因为通用 .icon-btn 用 var(--ee-radius-md) (矩形带圆角). ' +
      '否则 fold-btn + new-btn 是方形, 与 voice-btn 视觉不一致.'
    ).toBe(true)
  })

  // === 5. .sidebar-expand 也必须 border-radius: 50% ===
  it('.sidebar-expand 必须 border-radius: 50% (用户反馈其中一个是方的)', () => {
    const sidebarExpandBlock = indexSrc.match(/\.sidebar-expand\s*\{[\s\S]*?\n\}/)?.[0] || ''
    expect(
      /border-radius:\s*50%/.test(sidebarExpandBlock),
      'Sprint 111 · A12 fix-3: .sidebar-expand 必须 border-radius: 50%. ' +
      '用户实测反馈 (2026-09-17): "侧边栏俩按钮的其中一个"还是方的. ' +
      '原因: sidebar-expand 之前用 var(--ee-radius-md) (矩形).'
    ).toBe(true)
  })

  // === 6. fold-btn / new-btn / sidebar-expand 必须用 SVG 而非文字字符 ===
  it('fold-btn 必须用 SVG (不能是文字字符 ‹)', () => {
    const foldBtnBlock = indexSrc.match(/<button[\s\S]*?class="icon-btn fold-btn"[\s\S]*?<\/button>/)?.[0] || ''
    expect(
      /<svg[\s\S]*?>[\s\S]*?<\/svg>/.test(foldBtnBlock) && !/aria-hidden="true">‹</.test(foldBtnBlock),
      'Sprint 111 · A12 fix-5: fold-btn 必须用 SVG chevron-left 图标, ' +
      '不能用文字字符 ‹. 用户实测: "里面那个符号中心在上面, 所以才会偏下". ' +
      '文字字符视觉重心受字体 ascent 偏移, CSS 微调不能完美. ' +
      'SVG path 几何中心 = 视觉中心.'
    ).toBe(true)
  })

  it('new-btn 必须用 SVG (不能是文字字符 +)', () => {
    const newBtnBlock = indexSrc.match(/<button[\s\S]*?class="icon-btn new-btn"[\s\S]*?<\/button>/)?.[0] || ''
    expect(
      /<svg[\s\S]*?>[\s\S]*?<\/svg>/.test(newBtnBlock) && !/aria-hidden="true">\+</.test(newBtnBlock),
      'Sprint 111 · A12 fix-5: new-btn 必须用 SVG plus 图标, ' +
      '不能用文字字符 +. 用户实测: "加号怎么不是中轴对齐".'
    ).toBe(true)
  })

  it('sidebar-expand 必须用 SVG (不能是文字字符 ›)', () => {
    const expandBlock = indexSrc.match(/<button[\s\S]*?class="sidebar-expand"[\s\S]*?<\/button>/)?.[0] || ''
    expect(
      /<svg[\s\S]*?>[\s\S]*?<\/svg>/.test(expandBlock) && !/aria-hidden="true">›</.test(expandBlock),
      'Sprint 111 · A12 fix-5: sidebar-expand 必须用 SVG chevron-right 图标, ' +
      '不能用文字字符 ›. 同 fold-btn 视觉居中问题.'
    ).toBe(true)
  })

  it('SVG 图标必须 16x16 viewBox=0 0 24 24 (与 voice-btn SVG 一致)', () => {
    // 所有 sidebar header 的 SVG 应该是统一的 16x16 大小
    const svgMatches = indexSrc.match(/<svg[^>]*viewBox="0 0 24 24"[^>]*width="16"[^>]*height="16"[^>]*>/g) || []
    expect(
      svgMatches.length >= 3,
      'Sprint 111 · A12 fix-5: sidebar 内必须至少 3 个统一规格 SVG (fold-btn, new-btn, sidebar-expand). ' +
      `当前找到 ${svgMatches.length} 个.`
    ).toBe(true)
  })
})
