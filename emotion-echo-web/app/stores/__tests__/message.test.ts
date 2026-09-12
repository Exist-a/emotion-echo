// message store contentType 透传测试（Stage 79 RED）
//
// 背景（stage-78 核查 / docs/plans/file-upload-message-extension.md 注记）：
// 后端链路已全就绪（BFF SendMessageReq 透传 ContentType → chat-svc gRPC → DB
// VARCHAR(16)），但 store 的 sendMessage 硬编码 contentType:'text'，文件消息
// 无法落库。本文件锁定"调用方传 contentType，请求体必须原样透传"的装配契约。
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'

const postMock = vi.fn()

vi.mock('~/composables/useApi', () => ({
  post: (...args: unknown[]) => postMock(...args),
  get: vi.fn(),
  put: vi.fn(),
  patch: vi.fn(),
  del: vi.fn()
}))

import { useMessageStore } from '~/stores/message'

const okMessage = {
  id: 'm1',
  conversationId: 'c1',
  sender: 'user',
  content: 'hello',
  contentType: 'text',
  sendTime: Date.now(),
  createdAt: Date.now()
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
    const [, body] = postMock.mock.calls[0]
    expect(body.contentType).toBe('text')
  })

  it('传 contentType:image 时请求体原样透传（不硬编码 text）', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('https://minio/x.png', undefined, undefined, 'image')
    const [, body] = postMock.mock.calls[0]
    expect(body.contentType).toBe('image')
  })

  it('传 contentType:file 时透传 file', async () => {
    const store = useMessageStore()
    store.currentSessionId = 'c1'
    await store.sendMessage('https://minio/x.pdf', undefined, undefined, 'file')
    const [, body] = postMock.mock.calls[0]
    expect(body.contentType).toBe('file')
  })
})
