// E2E-17 plan §6 step 4 RED：段间断点修复 —— 队列化 + 入队即预取
//
// 断点成因（F-128 改造后仍存在，本测试钉死修法）：
//   旧 playStream 首行 stop() ⇒ 段 N 还在播时，段 N+1 到达会**打断段 N**，
//   然后 fetch + XTTS 推理（CPU 实测 ~25s/次）才播 N+1 ⇒
//   gap = 段 N 剩余时长 + 全量推理时长（plan §2.B 目标 gap < 200ms 完全不可能）。
//
// 修法（D-03 §2.B.6-8）：
//   1. **队列化**：playStream 不再 stop 前段，新段入队；前段 onended 后自动播下一段
//   2. **入队即预取**：段 N+1 的 fetch 在入队瞬间发起（与段 N 播放并行），
//      衔接时只等已就绪的 audio 创建，不等推理
//   3. **stop() 清队列 + 代际校验**：手动停止后，过期 fetch 结果不得复活已清段
//
// 测试桩：
//   - fetch → 手动 resolve 控制（模拟慢推理）
//   - Audio → FakeAudio（记录 instances / paused / onended 手动触发）
//   - URL.createObjectURL / revokeObjectURL → stub
//   - useRuntimeConfig → globalThis 注入（模式同 useAIStreamHandler.test.ts）
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { useTTSPlayer } from './useTTSPlayer'

// ===== 测试桩 =====

interface PendingFetch {
  url: string
  body: { text: string; language?: string; speed?: number; volume?: number }
  resolve: (v: unknown) => void
}

const pendingFetches: PendingFetch[] = []

class FakeAudio {
  static instances: FakeAudio[] = []
  src: string
  volume = 1
  playbackRate = 1
  preload = ''
  currentTime = 0
  paused = false
  playCalled = false
  ontimeupdate: (() => void) | null = null
  onended: (() => void) | null = null
  onerror: ((e?: unknown) => void) | null = null
  constructor(src: string) {
    this.src = src
    FakeAudio.instances.push(this)
  }
  play(): Promise<void> {
    this.playCalled = true
    this.paused = false
    return Promise.resolve()
  }
  pause(): void {
    this.paused = true
  }
  // 测试辅助：模拟播完
  end(): void {
    this.onended?.()
  }
}

function makeEnvelope(text: string) {
  // 4 音覆盖 duration=1.0（per-char 等分形态，与仓 server.py 实测一致）
  const b64 =
    typeof btoa === 'function'
      ? btoa('RIFF....WAVEfmt data')
      : Buffer.from('RIFF....WAVEfmt data').toString('base64')
  return {
    code: 0,
    message: 'ok',
    data: {
      audio: b64,
      sample_rate: 24000,
      text,
      language: 'zh-cn',
      phonemes: [
        { char: 'a', start: 0.0, duration: 0.25 },
        { char: 'b', start: 0.25, duration: 0.25 },
        { char: 'c', start: 0.5, duration: 0.25 },
        { char: 'd', start: 0.75, duration: 0.25 },
      ],
      duration: 1.0,
    },
  }
}

function okResp(text: string) {
  return { ok: true, status: 200, json: async () => makeEnvelope(text) }
}

/** 让微任务/已 resolve 的 promise 链跑完 */
async function flush(times = 3): Promise<void> {
  for (let i = 0; i < times; i++) {
    await new Promise((r) => setTimeout(r, 0))
  }
}

// ===== 环境注入 =====

