// plugins/init.ts - 应用初始化插件
import { getApiBaseUrl } from '~/lib/apiBaseUrl'
import { installClientErrorReporter } from '~/utils/clientErrorReporter'
import { useUserStore } from '~/stores/user'
import { useConversationStore } from '~/stores/conversation'
import { useMessageStore } from '~/stores/message'
import { refreshAccessToken } from '~/composables/useApi'
import { createTokenRenewal } from '~/lib/tokenRenewal'
import { watch } from 'vue'

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

  // E2E-F-207：滑动续期 —— 令牌仍有效时在其寿命 75% 处主动换新。
  // ⚠️ **不能** gate 在 isAuthenticated 上：该 computed 还要求 `userInfo.id`，而
  // userInfo 是页面元数据、此处可能尚未恢复（auth.global.ts v2 同样只看 accessToken，
  // 否则会把已登录用户踢回登录页）。无令牌时 schedule() 内部直接跳过 ⇒ 无条件调用安全。
  startTokenRenewal(userStore)

  // 应用用户主题设置
  const config = userStore.getUserConfig()
  if (config && config.theme) {
    userStore.applyTheme(config.theme)
  }

  // 2. 检查登录状态
  if (userStore.isAuthenticated) {
    console.log('✅ 用户已登录')

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

/**
 * 启动访问令牌滑动续期（E2E-F-207）。
 *
 * 触发点：① 应用启动（已登录时）；② 页面重新可见（长时间后台后计时器可能被节流，
 * 需按当前令牌重算）；③ 每次续期成功后自动重排。
 * 失败**不登出**：留待真正用到该令牌时由 401 兜底（避免网络抖动把人踢下线）。
 */
function startTokenRenewal(userStore: ReturnType<typeof useUserStore>) {
  if (typeof window === 'undefined') return

  const readToken = (): string | null => {
    // Pinia setup store 的 computed 取出来已是值；兼容个别场景下的 ref 包装
    const raw: any = userStore.getAccessToken
    return typeof raw === 'string' ? raw : (raw?.value ?? null)
  }

  const renewal = createTokenRenewal({
    getToken: readToken,
    renew: async () => {
      const token = await refreshAccessToken()
      if (token) console.log('[token-renewal] 续期成功，令牌寿命已重置')
      return token
    },
    onError: (e) => console.warn('[token-renewal] 续期失败（保留登录态，交由 401 兜底）', e),
  })

  renewal.schedule()

  // 登录 / 续期都会改变 store 中的令牌 ⇒ 重新排程。
  // 否则"登录后不刷新整页"就永远不排程（本轮 Playwright 实测抓到：SPA 登录不触发
  // plugin 重跑，调度器一直是未排程态）。
  watch(
    () => readToken(),
    () => renewal.schedule(),
  )

  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') renewal.schedule()
  })

  // 供 IAB / 自动化验证与排障（仅 dev 构建暴露，生产不挂）
  if (import.meta.dev) {
    ;(window as any).__tokenRenewal = {
      isScheduled: () => renewal.isScheduled(),
      renewNow: () => refreshAccessToken(),
      stop: () => renewal.stop(),
    }
  }
}
