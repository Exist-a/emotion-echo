import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import { useAIStreamHandler } from './useAIStreamHandler'

/**
 * Stage 33 PR-17 · SSE 协议对齐（TDD RED → GREEN）。
 *
 * BFF 实际输出 OpenAI 兼容格式：
 *   data: {"choices":[{"delta":{"content":"你好"}}]}\n\n
 *   data: [DONE]\n\n
 *
 * 旧的 switch(data.type) 解析永远收不到 delta —— 本测试覆盖正确解析。
 */

function makeSSEStream(chunks: string[]) {
  const encoder = new TextEncoder()
  const chunksUint8 = chunks.map((c) => encoder.encode(c))
  let i = 0
  return {
    ok: true,
    status: 200,
    body: {
      getReader() {
        return {
          async read() {
            if (i >= chunksUint8.length) {
              return { done: true, value: undefined }
            }
            const value = chunksUint8[i++]
            return { done: false, value }
          },
          releaseLock() {
            /* noop */
          }
        }
      }
    }
  } as unknown as Response
}

function makeErrorResponse(status: number, message: string) {
  return {
    ok: false,
    status,
    json: async () => ({ message })
  } as unknown as Response
}

describe('useAIStreamHandler · OpenAI 兼容 SSE 解析', () => {
  let fetchSpy: any

  beforeEach(() => {
    fetchSpy = vi.fn()
    ;(globalThis as any).fetch = fetchSpy
    ;(globalThis as any).localStorage = {
      getItem: vi.fn().mockReturnValue('')
    }
    ;(globalThis as any).useRuntimeConfig = () => ({
      public: { API_BASE_URL: 'http://test.local/api/v1' }
    })
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('累加 OpenAI delta content 并在 [DONE] 后触发 onFinish', async () => {
    fetchSpy.mockResolvedValue(
      makeSSEStream([
        'data: {"choices":[{"delta":{"content":"你好"}}]}\n\n',
        'data: {"choices":[{"delta":{"content":"，hello"}}]}\n\n',
        'data: [DONE]\n\n'
      ])
    )

    const { sendAIStream, isStreaming } = useAIStreamHandler()
    const onDelta = vi.fn()
    const onFinish = vi.fn()

    const result = await sendAIStream(
      { message: 'test', emotion: 'neutral' },
      { onDelta, onFinish }
    )

    expect(result).toEqual({ isOk: true, msg: '对话完成' })
    expect(onDelta).toHaveBeenCalledTimes(2)
    expect(onDelta).toHaveBeenNthCalledWith(1, '你好')
    expect(onDelta).toHaveBeenNthCalledWith(2, '，hello')
    expect(onFinish).toHaveBeenCalledTimes(1)
    expect(isStreaming.value).toBe(false)
  })

  it('空 choices 不触发 onDelta，但 [DONE] 仍触发 onFinish', async () => {
    fetchSpy.mockResolvedValue(
      makeSSEStream([
        'data: {"choices":[]}\n\n',
        'data: {"object":"chat.completion","choices":[]}\n\n',
        'data: [DONE]\n\n'
      ])
    )

    const { sendAIStream } = useAIStreamHandler()
    const onDelta = vi.fn()
    const onFinish = vi.fn()

    await sendAIStream({ message: 'test', emotion: 'neutral' }, { onDelta, onFinish })

    expect(onDelta).not.toHaveBeenCalled()
    expect(onFinish).toHaveBeenCalledTimes(1)
  })

  it('非 JSON 行累计 parseErrorCount 但不触发 onError；DONE 仍触发 onFinish', async () => {
    fetchSpy.mockResolvedValue(
      makeSSEStream([
        'data: not-a-json\n\n',
        'data: also-bad-{json\n\n',
        'data: [DONE]\n\n'
      ])
    )

    const { sendAIStream } = useAIStreamHandler()
    const onDelta = vi.fn()
    const onFinish = vi.fn()
    const onError = vi.fn()

    await sendAIStream({ message: 'test', emotion: 'neutral' }, { onDelta, onFinish, onError })

    expect(onError).not.toHaveBeenCalled()
    expect(onFinish).toHaveBeenCalledTimes(1)
  })

  it('Abort 后 streamCancelled=true → 返回 isOk=true msg=已取消，不调 onError', async () => {
    const abortedErr: any = new Error('aborted')
    abortedErr.name = 'AbortError'
    fetchSpy.mockRejectedValue(abortedErr)

    const { sendAIStream, cancelAIStream } = useAIStreamHandler()
    const onError = vi.fn()

    const promise = sendAIStream({ message: 'test', emotion: 'neutral' }, { onError })
    cancelAIStream()
    const result = await promise

    expect(result.isOk).toBe(true)
    expect(result.msg).toBe('已取消')
    expect(onError).not.toHaveBeenCalled()
  })

  it('HTTP 非 200 + JSON {message} → 调 onError + 返回 isOk=false', async () => {
    fetchSpy.mockResolvedValue(makeErrorResponse(500, 'boom'))

    const { sendAIStream } = useAIStreamHandler()
    const onError = vi.fn()
    const onFinish = vi.fn()

    const result = await sendAIStream({ message: 'test', emotion: 'neutral' }, { onError, onFinish })

    expect(result.isOk).toBe(false)
    expect(result.msg).toContain('boom')
    expect(onError).toHaveBeenCalledTimes(1)
    expect(onError).toHaveBeenCalledWith(expect.stringContaining('boom'))
    expect(onFinish).not.toHaveBeenCalled()
  })

  it('chunk 跨字节：reader.read 只返回半行时能正确缓冲并解析完整 delta', async () => {
    fetchSpy.mockResolvedValue(
      makeSSEStream([
        'data: {"choices":[{"delta":{"conte',
        'nt":"你好"}}]}\n\ndata: {"choices":[{"delta":{"content":"，hi"}}]}\n\n',
        'data: [DONE]\n\n'
      ])
    )

    const { sendAIStream } = useAIStreamHandler()
    const onDelta = vi.fn()
    const onFinish = vi.fn()

    await sendAIStream({ message: 'test', emotion: 'neutral' }, { onDelta, onFinish })

    expect(onDelta).toHaveBeenCalledTimes(2)
    expect(onDelta).toHaveBeenNthCalledWith(1, '你好')
    expect(onDelta).toHaveBeenNthCalledWith(2, '，hi')
    expect(onFinish).toHaveBeenCalledTimes(1)
  })

  it('多次 [DONE] 只触发一次 onFinish（去重）', async () => {
    fetchSpy.mockResolvedValue(
      makeSSEStream([
        'data: {"choices":[{"delta":{"content":"a"}}]}\n\n',
        'data: [DONE]\n\n',
        'data: [DONE]\n\n'
      ])
    )

    const { sendAIStream } = useAIStreamHandler()
    const onFinish = vi.fn()

    await sendAIStream({ message: 'test', emotion: 'neutral' }, { onFinish })

    expect(onFinish).toHaveBeenCalledTimes(1)
  })

  it('重复 sendAIStream 时第二次应立即返回 isOk=false msg=正在对话中', async () => {
    let resolveFirst!: (v: any) => void
    fetchSpy.mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve
        })
    )

    const { sendAIStream } = useAIStreamHandler()

    const first = sendAIStream({ message: 'a', emotion: 'neutral' })
    const second = await sendAIStream({ message: 'b', emotion: 'neutral' })

    expect(second).toEqual({ isOk: false, msg: '正在对话中' })

    resolveFirst(makeSSEStream(['data: [DONE]\n\n']))
    await first
  })
})
