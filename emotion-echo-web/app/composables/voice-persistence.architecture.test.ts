import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'

// 语音消息落库静态契约（E2E-16 plan §2.A.4 / 测试点 #5，2026-09-22 用户实测钉出）
//
// 用户实测：新会话发语音 → 成功上屏 + AI 回复 → **刷新后语音气泡消失**。
// 根因两段：① useVoiceRecorder 只 push 内存行（行为测试钉在 useVoiceRecorder.test.ts）；
// ② ai/stream 走 skipUserMessage=true 且不携带服务端真实消息 id ⇒ 融合/face 无绑定目标。
// 本文件钉接线层：sender 必须支持 userMessageId 透传，[id].vue 必须把它接进 ai/stream 参数。

const senderSrc = readFileSync('./app/composables/useConversationSender.ts', 'utf8')
const pageSrc = readFileSync('./app/pages/chat/conversation/[id].vue', 'utf8')

describe('语音消息落库 · 接线静态契约（测试点 #5）', () => {
  it('useConversationSender extraParams 必须声明 userMessageId', () => {
    expect(
      /userMessageId\??:\s*string/.test(senderSrc),
      'extraParams 必须含 userMessageId?: string —— 语音消息已由 useVoiceRecorder 真落库，' +
        'ai/stream 的 messageId 必须用该服务端 id（否则 face_emotion_results.message_id 绑到不存在的 clientMsgId，融合取不到 face 行）',
    ).toBe(true)
  })

  it('skipUserMessage 分支必须用 extraParams.userMessageId 覆盖 ai/stream 绑定 id', () => {
    // 找 sendToExistingConversation 的 persist 分支
    const persistIdx = senderSrc.indexOf('skipUserMessage')
    expect(persistIdx, 'skipUserMessage 逻辑必须存在').toBeGreaterThan(-1)
    expect(
      // \??\. 同时接受收窄写法 extraParams.userMessageId 与 extraParams?.userMessageId
      /userMessageId\s*=\s*extraParams\??\.userMessageId|extraParams\??\.userMessageId\s*&&\s*\(\s*userMessageId\s*=/.test(senderSrc),
      '当 skipUserMessage=true 且提供了 userMessageId 时，userMessageId 必须被赋为该值，' +
        'sendAIStream({ messageId: userMessageId }) 才能绑到真实消息',
    ).toBe(true)
  })

  it('[id].vue 的 handleVoiceStreamResponse 必须把 userMessageId 透传进 extraParams', () => {
    const fnIdx = pageSrc.indexOf('handleVoiceStreamResponse')
    expect(fnIdx, 'handleVoiceStreamResponse 必须存在').toBeGreaterThan(-1)
    // 窗口须覆盖整个函数体：D-14 轮新增 faceEmotion/faceConfidence 行后 900 不够
    //（skipUserMessage: true 被挤出窗口 ⇒ 假 FAIL，2026-09-23 CI 实测）
    const block = pageSrc.slice(fnIdx, fnIdx + 1600)
    expect(
      /userMessageId/.test(block),
      '[id].vue 必须把 onUploadSuccess 收到的 userMessageId 传给 sendToExistingConversation 的 extraParams，' +
        '否则语音 AI 链路的 messageId 仍是随机 clientMsgId，融合/face 绑定落空',
    ).toBe(true)
    expect(
      /skipUserMessage:\s*true/.test(block),
      '语音路径仍应 skipUserMessage（消息已在 useVoiceRecorder 里落库，避免双写）',
    ).toBe(true)
  })
})
