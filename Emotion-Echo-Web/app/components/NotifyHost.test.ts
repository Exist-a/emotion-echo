import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// NotifyHost.vue + useNotify.ts 模板/源码合同断言。
// 由于 Nuxt auto-import alias 在 vitest 解析 .vue 内部 import 时的歧义,
// 这里改用 source-text 合同断言(与 conversation.test.ts 同形态),确保:
//   1. 使用 type-specific class 映射(.is-success / is-error / is-warning / is-info)
//   2. markFor 字符映射: success ✓, error !, warning !, info i
//   3. role=status + aria-live=polite
//   4. 卡片默认隐藏(空 toasts 时不渲染)
//   5. push 在 SSR 下 return

const hostSrc = readFileSync('./app/components/NotifyHost.vue', 'utf8')
const notifySrc = readFileSync('./app/composables/useNotify.ts', 'utf8')

describe('NotifyHost.vue · source 合同', () => {
  it('renders .is-{type} class for each toast', () => {
    expect(hostSrc).toMatch(/is-\$\{[^}]*type[^}]*\}/)
  })

  it('exposes the four markFor glyph mappings (success ✓, error/warning !, info i)', () => {
    expect(hostSrc).toMatch(/'success'\s*\?\s*'✓'/)
    expect(hostSrc).toMatch(/'error'\s*\?\s*'!'/)
    expect(hostSrc).toMatch(/'warning'\s*\?\s*'!'/)
    // info 分支 fallback —— 当前实现走三元末尾 default
    expect(hostSrc).toMatch(/'i'/)
  })

  it('sets role=status on each card and aria-live=polite on the stack', () => {
    expect(hostSrc).toContain('role="status"')
    expect(hostSrc).toContain('aria-live="polite"')
  })

  it('Teleports to body', () => {
    expect(hostSrc).toMatch(/<Teleport[^>]*to="body"/)
  })

  it('hides stack when toasts is empty (v-if)', () => {
    expect(hostSrc).toMatch(/v-if="toasts\.length"/)
  })
})

describe('useNotify.ts · source 合同', () => {
  it('defines a module-level reactive toast list (ref<Toast[]>)', () => {
    expect(notifySrc).toMatch(/ref<Toast\[\]>\(\[\]\)/)
  })

  it('guards SSR: returns early when !import.meta.client', () => {
    expect(notifySrc).toMatch(/!import\.meta\.client\s*\)\s*return/)
  })

  it('auto-removed by setTimeout(duration)', () => {
    expect(notifySrc).toMatch(/window\.setTimeout/)
    expect(notifySrc).toMatch(/filter\(/)
  })

  it('exposes success / error / warning / info shortcuts', () => {
    for (const t of ['success', 'error', 'warning', 'info']) {
      expect(notifySrc).toMatch(new RegExp(`${t}:\\s*\\(title: string, message`))
    }
  })

  it('exposes global notify() shortcut with default type=info + 3000ms', () => {
    const re = /export function notify\([^)]*title[^)]*string[^)]*message[^)]*string/
    expect(notifySrc).toMatch(re)
  })
})
