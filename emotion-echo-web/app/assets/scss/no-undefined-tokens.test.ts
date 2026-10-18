// app/assets/scss/no-undefined-tokens.test.ts
//
// CL-06 · UI 轮：**"用了但没人定义"的 CSS 变量**门禁。
//
// 为什么需要它：角色页与切换器此前满屏 `var(--primary-color, #6366f1)` /
// `var(--border-color, #e5e7eb)` —— 这些名字**全仓从未定义**，所以浏览器每次都落到
// 逗号后面的 Tailwind 默认值：一个靛蓝按钮长在鼠尾草绿的应用里，且深色模式下纹丝不动
// （fallback 是硬编码 hex，不随 `html.dark` 变）。这类缺陷**不会报错、不会 500、
// 单测全绿**，只有人眼能看出来 —— 所以必须有一条机器检查：**用到的每个变量都得有人定义**。
//
// 两层：
//   ① 严格层 —— 本轮动过的文件必须**零**未定义变量（新增/回归即红）；
//   ② 棘轮层 —— 全仓其余历史遗留**冻结在 BASELINE**，只许减少不许增加。
//      这是"不扩大债务"，不是"假装没有债务"：BASELINE 里每一条都是真实待办。
import { describe, it, expect } from 'vitest'
import { readdirSync, readFileSync } from 'node:fs'
import { join, relative, resolve } from 'node:path'

const ROOT = resolve(process.cwd(), 'app')

/** 本轮收口的文件：必须零未定义变量（其余文件见 BASELINE） */
const STRICT_FILES = [
  'pages/chat/agents.vue',
  'components/AgentSwitcher.vue',
  'components/AgentCard.vue',
]

/**
 * 全仓历史遗留（2026-10-17 冻结，13 条）。**每一条都是真缺陷**，只是不在本轮范围内。
 * 修掉其中的变量后，**必须把它从这里删掉** —— 本测试会因"名单过期"而失败，
 * 免得这份清单变成一份没人维护的谎话（同 AGENTS §〇 对"账本与状态脱钩"的要求）。
 */
const BASELINE = [
  '--color-ink',
  '--color-ink-soft',
  '--color-mist',
  '--color-pencil',
  '--color-sage',
  '--color-sage-deep',
  '--color-surface-muted',
  '--color-text-secondary',
  '--ease-quiet',
  '--ee-primary-rgb',
  '--ee-radius-xl',
  '--motion-base',
]

function collectSources(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const p = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === 'node_modules') continue
      collectSources(p, out)
    } else if (/\.(?:vue|scss|css)$/.test(entry.name)) {
      out.push(p)
    }
  }
  return out
}

const SOURCES = collectSources(ROOT)

/**
 * 剥掉注释再扫。
 *
 * 为什么必须剥：本仓踩过"源码钉命中注释"的坑 —— 一条 `// 以前写的是 var(--primary-color, #6366f1)`
 * 的**说明性注释**会让门禁报出一个根本不存在的缺陷，反过来也会让某个真的缺陷躲在注释后面。
 * 注释里的变量名浏览器不会用，所以扫的应该是"剥掉注释之后剩下的代码"。
 */
function stripComments(src: string): string {
  return src
    .replace(/\/\*[\s\S]*?\*\//g, ' ')
    .replace(/<!--[\s\S]*?-->/g, ' ')
    // 行注释：`(?<!:)` 是为了不打掉 `url(http://…)` / `https://…` 里的那个 `//`
    .replace(/(?<!:)\/\/[^\n]*/g, ' ')
}

/** 所有被定义过的变量名（`--x:` 形式），同样剔掉注释里的"假定义" */
const DEFINED = new Set<string>()
for (const f of SOURCES) {
  for (const m of stripComments(readFileSync(f, 'utf8')).matchAll(/--([\w-]+)\s*:/g)) {
    DEFINED.add(m[1]!)
  }
}

/** 某个文件里"用了但全仓无人定义"的变量名 */
function undefinedIn(file: string): string[] {
  const src = stripComments(readFileSync(resolve(ROOT, file), 'utf8'))
  const bad = new Set<string>()
  for (const m of src.matchAll(/var\(\s*--([\w-]+)/g)) {
    if (!DEFINED.has(m[1]!)) bad.add(`--${m[1]}`)
  }
  return [...bad].sort()
}

describe('CSS 变量：用到的必须有人定义', () => {
  it.each(STRICT_FILES)('%s 不得使用未定义的变量', (file) => {
    expect(
      undefinedIn(file),
      `${file} 里这些变量全仓无人定义 ⇒ 浏览器会回落到 var() 的兜底值（硬编码 hex），` +
        '既不跟随主题、也不跟随深色模式。请改用 --ee-* 令牌（见 app/assets/scss/global.scss）。',
    ).toEqual([])
  })

  it('全仓未定义变量不得增加（只许减少）', () => {
    const repoWide = new Set<string>()
    for (const f of SOURCES) {
      const rel = relative(ROOT, f).replace(/\\/g, '/')
      if (STRICT_FILES.includes(rel)) continue // 已由上一层严格门禁覆盖
      for (const name of undefinedIn(rel)) repoWide.add(name)
    }
    const current = [...repoWide].sort()
    const added = current.filter((n) => !BASELINE.includes(n))
    const fixed = BASELINE.filter((n) => !current.includes(n))

    expect(
      added,
      '出现了**新的**未定义 CSS 变量。请定义它（推荐加进 global.scss 的 :root + html.dark），' +
        '或改用一个已存在的 --ee-* 令牌。',
    ).toEqual([])
    expect(
      fixed,
      '这些变量已经不再是未定义状态了 —— 请把它们从本文件的 BASELINE 里删掉，' +
        '让清单保持如实（否则它会在将来放行同名的重新引入）。',
    ).toEqual([])
  })
})
