// useTTSPlayer.ts — E2E-17 plan §6 step 3（phoneme 驱动）+ step 4（队列化消除段间断点）
//
// D-03 真口型同步 + 段间断点修复：
//
// step 3（F-128）：
//   - 删 150ms 固定节奏假动画 + random 状态机 + pcm-player 流式缓冲
//   - playStream 走 POST /api/v1/tts/phonemes → base64 → Blob → HTMLAudioElement
//   - 口型驱动：ontimeupdate → findPhonemeAt → charToLipShape → callback
//   - 播放结束/中断 → callback('neutral', 1)（防末帧卡死）
//
// step 4（F-129 段间断点）：
//   - **队列化**：新段入队不再 stop 前段（旧首行 stop() 是断点根因：
//     段 N 被打断 + fetch/推理 ~25s 才播 N+1）
//   - **入队即预取**：fetch 在入队瞬间发起，与当前段播放并行；
//     衔接时只等已就绪的 audio 创建，不等推理
//   - **代际校验（gen）**：stop() 清队列 + gen++，过期 fetch / 迟到 ended 不得复活
//   - playStream 返回即入队完成（不 await 整段播放 —— 与旧"流读完即返回"语义一致）
//
// 注意：服务端 XTTS CPU 推理延迟（实测 ~25s/4字符冷路径）不在本层可修范围，
// 本层消除的是前端侧 stop→重开→refetch 的结构性 gap；服务端基线记入 report。
//
// 关联 plan：E2E-17 [stages/e2e-17-digital-human-tts/plan.md] §2.B / §4 #13-15

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
 */
const base64ToWavBlob = (b64: string): Blob => {
  const binary = atob(b64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i)
  }
  return new Blob([bytes], { type: 'audio/wav' })
}

// ===== 共享状态 =====
const currentTime = ref(0)
const isPlaying = ref(false)
const currentVolume = ref(2.0)

let currentAudioEl: HTMLAudioElement | null = null
let currentObjectURL: string | null = null
let currentLipSync: LipSyncCallback | null = null

// ===== 队列状态（step 4 段间断点修复）=====
interface QueuedSegment {
  gen: number
  fetchPromise: Promise<TTSResponse>
  onLipSync: LipSyncCallback
  speed: number
  volume: number
}

let segmentQueue: QueuedSegment[] = []
let currentGen = 0 // 代际：stop() 时 ++，过期段全部作废
let isPumping = false
// currentSegmentDone 让 stop() 能解除 pump 对当前段 ended 的等待（pause 不触发 ended）
let currentSegmentDone: (() => void) | null = null

/**
 * fetchPhonemes 发起 /api/v1/tts/phonemes 请求并做信封校验。
 * 入队瞬间调用（预取）—— 与当前段播放并行。
 */
