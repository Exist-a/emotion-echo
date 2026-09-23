// useAIStreamHandler.d14.test.ts — D-14 前端契约钉
//
// D-14（E2E-16 plan §B.10）：前端 /ai/stream 请求体携带 face/voice 情绪上下文。
//
// 现状：AIStreamParams 只有 emotion（消息级 happy/sad/...），没有 face/voice 区分。
//
// 修法：
//   - AIStreamParams 加 faceEmotion?: string, faceConfidence?: number, voiceConfidence?: number
//     （voiceEmotion 已存在）
//   - useConversationSender.sendToExistingConversation options 透传 faceEmotion/faceConfidence
//   - /conversation/[id].vue handleSubmit 在发送前调 faceEmotion.getRecentEmotion()，
//     把最近一次表情结果随请求带上（若摄像头未开/超时则不带，BFF 不污染 prompt）
//
// 契约钉（3 项）：
//   1. AIStreamParams 含 faceEmotion 可选字段
//   2. AIStreamParams 含 faceConfidence 可选字段
//   3. useConversationSender 把 options.faceEmotion 透传到 sendAIStream 的 params

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const streamSrc = readFileSync(
  resolve(__dirname, './useAIStreamHandler.ts'),
  'utf8',
)
const senderSrc = readFileSync(
  resolve(__dirname, './useConversationSender.ts'),
  'utf8',
)

describe('useAIStreamHandler.ts · D-14 前端契约', () => {
  it('AIStreamParams 含 faceEmotion 可选字段', () => {
    expect(streamSrc, 'AIStreamParams 必须有 faceEmotion 字段').toMatch(/faceEmotion\?:/)
  })

  it('AIStreamParams 含 faceConfidence 可选字段', () => {
    expect(streamSrc, 'AIStreamParams 必须有 faceConfidence 字段').toMatch(/faceConfidence\?:/)
  })

  it('AIStreamParams 已有 voiceEmotion 字段', () => {
    expect(streamSrc, 'AIStreamParams 已有 voiceEmotion 字段（D-14 复用）').toMatch(/voiceEmotion\?:/)
  })
})

describe('useConversationSender.ts · D-14 前端契约', () => {
  it('sendToExistingConversation options 透传 faceEmotion 到 params', () => {
    // 两种合法写法：对象字面量字段 / 赋值表达式
    const patterns = [
      /params\.faceEmotion\s*=\s*options\?\.faceEmotion/,
      /faceEmotion:\s*options\?\.faceEmotion/,
      /faceEmotion:\s*extraParams\?\.faceEmotion/,
      /faceEmotion:\s*options\.faceEmotion/,
    ]
    const matched = patterns.some((re) => re.test(senderSrc))
    expect(matched, 'useConversationSender 必须把 faceEmotion 透传到 AIStreamParams').toBe(true)
  })

  it('sendToExistingConversation options 透传 faceConfidence', () => {
    const patterns = [
      /params\.faceConfidence\s*=\s*options\?\.faceConfidence/,
      /faceConfidence:\s*options\?\.faceConfidence/,
      /faceConfidence:\s*extraParams\?\.faceConfidence/,
      /faceConfidence:\s*options\.faceConfidence/,
    ]
    const matched = patterns.some((re) => re.test(senderSrc))
    expect(matched, 'useConversationSender 必须把 faceConfidence 透传到 AIStreamParams').toBe(true)
  })
})
