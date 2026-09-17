import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const MIDDLEWARE_SRC = readFileSync(resolve(__dirname, 'auth.global.ts'), 'utf8')

// Stage 112 修复 (Bug A)：dev mode 下从 /chat/conversation/new SPA 跳到
// /chat/dashboard/dailyReport 时，偶发被弹回 /login。
// 根因：isAuthenticated = !!accessToken.value && !!userInfo.value?.id；
//      SPA 内导航触发中间件时，如果 access_token cookie 已恢复但 userInfo
//      还在 fetchUserInfo 的 promise 中（async），userInfo.value?.id 为 falsy，
//      中间件判定 !isAuthenticated，navigateTo('/login')。
// 修复策略：中间件判定前，如果 cookie 有但 userInfo 缺失，必须 trigger fetchUserInfo 并 await。
describe('auth.global.ts middleware client-side restore (Stage 112 bug A)', () => {
  it('当 access_token 已恢复但 userInfo 仍为空时，中间件必须主动恢复 userInfo（不再放过 false 判定）', () => {
    // 不能简单"放过"——必须存在主动恢复逻辑
    // 形态：fetchUserInfo 调用 + 等待 userInfo.value?.id 落地
    expect(MIDDLEWARE_SRC).toMatch(/fetchUserInfo/)
    // 之前 line 64-68 已有 client 端兜底（isTokenExpired 分支），现在需要更激进：
    // 即使 token 没过期，只要 userInfo.id 缺失，就必须 await fetchUserInfo
    expect(MIDDLEWARE_SRC).toMatch(/!isAuthenticated\s*&&\s*userStore\.accessToken/)
    // 必须有 await 等待 userInfo 落地的机制（避免再放过 false 判定）
    expect(MIDDLEWARE_SRC).toMatch(/await\s+userStore\.fetchUserInfo/)
  })

  it('中间件不应在 !isAuthenticated && !isInWhiteList 时立即 navigateTo(\"/login\")，必须先尝试恢复', () => {
    // 现在的代码：
    //   if (!isInWhiteList && !isAuthenticated) return navigateTo("/login", { replace: true })
    // 修复后：在 navigateTo("/login") 之前，必须先尝试 token+userInfo 恢复
    const protectedBlock = MIDDLEWARE_SRC.match(
      /if\s*\(!isInWhiteList\s*&&\s*!isAuthenticated\)\s*\{([\s\S]*?)\n\s*\}/,
    )
    expect(protectedBlock, '必须存在 !isInWhiteList && !isAuthenticated 守卫块').not.toBeNull()
    // 该块内应包含 fetchUserInfo 主动恢复逻辑（不是单纯 navigateTo 退出）
    expect(protectedBlock?.[1]).toMatch(/fetchUserInfo/)
    expect(protectedBlock?.[1]).toMatch(/await/)
  })
})