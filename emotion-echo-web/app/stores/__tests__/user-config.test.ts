// user store config 持久化测试（E2E-12）
//
// 背景：设置页 setFontSize/setTheme 调 updateProfile({config}) → PATCH /users/me，
// 但 BFF 层 config 被静默丢弃（E2E-F-82）。本测试锁定 store 层的装配契约：
// 1. setFontSize/setTheme 必须通过 API 持久化 config
// 2. getUserConfig 必须从服务端响应读取（不只靠本地状态）
// 3. applyTheme 必须操作 DOM classList
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

import { useUserStore } from '~/stores/user'

const patchMock = vi.fn()
const getMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  post: vi.fn(),
  get: (...args: unknown[]) => getMock(...args),
  put: vi.fn(),
  patch: (...args: unknown[]) => patchMock(...args),
  del: vi.fn(),
}))

const baseUser = {
  id: '1',
  username: 'echo',
  nickname: 'Echo',
  avatar: '/imgs/default-avatar.webp',
  age: 18,
  config: {} as Record<string, any>,
  createdAt: '2026-01-01T00:00:00Z',
}

describe('user store config 持久化（E2E-12）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    patchMock.mockReset()
    getMock.mockReset()
    patchMock.mockResolvedValue({ code: 0, message: 'ok', data: {} })
  })

  describe('setFontSize', () => {
    it('调 API 时 config 包含 fontSize', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setFontSize('large')

      expect(patchMock).toHaveBeenCalledTimes(1)
      const [path, body] = patchMock.mock.calls[0]!
      expect(path).toContain('/users/me')
      expect(body.config).toBeDefined()
      expect(body.config.fontSize).toBeDefined()
    })

    it('API 成功后本地 config 同步更新', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setFontSize('small')

      expect(store.getUserConfig().fontSize).toBe('small')
    })
  })

  describe('setTheme', () => {
    it('调 API 时 config 包含 theme', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setTheme('dark')

      expect(patchMock).toHaveBeenCalledTimes(1)
      const [, body] = patchMock.mock.calls[0]!
      expect(body.config).toBeDefined()
      expect(body.config.theme).toBe('dark')
    })

    it('API 成功后本地 config.theme 同步更新', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setTheme('dark')
      expect(store.getUserConfig().theme).toBe('dark')

      await store.setTheme('light')
      expect(store.getUserConfig().theme).toBe('light')
    })
  })

  describe('getUserConfig', () => {
    it('从 userInfo.config 读取已持久化的值', () => {
      const store = useUserStore()
      store.userInfo = {
        ...baseUser,
        config: { fontSize: '18px', theme: 'dark' },
      }

      const config = store.getUserConfig()
      expect(config.fontSize).toBe('large') // 18px → large（pxToFontSize 映射）
      expect(config.theme).toBe('dark')
    })

    it('config 为空时返回默认值（medium/light）', () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      const config = store.getUserConfig()
      expect(config.fontSize).toBe('medium')
      expect(config.theme).toBe('light')
    })
  })

  // E2E-12 测试点 #8：跟随系统主题的**运行时**变化
  // （plan §2「做」表明确要求 matchMedia 监听；建档实测 applyTheme 只在
  //  init/setTheme 各跑一次、无 change 监听 ⇒ 系统换主题页面不跟）
  describe('#8 跟随系统主题运行时监听', () => {
    let listeners: Array<(e: { matches: boolean }) => void>
    let matchesValue: boolean

    beforeEach(() => {
      listeners = []
      matchesValue = false
      vi.stubGlobal('matchMedia', (query: string) => ({
        matches: matchesValue,
        media: query,
        addEventListener: (_type: string, cb: (e: { matches: boolean }) => void) => {
          listeners.push(cb)
        },
        removeEventListener: (_type: string, cb: (e: { matches: boolean }) => void) => {
          listeners = listeners.filter((l) => l !== cb)
        },
      }))
    })

    afterEach(() => {
      vi.unstubAllGlobals()
      document.documentElement.classList.remove('dark')
    })

    it('setTheme(auto) 会注册 matchMedia change 监听', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setTheme('auto')

      expect(listeners.length).toBeGreaterThan(0)
    })

    it('系统切到深色时 html.dark 跟随变化（无需刷新）', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }
      await store.setTheme('auto')
      expect(document.documentElement.classList.contains('dark')).toBe(false)

      matchesValue = true
      listeners.forEach((cb) => cb({ matches: true }))

      expect(document.documentElement.classList.contains('dark')).toBe(true)
    })

    it('系统切回浅色时 html.dark 被移除', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }
      matchesValue = true
      await store.setTheme('auto')
      expect(document.documentElement.classList.contains('dark')).toBe(true)

      matchesValue = false
      listeners.forEach((cb) => cb({ matches: false }))

      expect(document.documentElement.classList.contains('dark')).toBe(false)
    })

    it('切离 auto（改为 dark）后解除跟随，系统变化不再影响', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }
      await store.setTheme('auto')
      await store.setTheme('dark')

      const before = document.documentElement.classList.contains('dark')
      matchesValue = false
      listeners.forEach((cb) => cb({ matches: false }))

      expect(document.documentElement.classList.contains('dark')).toBe(before)
      expect(before).toBe(true)
    })
  })

  // E2E-12 §2「契约漂移清理」①：fontSize 的**唯一表示**。
  // 真实契约：wire（服务端 users.config）存 px；UI 对外是语义名。
  // 此前 api.ts 把两种形态并成一个 union 把漂移藏住，store 又在本地存语义名
  // （靠 `as any` 绕过类型）⇒ "选哪档 / 读回哪档" 没有单一事实源。
  describe('#12 值契约一致性（px ↔ 语义名 唯一映射）', () => {
    afterEach(() => {
      document.documentElement.classList.remove('dark')
    })

    it('本地 config 与服务端同形态（px），对外读取才转成语义名', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setFontSize('large')

      expect(
        store.userInfo!.config.fontSize,
        '本地缓存应与写入服务端的值同形态（px），不能再存语义名',
      ).toBe('18px')
      expect(store.getUserConfig().fontSize, '对外读取转成语义名').toBe('large')
    })

    it('三档 px → 语义名 映射唯一且完整', () => {
      const store = useUserStore()
      const pairs: Array<[string, string]> = [
        ['14px', 'small'],
        ['16px', 'medium'],
        ['18px', 'large'],
      ]
      for (const [px, name] of pairs) {
        store.userInfo = { ...baseUser, config: { fontSize: px as never } }
        expect(store.getUserConfig().fontSize, `${px} 应映射为 ${name}`).toBe(name)
      }
    })

    it('未知 px 值按原样回退（不崩）', () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: { fontSize: '99px' as never } }
      expect(store.getUserConfig().fontSize).toBe('99px')
    })
  })

  // E2E-12 #5/#9 根因：冷启动时 init.ts 的「fetchUserInfo 后重应用主题」整块被
  // gated 在 isAuthenticated 上，而 userInfo 来自 localStorage、冷启动为空 ⇒
  // 整块被跳过 ⇒ 服务端 config 已取到但主题从不应用（页面停在默认浅色）。
  // 契约：只要 store 拿到了服务端 config，就必须把主题应用出去。
  describe('#5/#9 拿到服务端 config 后必须应用主题', () => {
    afterEach(() => {
      document.documentElement.classList.remove('dark')
    })

    it('fetchUserInfo 取回 theme=dark 后 html 应带上 dark', async () => {
      const store = useUserStore()
      getMock.mockResolvedValueOnce({ ...baseUser, config: { theme: 'dark' } })

      await store.fetchUserInfo()

      expect(
        document.documentElement.classList.contains('dark'),
        'fetchUserInfo 拿到 dark 后应立即应用，而不是等下一次 setTheme',
      ).toBe(true)
    })

    it('fetchUserInfo 取回 theme=light 后 html 不应带 dark', async () => {
      const store = useUserStore()
      document.documentElement.classList.add('dark')
      getMock.mockResolvedValueOnce({ ...baseUser, config: { theme: 'light' } })

      await store.fetchUserInfo()

      expect(document.documentElement.classList.contains('dark')).toBe(false)
    })
  })

  // E2E-12 测试点 #10：冷启动无主题闪烁
  // SSR 首屏要能直接渲染出正确的 html class，就必须有一个**服务端可读**的
  // 主题镜像（用户的 config 存在服务端，但 SSR 渲染时拿不到 API 往返）。
  // 方案：applyTheme 把生效主题镜像进非 HttpOnly 的 ee_theme cookie。
  describe('#10 首屏主题镜像 cookie', () => {
    beforeEach(() => {
      // 清掉可能残留的 cookie
      document.cookie = 'ee_theme=; path=/; max-age=0'
    })

    afterEach(() => {
      document.documentElement.classList.remove('dark')
    })

    it('applyTheme(dark) 写入 ee_theme=dark 镜像 cookie', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setTheme('dark')

      expect(document.cookie).toContain('ee_theme=dark')
    })

    it('applyTheme(light) 把镜像 cookie 更新为 light', async () => {
      const store = useUserStore()
      store.userInfo = { ...baseUser, config: {} }

      await store.setTheme('dark')
      expect(document.cookie).toContain('ee_theme=dark')

      await store.setTheme('light')
      expect(document.cookie).toContain('ee_theme=light')
      expect(document.cookie).not.toContain('ee_theme=dark')
    })
  })
})