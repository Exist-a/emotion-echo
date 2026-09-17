import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// Sprint 108 · useConversationSender 架构债 regression (static-source)
//
// Stage 107 浏览器实测发现 (dev 模式端到端):
//   new.vue handleSubmit → conversationSender.createNewConversation(value)
//   → 内部 await navigateTo(...) → new.vue unmount
//   → useConversationSender onUnmounted(() => { stopTTS(); cancelAIStream() }) 触发
//   → 闭包内 accumulatedDeltaText / callbacks / streamAbortController 全丢
//   → sendToExistingConversation 后续跑 sendAIStream 时, 状态丢失 → SSE 流无消费者
//   → 浏览器实测只看到 POST /conversations, 没有 POST /conversations/:id/messages / POST /ai/stream
//
// 4 个备选方案 (stage-107 §四):
//   A) 把 sender 状态抬升到 Pinia store (单例)
//   B) 把 sender 状态抬升到 useState() 顶层 composable (SSR-friendly 单例) [推荐]
//   C) 改 createNewConversation 用 setTimeout 替代 await navigateTo (hack)
//   D) [id].vue onMounted 检测 query param 自动触发 (解耦清楚, 每个入口都要改)
//
// 本测试钉住:
//   1) sendAIStream 的 fetch 状态 (AbortController + isStreaming) 不应依赖组件实例
//   2) accumulatedDeltaText 不应作为 composable 局部 ref (跨实例必丢)
//   3) createNewConversation 必须真的调用 sendToExistingConversation (源码合同)

const senderSrc = readFileSync('./app/composables/useConversationSender.ts', 'utf8')
const streamSrc = readFileSync('./app/composables/useAIStreamHandler.ts', 'utf8')

describe('useConversationSender · 架构债 (Stage 107 → Sprint 108)', () => {
  // 1) streamAbortController 不应只作为 composable 局部 let
  //    必须提到 useState() / Pinia store / module scope 才能跨组件实例共享
  //
  // 当前 (BUG): useAIStreamHandler.ts:49 let streamAbortController: AbortController | null = null
  // 期望 (FIX): useState('streamAbortController', ...) 或 module-scope let
  it('streamAbortController MUST NOT be component-instance-local (跨实例 unmount 必丢)', () => {
    // 在 composable 函数体内声明的 'let streamAbortController' 是组件实例局部
    // 必须用 useState() / Pinia store 才能跨实例共享
    // (注: streamAbortController 是非响应式 AbortController, 实际方案是 module-scope let)
    const isComponentLocal = /export function useAIStreamHandler[^{]*\{[\s\S]*?let streamAbortController[\s\S]*?\n\}/
    const usesUseState = /useState\s*(?:<[^>]*>\s*)?\(\s*['"`][^'"`]*stream/i.test(streamSrc)

    // 不应同时是"组件实例局部"且"没用 useState"
    const isBug = isComponentLocal.test(streamSrc) && !usesUseState
    expect(
      !isBug,
      'streamAbortController 不能仅作为 composable 局部变量 (Stage 107 浏览器实测确认: ' +
      'navigateTo 后 unmount → AbortController 引用丢失, fetch 状态不可控). ' +
      '必须改 useState() / module scope.'
    ).toBe(true)
  })

  // 2) accumulatedDeltaText 是 TTS 流式拼接缓冲, 也必须跨实例共享
  //    否则 A 实例 add delta, B 实例看到空字符串
  it('accumulatedDeltaText MUST NOT be component-instance-local ref (跨实例必丢)', () => {
    // 当前 (BUG): useConversationSender.ts:23 const accumulatedDeltaText = ref('')
    // 期望 (FIX): useState('accumulatedDeltaText', () => '') 或 Pinia store
    const isComponentLocal = /const accumulatedDeltaText\s*=\s*ref\(/.test(senderSrc)
    const usesSharedState = /useState\s*(?:<[^>]*>\s*)?\(\s*['"`][^'"`]*accumulatedDeltaText/i.test(senderSrc) ||
                            /useState\s*(?:<[^>]*>\s*)?\(\s*['"`][^'"`]*tts/i.test(senderSrc)

    const isBug = isComponentLocal && !usesSharedState
    expect(
      !isBug,
      'accumulatedDeltaText 不能仅作为 composable 局部 ref (Stage 107 浏览器实测: ' +
      'A 实例 sendAIStream 中累加 delta, 但 A 被 unmount 后状态丢失, B 实例看到空字符串). ' +
      '必须改 useState() / Pinia store.'
    ).toBe(true)
  })

  // 3) createNewConversation 必须真的调 sendToExistingConversation 触发 sendAIStream
  //    (源码合同; Stage 107 修后已满足, 但要钉住未来不被退化)
  it('createNewConversation MUST call sendToExistingConversation (Sprint 108 回归钉子)', () => {
    // 抽 createNewConversation 函数体
    const start = senderSrc.indexOf('const createNewConversation')
    const end = senderSrc.indexOf('const handleSubmit', start) // 可能在内部定义 handleSubmit 之前的另一个锚
    const endIdx = end === -1 ? senderSrc.length : end
    const block = senderSrc.slice(start, endIdx)

    expect(
      /sendToExistingConversation\s*\(/.test(block),
      'createNewConversation 必须内部调用 sendToExistingConversation 才能触发完整链路 (Stage 107 修后已满足, 钉住未来)'
    ).toBe(true)
  })
})