beforeEach(() => {
  pendingFetches.length = 0
  FakeAudio.instances = []

  ;(globalThis as any).useRuntimeConfig = () => ({
    public: { API_BASE_URL: 'http://gw.test/api/v1' },
  })

  vi.stubGlobal(
    'fetch',
    (url: unknown, opts?: { body?: string }) =>
      new Promise((resolve) => {
        pendingFetches.push({
          url: String(url),
          body: JSON.parse(opts?.body ?? '{}'),
          resolve,
        })
      }),
  )

  vi.stubGlobal(
    'URL',
    Object.assign(Object.create(URL), {
      createObjectURL: vi.fn(() => 'blob:fake-object-url'),
      revokeObjectURL: vi.fn(),
    }),
  )

  vi.stubGlobal('Audio', FakeAudio)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

// ===== 测试 =====

describe('useTTSPlayer 段间断点修复（E2E-17 plan §6 step 4）', () => {
  it('段 N 播放中，段 N+1 到达不得打断段 N（队列化核心契约）', async () => {
    const { playStream, stop } = useTTSPlayer()

    // 段 1：发起 + resolve + 播放
    const p1 = playStream('第一段文字', () => {}, 1, 1)
    await flush()
    expect(pendingFetches.length, '段 1 fetch 必须发出').toBe(1)
    pendingFetches[0]!.resolve(okResp('第一段文字'))
    await flush()
    expect(FakeAudio.instances.length, '段 1 audio 已创建').toBe(1)
    const audio1 = FakeAudio.instances[0]!
    expect(audio1.playCalled, '段 1 必须在播放').toBe(true)
    expect(audio1.paused).toBe(false)

    // 段 2 到达（段 1 仍在播）
    const p2 = playStream('第二段文字', () => {}, 1, 1)
    await flush()

    // ★ RED 断言：旧实现首行 stop() 会 pause 段 1
    expect(
      audio1.paused,
      '段 1 不得被段 2 到达打断（旧 playStream 首行 stop() 是断点根因，plan §2.B.8）',
    ).toBe(false)
    expect(FakeAudio.instances.length, '段 2 的 audio 不得提前创建（段 1 未 ended）').toBe(1)

    await Promise.allSettled([p1, p2])
    stop()
  })

  it('段 N+1 入队即预取：fetch 在入队瞬间发起，不等段 N 播完', async () => {
    const { playStream, stop } = useTTSPlayer()

    const p1 = playStream('AAA', () => {}, 1, 1)
    await flush()
    pendingFetches[0]!.resolve(okResp('AAA'))
    await flush()
    expect(FakeAudio.instances.length).toBe(1)

    // 段 2 入队 —— 预取：fetch 立即发出（段 1 还在播、未 ended）
    const p2 = playStream('BBB', () => {}, 1, 1)
    await flush()
    expect(
      pendingFetches.length,
      '段 2 fetch 必须在入队瞬间发出（预取与段 1 播放并行，否则衔接时要等全量推理 ~25s）',
    ).toBe(2)
    expect(pendingFetches[1]!.url).toContain('/tts/phonemes')

    // 段 2 数据就绪，但段 1 未 ended → 不得开播
    pendingFetches[1]!.resolve(okResp('BBB'))
    await flush()
    expect(
      FakeAudio.instances.length,
      '段 2 数据就绪也不得抢播（须等段 1 ended 衔接）',
    ).toBe(1)

    await Promise.allSettled([p1, p2])
    stop()
  })

  it('段 1 ended 后段 2 立即衔接开播（衔接 gap = 微任务级，无 fetch 等待）', async () => {
    const { playStream, stop } = useTTSPlayer()

    const p1 = playStream('AAA', () => {}, 1, 1)
    await flush()
    pendingFetches[0]!.resolve(okResp('AAA'))
    await flush()
    const audio1 = FakeAudio.instances[0]!

    const p2 = playStream('BBB', () => {}, 1, 1)
    await flush()
    pendingFetches[1]!.resolve(okResp('BBB'))
    await flush()
    expect(FakeAudio.instances.length, '段 1 未 ended 前段 2 不创建').toBe(1)

    // 段 1 播完 → 衔接
    audio1.end()
    await flush()
    expect(FakeAudio.instances.length, '段 1 ended 后段 2 必须创建').toBe(2)
    const audio2 = FakeAudio.instances[1]!
    expect(audio2.playCalled, '段 2 必须开播').toBe(true)
    // 预取已就绪 ⇒ 衔接只经过微任务（本 flush 内完成），无 setTimeout(500ms) 之类定时器

    await Promise.allSettled([p1, p2])
    stop()
  })

  it('stop() 清空队列：已入队未播的段被丢弃（代际校验，过期 fetch 不复活）', async () => {
    const { playStream, stop } = useTTSPlayer()

    const p1 = playStream('AAA', () => {}, 1, 1)
    await flush()
    pendingFetches[0]!.resolve(okResp('AAA'))
    await flush()
    const audio1 = FakeAudio.instances[0]!

    const p2 = playStream('BBB', () => {}, 1, 1)
    await flush()
    expect(pendingFetches.length).toBe(2)

    // 手动停止（用户取消 / 切会话 / handleCancel → stopTTS）
    stop()
    expect(audio1.paused, 'stop() 必须停掉当前段').toBe(true)

    // 过期的段 2 fetch 此后才 resolve —— 不得复活
    pendingFetches[1]!.resolve(okResp('BBB'))
    await flush()
    // 段 1 被 pause 后 onended 不会自然触发；即便外部触发也不得创建段 2
    audio1.end()
    await flush()
    expect(
      FakeAudio.instances.length,
      'stop() 后过期 fetch / 迟到 ended 不得创建新 audio（代际校验）',
    ).toBe(1)

    await Promise.allSettled([p1, p2])
  })

  it('多段连续入队按 FIFO 顺序播放', async () => {
    const { playStream, stop } = useTTSPlayer()

    const p1 = playStream('S1', () => {}, 1, 1)
    const p2 = playStream('S2', () => {}, 1, 1)
    const p3 = playStream('S3', () => {}, 1, 1)
    await flush()
    expect(pendingFetches.length, '三段 fetch 全部预取').toBe(3)

    // 全部数据就绪
    pendingFetches.forEach((f, i) => f.resolve(okResp(`S${i + 1}`)))
    await flush()
    expect(FakeAudio.instances.length, '只播第 1 段').toBe(1)

    // 依次 ended 衔接
    FakeAudio.instances[0]!.end()
    await flush()
    expect(FakeAudio.instances.length).toBe(2)
    FakeAudio.instances[1]!.end()
    await flush()
    expect(FakeAudio.instances.length, 'FIFO 第 3 段').toBe(3)
    expect(FakeAudio.instances[2]!.playCalled).toBe(true)

    // 三段文本顺序 = 请求体顺序
    expect(pendingFetches.map((f) => f.body.text)).toEqual(['S1', 'S2', 'S3'])

    await Promise.allSettled([p1, p2, p3])
    stop()
  })
})