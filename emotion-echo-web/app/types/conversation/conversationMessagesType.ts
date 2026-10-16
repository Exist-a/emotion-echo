export type conversationMessagesType = conversationMessageType[]

/**
 * 消息状态 —— **全仓唯一来源**（CL-05 Phase 1 · T-13）。
 * 其余文件必须 `import type { MessageStatus }`，**不得**重声明同名字面量联合；
 * 契约钉：`app/types/conversation/message-status-single-source.architecture.test.ts`。
 *
 * 取值：sending 已发出待确认 / sent 成功 / failed 失败可重试 /
 *       streaming 流式进行中 / truncated 超长截断
 */
export type MessageStatus = 'sending' | 'sent' | 'failed' | 'streaming' | 'truncated'

export interface conversationMessageType {
  id: string | number // 改为string | number，兼容临时ID和服务器ID
  sender: 'user' | 'AI'
  sendTime: number
  content: string | null
  contentType: 'text' | 'audio' | 'img'
  emotionTag?: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral' // 情绪标签（用户消息）
}

// 数据库存储所需
export interface StoredMessage extends conversationMessageType {
  sessionId: string // 所属会话ID
  /**
   * 归属用户（CL-F-20）。Dexie 是**按浏览器**而非按账号的存储：同浏览器换账号后，
   * 上一账号的 `failed` 行仍在库里。没有 userId 就无从判断"这条待补发的消息是谁的"，
   * 会在登录完成前拿新账号令牌补旧账号的消息（后端 `client_msg_id` 唯一索引只保证
   * 不重复入库，**不保证归属正确**）。可选是为了兼容修复前落下的旧行。
   */
  userId?: string
  status?: MessageStatus // 消息状态（可选，用于本地缓存）
  retryCount?: number // 重试次数
}

// 带状态的消息（用于Store）—— **存储行/本地缓存侧**形态（`CL-F-18`）。
// ⚠️ 边界：与 `types/api.ts` 的**同名**类型不是同一个东西（此处 `sender` 取 `'user' | 'AI'`、
//   以 `StoredMessage` 为基；api 侧取 `'user' | 'ai'`、以 `MessageItem` 为基）⇒ 暂不可合并，
//   合并需先统一 `sender` 拼写（被用户裁定 `CL-D-12`「只登记不动」阻塞）。
//   契约钉：app/types/message-with-status-single-source.architecture.test.ts
export interface MessageWithStatus extends StoredMessage {
  status: MessageStatus
  retryCount: number
}
