// app/lib/clientAccessToken.ts
//
// Sprint 111 · R-09 修复: HttpOnly cookie 浏览器 JS 读不到 (document.cookie 为空,
// useCookie('access_token').value 也读不到), 直接读 cookie 永远空字符串,
// 导致 Authorization 头空 → APISIX jwt-auth 401 "JWT token invalid".
//
// 解决: 集中读 token 到此 helper, 优先 userStore.accessToken (Pinia ref, 浏览器可见),
// fallback useCookie (SSR / store 未初始化场景). 与 useApi.ts getAccessToken 行为对齐.
//
// 使用:
//   import { getClientAccessToken } from '~/lib/clientAccessToken'
//   const token = getClientAccessToken()
//
// 决策: 不直接导出 useApi.getAccessToken (内部函数), 单独模块避免循环依赖.

import { useUserStore } from '~/stores/user'
import { useCookie } from '#app'

// Sprint 111 · R-09 trace (DEBUG_BUILD 用) — 在浏览器 console 打印 helper
// 内部状态, 排查 userStore 拿不到 token 时是哪个分支失败。生产构建会
// tree-shake 掉 (DEBUG_BUILD = false 时 console.log 整段 dead code)。
const DEBUG_BUILD = false

export function getClientAccessToken(): string {
  // SSR: 没 userStore, 直接 cookie
  if (!import.meta.client) {
    return useCookie('access_token').value || ''
  }
  // CSR: 优先 userStore (Pinia ref, 浏览器 JS 可见)
  try {
    const userStore = useUserStore()
    // 注意: user.ts:30 getAccessToken = computed(() => accessToken.value) — ref 对象!
    // 必须 .value 拿原始字符串, 否则 typeof check 永远 truthy 但拼字符串是 "[object Object]"
    const raw = userStore?.getAccessToken
    const storeToken = typeof raw === 'string' ? raw : (raw as any)?.value || ''
    if (DEBUG_BUILD) {
      console.log('[R-09 debug] useUserStore ok, raw type=', typeof raw, ', token=', storeToken ? storeToken.substring(0, 20) + '...' : '(empty)')
    }
    if (storeToken) return storeToken
  } catch (e: any) {
    if (DEBUG_BUILD) {
      console.warn('[R-09 debug] useUserStore failed:', e?.message || e)
    }
  }
  // Fallback: cookie
  const cookieToken = useCookie('access_token').value || ''
  if (DEBUG_BUILD) {
    console.log('[R-09 debug] cookie fallback, token=', cookieToken ? cookieToken.substring(0, 20) + '...' : '(empty)')
  }
  return cookieToken
}
