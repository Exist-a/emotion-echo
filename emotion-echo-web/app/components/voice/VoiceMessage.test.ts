// VoiceMessage.test.ts — F-117 契约钉
//
// 背景：E2E-16 用户第二轮复测（2026-09-22）截图实证：
//   - 刷新后语音气泡下方渲染 `/api/v1/voice/audio/2e975139-….webm` 文字（transcript 槽被错误填充为 URL）
//   - 气泡时长恒为 0:00（audioDuration 不在 SendMessageReq，DB 无该字段，加载行 duration=undefined）
//
// 两段根因：
//   1. [id].vue:20 :transcript="item.content" —— 后端落库 content=音频 URL，无 transcript 字段
//      ⇒ 加载行 transcript=URL 直接渲染
//   2. [id].vue:19 :duration="item.audioDuration" —— audioDuration 字段缺失 ⇒ 加载行 duration=undefined
//
// 修法（前端兜底，后端 schema 不变）：
//   - VoiceMessage 内部判断 transcript 是不是 URL（http/https//api 前缀），是则不渲染 transcript 槽
//   - VoiceMessage 内部监听 <audio>.loadedmetadata 事件拿真实 duration，覆盖 prop=0
//
// 契约钉（4 项）：
//   1. 源码必须含 URL 识别（startsWith http/https /api/）
//   2. 源码必须含 loadedmetadata 事件监听（或等价的 audio.duration 读取）
//   3. transcript 槽必须有 v-if 守卫（不能无条件渲染）
//   4. duration 显示必须读 actualDuration（不是直接用 prop duration）

import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const src = readFileSync(
  resolve(__dirname, './VoiceMessage.vue'),
  'utf8',
)

describe('VoiceMessage.vue · F-117 契约钉', () => {
  it('含 URL 识别逻辑 —— 不把 URL 当 transcript 渲染', () => {
    // 三种 URL 前缀必须有一个匹配
    const urlPatterns = [
      /startsWith\(['"]https?:/,
      /startsWith\(['"]\/api\//,
      /startsWith\(['"]http:/,
      /isUrl|isTranscriptUrl|isHttpUrl/,
    ]
    const matched = urlPatterns.some((re) => re.test(src))
    expect(matched, '源码必须含 URL 识别模式（startsWith http/https//api 或 isUrl* computed）').toBe(true)
  })

  it('含 loadedmetadata 事件监听 —— 从 audio metadata 恢复真实 duration', () => {
    // 必须有 loadedmetadata 监听或 audioRef.value?.duration 读取
    const patterns = [
      /loadedmetadata/,
      /onloadedmetadata/i,
      /addEventListener\(['"]loadedmetadata/,
      /audioRef\.value\?\.duration/,
    ]
    const matched = patterns.some((re) => re.test(src))
    expect(matched, '源码必须含 loadedmetadata 监听或 audioRef.duration 读取').toBe(true)
  })

  it('transcript 槽必须有 v-if 守卫 —— 不能无条件渲染', () => {
    // transcript 元素块必须有 v-if 表达式（不能是 {{ transcript }} 直绑）
    // 属性顺序两种都接受（v-if 在 class 前 / 后）
    const hasGuardVIfFirst = /<div[^>]*v-if=[^>]*class="transcript"/i.test(src)
    const hasGuardClassFirst = /<div[^>]*class="transcript"[^>]*v-if=/i.test(src)
    const hasGuard = hasGuardVIfFirst || hasGuardClassFirst
    expect(hasGuard, 'transcript 槽 div 必须含 v-if 守卫（F-117: URL 时隐藏）').toBe(true)
  })

  it('duration 显示读 actualDuration 或 audio metadata —— 不是直接 prop', () => {
    // {{ formatTime(duration) }} 是原写法（坏），{{ formatTime(actualDuration) }} 是修法
    const badPattern = /formatTime\(duration\)/
    const goodPattern = /formatTime\((actualDuration|resolvedDuration|durationRef|audioDuration)/i

    expect(src, '不应直接 formatTime(duration)（E2E-16 PR #62 修复前写法）').not.toMatch(badPattern)
    expect(src, 'duration 显示必须读 actualDuration / resolvedDuration / audio metadata').toMatch(goodPattern)
  })
})
