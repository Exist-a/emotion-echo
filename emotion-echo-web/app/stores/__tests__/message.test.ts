// message store contentType 透传测试（Stage 79 RED）
//
// 背景（stage-78 核查 / docs/plans/file-upload-message-extension.md 注记）：
// 后端链路已全就绪（BFF SendMessageReq 透传 ContentType → chat-svc gRPC → DB
// VARCHAR(16)），但 store 的 sendMessage 硬编码 contentType:'text'，文件消息
// 无法落库。本文件锁定"调用方传 contentType，请求体必须原样透传"的装配契约。
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

import { useMessageStore } from '~/stores/message'

const postMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  post: (...args: unknown[]) => postMock(...args),
  get: vi.fn(),
  put: vi.fn(),
  patch: vi.fn(),
  del: vi.fn(),
}))

const okMessage = {
  id: 'm1',
  conversationId: 'c1',
  sender: 'user',
  content: 'hello',
  contentType: 'text',
  sendTime: Date.now(),
  createdAt: Date.now(),
}

describe('message store sendMessage contentType 透传（Stage 79）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    postMock.mockReset()
    postMock.mockResolvedValue({ ...okMessage })
  })

  it('默认不传 contentType 时请求体为 text（向后兼容）', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('hello')
    expect(postMock).toHaveBeenCalledTimes(1)
    const [, body] = postMock.mock.calls[0]!
    expect(body.contentType).toBe('text')
  })

  it('传 contentType:image 时请求体原样透传（不硬编码 text）', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('https://minio/x.png', undefined, undefined, 'image')
    const [, body] = postMock.mock.calls[0]!
    expect(body.contentType).toBe('image')
  })

  it('传 contentType:file 时透传 file', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('https://minio/x.pdf', undefined, undefined, 'file')
    const [, body] = postMock.mock.calls[0]!
    expect(body.contentType).toBe('file')
  })
})

describe('message store sendMessage fileName 透传（Stage 89 PR-5 RED）', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    postMock.mockReset()
    postMock.mockResolvedValue({ ...okMessage })
  })

  it('传 fileName 时请求体原样透传（文件理解特性：LLM 与 ChatFile 都需要原始名）', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('https://minio/x.pdf', undefined, undefined, 'file', '季度报告.pdf')
    const [, body] = postMock.mock.calls[0]!
    expect(body.fileName).toBe('季度报告.pdf')
  })

  it('不传 fileName 时请求体不带该字段（向后兼容）', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('纯文字')
    const [, body] = postMock.mock.calls[0]!
    expect(body.fileName).toBeUndefined()
  })
})

// ==== 语音消息落库 · 加载映射（E2E-16 测试点 #5，2026-09-22 用户实测钉出）====
// 后端行形态：content=音频URL、contentType=audio（无 audioUrl 字段）。
// 前端气泡渲染条件是 item.audioUrl（[id].vue v-if="item.audioUrl"）——
// 不做映射的话，即使落库成功，刷新后气泡照样不出来（等于白落库）。
describe('loadMoreMessages 音频行映射（测试点 #5）', () => {
  it('contentType=audio 的行必须把 content 映射到 audioUrl', async () => {
    setActivePinia(createPinia())
    const { get } = await import('~/composables/useApi')
    vi.mocked(get).mockResolvedValue({
      list: [
        {
          id: 'srv-42',
          conversationId: 'c1',
          sender: 'user',
          content: 'http://localhost:19080/api/v1/voice/audio/x.webm',
          contentType: 'audio',
          fileName: 'recording.webm',
          sendTime: Date.now(),
          createdAt: Date.now(),
        },
      ],
      cursor: 0,
      hasMore: false,
    })

    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.loadMoreMessages()

    const row = store.currentMessages.find((m: any) => m.id === 'srv-42') as any
    expect(row, '加载后必须有该行').toBeTruthy()
    expect(
      row.audioUrl,
      'audio 行必须映射 audioUrl=content，否则 [id].vue 的 v-if="item.audioUrl" 永假、刷新后语音气泡仍消失',
    ).toBe('http://localhost:19080/api/v1/voice/audio/x.webm')
    // 非音频行不受影响（text 不该长出 audioUrl）
    expect(row.contentType).toBe('audio')
  })
})
