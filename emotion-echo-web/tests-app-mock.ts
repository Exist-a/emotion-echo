/**
 * Mock #app module for vitest.
 *
 * Nuxt auto-imports (useCookie, useRuntimeConfig, etc.) are unavailable in
 * vitest because tests bypass Nuxt's transform pipeline. This file provides
 * minimal stubs so that modules importing from '#app' can be loaded.
 */
import { ref, type Ref } from 'vue'

// Nuxt useState: SSR-safe shared singleton keyed by string.
// In tests we use a simple Map so all components see the same ref for a given key.
const __stateMap = new Map<string, Ref>()
export function useState<T>(key: string, init?: () => T): Ref<T> {
  if (!__stateMap.has(key)) {
    __stateMap.set(key, ref(init ? init() : undefined) as Ref)
  }
  return __stateMap.get(key) as Ref<T>
}

export function useCookie<T = string>(name: string) {
  if (typeof document === 'undefined') return ref(undefined as T | undefined)
  const match = document.cookie.split('; ').find((row) => row.startsWith(`${name}=`))
  return ref(match ? decodeURIComponent(match.slice(name.length + 1)) : undefined) as any
}

export function useRuntimeConfig() {
  return { public: { API_BASE_URL: 'http://localhost:19080/api/v1' } }
}

export function navigateTo() {
  return Promise.resolve()
}
