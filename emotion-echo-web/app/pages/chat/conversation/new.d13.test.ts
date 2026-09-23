// new.d13.test.ts — D-13 /new 页入口契约钉
//
// 背景：E2E-16 plan §A.5/roadmap §52：D-13 /new 页"补齐"——
//   原 /new 页 voice-record-btn 是 stub（toggleRecording 只切 isRecording 状态+toast，
//   未真实录音上传），与 /conversation/[id].vue 的 useVoiceRecorder 接线不一致。
//
// 修法：把 /new 页 voice-record-btn 接入 useVoiceRecorder，与 [id].vue 同模式：
//   - import { useVoiceRecorder }
//   - const voiceRecorder = useVoiceRecorder({ onUploadSuccess, onUploadError })
//   - toggleRecording 调用 voiceRecorder.startRecording() / stopRecording()
//   - 模板 :class="{ recording: voiceRecorder.isRecording.value }"
//
// 契约钉（4 项）：
//   1. import useVoiceRecorder（与 [id].vue 同款接线）
//   2. 调用 voiceRecorder.startRecording()（不是只 toggle 状态）
//   3. 调用 voiceRecorder.stopRecording()
//   4. 模板绑定 voiceRecorder.isRecording.value

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const src = readFileSync(
  resolve(__dirname, './new.vue'),
  'utf8',
)

describe('new.vue · D-13 /new 页多模态入口', () => {
  it('import useVoiceRecorder —— 与 [id].vue 同款接线', () => {
    expect(src).toMatch(/import\s*\{[^}]*useVoiceRecorder[^}]*\}\s*from\s*['"]~?\/composables\/useVoiceRecorder/)
  })

  it('toggleRecording 调用 voiceRecorder.startRecording() —— 不是 stub toggle', () => {
    // 必须含 voiceRecorder.startRecording() 调用
    expect(src).toMatch(/voiceRecorder\.startRecording\(\)/)
    // 反向断言：不应只剩 isRecording.value = !isRecording.value 的 stub
    expect(src).not.toMatch(/isRecording\.value\s*=\s*!isRecording\.value/)
  })

  it('toggleRecording 调用 voiceRecorder.stopRecording()', () => {
    expect(src).toMatch(/voiceRecorder\.stopRecording\(\)/)
  })

  it('模板绑定 voiceRecorder.isRecording.value —— 不是局部 isRecording', () => {
    // 模板里 recording class 应读 voiceRecorder.isRecording.value
    expect(src).toMatch(/recording:\s*voiceRecorder\.isRecording\.value/)
  })
})
