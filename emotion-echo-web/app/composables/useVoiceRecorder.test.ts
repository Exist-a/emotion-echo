import { describe, it, expect, beforeEach, vi } from 'vitest'

/**
 * E2E-F-110 回归钉：**录音停止后必须真的上传**。
 *
 * 缺陷（IAB 浏览器实测发现）：`stopRecording()` 先置 `isStopped = true` 再调
 * `mediaRecorder.stop()`，而 `mediaRecorder.onstop` 回调首行是
 * `if (isStopped) return` ⇒ `handleUpload()`（POST /voice/upload）**永不执行**；
 * 同一个标志还挡住 `ondataavailable` 的 `if (!isStopped ...)` ⇒ 连最后一个
 * 音频块都不收集。表现：用户点录音→点停止，**页面无提示、无请求、静默失败**。
 *
 * 本测试用假的 MediaRecorder / getUserMedia 把"停止 → 收集数据 → onstop → 上传"
 * 这条真实调用链跑出来，断言 post 被调用且路径为 voiceUpload。
 */

const postMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  post: (...args: unknown[]) => postMock(...args),
  useApi: () => ({ post: postMock }),
}))

vi.mock('~/stores/message', () => ({
  useMessageStore: () => ({
    currentSessionId: '277',
    currentMessages: [] as unknown[],
    switchSession: vi.fn(async () => {}),
  }),
}))

vi.mock('~/stores/conversation', () => ({
  useConversationStore: () => ({
    conversationList: [] as unknown[],
  }),
}))

vi.mock('~/lib/apiRoutes', () => ({
  API_ROUTES: { voiceUpload: { method: 'POST', path: '/voice/upload' } },
}))

/** 最小可用的 MediaRecorder 替身：把回调暴露出来供测试手动触发。 */
class FakeMediaRecorder {
  static last: FakeMediaRecorder | null = null
  state = 'inactive'
  ondataavailable: ((e: { data: Blob }) => void) | null = null
  onstop: (() => void | Promise<void>) | null = null
  onerror: (() => void) | null = null

  constructor(
    public stream: unknown,
    public opts?: unknown,
  ) {
    FakeMediaRecorder.last = this
  }

  start() {
    this.state = 'recording'
  }

  stop() {
    this.state = 'inactive'
    // 真实 MediaRecorder 语义：先 ondataavailable（带最后一块），再 onstop
    this.ondataavailable?.({ data: new Blob(['audio-bytes'], { type: 'audio/webm' }) })
    // 让 onstop 里的 async 逻辑跑完
    return Promise.resolve(this.onstop?.())
  }
}

function installBrowserStubs() {
  vi.stubGlobal('MediaRecorder', FakeMediaRecorder)
  Object.defineProperty(navigator, 'mediaDevices', {
    configurable: true,
    value: {
      getUserMedia: vi.fn(async () => ({
        getTracks: () => [{ stop: vi.fn() }],
      })),
    },
  })
}

describe('useVoiceRecorder（E2E-F-110）', () => {
  beforeEach(() => {
    postMock.mockReset()
    postMock.mockResolvedValue({
      messageId: 'm-1',
      transcript: '我太开心了',
      emotion: 'happy',
      audioUrl: 'http://minio/voice/x.webm',
    })
    installBrowserStubs()
  })

  it('停止录音后必须发起 POST /voice/upload（缺陷回归钉）', async () => {
    const { useVoiceRecorder } = await import('./useVoiceRecorder')
    const onUploadSuccess = vi.fn()
    const rec = useVoiceRecorder({ conversationId: '277', onUploadSuccess })

    await rec.startRecording()
    expect(rec.isRecording.value).toBe(true)

    rec.stopRecording()
    // 让 onstop 内的 await handleUpload 完成
    await Promise.resolve()
    await Promise.resolve()
    await new Promise((r) => setTimeout(r, 0))

    expect(postMock).toHaveBeenCalledTimes(1)
    // 显式取首调用（strict 下 mock.calls[0] 可能 undefined，用 non-null 断言表达
    // "已断言调用次数为 1" 这一前提）
    const firstCall = postMock.mock.calls[0]!
    expect(firstCall[0]).toBe('/voice/upload')
    const form = firstCall[1] as FormData
    expect(form).toBeInstanceOf(FormData)
    expect(form.get('conversationId')).toBe('277')
    const file = form.get('file') as File
    expect(file, 'file 字段必须带上录音 blob').toBeTruthy()
  })

  it('上传成功后回调 onUploadSuccess 并带上服务端返回的 transcript', async () => {
    const { useVoiceRecorder } = await import('./useVoiceRecorder')
    const onUploadSuccess = vi.fn()
    const rec = useVoiceRecorder({ conversationId: '277', onUploadSuccess })

    await rec.startRecording()
    rec.stopRecording()
    await Promise.resolve()
    await Promise.resolve()
    await new Promise((r) => setTimeout(r, 0))

    expect(onUploadSuccess).toHaveBeenCalledTimes(1)
    const successCall = onUploadSuccess.mock.calls[0]!
    expect(successCall[0]).toMatchObject({ transcript: '我太开心了' })
  })
})