async function fetchPhonemes(
  text: string,
  speed: number,
  volume: number,
): Promise<TTSResponse> {
  const base = getApiBaseUrl(useRuntimeConfig())
  const token = getClientAccessToken()
  const headers: Record<string, string> = {
    'Content-Type': 'application/json; charset=utf-8',
  }
  if (token) headers['Authorization'] = `Bearer ${token}`

  const resp = await fetch(`${base}${API_ROUTES.ttsPhonemes.path}`, {
    method: 'POST',
    headers,
    body: JSON.stringify({ text, language: 'zh-cn', speed, volume }),
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
  return envelope.data
}

/**
 * createAndPlayAudio 创建 HTMLAudioElement 并开播，返回 Promise 在
 * 播放结束（onended）或出错（onerror）时 resolve —— 供 pump 衔接下一段。
 */
function createAndPlayAudio(
  data: TTSResponse,
  onLipSync: LipSyncCallback,
  speed: number,
  volume: number,
  gen: number,
): Promise<void> {
  return new Promise<void>((resolve, reject) => {
    // 代际再校验（fetch 期间 stop() 过）
    if (gen !== currentGen) {
      resolve()
      return
    }

    const blob = base64ToWavBlob(data.audio)
    const url = URL.createObjectURL(blob)
    const audio = new Audio(url)
    audio.volume = volume
    audio.playbackRate = speed
    audio.preload = 'auto'

    const { phonemes, duration } = data
    // fetchPhonemes 已校验非空，此处显式收窄供 TS（避免 TS2345/TS18048）
    if (!phonemes || phonemes.length === 0 || !duration) {
      onLipSync?.('neutral', 1)
      reject(new Error('TTS phonemes data invalid'))
      return
    }
    let settled = false
    const settle = (fn: () => void) => {
      if (settled) return
      settled = true
      // 清理当前段引用（仅当仍是自己）
      if (currentAudioEl === audio) {
        currentAudioEl = null
        currentObjectURL = null
        currentLipSync = null
      }
      URL.revokeObjectURL(url)
      isPlaying.value = segmentQueue.length > 0 // 队列还有段则保持 playing 语义
      fn()
    }

    currentAudioEl = audio
    currentObjectURL = url
    currentLipSync = onLipSync

    // 真口型同步：timeupdate 驱动
    audio.ontimeupdate = () => {
      currentTime.value = audio.currentTime
      const ph = findPhonemeAt(phonemes, audio.currentTime)
      if (ph) {
        onLipSync?.(charToLipShape(ph.char), audio.currentTime / duration)
      }
    }

    audio.onended = () => {
      onLipSync?.('neutral', 1)
      settle(() => resolve())
    }
    audio.onerror = (e?: unknown) => {
      onLipSync?.('neutral', 1)
      settle(() => reject(e instanceof Error ? e : new Error('audio play error')))
    }

    currentSegmentDone = () => {
      // stop() 主动打断：不等 ended，直接放行 pump（pause 不触发 ended）
      settle(() => resolve())
    }

    void audio.play().catch((e) => {
      onLipSync?.('neutral', 1)
      settle(() => reject(e))
    })
  })
}

/**
 * pumpQueue 队列泵：串行消费队列 —— 取段 → await 预取结果 → 播放 →
 * ended 衔接下一段。同一时刻至多一个 pump 运行（isPumping 互斥）。
 */
async function pumpQueue(): Promise<void> {
  if (isPumping) return
  isPumping = true
  try {
    while (segmentQueue.length > 0) {
      const seg = segmentQueue[0]
      if (!seg || seg.gen !== currentGen) {
        // 过期段（stop 后残留）→ 出队
        if (seg) segmentQueue.shift()
        continue
      }

      let data: TTSResponse
      try {
        data = await seg.fetchPromise
      } catch (e) {
        // 预取失败：丢弃该段继续下一段（不打断当前播放）
        console.error('[TTS] phonemes prefetch failed:', e)
        if (segmentQueue[0] === seg) segmentQueue.shift()
        continue
      }

      // await 期间可能 stop() → gen 变化
      if (seg.gen !== currentGen) {
        if (segmentQueue[0] === seg) segmentQueue.shift()
        continue
      }

      try {
        await createAndPlayAudio(data, seg.onLipSync, seg.speed, seg.volume, seg.gen)
      } catch (e) {
        // 单段播放错误不终止队列（下一段继续）
        console.error('[TTS] segment play failed:', e)
      }

      currentSegmentDone = null
      if (segmentQueue[0] === seg) segmentQueue.shift()
    }
  } finally {
    isPumping = false
  }
}

const stop = () => {
  // 代际递增 + 清队列（过期 fetch / 迟到 ended 不得复活）
  currentGen++
  segmentQueue = []

  // 先停当前 audio（settle 会把 currentAudioEl 置 null，故 pause 必须在 done() 之前）
  if (currentAudioEl) {
    currentAudioEl.pause()
    currentAudioEl.currentTime = 0
    currentAudioEl = null
  }
  if (currentObjectURL) {
    URL.revokeObjectURL(currentObjectURL)
    currentObjectURL = null
  }

  // 再解除 pump 对当前段 ended 的等待（pause 不触发 ended，否则泵挂死）
  const done = currentSegmentDone
  currentSegmentDone = null
  done?.()

  // 中断也须闭嘴（plan §4 #12：结束/中断 → neutral，防末帧卡死）
  currentLipSync?.('neutral', 0)
  currentLipSync = null

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
 * flushBuffer 兼容占位（phonemes 路径无 chunk buffer；签名保留供
 * useTTSManager.flushRemainingText 调用。队列化后衔接由 pumpQueue 负责）。
 */
const flushBuffer = async (_onLipSync?: LipSyncCallback): Promise<void> => {
  /* no-op: phonemes 队列路径无 chunk buffer（plan §2.A.3 / §2.B） */
}

/**
 * playStream —— 入队即返回（不 await 播放）。
 *
 * step 4 语义：
 *   1. 文本清洗
 *   2. **立即发起 fetch（预取，与当前段播放并行）**
 *   3. 入队 + 踢泵（当前空闲则开播；在播则等 ended 自动衔接）
 *   4. **不再首行 stop()**（旧断点根因 —— 队列化取代打断）
 *
 * 返回 Promise 在入队完成后 resolve（与旧"流读完即返回"语义一致偏早，
 * 调用方 await 的是"已受理"而非"已播完"；播放完成由 onLipSync neutral 回调观察）。
 */
const playStream = async (
  text: string,
  onLipSync: LipSyncCallback,
  speed: number = 0.75,
  volume: number = 2.0,
): Promise<void> => {
  const cleanText = stripMarkdown(text).trim()
  if (!cleanText) return

  const readableText = extractReadableText(cleanText)
  if (!readableText) return

  const gen = currentGen
  // 入队即预取：与当前段播放并行（消除衔接时的全量推理等待）
  const fetchPromise = fetchPhonemes(readableText, speed, volume)
  segmentQueue.push({ gen, fetchPromise, onLipSync, speed, volume })
  void pumpQueue()
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