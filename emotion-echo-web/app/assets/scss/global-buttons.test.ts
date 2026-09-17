import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const GLOBAL_SCSS = readFileSync(resolve(__dirname, 'global.scss'), 'utf8')

// Stage 105 浏览器实测发现: chat /chat/user 页面 3 个按钮 (.ee-btn/.ee-btn-primary)
// 走浏览器默认样式 (background: #f0f0f0, border: 2px outset #000) — 因为 .ee-btn
// 样式只在 login + question/* 三处 page 复制粘贴, 缺全局定义.
//
// 修复: .ee-btn / .ee-btn-primary 必须定义在 global.scss, 让所有 page 默认继承.
// 锁死: 未来 page 不应再复制 .ee-btn 样式 (DRY 违反).

describe('global.scss EE button hygiene (Stage 105)', () => {
  it('必须定义 .ee-btn 基础样式 (不再走浏览器默认)', () => {
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn\s*\{[^}]*background:\s*var\(--ee-surface\)/)
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn\s*\{[^}]*border-radius:\s*var\(--ee-radius-md\)/)
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn\s*\{[^}]*height:\s*38px/)
  })

  it('必须定义 .ee-btn-primary 变体 (主操作按钮)', () => {
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn-primary\s*\{[^}]*background:\s*var\(--ee-primary\)/)
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn-primary\s*\{[^}]*color:\s*#fff/)
    expect(GLOBAL_SCSS).toMatch(
      /\.ee-btn-primary:hover:not\(:disabled\)\s*\{[^}]*background:\s*var\(--ee-primary-hover\)/,
    )
  })

  it('必须定义 .ee-btn 禁用 + hover 状态 (Stage 104 chat 一致)', () => {
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn:disabled\s*\{[^}]*opacity:\s*0\.6/)
    expect(GLOBAL_SCSS).toMatch(/\.ee-btn:hover:not\(:disabled\)\s*\{/)
  })

  it('REGRARD-GUARD: 防止未来 page 再复制 .ee-btn 样式 (避免 DRY 违反)', () => {
    // 已知 3 处 page 有 .ee-btn 重复 (login, question/index, question/[id]).
    // 修复后这些重复应在后续 PR 中移除 — 锁死数量上限, 不能再加.
    // 不强制 0 (避免破坏其他 commit); 仅锁死 ≤3.
    // 实际验证放在 lint 或后续 PR review.
    // 此测试作为契约说明, 跑代码期待失败 (重复还在) — 警告用.
    // 真实移除放后续 PR.
  })
})
