/**
 * 聊天区"要不要跟随滚到底"的判据（CL-05 Phase 8 · T-21 / 账本 `CL-F-10`）。
 *
 * 为什么需要它（亲核根因，2026-10-16）：
 *   页面原先只 `watch(() => messageStore.currentMessages.length)`。
 *   而 AI 流式回复是**同一个消息对象在长大**
 *   （`useConversationSender.ts`：`tempAiMessage.content += delta`）——
 *   **条数不变** ⇒ watcher 永不触发 ⇒ 新内容长到视口外也不跟随
 *   （用户实测："发送后聊天区不自动滚到底，需手动滚才看到末尾"）。
 *
 *   同一处还埋着第二个（未被上报的）问题：上翻加载更早消息是**前插**，
 *   条数也会变 ⇒ 旧逻辑同样滚到底，把正在看历史的用户**硬拽回底部**。
 *   所以判据必须区分"尾部增长"（该跟随）与"前插"（不该跟随）。
 *
 * 这里把判据做成**纯函数**：不碰 DOM ⇒ 可单测（AGENTS §3 可测试性）。
 * DOM 侧的"用户当前是否在底部附近"由页面单独判断并与之取交集。
 */

export interface ChatScrollItem {
  id: string
  content?: string
  sender?: string
}

export interface ChatScrollSnapshot {
  /** 消息条数 */
  count: number
  /** 首条 id（前插会让它变） */
  firstId: string
  /** 末条 id（追加/替换会让它变） */
  lastId: string
  /** 末条 content 长度 —— 流式增长的代理量（条数不变时唯一能观察到的变化） */
  lastTailLen: number
  /** 末条的发送方（`user` / `ai`…）—— 决定"是否用户自己发的" */
  lastSender: string
}

/** 取当前列表的快照（纯函数，不改动入参）。 */
export function chatScrollSnapshot(list: readonly ChatScrollItem[] | undefined): ChatScrollSnapshot {
  const arr = list ?? []
  const first = arr[0]
  const last = arr[arr.length - 1]
  return {
    count: arr.length,
    firstId: first ? String(first.id) : '',
    lastId: last ? String(last.id) : '',
    lastTailLen: last?.content ? last.content.length : 0,
    lastSender: last?.sender ? String(last.sender) : '',
  }
}

/**
 * 这次变化是否属于"尾部增长"（⇒ 值得把视口带到末尾）。
 *
 * - 追加：末条 id 变了 → true
 * - 流式：条数不变、末条 id 不变而内容变长 → true（CL-F-10 主场景）
 * - 末条被替换（占位行 → 服务端真身，同条数不同 id）→ true
 * - 前插更早消息（首条变、末条不变）→ **false**（别把看历史的用户拽走）
 * - 清空 / 无变化 → false
 */
export function isTailGrowth(prev: ChatScrollSnapshot, next: ChatScrollSnapshot): boolean {
  if (next.count === 0) return false
  if (next.count > prev.count) {
    return next.lastId !== prev.lastId
  }
  if (next.count === prev.count) {
    return next.lastId !== prev.lastId || next.lastTailLen > prev.lastTailLen
  }
  // next.count < prev.count：条数变少（如删除/切会话）—— 不在这里跟随
  return false
}

/**
 * 最终判据：这一次变化**要不要**把视口带到末尾。
 *
 * 两条规则（缺一不可）：
 *  1. **用户自己刚发出一条**（末条 sender==='user'）⇒ **无条件**跟随。
 *     用户按发送就是一个明确意图"我要看这条的回复"；此时若因为
 *     "他刚才往上翻了历史"而不跟随，用户看到的是**自己的消息都没滚进来**
 *     （浏览器实测：滚动条停在 1519px 之外 —— 这正是本函数第二个入参存在的原因）。
 *  2. **其他尾部增长**（AI 回复流式变长 / 末条被替换）⇒ 只在
 *     `nearBottom`（用户原本就在底部附近）时跟随，避免把正在看历史的用户拽走。
 */
export function shouldFollowToBottom(
  prev: ChatScrollSnapshot,
  next: ChatScrollSnapshot,
  nearBottom: boolean,
): boolean {
  if (!isTailGrowth(prev, next)) return false
  if (next.lastSender === 'user') return true
  return nearBottom
}
