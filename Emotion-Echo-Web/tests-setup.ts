// vitest setup:让 Node 环境下能解析 Nuxt 的 auto-import(ref/computed)。
// Emotion-Echo-Web/app/{composables,components,pages}/*.vue 在 Nuxt 编译期
// 由 @nuxt/imports 注入 ref/computed/watch 等响应式工具;vitest 不经 Nuxt 注入,
// 这里显式 import 并挂在 globalThis 上供模块顶层调用解析。
import { ref, computed, watch, watchEffect, reactive } from 'vue'

;(globalThis as any).ref = ref
;(globalThis as any).computed = computed
;(globalThis as any).watch = watch
;(globalThis as any).watchEffect = watchEffect
;(globalThis as any).reactive = reactive
