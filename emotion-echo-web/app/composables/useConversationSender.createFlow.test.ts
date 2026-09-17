import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// Sprint 110 · createNewConversation 跨实例 SSE race 修复 (static-source)
//
// IAB 实测 2026-09-17 复现 (docs/evidence/gui-test-screenshots/a8-02-after-send.png):
//   - 浏览器点发送, URL 跳到 /chat/conversation/78 ✓
//   - fetch hook: POST /conversations + GET /messages + POST /messages ✓
//   - fetch hook: POST /api/v1/ai/stream 缺席 ✗
//   - BFF 日志 0 条 ai-stream 调用 ✗
//   - chat-svc DB 无 AI 消息入库 ✗
//
// 根因 (useConversationSender.ts:177-219 + useAIStreamHandler.ts:81-83):
//   1. createNewConversation 内 await navigateTo → new.vue 同步触发 onUnmounted → cancelAIStream
//   2. cancelAIStream 检查 module-scope streamAbortController (此时 null) → no-op
//   3. 但路由切换 race 下 finally 块可能未跑完, isStreaming 卡 true → 下次 sendAIStream 被 guard 拒
//   4. streamCancelled.value 也未在 finally 清零, 残留状态污染下次调用
//
// 修复方案 (Sprint 110):
//   A) useConversationSender 新增 isInCreateFlow useState (共享 flag)
//   B) onUnmounted 加 guard: isInCreateFlow=true 时不 cancel
//   C) createNewConversation 顺序: createConversation → isInCreateFlow=true →
//      navigateTo (fire-and-forget, 不 await) → switchSession → sendToExistingConversation →
//      finally: isInCreateFlow=false
//   D) useAIStreamHandler finally 块清 streamCancelled.value

const senderSrc = readFileSync('./app/composables/useConversationSender.ts', 'utf8')
const streamSrc = readFileSync('./app/composables/useAIStreamHandler.ts', 'utf8')

describe('useConversationSender · createFlow 跨实例 SSE race (Sprint 110)', () => {
  // 1) isInCreateFlow 必须作为 useState 共享 flag (跨实例可见)
  it('isInCreateFlow MUST be declared via useState (Nuxt 3 SSR-safe singleton)', () => {
    const usesUseState = /useState\s*(?:<[^>]*>)?\s*\(\s*['"`][^'"`]*isInCreateFlow/i.test(senderSrc)
    expect(
      usesUseState,
      'useConversationSender 必须新增 isInCreateFlow = useState<boolean>("conv-sender:isInCreateFlow", () => false), ' +
      '这样 new.vue / [id].vue 跨实例都能读到同一 flag, unmount guard 才能正确跳过 cancel.'
    ).toBe(true)
  })

  // 2) createNewConversation 内, isInCreateFlow=true 必须在 navigateTo 之前置位
  //    (关键顺序: 先置 flag, 再触发路由切换, 否则 new.vue unmount 时 flag 仍 false → abort)
  it('createNewConversation MUST set isInCreateFlow.value=true BEFORE navigateTo', () => {
    const createStart = senderSrc.indexOf('const createNewConversation')
    expect(createStart, 'createNewConversation 函数必须存在').toBeGreaterThan(-1)
    // 找下一个顶层 const 声明 (函数结束)
    const nextConst = senderSrc.indexOf('\n  const ', createStart + 1)
    const block = senderSrc.slice(createStart, nextConst === -1 ? senderSrc.length : nextConst)

    const flagIdx = block.indexOf('isInCreateFlow.value = true')
    const navIdx = block.indexOf('navigateTo({')
    expect(
      flagIdx > -1 && navIdx > -1 && flagIdx < navIdx,
      'createNewConversation 内, isInCreateFlow.value=true 必须出现在 navigateTo({ 之前. ' +
      'IAB 实测 2026-09-17: new.vue 的 onUnmounted 在 navigateTo 期间同步触发, ' +
      '若 flag 未提前置 true → cancelAIStream 把 SSE abort → POST /api/v1/ai/stream 永远不出栈.'
    ).toBe(true)
  })

  // 3) createNewConversation 必须有 finally 块复位 isInCreateFlow=false
  //    (防止一次创建卡死 flag, 后续 unmount 永远跳过 cancel)
  it('createNewConversation MUST reset isInCreateFlow=false in finally', () => {
    const createStart = senderSrc.indexOf('const createNewConversation')
    const nextConst = senderSrc.indexOf('\n  const ', createStart + 1)
    const block = senderSrc.slice(createStart, nextConst === -1 ? senderSrc.length : nextConst)

    const hasFinally = /finally\s*\{[\s\S]*?isInCreateFlow\.value\s*=\s*false/.test(block)
    expect(
      hasFinally,
      'createNewConversation 必须有 try/finally 结构, finally 块复位 isInCreateFlow=false. ' +
      '否则某次创建异常后 flag 永久卡 true, 所有 [id].vue 的 onUnmounted 都不 cancel → 下次 sendAIStream ' +
      '的 isStreaming guard 失效风险累积.'
    ).toBe(true)
  })

  // 4) onUnmounted 内 cancelAIStream 必须被 isInCreateFlow guard
  it('onUnmounted MUST guard cancelAIStream by isInCreateFlow', () => {
    // 找 onUnmounted 块 (从 'onUnmounted(' 到匹配的 '})' )
    const onUnmountedStart = senderSrc.indexOf('onUnmounted(')
    expect(onUnmountedStart, 'onUnmounted 钩子必须存在').toBeGreaterThan(-1)
    // 简化: 截取 onUnmounted 后 300 字符
    const block = senderSrc.slice(onUnmountedStart, onUnmountedStart + 400)

    const guardPattern = /if\s*\(\s*!?isInCreateFlow\.value\s*\)\s*\{[\s\S]*?cancelAIStream\(\)/
    expect(
      guardPattern.test(block),
      'onUnmounted 内 cancelAIStream 必须被 if (!isInCreateFlow.value) guard. ' +
      '否则 new.vue unmount 时即使 isInCreateFlow=true (创建流中) 也会 abort SSE, ' +
      '前端看到 POST /ai/stream 永远缺席 (A8 现象).'
    ).toBe(true)
  })

  // 5) useAIStreamHandler finally 块必须清 streamCancelled=false
  //    (防御性: 上一次流 aborted 后残留 cancelled=true, 下次 sendAIStream 会受影响)
  it('useAIStreamHandler finally block MUST reset streamCancelled.value=false', () => {
    // 找 sendAIStream 的 finally 块
    const finallyStart = streamSrc.lastIndexOf('} finally {')
    expect(finallyStart, 'sendAIStream finally 块必须存在').toBeGreaterThan(-1)
    const block = streamSrc.slice(finallyStart, finallyStart + 400)

    const hasReset = /streamCancelled\.value\s*=\s*false/.test(block)
    expect(
      hasReset,
      'useAIStreamHandler sendAIStream 的 finally 块必须显式 streamCancelled.value=false. ' +
      '否则上一次 cancelled=true 残留, 下次 sendAIStream 的 catch 分支 (line 205-207) ' +
      '会判定为 cancelled → 返回 "已取消" 但实际 fetch 已正常返回, UI 永远看不到 AI 回复.'
    ).toBe(true)
  })
})
