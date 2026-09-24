/**
 * useTTSPlayer 音量 clamp 契约测试（E2E-F-140）
 *
 * 背景（2026-09-24 IAB 实测「嘴动没声音」最终根因）：
 * `digitalHumanStore.volume` 默认 **2.0**（这是给 XTTS 服务端的 PCM 增益参数，
 * server.py `pcm_chunk_shape(volume=2.0)` 合法），但 `useTTSPlayer.playSegment`
 * 把它**直接赋给 DOM**：`audio.volume = volume`。
 * HTMLMediaElement.volume 的合法范围是 [0, 1] —— 赋 2.0 抛
 * `IndexSizeError: The volume provided (2) is outside the range [0, 1]`。
 *
 * 后果：`new Audio(url)` 已创建（对象存在），但紧随的 `audio.volume = 2.0`
 * 抛错 ⇒ **`audio.play()` 永不执行** ⇒ 完全没有声音；异常又被 Promise 链
 * 静默吞掉 ⇒ 界面上无任何提示。口型仍动（phonemes 独立驱动），
 * 于是表现为用户原话「嘴动没声音」。
 *
 * 实测证据（IAB, 2026-09-24）：
 *   - fetch 探针：POST /api/v1/tts/phonemes 200（phonemes=66, duration=14.35, audio 918KB）
 *   - Audio 探针：`new Audio()` 记录存在，`play()` **零次**记录
 *   - 页面自测：`a.volume = 2.0` → IndexSizeError；`a.volume = 1.0` → ok
 *
 * 修法：DOM 音量必须 clamp 到 [0, 1]（服务端 volume 语义保持不变，仍传 2.0）。
 *
 * 说明：本项目对依赖浏览器 API 的场景优先「字面量契约测试」而非 happy-dom
 * 行为测试（mock 的 volume setter 不抛 IndexSizeError，行为测试会假绿）。
 */
import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const src = readFileSync(resolve(__dirname, 'useTTSPlayer.ts'), 'utf-8')

describe('useTTSPlayer · DOM 音量 clamp（E2E-F-140）', () => {
  it('audio.volume 赋值必须经过 clamp，不得直接赋服务端 volume', () => {
    // 匹配 `audio.volume = <expr>` 的所有赋值
    const assigns = [...src.matchAll(/audio\.volume\s*=\s*([^\n]+)/g)].map((m) => m[1]!.trim())
    expect(
      assigns.length,
      'useTTSPlayer.ts 应存在 audio.volume 赋值（playSegment 内）',
    ).toBeGreaterThan(0)

    for (const expr of assigns) {
      // 允许的写法：包含 Math.min / Math.max / clamp / normalizeVolume
      const clamped = /Math\.min|Math\.max|clamp|normalizeVolume/.test(expr)
      expect(
        clamped,
        `audio.volume 赋值必须 clamp 到 [0,1]，实际为: "audio.volume = ${expr}"。` +
          'E2E-F-140：直接赋服务端 volume（默认 2.0）会抛 ' +
          "IndexSizeError: The volume provided (2) is outside the range [0, 1] " +
          '⇒ audio.play() 永不执行 ⇒ 无声音（IAB 实测：new Audio() 有记录、play() 零记录）。',
      ).toBe(true)
    }
  })

  it('服务端 volume 语义不受影响（仍以 2.0 传给 /tts/phonemes 的请求体）', () => {
    // clamp 只应作用于 DOM 赋值，不应改动发起请求时传给 XTTS 的 volume
    expect(
      /body:\s*JSON\.stringify\(\{\s*text,\s*language:\s*'zh-cn',\s*speed,\s*volume\s*\}\)/.test(src),
      'fetchPhonemes 的请求体必须继续透传原始 volume（服务端 PCM 增益语义）',
    ).toBe(true)
  })
})
