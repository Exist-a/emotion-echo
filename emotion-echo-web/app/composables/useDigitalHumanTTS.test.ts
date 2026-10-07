// useDigitalHumanTTS 语速解析契约（F-199 后续，2026-10-07 用户反馈 0.75 偏慢）
//
// 语速单一样本源 = userConfig.ttsSpeed（设置页三档 slow/normal/fast，
// userConfig JSONB 持久化、BFF map[string]any 透传）。playText 时解析：
//
//   customSpeed > options.speed > userConfig.ttsSpeed（缺省 normal = 1.0）
//
// 旧实现是 composable 局部 ref(0.75)，无任何 UI/持久化入口 ⇒ 用户永远
// 听 0.75 倍速。本文件钉死"配置驱动的语速解析"契约。
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useDigitalHumanTTS } from './useDigitalHumanTTS'

const playStreamMock = vi.fn()
const flushBufferMock = vi.fn()
const stopMock = vi.fn()
const setVolumeMock = vi.fn()

vi.mock('~/composables/useTTSPlayer', () => ({
  useTTSPlayer: () => ({
    playStream: playStreamMock,
    flushBuffer: flushBufferMock,
    stop: stopMock,
    setVolume: setVolumeMock,
  }),
}))

const ttsStoreState = { voiceEnabled: true, volume: 2 }
vi.mock('~/stores/digitalHuman', () => ({
  useDigitalHumanStore: () => ttsStoreState,
}))

let userConfigState: Record<string, any> = {}
vi.mock('~/stores/user', () => ({
  useUserStore: () => ({
    getUserConfig: () => userConfigState,
  }),
}))

describe('useDigitalHumanTTS 语速解析（配置驱动，F-199 后续）', () => {
  beforeEach(() => {
    playStreamMock.mockReset()
    ttsStoreState.voiceEnabled = true
    ttsStoreState.volume = 2
    userConfigState = {}
  })

  it('userConfig.ttsSpeed=fast → playStream 收到 speed=1.25', async () => {
    userConfigState.ttsSpeed = 'fast'
    const { playText } = useDigitalHumanTTS()

    await playText('你好')

    expect(playStreamMock).toHaveBeenCalledTimes(1)
    const [, , speed, volume] = playStreamMock.mock.calls[0]!
    expect(speed).toBe(1.25)
    expect(volume).toBe(2)
  })

  it('config 未设置 → 缺省 normal = 1.0（自然语速）', async () => {
    const { playText } = useDigitalHumanTTS()

    await playText('你好')

    expect(playStreamMock.mock.calls[0]![2]).toBe(1)
  })

  it('customSpeed 优先于 userConfig', async () => {
    userConfigState.ttsSpeed = 'fast'
    const { playText } = useDigitalHumanTTS()

    await playText('你好', 0.9)

    expect(playStreamMock.mock.calls[0]![2]).toBe(0.9)
  })

  it('voiceEnabled=false 时跳过播放（既有行为钉死）', async () => {
    ttsStoreState.voiceEnabled = false
    const { playText } = useDigitalHumanTTS()

    await playText('你好')

    expect(playStreamMock).not.toHaveBeenCalled()
  })
})
