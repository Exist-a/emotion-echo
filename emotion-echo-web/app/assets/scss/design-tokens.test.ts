import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const tokens = readFileSync(resolve(process.cwd(), 'app/assets/scss/global.scss'), 'utf8')
const variables = readFileSync(resolve(process.cwd(), 'app/assets/scss/variables.scss'), 'utf8')

describe('Quiet Companion design tokens', () => {
  it('exposes light and dark token sets in global.scss', () => {
    expect(tokens).toMatch(/:root\s*\{/)
    expect(tokens).toMatch(/--ee-bg:\s*#f7f8f7/i)
    expect(tokens).toMatch(/--ee-primary:\s*#5f8f7b/i)
    expect(tokens).toMatch(/--ee-text:\s*#202522/i)
    expect(tokens).toMatch(/html\.dark\s*\{/)
    expect(tokens).toMatch(/color-scheme:\s*dark/)
  })

  it('exposes focus and motion tokens', () => {
    expect(tokens).toMatch(/--ee-focus:/)
    expect(tokens).toMatch(/--ee-transition:/)
  })

  it('respects prefers-reduced-motion', () => {
    expect(tokens).toMatch(/@media\s*\(prefers-reduced-motion:\s*reduce\)/)
  })

  it('exposes a quiet-pulse keyframe and animation class', () => {
    expect(tokens).toMatch(/@keyframes\s+ee-quiet-pulse/)
    expect(tokens).toMatch(/\.quiet-pulse/)
  })

  it('keeps SCSS variable aliases in variables.scss for legacy consumers', () => {
    expect(variables).toMatch(/\$color-primary:\s*#5f8f7b/i)
    expect(variables).toMatch(/\$bg-color:\s*\$color-bg/i)
  })

  // CL-06 · UI 轮：删除角色是**不可撤销的破坏性动作**，需要一枚语义色。
  // 之前页面写的是 `var(--danger-color, #ef4444)` —— 那是个**从未定义**的变量名，
  // 于是永恒回落到 Tailwind 红，且深色模式下不跟随主题（见 no-undefined-tokens.test.ts）。
  it('exposes a danger token pair in both light and dark sets', () => {
    expect(tokens).toMatch(/--ee-danger:\s*#[0-9a-f]{6}/i)
    expect(tokens).toMatch(/--ee-danger-soft:\s*(#|color-mix)/i)
    // 深色集里也必须重新给值（否则暗色背景上对比度不够）
    const dark = tokens.slice(tokens.indexOf('html.dark'))
    expect(dark).toMatch(/--ee-danger:\s*#[0-9a-f]{6}/i)
    expect(dark).toMatch(/--ee-danger-soft:\s*(#|color-mix)/i)
  })

  // 「主色背景上的文字」必须是一个令牌而不是写死的 #fff：
  // 深色模式下 --ee-primary 是**浅**鼠尾草绿（#8ebaa8），白字压上去对比度约 1.9:1
  // （WCAG AA 要 4.5:1）—— 实测确认过。写死 #fff 就会在深色模式下瞎掉。
  // 注：`--ee-on-primary` 此前只被 app/pages/chat/user/index.vue **引用**却从未定义，
  // 属本仓的既有漂移（见 no-undefined-tokens.test.ts 的基线），这里一并补上。
  it('exposes an on-primary ink token in both light and dark sets', () => {
    expect(tokens).toMatch(/--ee-on-primary:\s*#[0-9a-f]{6}/i)
    const dark = tokens.slice(tokens.indexOf('html.dark'))
    expect(dark).toMatch(/--ee-on-primary:\s*#[0-9a-f]{6}/i)
  })
})
