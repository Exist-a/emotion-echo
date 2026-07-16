// middleware/auth.global.ts - 全局认证中间件
import { useUserStore } from "~/stores/user";

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
  const runtimeConfig = useRuntimeConfig();
  const isAuthDisabled = String(runtimeConfig.public.DISABLE_AUTH).toLowerCase() === "true";
  
  if (isAuthDisabled) {
    // 调试模式下，打印日志但不拦截
    if (to.path.startsWith("/chat")) {
      console.log("🔓 [调试模式] 登录拦截已禁用，允许访问:", to.path);
    }
    return;
  }

  // ==================== 白名单路由（无需登录） ====================
  const whiteList = [
    "/login", 
    "/login/forget", 
    "/login/forget/verify", 
    "/login/forget/modify", 
    "/login/forget/success"
  ];
  const isInWhiteList = whiteList.some((path) => to.path.startsWith(path));

  // ==================== 获取用户登录状态 ====================
  // SSR 时通过 cookie 判断，CSR 时通过 Pinia store 判断
  const userStore = useUserStore();
  let isAuthenticated = userStore.isAuthenticated;
  
  // SSR 环境：从 cookie 判断登录状态（简化方案，不做自动刷新）
  // access_token cookie 有效期已延长到 1 小时（记住我）或 Session（不记住我）
  if (import.meta.server && !isAuthenticated) {
    const tokenCookie = useCookie("access_token");
    if (tokenCookie.value) {
      // 有 access_token，认为已登录（即使可能已过期，让 API 调用时处理）
      isAuthenticated = true;
    }
    // 注意：不尝试读取 refreshToken（HttpOnly + Path 限制，SSR 无法可靠获取）
    // Token 刷新完全交给客户端处理
  }
  
  // CSR 环境兜底：accessToken 已恢复但 userInfo 可能还在加载中，避免误判未登录
  if (import.meta.client && !isAuthenticated && userStore.accessToken) {
    console.log("[Auth Middleware] accessToken 存在但 userInfo 未恢复，视为已登录");
    isAuthenticated = true;
  }

  // ==================== 路由拦截逻辑 ====================
  
  // 1. 已登录用户访问登录相关页面，重定向到首页
  if (isInWhiteList && isAuthenticated) {
    console.log("[Auth Middleware] 已登录用户访问登录页，重定向到首页");
    return navigateTo("/chat/conversation", { replace: true });
  }

  // 2. 未登录用户访问非白名单页面，重定向到登录页
  if (!isInWhiteList && !isAuthenticated) {
    console.log("[Auth Middleware] 未登录用户访问受保护页面:", to.path);
    
    return navigateTo("/login", {
      replace: true,
    });
  }

  // 3. 检查 Token 是否即将过期（仅在客户端执行）
  if (!isInWhiteList && isAuthenticated && import.meta.client && userStore.isTokenExpired()) {
    console.warn("[Auth Middleware] Token 即将过期，自动刷新中...");
    userStore.fetchUserInfo().catch(() => {
      // 静默处理，失败时不阻断导航
    });
  }

  // 4. 正常放行
  return;
});
