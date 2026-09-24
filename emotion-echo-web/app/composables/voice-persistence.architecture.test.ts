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

// E2E-F-133（2026-09-23 IAB 用户实测「嘴动没声音」第二根因）：
  // handleVoiceStreamResponse 在 AI 流式**完成**时调 conversationSender.stopTTS() →
  // 清 debounce 计时器 + 清累积 deltaText + stop()，**把正要推送的 TTS 自杀**
  // （流完成 = 正是要 TTS 播放的时候）。修法：流完成回调应调 flushTTS（保证最后一波
  // 推送+不杀队列），而非 stopTTS。
  //
  // 边界：仅约束 handleVoiceStreamResponse **函数体内**——函数体外的
  // handleCancel/handleSubmit/onUnmounted 等合法 stopTTS 调用不受此约束。
  // 函数体用大括号配对追踪切（避免误匹配 <style scoped> 块里的 `}`）。
  it('[id].vue 的 handleVoiceStreamResponse 函数体内必须 flushTTS 而非 stopTTS（E2E-F-133）', () => {
    const fnDeclIdx = pageSrc.indexOf('const handleVoiceStreamResponse = async')
    expect(fnDeclIdx, 'handleVoiceStreamResponse 函数定义必须存在').toBeGreaterThan(-1)
    const bodyStart = pageSrc.indexOf('=> {', fnDeclIdx) + 4
    expect(bodyStart, 'handleVoiceStreamResponse 函数体起始 `=> {` 必须存在').toBeGreaterThan(3)
    // 大括号配对追踪：找匹配 `=> {` 的 `}`（跳过字符串/正则字面量里的 `{`/`}`）
    // 简化：仅统计非字符串上下文中的 `{`/`}` —— Vue SFC 模板里 `{` 都是表达式，
    // 但 `<style scoped>` 里的 `}` 也在文件里 —— 用 `\n  }` 单行匹配先过滤 style 块的 `}`。
    // 更稳：要求 `{` 在字符串/模板外的简单统计 + 行首 2 空格 } 收尾 —— 用栈。
    let depth = 1 // 已计入 `=> {`
    let bodyEnd = bodyStart
    for (let i = bodyStart; i < pageSrc.length; i++) {
      const c = pageSrc[i]
      if (c === '{') depth++
      else if (c === '}') {
        depth--
        if (depth === 0) { bodyEnd = i; break }
      }
    }
    expect(bodyEnd > bodyStart, 'handleVoiceStreamResponse 函数体必须配对 `}` 存在').toBe(true)
    const fnBody = pageSrc.slice(bodyStart, bodyEnd)
    expect(
      /conversationSender\.flushTTS\s*\(/.test(fnBody),
      'handleVoiceStreamResponse 函数体内必须用 conversationSender.flushTTS() 推最后一波 AI 文本到 TTS，' +
        '而不是 stopTTS()——后者会清 debounce 计时器 + 累积文本 + stop()，' +
        '把 AI 流完成时正要播的 TTS 自杀（用户实测「没声音」根因 E2E-F-133）',
    ).toBe(true)
    expect(
      !/conversationSender\.stopTTS\s*\(\s*\)/.test(fnBody),
      'handleVoiceStreamResponse 函数体内禁止出现裸调 conversationSender.stopTTS()（语义错：' +
        'AI 完成 ≠ 停止 TTS，正是要 flush + 播放的时候）。流中途切换/取消/卸载等其他场景的' +
        'stopTTS 调用在函数体外不受此约束。',
    ).toBe(true)
  })
})
