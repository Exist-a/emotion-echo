// useTTSPlayer.ts — E2E-17 plan §6 step 3 改造后
//
// D-03 真口型同步：播放层从随机轮播假动画改为按 phoneme 时间戳驱动。
//
// 关键变化（与改造前对比）：
//   - 删 150ms 固定节奏假动画（plan §4 #10 字面契约：源码不引用该函数）
//   - 删 pcm-player 流式依赖（不再做流式 WAV chunk 拼接）
//   - 删 chunksBuffer / flushBuffer / playStreamChunks（流式缓冲，phonemes 一次性返全音频）
//   - 删 random 状态机变量（setInterval 状态机整体删除）
//   - 删 abortController（phonemes 一次性请求，无需中止）
//   - playStream 改为 POST /api/v1/tts/phonemes → base64 → Blob → ObjectURL → HTMLAudioElement
//   - 口型驱动：HTMLAudioElement.ontimeupdate → findPhonemeAt(phonemes, currentTime) → charToLipShape → callback
//   - 播放结束 / 中断 → callback('neutral', 1)（resetLipShape 防止末帧卡死）
//   - 新增导出助手：charToLipShape / findPhonemeAt（便于 Vitest 单元测试 + 复用）
//
// 关联 plan：E2E-17 [stages/e2e-17-digital-human-tts/plan.md] §2.A.3 / §4 #7-#12

import { ref, onUnmounted } from 'vue'
import { stripMarkdown, extractReadableText } from '~/utils/stripMarkdown'
import { API_ROUTES } from '~/lib/apiRoutes'
import { getApiBaseUrl } from '../lib/apiBaseUrl'
import { getClientAccessToken } from '~/lib/clientAccessToken'

export type LipShape = 'aa' | 'ee' | 'ih' | 'oh' | 'ou' | 'neutral'

export interface Phoneme {
  char: string
  start: number
  duration: number
}

export interface TTSRequest {
  text: string
  language?: string
}

export interface TTSResponse {
  audio: string
  sample_rate: number
  text?: string
  phonemes?: Phoneme[]
  duration?: number
}

type LipSyncCallback = (shape: LipShape, progress: number) => void

// VOWEL_TO_LIP 元音→口型（plan §2.A.3 复用 — 原文件字面 38-57 行死代码激活）
// 选词逻辑：v/n/m 之所以归 ih（实际是辅音闭唇），因仓 server.py 对汉字按字符
// 给 per-char 等分 start/duration；这些字符常出现在词尾闭嘴，归 ih 是务实简化。
const VOWEL_TO_LIP: Record<string, LipShape> = {
  a: 'aa',
  o: 'oh',
  e: 'ee',
  i: 'ih',
  u: 'ou',
  ü: 'ee',
  v: 'ih',
  n: 'ih',
  m: 'ih',
}

// CONSONANT_CLOSE 辅音→口型（同上，原文件字面 59-84 行死代码激活）
const CONSONANT_CLOSE: Record<string, LipShape> = {
  b: 'aa',
  p: 'aa',
  m: 'aa',
  f: 'oh',
  v: 'oh',
  w: 'ou',
  d: 'ih',
  t: 'ih',
  n: 'ih',
  l: 'ih',
  z: 'ih',
  c: 'ih',
  s: 'ih',
  zh: 'ih',
  ch: 'ih',
  sh: 'ih',
  r: 'ih',
  j: 'ih',
  q: 'ih',
  x: 'ih',
  y: 'ih',
  g: 'ih',
  k: 'ih',
  h: 'ih',
}

/**
 * charToLipShape 单字符 → 口型（激活死代码 VOWEL_TO_LIP / CONSONANT_CLOSE）。
 *
 * 优先级：VOWEL > CONSONANT（v/n/m 在两边都有，按 VOWEL 优先；语义上 v/n/m
 * 在尾音是闭嘴，ih 实际是闭嘴对位，故归元音侧更合理）。
 *
 * 大小写不敏感；未知字符 / 空串 → 'neutral'（graceful fallback，plan §4 #8 钉死）。
 *
 * 单元测试：app/composables/useTTSPlayer.phoneme.test.ts (Vitest)。
 */
export function charToLipShape(char: string): LipShape {
  if (!char) return 'neutral'
  const lower = char.toLowerCase()
  return VOWEL_TO_LIP[lower] ?? CONSONANT_CLOSE[lower] ?? 'neutral'
}

/**
 * findPhonemeAt 在 phonemes 数组中找覆盖 timeSec（秒）的音素。
 *
 * 区间判定：[start, start+duration) 半开（不包含右端点；连读时下一音从
 * 前一音的 end_ms 起步，避免双音重叠时的闪烁）。
 *
 * 边界：time < first.start 或 time >= last.start+last.duration → null（区间外）；
 * 区间间隙（两个 phoneme 之间没有交集）→ null。
 *
 * 单元测试：app/composables/useTTSPlayer.phoneme.test.ts (Vitest)。
 */
export function findPhonemeAt(phonemes: Phoneme[], timeSec: number): Phoneme | null {
  if (!phonemes || phonemes.length === 0) return null
  // 二分查找：phonemes 按 start 升序（仓 server.py 按字符顺序生成，天然升序）
  let lo = 0
  let hi = phonemes.length - 1
  while (lo <= hi) {
    const mid = (lo + hi) >>> 1
    const p = phonemes[mid]
    if (!p) break // noUncheckedIndexedAccess：mid 在 [0, length) 外时防御（lo<=hi 理论不发生）
    if (timeSec < p.start) {
      hi = mid - 1
    } else if (timeSec >= p.start + p.duration) {
      lo = mid + 1
    } else {
      return p
    }
  }
  return null
}

