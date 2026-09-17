// middleware/auth.global.ts - 全局认证中间件
import { useUserStore } from '~/stores/user'

/**
 * 认证中间件
 * 功能：
 * 1. 检查用户登录状态
 * 2. 未登录用户重定向到登录页
 * 3. 已登录用户访问登录页重定向到首页
 *
 * SSR 支持：
 * 服务端通过 access_token cookie 判断登录状态，避免 hydration 闪烁
 * 不做 SSR 自动刷新（简化方案），Token 刷新完全由客户端处理
 *
 * 调试模式：
 * 在 .env 中设置 NUXT_PUBLIC_DISABLE_AUTH=true 可禁用登录拦截
 * 注意：生产环境不要设置此配置
 */
export default defineNuxtRouteMiddleware(async (to, from) => {
  // ==================== 调试模式检查 ====================
  const runtimeConfig = useRuntimeConfig()
  const isAuthDisabled = String(runtimeConfig.public.DISABLE_AUTH).toLowerCase() === 'true'

  if (isAuthDisabled) {
    // 调试模式下，打印日志但不拦截
    if (to.path.startsWith('/chat')) {
      console.log('🔓 [调试模式] 登录拦截已禁用，允许访问:', to.path)
    }
    return
  }

  // ==================== 白名单路由（无需登录） ====================
  // P1-R2-3: 严格使用 === 或带边界检查的 prefix 匹配
  // 原因：原 startsWith("/login") 会被 "/loginxxx" "/Login" "/loginbackdoor" 绕过
  // 修复：对带子路径的前缀（如 /login/forget）用 === + "/" 边界；
  //      对 /login 单独要求 === 或 /login/...
  const whiteListExact = new Set(['/login'])
  const whiteListPrefix = [
    '/login/forget', // /login/forget, /login/forget/verify, /login/forget/modify, /login/forget/success
  ]
  const isInWhiteList =
    whiteListExact.has(to.path) ||
    whiteListPrefix.some((p) => to.path === p || to.path.startsWith(p + '/'))

  // ==================== 获取用户登录状态 ====================
  // Sprint 112 v2：判定只看 accessToken 存在性，不依赖 userInfo。
  // 原因：dev mode 下 SPA 跳转触发 SSR 重渲染时，userStore.userInfo 异步 fetchUserInfo
  //       还没完成（userInfo.value?.id 为 falsy），旧版用 isAuthenticated（复合状态）
  //       判定会把已登录用户踢回 /login。改用 hasAccessToken（仅 accessToken 存在性），
  //       即便 userInfo 缺失也视为已登录——userInfo 是页面元数据，由各页面 onMounted
  //       自取（fetchUserInfo），token 失效由 401 兜底重定向，不在中间件判。
  const userStore = useUserStore()
  let hasAccessToken = !!userStore.accessToken

  // SSR 段：cookie 中的 access_token 也视为已登录（dev mode 下 useCookie CSR 写入
  //        与 SSR 读取可能跨上下文不同步，但 SSR 的 request headers 一定带 cookie）
  if (import.meta.server) {
    const tokenCookie = useCookie('access_token')
    if (tokenCookie.value) {
      // 把 SSR 段读到的 token 同步到 store，让后续 SSR 渲染时 store.accessToken 也可用
      userStore.accessToken = tokenCookie.value
      hasAccessToken = true
    }
  }

  // ==================== 路由拦截逻辑 ====================

  // 1. 已登录用户访问登录相关页面，重定向到首页
  if (isInWhiteList && hasAccessToken) {
    console.log('[Auth Middleware] 已登录用户访问登录页，重定向到首页')
    return navigateTo('/chat/conversation', { replace: true })
  }

  // 2. 未登录用户访问非白名单页面，重定向到登录页
  // v2：判定只用 accessToken 存在性。userInfo 缺失不算未登录，由各页面 fetchUserInfo 兜底。
  if (!isInWhiteList && !hasAccessToken) {
    console.log('[Auth Middleware] 未登录用户访问受保护页面:', to.path)
    return navigateTo('/login', {
      replace: true,
    })
  }

  // 3. 检查 Token 是否即将过期（仅在客户端执行）
  if (!isInWhiteList && hasAccessToken && import.meta.client && userStore.isTokenExpired()) {
    console.warn('[Auth Middleware] Token 即将过期，自动刷新中...')
    userStore.fetchUserInfo().catch(() => {
      // 静默处理，失败时不阻断导航
    })
  }

  // 4. 正常放行
  return
})
