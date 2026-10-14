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
  status?: MessageStatus // 消息状态（可选，用于本地缓存）
  retryCount?: number // 重试次数
}

// 带状态的消息（用于Store）
export interface MessageWithStatus extends StoredMessage {
  status: MessageStatus
  retryCount: number
}
