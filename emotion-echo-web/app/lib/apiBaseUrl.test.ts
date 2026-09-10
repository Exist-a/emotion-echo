// app/lib/apiBaseUrl.test.ts
//
// PR-A · decision 18 #24 (2026-09-10) — API base URL fail-fast
//
// 目的：getApiBaseUrl() 在 NUXT_PUBLIC_API_BASE_URL 未配/空串/字段缺失时抛错，
//       而不是静默回退到死端口（之前 3 处 fallback 字面值 8894 / 8080 的 bug）。
//
// 来源：
//   - emotion-echo-web/app/lib/apiBaseUrl.ts（被测代码）
//   - doc-drift-registry.md #24
//   - emotion-echo-web/nuxt.config.ts:19 默认值 'http://localhost:19080/api/v1'

import { describe, it, expect, beforeEach, afterEach } from 'vitest'
import { getApiBaseUrl } from './apiBaseUrl'

describe('getApiBaseUrl · PR-A fail-fast (decision 18 #24)', () => {
  const originalConfig = (globalThis as any).useRuntimeConfig

  afterEach(() => {
    ;(globalThis as any).useRuntimeConfig = originalConfig
  })

  it('happy path：API_BASE_URL 已配 → 直接返回', () => {
    expect(getApiBaseUrl({ public: { API_BASE_URL: 'http://localhost:19080/api/v1' } } as any))
      .toBe('http://localhost:19080/api/v1')
  })

  it('PR-A · API_BASE_URL 未配（空串） → 抛错，提示 NUXT_PUBLIC_API_BASE_URL', () => {
    expect(() => getApiBaseUrl({ public: { API_BASE_URL: '' } } as any))
      .toThrow(/NUXT_PUBLIC_API_BASE_URL 未配置/)
  })

  it('PR-A · public.API_BASE_URL 字段缺失 → 抛错，不静默回退', () => {
    expect(() => getApiBaseUrl({ public: {} } as any))
      .toThrow(/NUXT_PUBLIC_API_BASE_URL 未配置/)
  })

  it('PR-A · config 不传且 useRuntimeConfig 不可用 → 抛错', () => {
    ;(globalThis as any).useRuntimeConfig = undefined
    expect(() => getApiBaseUrl()).toThrow(/NUXT_PUBLIC_API_BASE_URL 未配置/)
  })

  it('PR-A · useRuntimeConfig 抛错（无 Nuxt 上下文） → 抛错，不静默回退', () => {
    ;(globalThis as any).useRuntimeConfig = () => { throw new Error('no Nuxt') }
    expect(() => getApiBaseUrl()).toThrow(/NUXT_PUBLIC_API_BASE_URL 未配置/)
  })

  it('PR-A · fallback 不能落到 localhost:8080 或 localhost:8894（项目无此服务）', () => {
    // 显式确认抛错信息中不含这两个端口作为"成功返回值"
    let errMsg = ''
    try {
      getApiBaseUrl({ public: { API_BASE_URL: '' } } as any)
    } catch (e: any) {
      errMsg = e.message
    }
    expect(errMsg).not.toContain('localhost:8080')
    expect(errMsg).not.toContain('localhost:8894')
  })
})