/**
 * base64ToWavBlob 把 BFF 返回的 base64 音频字符串解码为 WAV Blob（mime=audio/wav）。
 * 用于 HTMLAudioElement.src = URL.createObjectURL(blob)。
 */
const base64ToWavBlob = (b64: string): Blob => {
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return new Blob([bytes], { type: 'audio/wav' })
}

// 单例/共享状态（HT 前的全局 audioElement 已被拆为 per-call local）
const currentTime = ref(0)
const isPlaying = ref(false)
const currentVolume = ref(2.0)
let currentAudioEl: HTMLAudioElement | null = null
let currentObjectURL: string | null = null

/**
 * resetLipShape 通知上层把嘴闭上（防 phoneme 末帧卡死）。
 * 由 onended / onerror / 主动 stop 触发。
 */
const resetLipShape = (onLipSync?: LipSyncCallback | null) => {
  onLipSync?.('neutral', 0)
}

const stop = () => {
  if (currentAudioEl) {
    currentAudioEl.pause()
    currentAudioEl.currentTime = 0
    currentAudioEl = null
  }
  if (currentObjectURL) {
    URL.revokeObjectURL(currentObjectURL)
    currentObjectURL = null
  }
  isPlaying.value = false
}

const setVolume = (volume: number) => {
  currentVolume.value = volume
  if (currentAudioEl) {
    currentAudioEl.volume = volume
  }
}

const pause = () => {
  if (currentAudioEl && isPlaying.value) {
    currentAudioEl.pause()
    isPlaying.value = false
  }
}

const resume = () => {
  if (currentAudioEl && !isPlaying.value && currentTime.value > 0) {
    void currentAudioEl.play()
    isPlaying.value = true
  }
}

/**
 * flushBuffer 兼容占位（plan §6 step 4 处理段间断点时改造 useTTSManager 移除 500ms debounce）。
 * 当前 phonemes 路径一次性返全音频，无 chunk buffer 可 flush；保留函数签名避免
 * useTTSManager.flushRemainingText 编译失败。语义上 no-op。
 */
const flushBuffer = async (_onLipSync?: LipSyncCallback): Promise<void> => {
  /* no-op: phonemes 路径无 chunk buffer（plan §2.A.3） */
}

const playStream = async (
  text: string,
  onLipSync: LipSyncCallback,
  speed: number = 0.75,
  volume: number = 2.0,
) => {
  const cleanText = stripMarkdown(text).trim()
  if (!cleanText) return

  const readableText = extractReadableText(cleanText)
  if (!readableText) return

  // 先停前一次（断旧音频 + 释放 URL + 重置嘴型）
  resetLipShape(onLipSync)
  stop()

  const base = getApiBaseUrl(useRuntimeConfig())
  const token = getClientAccessToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json; charset=utf-8',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const resp = await fetch(`${base}${API_ROUTES.ttsPhonemes.path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify({
      text: readableText,
      language: 'zh-cn',
      speed,
      volume,
    }),
  })

  if (!resp.ok) {
    throw new Error(`TTS phonemes request failed: ${resp.status}`)
  }

  const envelope = (await resp.json()) as {
    code: number
    message?: string
    data?: TTSResponse
  }
  if (envelope.code !== 0 || !envelope.data) {
    throw new Error(`TTS phonemes envelope error: ${envelope.message ?? 'no data'}`)
  }

  const { audio: audioB64, phonemes, duration } = envelope.data
  if (!audioB64 || !phonemes || phonemes.length === 0 || !duration) {
    throw new Error('TTS phonemes response missing audio/phonemes/duration')
  }

  // base64 → Blob → ObjectURL（plan §2.A.3：phonemes 一次性返全音频，非流式）
  const blob = base64ToWavBlob(audioB64)
  const url = URL.createObjectURL(blob)
  const audio = new Audio(url)
  audio.volume = volume
  audio.playbackRate = speed  // HTMLAudioElement 用 playbackRate 控速（仓 server.py 已接收 speed）
  audio.preload = 'auto'

  currentAudioEl = audio
  currentObjectURL = url
  isPlaying.value = true

  // 真口型同步：timeupdate 事件驱动（~250ms 一次，原 PCM 播放器无此事件订阅 —— plan 备注）
  audio.ontimeupdate = () => {
    currentTime.value = audio.currentTime
    const ph = findPhonemeAt(phonemes, audio.currentTime)
    if (ph) {
      const shape = charToLipShape(ph.char)
      onLipSync?.(shape, audio.currentTime / duration)
    }
  }

  // 结束：闭嘴 + 释放（plan §4 #12 播放结束 reset）
  audio.onended = () => {
    onLipSync?.('neutral', 1)
    if (currentAudioEl === audio) currentAudioEl = null
    if (currentObjectURL === url) {
      URL.revokeObjectURL(url)
      currentObjectURL = null
    }
    isPlaying.value = false
  }

  // 错误：同样闭嘴 + 释放
  audio.onerror = () => {
    onLipSync?.('neutral', 1)
    if (currentAudioEl === audio) currentAudioEl = null
    if (currentObjectURL === url) {
      URL.revokeObjectURL(url)
      currentObjectURL = null
    }
    isPlaying.value = false
  }

  try {
    await audio.play()
  } catch (e) {
    onLipSync?.('neutral', 1)
    URL.revokeObjectURL(url)
    currentObjectURL = null
    currentAudioEl = null
    isPlaying.value = false
    throw e
  }
}

export function useTTSPlayer() {
  onUnmounted(() => {
    // 组件卸载时不停止播放器，因为是单例
  })

  return {
    currentTime,
    isPlaying,
    playStream,
    stop,
    pause,
    resume,
    flushBuffer,
    setVolume,
  }
}