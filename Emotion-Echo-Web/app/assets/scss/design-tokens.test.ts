import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const tokens = readFileSync(resolve(process.cwd(), 'app/assets/scss/global.scss'), 'utf8')
const variables = readFileSync(resolve(process.cwd(), 'app/assets/scss/variables.scss'), 'utf8')

describe('Quiet Companion design tokens', () => {
  it('exposes light and dark token sets in global.scss', () => {
    expect(tokens).toMatch(/:root\s*{/)
    expect(tokens).toMatch(/--ee-bg:\s*#f7f8f7/i)
    expect(tokens).toMatch(/--ee-primary:\s*#5f8f7b/i)
    expect(tokens).toMatch(/--ee-text:\s*#202522/i)
    expect(tokens).toMatch(/html\.dark\s*{/)
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
})
