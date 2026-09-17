import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { resolve, dirname } from 'node:path'

const __filename = fileURLToPath(import.meta.url)
const __dirname = dirname(__filename)
const MIDDLEWARE_SRC = readFileSync(resolve(__dirname, 'auth.global.ts'), 'utf8')

// Stage 112 修复 v2 (Bug A)：
// 之前修复：!isAuthenticated 时 await fetchUserInfo 恢复。但实测 dev mode 仍 ~50% 失败：
//   - isAuthenticated 是复合状态 (token AND userInfo.id)，SPA 跳转后 userInfo 异步等待中 → false
//   - 即便加 await fetchUserInfo，userInfo.value.id 落地前中间件已 navigateTo('/login')
// v2 修复：受保护路由的"是否已登录"判定只看 accessToken 存在性，不依赖 userInfo。
//   userInfo 缺失只表示页面元数据未加载，由各页面 onMounted 自取 (fetchUserInfo)，
//   取失败（401）才显示未登录或重定向。
describe('auth.global.ts middleware token-presence-only contract (Stage 112 v2)', () => {
  it('守卫块 !isInWhiteList && !isAuthenticated 必须改为 !isInWhiteList && !hasAccessToken', () => {
    // 之前的判定是 !isInWhiteList && !isAuthenticated（依赖 userInfo），导致 dev mode 抖
    // 现在的判定应是 !isInWhiteList && !hasAccessToken（仅依赖 accessToken）
    expect(MIDDLEWARE_SRC, '不能用 !isInWhiteList && !isAuthenticated 复合判定').not.toMatch(
      /if\s*\(!isInWhiteList\s*&&\s*!isAuthenticated\)/,
    )
    expect(MIDDLEWARE_SRC, '必须用 !isInWhiteList && !hasAccessToken').toMatch(
      /if\s*\(!isInWhiteList\s*&&\s*!hasAccessToken\)/,
    )
  })

  it('hasAccessToken 变量必须被声明（cookie 优先 + store.accessToken fallback）', () => {
    // dev mode 下 store.accessToken 可能为空，但 cookie 仍有 → SSR 必须能从 cookie 拿到
    const guardIdx = MIDDLEWARE_SRC.lastIndexOf('if (!isInWhiteList')
    expect(guardIdx).toBeGreaterThan(-1)
    const beforeGuard = MIDDLEWARE_SRC.slice(Math.max(0, guardIdx - 800), guardIdx)
    expect(beforeGuard, 'hasAccessToken 变量必须在守卫块前声明').toMatch(/hasAccessToken\s*[:=]/)
    // 必须从 cookie 或 accessToken 派生
    expect(beforeGuard).toMatch(/tokenCookie|userStore\.accessToken/)
  })

  it('hasAccessToken true 时（不论 userInfo）必须 return 放行，不再踢回 /login', () => {
    // 找到 !isInWhiteList && !hasAccessToken 守卫块（unauth guard）
    const guardBlocks = [
      ...MIDDLEWARE_SRC.matchAll(/if\s*\(!isInWhiteList\s*&&\s*!hasAccessToken\)/g),
    ]
    expect(guardBlocks.length, '必须存在 !isInWhiteList && !hasAccessToken 守卫块').toBeGreaterThan(
      0,
    )
    const guardIdx = guardBlocks[0]!.index
    const afterGuard = MIDDLEWARE_SRC.slice(guardIdx, guardIdx + 500)
    expect(afterGuard).toMatch(/navigateTo\(["']\/login["']/)
  })
})
