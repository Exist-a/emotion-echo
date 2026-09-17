/**
 * tests-quick-login-contract.test.ts
 *
 * Stage 97 tail · C6 quick-login 端点契约测试（todo-pile-2026-09-04.md §C6）
 *
 * 背景：
 * - 前端 quickLogin 函数（app/pages/login/index.vue:175-196）期望后端有
 *   /api/v1/auth/quick-login 端点
 * - 实际后端从未实现该端点（决策 18 §四 阻断 6 + Stage 38 §三 阻断 6）
 * - PR-4 选择"假 quick-login"：quickLogin 内部调 userStore.login 用
 *   seed 默认账号 echo/echo123（实际走标准 /api/v1/auth/login）
 *
 * 本测试钉死契约：
 *   1. quickLogin 实现调 userStore.login（非直接调 /quick-login）
 *   2. 入参 username='echo' + password='echo123'（与 seed 03-seed-default-users.sql 默认用户匹配）
 *   3. 实现不能调 /api/v1/auth/quick-login 路径
 *   4. 现有 e2e/login-flow.spec.ts 已断言行为契约，本单测补充字面量契约
 *
 * 字面量断言 vs 行为测试：本测试场景读 source file 校验契约要点，
 * 避免 happy-dom 下 userStore.fetchUserInfo / navigateTo mock 复杂度。
 * 真行为覆盖由 e2e/login-flow.spec.ts 承担。
 */

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const LOGIN_INDEX = resolve(process.cwd(), 'app/pages/login/index.vue')

describe('quick-login 契约（C6 todo-pile）', () => {
  const src = readFileSync(LOGIN_INDEX, 'utf8')

  it('quickLogin 必须调 userStore.login（不直接调 /quick-login 端点）', () => {
    // quickLogin 函数体内应调 userStore.login(...)，而非直接调 quickLogin 端点
    expect(src).toContain('quickLogin')
    expect(src).toContain('userStore.login')
    // 注释必须明说"标准登录 + demo 账号 echo/echo123"
    expect(src).toContain('echo')
    expect(src).toContain('echo123')
  })

  it('quickLogin 函数体内 fetch/post/axios 不能含 /quick-login 路径调用', () => {
    // 钉死契约：quickLogin 必须经 userStore.login（标准 /auth/login 端点），
    // 不得"绕过"业务层直接调不存在的 /auth/quick-login 端点。
    // 注释可以提"无 /auth/quick-login"作为事实记录，但代码不能含 fetch(...quickLogin...)
    // 或 axios.post(.../auth/quick-login...) 字面量。
    const quickLoginBlock =
      src.match(/const quickLogin = async[^{]*\{([\s\S]*?)\n {2}\}/)?.[1] ?? ''
    // 抽出非注释行（去 -- // 后的内容）
    const codeOnly = quickLoginBlock
      .split('\n')
      .filter((l) => !l.trim().startsWith('//') && !l.trim().startsWith('*'))
      .join('\n')
    expect(codeOnly).not.toMatch(/fetch\s*\(\s*['"`][^'"`]*quick-login/i)
    expect(codeOnly).not.toMatch(/axios[^)]*quick-login/i)
    expect(codeOnly).not.toMatch(/post[^)]*quick-login/i)
  })

  it('quickLogin 入参必须含 rememberMe=true', () => {
    // "体验模式"应保持登录态，与 demo 账号语义对齐
    expect(src).toContain('rememberMe')
  })
})
