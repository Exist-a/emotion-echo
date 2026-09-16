import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

// chat/conversation/new.vue · handleSubmit 合同 (static-source 断言)
//
// Bug (Stage 105 后续 / 2026-09-16 session 未修):
//   new.vue:80-86 handleSubmit 只创建会话并 navigateTo, 完全没调
//   conversationSender —— sendMessage + sendAIStream 链路没触发, 用户在
//   /chat/conversation/new 输入文本点发送后永远看不到 AI 回复。
//
//   useConversationSender.createNewConversation (useConversationSender.ts:173)
//   已经写好 createConversation → navigateTo → switchSession →
//   sendToExistingConversation 的完整链路; new.vue 应直接调它。
//
// 本测试 (RED, 修 bug 前必 FAIL):
//   1. handleSubmit 必须调用 conversationSender.createNewConversation 或
//      sendToExistingConversation（任一即可,因为 createNewConversation 内部就是
//      包了 sendToExistingConversation）
//   2. 不允许只调 createConversation + navigateTo 然后 return —— 这是 bug 的精确模式
//   3. user message 内容(value)必须作为参数透传, 不能被丢掉或仅截前 30 字作为 title

const src = readFileSync('./app/pages/chat/conversation/new.vue', 'utf8')

// 抽 handleSubmit 函数体
function handleSubmitBlock(): string {
  const start = src.indexOf('const handleSubmit')
  const endIdx = src.indexOf('const handleAttachment', start)
  return src.slice(start, endIdx === -1 ? src.length : endIdx)
}

describe('chat/conversation/new.vue · handleSubmit 合同', () => {
  const block = handleSubmitBlock()

  it('MUST call conversationSender.createNewConversation or sendToExistingConversation (发送链路入口)', () => {
    // 修 bug 前: handleSubmit 只有 post(API_ROUTES.createConversation) + navigateTo,
    //              完全没引用 conversationSender.send*
    // 修 bug 后: handleSubmit 应调 conversationSender.createNewConversation(value, ...)
    const usesSender =
      /conversationSender[\s\S]*createNewConversation/.test(block) ||
      /conversationSender[\s\S]*sendToExistingConversation/.test(block)
    expect(
      usesSender,
      'new.vue handleSubmit 必须调用 conversationSender.createNewConversation 或 sendToExistingConversation (Stage 105 未修 bug)'
    ).toBe(true)
  })

  it('MUST NOT bypass sender by only calling conversationStore.createConversation + navigateTo', () => {
    // 精确禁止"只建会话不发言"的旧实现
    const usesBareCreate =
      /post\(\s*API_ROUTES\.createConversation\.path/.test(block) &&
      /navigateTo\(\s*\{\s*name:\s*['"`]chat-conversation-detail['"`]/.test(block) &&
      !/conversationSender/.test(block)
    expect(
      usesBareCreate,
      'handleSubmit 不应再走 "只创建会话 + navigateTo" 路径, 必须经过 conversationSender 触发 sendMessage + sendAIStream'
    ).toBe(false)
  })

  it('user message content (value) MUST be passed as parameter, not just truncated to title', () => {
    // 旧实现: { title: value.slice(0, 30) } —— 把全文截前 30 字当 title, message 丢了
    // 新实现: createNewConversation(value, ...) —— value 全文作为消息内容透传
    const callsBlockWithFullValue =
      /createNewConversation\(\s*value\b/.test(block) ||
      /sendToExistingConversation\(\s*[^,]*,\s*value\b/.test(block)
    expect(
      callsBlockWithFullValue,
      'handleSubmit 必须把 value 全文作为消息透传给 sender, 不能仅截前 30 字当 title'
    ).toBe(true)
  })

  // Sprint 108: 端到端行为契约 - handleSubmit 必须触发完整发送链路
  //
  // Stage 107 修后浏览器实测发现: handleSubmit 调 conversationSender.createNewConversation(value),
  // 但 fetch hook 只看到 POST /conversations, 没有后续 POST /messages 和 POST /ai/stream。
  //
  // 根因: createNewConversation 内部 await navigateTo() 触发 new.vue unmount →
  //       useConversationSender composable 实例销毁 → 闭包内 callbacks / streamAbortController
  //       / accumulatedDeltaText 全丢 → sendAIStream 拿到的是清空状态, SSE 流即使发出去也
  //       没回调消费。
  //
  // 本断言钉住 handleSubmit 链路必须真的"发消息 + 等 AI 回复"行为, 不只"创建会话"。
  // 实现策略: createNewConversation 应使用跨实例共享的状态(useState/Pinia store/singleton),
  //          或者改用 setTimeout 替代 await navigateTo(详见 stage-107 §四 4 个备选方案)。
  it('handleSubmit MUST result in user message POST + AI stream POST (not only conversation POST)', () => {
    // 钉: 调用 createNewConversation 后, 必须有后续的 messageStore.sendMessage + sendAIStream 触发点
    //   (检查 useConversationSender.ts: createNewConversation 必须内部调 sendToExistingConversation)
    const senderSrc = readFileSync(
      resolve(__dirname, '../../../composables/useConversationSender.ts'),
      'utf8'
    )
    // 抽 createNewConversation 函数体（从定义到 const handleSubmit 前）
    const start = senderSrc.indexOf('const createNewConversation')
    const endIdx = senderSrc.indexOf('\n  return {', start)
    const block = endIdx === -1 ? senderSrc.slice(start) : senderSrc.slice(start, endIdx)
    const callsSendToExisting = /sendToExistingConversation\s*\(/.test(block)
    expect(
      callsSendToExisting,
      'createNewConversation 必须内部调用 sendToExistingConversation 才能触发完整链路\n' +
      '(POST /messages + POST /ai/stream + SSE 流). 否则 navigateTo 之后 fetch 链路被组件\n' +
      'unmount 钩子打断 (Stage 107 浏览器实测确认, Sprint 108 架构债修复要求)'
    ).toBe(true)
  })
})
