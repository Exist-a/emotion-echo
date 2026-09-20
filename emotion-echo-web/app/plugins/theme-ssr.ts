// plugins/theme-ssr.ts
//
// E2E-12 #10：SSR 首屏就把主题 class 渲染进 <html>，消除冷启动闪烁（FOUC）。
//
// 为什么需要它：主题的真相在服务端 `users.config.theme`，但 SSR 渲染首屏时
// 无法为此做一次 API 往返（`access_token` 是 HttpOnly，且 BFF 调用成本与首屏
// 延迟直接相关）。因此改读 `applyTheme` 写下的 `ee_theme` 镜像 cookie
// —— 非 HttpOnly、随请求自动携带，服务端可直接读。
//
// ⚠️ 只在「深色」时声明 htmlAttrs，浅色时**完全不声明**：
// `useHead` 的 htmlAttrs 会进入 SSR payload 并在客户端 hydration 时回写，
// 若浅色时声明 `class: ''`，它会覆盖掉 `stores/user.ts` 的 `applyTheme`
// 在客户端加上的 `dark`（实测：主题切换整体失效）。不声明即无冲突。
export default defineNuxtPlugin(() => {
  if (!import.meta.server) return

  const themeCookie = useCookie<'dark' | 'light'>('ee_theme')
  if (themeCookie.value !== 'dark') return

  useHead({
    htmlAttrs: { class: 'dark' },
  })
})
