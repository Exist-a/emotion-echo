// plugins/init.ts - 应用初始化插件
import { getApiBaseUrl } from '~/lib/apiBaseUrl'
import { installClientErrorReporter } from '~/utils/clientErrorReporter'
import { useUserStore } from '~/stores/user'
import { useConversationStore } from '~/stores/conversation'
import { useMessageStore } from '~/stores/message'

/**
 * 应用初始化插件
 * 在 Nuxt 应用启动时执行，负责：
 * 1. 恢复用户登录状态
 * 2. 初始化用户配置
 * 3. 初始化会话列表
 * 4. 网络状态监听
 */
export default defineNuxtPlugin(async (nuxtApp) => {
  // 仅在客户端执行
  if (!import.meta.client) return

  // E2E-F-148：先装错误捕获，再做初始化 —— 否则初始化阶段自己抛的错抓不到。
  // 它内部 try/catch 且永不 reject，装失败也不影响后续初始化。
  try {
    // base URL 在这里（Nuxt plugin 上下文）取，事件回调里取不到 runtimeConfig。
    // ⚠️ 必须显式传 useRuntimeConfig()：getApiBaseUrl() 不传参时走的是
    // `globalThis.useRuntimeConfig?.()` 兜底，而 Nuxt 自动导入是**按文件注入**的
    // —— apiBaseUrl.ts 里并没有 useRuntimeConfig 这个符号，页面上
    // globalThis.useRuntimeConfig 也是 undefined ⇒ getApiBaseUrl() 抛
    // "API_BASE_URL 未配置" ⇒ 被下面这个 catch 吞掉 ⇒ 监听器压根没装。
    // （2026-09-29 IAB 实测抓到；单测 stub 了全局，测不出来。）
    installClientErrorReporter(getApiBaseUrl(useRuntimeConfig()))
  } catch (e) {
    // 这里原本只 console.warn —— 2026-09-29 的安装失败就是被它**静默吞掉**的，
    // 表现为"代码在、监听器没装、一条日志都没有"且毫无线索。
    // 改成把失败原因挂到 window 上：IAB/自动化可以直接读到，
    // 生产则至少能在用户反馈里看到一句明确的报错。
    console.error('[client-error] reporter install FAILED', e)
    ;(window as any).__CLIENT_ERROR_REPORTER_INIT_ERROR__ =
      e instanceof Error ? e.message : String(e)
  }

  console.log('🚀 应用初始化开始...')

  // 1. 初始化用户 Store
  const userStore = useUserStore()
  userStore.init()

  // 应用用户主题设置
  const config = userStore.getUserConfig()
  if (config && config.theme) {
    userStore.applyTheme(config.theme)
  }

  // 2. 检查登录状态
  if (userStore.isAuthenticated) {
    console.log('✅ 用户已登录')

    // Token 即将过期，提醒刷新
    if (userStore.isTokenExpired()) {
      console.warn('⚠️ Token 即将过期，建议刷新')
      // 这里可以触发自动刷新逻辑
    }

    // 获取最新用户信息
    try {
      await userStore.fetchUserInfo()
      // 获取后端配置后重新应用主题（覆盖之前的默认值）
      const latestConfig = userStore.getUserConfig()
      userStore.applyTheme(latestConfig.theme)
    } catch (error) {
      console.warn('获取用户信息失败', error)
    }

    // 3. 初始化会话列表
    const conversationStore = useConversationStore()
    try {
      await conversationStore.init()
    } catch (error) {
      console.warn('初始化会话列表失败', error)
    }
  } else {
    console.log('👤 用户未登录')
  }

  // 4. 监听网络状态变化
  setupNetworkListener()

  // 5. 监听页面可见性变化（用于处理后台切回前台时的数据同步）
  setupVisibilityListener()

  console.log('✨ 应用初始化完成')
})

/**
 * 设置网络状态监听
 */
function setupNetworkListener() {
  if (typeof window === 'undefined') return
  if (!navigator.onLine) {
    console.log('📡 当前处于离线状态')
  }

  window.addEventListener('online', () => {
    console.log('📡 网络已恢复')

    // TODO: 实现离线消息重试机制
    // const messageStore = useMessageStore();
    // messageStore.retryFailedMessages?.();
  })

  window.addEventListener('offline', () => {
    console.log('📡 网络已断开')
  })
}

function setupVisibilityListener() {
  if (typeof document === 'undefined') return
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') {
      console.log('👁️ 页面重新可见')

      // 检查是否需要刷新数据
      const userStore = useUserStore()
      if (userStore.isAuthenticated) {
        // 可以在这里触发数据同步
      }
    }
  })
}
