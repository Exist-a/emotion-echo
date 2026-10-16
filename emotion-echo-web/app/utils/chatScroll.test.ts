import { describe, it, expect } from 'vitest'

import { chatScrollSnapshot, isTailGrowth, shouldFollowToBottom } from '~/utils/chatScroll'

// CL-05 Phase 8 · T-21（CL-F-10）：聊天区"要不要跟随滚到底"的判据。
//
// 用户实测缺陷：发送后聊天区**不**自动滚到底。
// 亲核根因（`app/pages/chat/conversation/[id].vue`）：页面只 `watch` 了
// `messageStore.currentMessages.length`；而流式回复是**同一个消息对象在长大**
// （`useConversationSender.ts`：`tempAiMessage.content += delta`）——
// **条数不变**，所以 watcher 永不触发，新内容长到视口外也不跟随。
//
// 同时暴露第二个（未上报的）问题：上翻加载更早消息是**前插**，条数也会变，
// 旧逻辑照样滚到底 ⇒ 用户被硬拽回底部。所以判据必须区分"尾部增长"与"前插"。

const msg = (id: string, content = 'x') => ({ id, content })

describe('chatScroll · isTailGrowth（是否"尾部增长"⇒ 该跟随到底）', () => {
  it('追加一条新消息（末条 id 变了）⇒ true', () => {
    const prev = chatScrollSnapshot([msg('1'), msg('2')])
    const next = chatScrollSnapshot([msg('1'), msg('2'), msg('3')])
    expect(isTailGrowth(prev, next)).toBe(true)
  })

  it('**流式同条变长**（条数不变、末条内容变长）⇒ true（这正是 CL-F-10）', () => {
    const prev = chatScrollSnapshot([msg('1'), msg('2', 'abc')])
    const next = chatScrollSnapshot([msg('1'), msg('2', 'abcdef')])
    expect(isTailGrowth(prev, next)).toBe(true)
  })

  it('**前插**更早消息（首条变了、末条没变）⇒ false（不得把用户拽回底部）', () => {
    const prev = chatScrollSnapshot([msg('2'), msg('3')])
    const next = chatScrollSnapshot([msg('0'), msg('1'), msg('2'), msg('3')])
    expect(isTailGrowth(prev, next)).toBe(false)
  })

  it('空列表 → 首条 ⇒ true', () => {
    expect(isTailGrowth(chatScrollSnapshot([]), chatScrollSnapshot([msg('1')]))).toBe(true)
  })

  it('完全未变 ⇒ false（不做无谓滚动）', () => {
    const s = chatScrollSnapshot([msg('1'), msg('2', 'abc')])
    expect(isTailGrowth(s, chatScrollSnapshot([msg('1'), msg('2', 'abc')]))).toBe(false)
  })

  it('末条被替换（同条数、不同 id ⇒ 例如服务端真身替换占位行）⇒ true', () => {
    const prev = chatScrollSnapshot([msg('1'), msg('local-1', 'hi')])
    const next = chatScrollSnapshot([msg('1'), msg('42', 'hi')])
    expect(isTailGrowth(prev, next)).toBe(true)
  })

  it('清空（切会话）⇒ false（由页面显式 scrollToBottom 处理）', () => {
    expect(isTailGrowth(chatScrollSnapshot([msg('1')]), chatScrollSnapshot([]))).toBe(false)
  })
})

describe('chatScroll · snapshot', () => {
  it('取首/末条 id 与末条内容长度；空列表用空串/0', () => {
    expect(chatScrollSnapshot([])).toEqual({
      count: 0,
      firstId: '',
      lastId: '',
      lastTailLen: 0,
      lastSender: '',
    })
    expect(chatScrollSnapshot([msg('a', 'xyz'), msg('b', '12')])).toEqual({
      count: 2,
      firstId: 'a',
      lastId: 'b',
      lastTailLen: 2,
      lastSender: '',
    })
  })

  it('末条没有 content 时不炸（音频/文件行）', () => {
    expect(chatScrollSnapshot([{ id: 'a' }]).lastTailLen).toBe(0)
  })
})

describe('chatScroll · shouldFollowToBottom（用户发送 vs 远端流式）', () => {
  const userMsg = (id: string, content = 'hi') => ({ id, content, sender: 'user' })
  const aiMsg = (id: string, content = 'x') => ({ id, content, sender: 'ai' })

  it('**用户自己发出**（末条 sender=user）⇒ 即使他正在上翻看历史，也必须跟随到底', () => {
    // 浏览器实测（CL-05 T-21 首跑）：只加 nearBottom 守卫时，用户在顶部按发送
    // ⇒ 视口停在 1519px 之外，**连自己的消息都看不见**。用户按发送是明确意图。
    const prev = chatScrollSnapshot([aiMsg('1'), aiMsg('2')])
    const next = chatScrollSnapshot([aiMsg('1'), aiMsg('2'), userMsg('3')])
    expect(isTailGrowth(prev, next)).toBe(true)
    expect(shouldFollowToBottom(prev, next, false), 'not nearBottom 也必须跟随').toBe(true)
  })

  it('AI 流式变长 + 用户不在底部 ⇒ **不**跟随（别拽走看历史的人）', () => {
    const prev = chatScrollSnapshot([userMsg('1'), aiMsg('2', 'abc')])
    const next = chatScrollSnapshot([userMsg('1'), aiMsg('2', 'abcdef')])
    expect(shouldFollowToBottom(prev, next, false)).toBe(false)
  })

  it('AI 流式变长 + 用户在底部 ⇒ 跟随', () => {
    const prev = chatScrollSnapshot([userMsg('1'), aiMsg('2', 'abc')])
    const next = chatScrollSnapshot([userMsg('1'), aiMsg('2', 'abcdef')])
    expect(shouldFollowToBottom(prev, next, true)).toBe(true)
  })

  it('前插更早消息 ⇒ 任何情况都不跟随', () => {
    const prev = chatScrollSnapshot([aiMsg('2'), aiMsg('3')])
    const next = chatScrollSnapshot([aiMsg('0'), aiMsg('1'), aiMsg('2'), aiMsg('3')])
    expect(shouldFollowToBottom(prev, next, true)).toBe(false)
  })
})
