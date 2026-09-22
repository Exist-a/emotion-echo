// vitest setup:让 Node 环境下能解析 Nuxt 的 auto-import(ref/computed)。
// emotion-echo-web/app/{composables,components,pages}/*.vue 在 Nuxt 编译期
// 由 @nuxt/imports 注入 ref/computed/watch 等响应式工具;vitest 不经 Nuxt 注入,
// 这里显式 import 并挂在 globalThis 上供模块顶层调用解析。
import { ref, computed, watch, watchEffect, reactive, isRef, onUnmounted, onMounted } from 'vue'

;(globalThis as any).ref = ref
;(globalThis as any).computed = computed
;(globalThis as any).watch = watch
;(globalThis as any).watchEffect = watchEffect
;(globalThis as any).reactive = reactive
// E2E-F-110：useVoiceRecorder 用了 isRef（同为 Nuxt auto-import），此前 setup 未提供
// ⇒ 该 composable 在 vitest 下连模块都加载不了（isRef is not defined）。
;(globalThis as any).isRef = isRef
;(globalThis as any).onUnmounted = onUnmounted
;(globalThis as any).onMounted = onMounted

// Stage 79: pinia store 测试需要 defineStore（同为 Nuxt auto-import 注入）。
import { defineStore as __piniaDefineStore, createPinia as __piniaCreatePinia, setActivePinia as __piniaSetActivePinia } from 'pinia'
;(globalThis as any).defineStore = __piniaDefineStore
;(globalThis as any).createPinia = __piniaCreatePinia
;(globalThis as any).setActivePinia = __piniaSetActivePinia

// Stage 101 follow-up: useCookie Nuxt auto-import fake。
// Nuxt 的 useCookie 依赖 #build/nuxt.config.mjs / useNuxtApp 等内部模块，
// vitest 拉不动。这里提供一个最小 mock：只覆盖 useAIStreamHandler / useApi /
// useTTSPlayer / useAIStream 实际用到的 `.value` getter，cookie 写走 happy-dom
// 的 document.cookie（happy-dom 默认 globalThis.document 已就位）。
function fakeUseCookie<T = string>(name: string) {
  const read = (): T | undefined => {
    if (typeof document === 'undefined') return undefined
    const match = document.cookie.split('; ').find((row) => row.startsWith(`${name}=`))
    if (!match) return undefined
    const raw = decodeURIComponent(match.slice(name.length + 1))
    if (raw === 'undefined') return undefined as T | undefined
    return raw as unknown as T
  }
  const cookieRef = ref(read() as T)
  // 监听 document.cookie 变化（happy-dom 支持）
  if (typeof document !== 'undefined') {
    setInterval(() => {
      const next = read()
      if (next !== cookieRef.value) cookieRef.value = next as T
    }, 50)
  }
  return cookieRef
}
;(globalThis as any).useCookie = fakeUseCookie

// E2E-F-39: useState 是 Nuxt auto-import, useAIStreamHandler / useConversationSender
// 等模块顶层直接调用 useState() 而不显式 import.
// 从 #app mock 模块取 useState 实现并挂到 globalThis.
import { useState as __useState } from '#app'
;(globalThis as any).useState = __useState
