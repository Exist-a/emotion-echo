import type { returnMsgType } from '~/types/commonType'
import type {
  LoginParams,
  LoginData,
  RegisterParams,
  UserInfo,
  UpdateProfileParams,
  SendVerificationCodeParams
} from '~/types/api'
import { get, post, put, patch } from '~/composables/useApi'
import { API_ROUTES } from '~/lib/apiRoutes'

export const useUserStore = defineStore('user', () => {
  // ==================== State ====================

  const userInfo = ref<UserInfo | null>(null)
  const accessToken = ref<string>('')
  const tokenExpiry = ref<number>(0)
  const isLoading = ref(false)
  let fetchUserInfoPromise: Promise<any> | null = null

  // ==================== Getters ====================

  const isAuthenticated = computed(() => !!accessToken.value && !!userInfo.value?.id)

  const getNickname = computed(() => userInfo.value?.nickname || '用户')
  const getAvatarPath = computed(() => userInfo.value?.avatar || '/imgs/default-avatar.webp')
  const getId = computed(() => userInfo.value?.id || '')
  const getAge = computed(() => userInfo.value?.age || 18)
  const getAccessToken = computed(() => accessToken.value)

  /**
   * 获取用户配置
   */
  // 字体大小映射：语义名称 <-> px值
  const fontSizeToPx: Record<string, string> = {
    small: '14px',
    medium: '16px',
    large: '18px'
  }
  const pxToFontSize: Record<string, string> = {
    '14px': 'small',
    '16px': 'medium',
    '18px': 'large'
  }

  const getUserConfig = () => {
    const config = userInfo.value?.config
    // 后端可能存储px值，转换回语义名称
    const fontSize = config?.fontSize ? pxToFontSize[config.fontSize] || config.fontSize : 'medium'
    return {
      fontSize,
      theme: config?.theme || 'light'
    }
  }

  /**
   * 设置字体大小
   */
  const setFontSize = async (size: string) => {
    if (!userInfo.value) return
    if (!userInfo.value.config) {
      userInfo.value.config = {}
    }

    // 先调 API 成功，再更新本地
    const pxSize = fontSizeToPx[size] || size
    const result = await updateProfile({
      config: {
        ...userInfo.value.config,
        fontSize: pxSize as any
      }
    })
    if (!result.isOk) return

    userInfo.value.config.fontSize = size as any
  }

  const setTheme = async (theme: 'light' | 'dark' | 'auto') => {
    if (!userInfo.value) return
    if (!userInfo.value.config) {
      userInfo.value.config = {}
    }

    // 先调 API 成功，再更新本地和 DOM
    const result = await updateProfile({
      config: {
        ...userInfo.value.config,
        theme
      }
    })
    if (!result.isOk) return

    userInfo.value.config.theme = theme
    applyTheme(theme)
  }

  /**
   * 应用主题到 DOM
   */
  const applyTheme = (theme: 'light' | 'dark' | 'auto') => {
    if (!import.meta.client) return
    const html = document.documentElement
    let effectiveTheme = theme
    if (theme === 'auto') {
      effectiveTheme = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
    }
    if (effectiveTheme === 'dark') {
      html.classList.add('dark')
    } else {
      html.classList.remove('dark')
    }
  }

  // ==================== Actions ====================

  /**
   * 设置 AccessToken
   * P0-R2-1: 移除 localStorage/sessionStorage 存储（防 XSS 窃取持久 token）
   * 仅保留 cookie（SameSite=Lax）供 SSR 读取 + APISIX jwt-auth 验证
   * @param rememberMe 保留参数兼容性，不再影响存储策略
   */
  const setAccessToken = (token: string, expiresIn: number = 900, rememberMe: boolean = false) => {
    accessToken.value = token
    tokenExpiry.value = Date.now() + expiresIn * 1000

    // SSR 支持：设置 cookie，maxAge 使用实际的 expiresIn
    // cookie 同时被 APISIX jwt-auth 读取（seed.sh 配置 cookie: "access_token"）
    const tokenCookie = useCookie('access_token', {
      maxAge: expiresIn,
      sameSite: 'lax',
      path: '/'
    })
    tokenCookie.value = token
  }

  /**
   * 清除 Token
   */
  const clearToken = () => {
    accessToken.value = ''
    tokenExpiry.value = 0
    userInfo.value = null

    if (import.meta.client) {
      localStorage.removeItem('user_info')
      sessionStorage.removeItem('user_info')
    }

    // 清除 SSR cookie
    const tokenCookie = useCookie('access_token', {
      maxAge: -1,
      sameSite: 'lax',
      path: '/'
    })
    tokenCookie.value = null
  }

  /**
   * 检查 Token 是否过期
   */
  const isTokenExpired = (): boolean => {
    if (!tokenExpiry.value) return true
    return Date.now() > tokenExpiry.value - 60000 // 提前1分钟认为过期
  }

  /**
   * 发送验证码
   */
  const sendVerificationCode = async (
    params: SendVerificationCodeParams
  ): Promise<returnMsgType> => {
    try {
      await post(API_ROUTES.authVerificationCode.path, params)
      return { isOk: true, msg: '验证码已发送' }
    } catch (error: any) {
      return { isOk: false, msg: error.message || '发送失败' }
    }
  }

  /**
   * 用户注册
   */
  const register = async (params: RegisterParams): Promise<returnMsgType> => {
    try {
      isLoading.value = true
      const data = await post<LoginData>(API_ROUTES.authRegister.path, params)

      // 保存 Token（注册默认不记住）
      setAccessToken(data.accessToken, data.expiresIn, false)

      // 保存用户信息
      userInfo.value = data.user

      // 持久化 userInfo（与 token 同策略）
      if (import.meta.client) {
        sessionStorage.setItem('user_info', JSON.stringify(data.user))
        localStorage.removeItem('user_info')
      }

      return { isOk: true, msg: '注册成功' }
    } catch (error: any) {
      return { isOk: false, msg: error.message || '注册失败' }
    } finally {
      isLoading.value = false
    }
  }

  /**
   * 用户登录
   */
  const login = async (params: LoginParams): Promise<returnMsgType> => {
    try {
      isLoading.value = true
      const data = await post<LoginData>(API_ROUTES.authLogin.path, params)

      // 根据 rememberMe 差异化存储
      const rememberMe = params.rememberMe ?? false
      setAccessToken(data.accessToken, data.expiresIn, rememberMe)

      // 保存用户信息
      userInfo.value = data.user

      // 持久化 userInfo（与 token 同策略）
      if (import.meta.client) {
        const storage = rememberMe ? localStorage : sessionStorage
        const other = rememberMe ? sessionStorage : localStorage
        storage.setItem('user_info', JSON.stringify(data.user))
        other.removeItem('user_info')
      }

      return { isOk: true, msg: '登录成功' }
    } catch (error: any) {
      return { isOk: false, msg: error.message || '登录失败' }
    } finally {
      isLoading.value = false
    }
  }

  /**
   * 获取用户信息
   */
  const fetchUserInfo = async (): Promise<returnMsgType> => {
    // 防止并发请求
    if (fetchUserInfoPromise) {
      return fetchUserInfoPromise
    }

    fetchUserInfoPromise = (async () => {
      try {
        const data = await get<UserInfo>(API_ROUTES.userProfile.path)
        userInfo.value = data
        return { isOk: true, msg: '获取成功' }
      } catch (error: any) {
        return { isOk: false, msg: error.message || '获取失败' }
      } finally {
        fetchUserInfoPromise = null
      }
    })()

    return fetchUserInfoPromise
  }

  /**
   * 更新用户信息
   *
   * Stage 71 PR-C8：改用 PATCH /users/me（与 BFF user_handler.go:63 对齐）。
   * 修复前 PUT /user/profile → 404（BFF 无此 method+path 组合）。
   */
  const updateProfile = async (params: UpdateProfileParams): Promise<returnMsgType> => {
    try {
      await patch(API_ROUTES.userUpdateProfile.path, params)
      // 更新本地数据
      if (userInfo.value) {
        userInfo.value = { ...userInfo.value, ...params }
      }
      return { isOk: true, msg: '更新成功' }
    } catch (error: any) {
      return { isOk: false, msg: error.message || '更新失败' }
    }
  }

  /**
   * 修改昵称
   */
  const editNickname = async (name: string): Promise<returnMsgType> => {
    return updateProfile({ nickname: name })
  }

  /**
   * 修改年龄
   */
  const editAge = async (age: number): Promise<returnMsgType> => {
    return updateProfile({ age })
  }

  /**
   * 用户登出
   */
  const logout = async (): Promise<returnMsgType> => {
    try {
      await post(API_ROUTES.authLogout.path)
      clearToken()
      userInfo.value = null
      return { isOk: true, msg: '登出成功' }
    } catch (error: any) {
      // 即使失败也要清除本地状态
      clearToken()
      userInfo.value = null
      return { isOk: true, msg: '登出成功' }
    }
  }

  /**
   * 初始化（P0-R2-1: 从 cookie 恢复，不再读 localStorage/sessionStorage）
   */
  const init = () => {
    if (!import.meta.client) return

    // 从 cookie 恢复 token（页面刷新后 Pinia 状态丢失，cookie 仍在）
    const tokenCookie = useCookie('access_token', { sameSite: 'lax', path: '/' })
    const storedToken = tokenCookie.value

    if (storedToken) {
      // 简单校验 token 格式（JWT = 3 段 base64）
      const parts = storedToken.split('.')
      if (parts.length !== 3) {
        clearToken()
        return
      }

      // 从 JWT payload 解析过期时间
      try {
        const base64 = parts[1].replace(/-/g, '+').replace(/_/g, '/')
        const payload = JSON.parse(atob(base64))
        if (payload.exp) {
          const expiryMs = payload.exp * 1000
          if (expiryMs <= Date.now()) {
            clearToken()
            return
          }
          tokenExpiry.value = expiryMs
        }
      } catch {
        // JWT 解析失败，不清除 token（可能是格式不同的 JWT）
      }

      accessToken.value = storedToken

      // 恢复 userInfo（让 isAuthenticated 立即为 true）
      if (import.meta.client) {
        const storedUser = localStorage.getItem('user_info')
        if (storedUser) {
          try {
            userInfo.value = JSON.parse(storedUser)
          } catch {
            userInfo.value = null
          }
        }
      }
    }
  }

  return {
    // State
    userInfo,
    accessToken,
    isLoading,
    // Getters
    isAuthenticated,
    getNickname,
    getAvatarPath,
    getId,
    getAge,
    getAccessToken,
    getUserConfig,
    setFontSize,
    setTheme,
    applyTheme,
    // Actions
    setAccessToken,
    clearToken,
    isTokenExpired,
    sendVerificationCode,
    register,
    login,
    fetchUserInfo,
    updateProfile,
    editNickname,
    editAge,
    logout,
    init
  }
})
