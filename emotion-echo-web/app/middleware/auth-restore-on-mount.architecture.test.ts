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
// 修复策略：中间件判定前，client-side 必须 await fetchUserInfo。
// 进一步（SSR）：dev mode SSR 重渲染时 userStore.accessToken 可能为空，
//      必须显式把 cookie 中的 token 注入 store，再触发 fetchUserInfo。
describe('auth.global.ts middleware restore contract (Stage 112 bug A)', () => {
  it('中间件必须主动调用 fetchUserInfo（client 段）', () => {
    expect(MIDDLEWARE_SRC).toMatch(/fetchUserInfo/)
    expect(MIDDLEWARE_SRC).toMatch(/import\.meta\.client\s*&&\s*userStore\.accessToken/)
    expect(MIDDLEWARE_SRC).toMatch(/await\s+userStore\.fetchUserInfo/)
  })

  it('中间件不应在 !isAuthenticated && !isInWhiteList 时立即 navigateTo("/login")，必须先尝试恢复', () => {
    const protectedBlock = MIDDLEWARE_SRC.match(
      /if\s*\(!isInWhiteList\s*&&\s*!isAuthenticated\)\s*\{([\s\S]*?)\n\s*\}/,
    )
    expect(protectedBlock, '必须存在 !isInWhiteList && !isAuthenticated 守卫块').not.toBeNull()
    expect(protectedBlock?.[1]).toMatch(/fetchUserInfo/)
    expect(protectedBlock?.[1]).toMatch(/await/)
  })

  it('中间件整体必须包含 SSR 段：cookie 注入 store + await fetchUserInfo + client 段也 fetchUserInfo', () => {
    // 简化版：直接断言源码字面量，不依赖 regex 块定位
    expect(MIDDLEWARE_SRC).toMatch(/import\.meta\.server/)
    expect(MIDDLEWARE_SRC).toMatch(/import\.meta\.client/)
    // 找最末尾的 !isInWhiteList && !isAuthenticated 守卫块起始位置（含 SSR 段）
    const guardIdx = MIDDLEWARE_SRC.lastIndexOf('if (!isInWhiteList && !isAuthenticated)')
    expect(guardIdx, '必须存在 !isInWhiteList && !isAuthenticated 守卫块').toBeGreaterThan(-1)
    // 从该守卫开始到下个 console.warn 或 navigateTo，定位 SSR 段 + Client 段
    const guardSlice = MIDDLEWARE_SRC.slice(guardIdx, guardIdx + 1200)
    expect(guardSlice).toMatch(/useCookie\(\s*['"]access_token['"]\s*\)/)
    expect(guardSlice).toMatch(/userStore\.accessToken\s*=\s*tokenCookie\.value/)
    expect(guardSlice).toMatch(/await\s+userStore\.fetchUserInfo/)
    // Client 段：accessToken 存在时主动恢复
    expect(guardSlice).toMatch(/import\.meta\.client\s*&&\s*userStore\.accessToken/)
  })
})