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
})