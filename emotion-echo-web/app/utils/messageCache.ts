// utils/messageCache.ts - 消息本地缓存管理
import type {
  StoredMessage,
  MessageStatus,
  conversationMessageType,
} from '~/types/conversation/conversationMessagesType'
import { getDb } from './db'

// Lazy Dexie key constants (accessed after getDb() initializes Dexie)
async function getDexieKeys() {
  const { default: Dexie } = await import('dexie')
  return { minKey: Dexie.minKey, maxKey: Dexie.maxKey }
}

// CL-F-18：这里曾有一份文件内私有 `interface MessageWithStatus extends StoredMessage`，
// 逐字等于已导出的 `StoredMessage`（`status?` / `retryCount?` 本就可选）⇒ 已删除，
// 统一改用公共存储行类型 `StoredMessage`。契约钉：
// app/types/message-with-status-single-source.architecture.test.ts

// ==================== 基础CRUD操作 ====================

/**
 * 保存单条消息（自动关联当前会话）
 * @param sessionId 会话ID
 * @param message 消息对象（可选包含status）
 * @param userId 归属用户（CL-F-20：用于按用户隔离，换账号后不误补上一账号的消息）
 */
export async function saveMessage(
  sessionId: string,
  message: conversationMessageType & { status?: MessageStatus; retryCount?: number },
  userId?: string,
): Promise<void> {
  const db = await getDb()
  const stored: StoredMessage = {
    ...message,
    sessionId,
    ...(userId ? { userId } : {}),
    status: message.status || 'sent',
    retryCount: message.retryCount || 0,
  }
  await db.messages.put(stored)
}

/**
 * 批量保存消息
 * @param sessionId 会话ID
 * @param messages 消息数组
 * @param userId 归属用户（CL-F-20）
 */
export async function saveMessages(
  sessionId: string,
  messages: (conversationMessageType & { status?: MessageStatus })[],
  userId?: string,
): Promise<void> {
  const db = await getDb()
  const stored = messages.map((msg) => ({
    ...msg,
    sessionId,
    ...(userId ? { userId } : {}),
    status: msg.status || 'sent',
    retryCount: 0,
  }))
  await db.messages.bulkPut(stored)
}

/**
 * 更新消息状态
 * @param messageId 消息ID
 * @param status 新状态
 */
export async function updateMessageStatus(
  messageId: string | number,
  status: MessageStatus,
): Promise<void> {
  const db = await getDb()
  await db.messages.update(messageId, { status })
}

/**
 * 批量更新消息状态
 * @param messageIds 消息ID数组
 * @param status 新状态
 */
export async function batchUpdateMessageStatus(
  messageIds: (string | number)[],
  status: MessageStatus,
): Promise<void> {
  const db = await getDb()
  await db.messages.bulkUpdate(messageIds.map((id) => ({ key: id, changes: { status } })))
}

// ==================== 查询操作 ====================

/**
 * 根据会话ID分页查询历史消息（按 sendTime 倒序，最新的在前）
 * @param sessionId 会话ID
 * @param pageSize 每页数量
 * @param beforeSendTime 游标：获取比此时间戳更早的消息
 * @returns 消息列表
 */
export async function getMessagesBySession(
  sessionId: string,
  pageSize: number,
  beforeSendTime?: number,
): Promise<StoredMessage[]> {
  const db = await getDb()
  const { minKey, maxKey } = await getDexieKeys()
  const query = db.messages
    .where('[sessionId+sendTime]')
    .between([sessionId, minKey], [sessionId, beforeSendTime ?? maxKey])

  const messages = await query.reverse().limit(pageSize).toArray()

  return messages
}

/**
 * 获取某个会话的最新 N 条消息（用于恢复当前会话）
 * @param sessionId 会话ID
 * @param limit 限制数量
 * @returns 消息列表
 */
export async function getLatestMessages(sessionId: string, limit = 50): Promise<StoredMessage[]> {
  const db = await getDb()
  return db.messages.where('sessionId').equals(sessionId).reverse().limit(limit).toArray()
}

/**
 * 获取指定ID的消息
 * @param messageId 消息ID
 */
export async function getMessageById(
  messageId: string | number,
): Promise<StoredMessage | undefined> {
  const db = await getDb()
  return db.messages.get(messageId)
}

/**
 * 获取发送中的消息（用于断网重连后重试）
 * @param sessionId 会话ID
 * @param userId 归属用户（CL-F-20；不传则不限用户）
 */
export async function getPendingMessages(
  sessionId: string,
  userId?: string,
): Promise<StoredMessage[]> {
  const db = await getDb()
  const rows = await db.messages.where({ sessionId, status: 'sending' }).toArray()
  return userId ? rows.filter((m: StoredMessage) => m.userId === userId) : rows
}

/**
 * 获取发送失败的消息
 * @param sessionId 会话ID
 * @param userId 归属用户（CL-F-20；不传则不限用户）
 */
export async function getFailedMessages(
  sessionId: string,
  userId?: string,
): Promise<StoredMessage[]> {
  const db = await getDb()
  const rows = await db.messages.where({ sessionId, status: 'failed' }).toArray()
  return userId ? rows.filter((m: StoredMessage) => m.userId === userId) : rows
}

/**
 * 获取**全部会话**里发送失败的消息（CL-05 Phase 4 · T-18 断网重扫用）。
 *
 * 离线队列是**会话无关**的：用户可能在 A 会话里断网、切到 B 会话才联网 ——
 * 只扫当前会话会漏掉 A 里那条待重发的消息。
 * ⚠️ 用 `filter`（全表扫描）而非 `where('status')`：schema 里没有**单列** status 索引
 * （只有 `[sessionId+status]` 复合索引），`where('status')` 会抛 SchemaError。
 * 本地待重发队列规模很小，全表扫描代价可接受。
 *
 * **CL-F-20（按用户隔离）**：传了 `userId` 就只返回**该用户自己的**失败行 ——
 * 换账号登录后，上一账号留在同一浏览器库里的 `failed` 行**看不见也补不了**，
 * 否则会拿新账号的令牌去补旧账号的消息。不传 `userId` 时保留旧的全量语义，
 * 仅用于"未登录/遗留行"的兜底查询；自动补发路径**必须**传 userId（见 stores/message.ts）。
 */
export async function getAllFailedMessages(userId?: string): Promise<StoredMessage[]> {
  const db = await getDb()
  return db.messages
    .filter((m: StoredMessage) => m.status === 'failed' && (!userId || m.userId === userId))
    .toArray()
}

// ==================== 删除和清理操作 ====================

/**
 * 删除单条消息
 * @param messageId 消息ID
 */
export async function deleteMessage(messageId: string | number): Promise<void> {
  const db = await getDb()
  await db.messages.delete(messageId)
}

/**
 * 清空某个会话的所有缓存
 * @param sessionId 会话ID
 */
export async function clearSessionMessages(sessionId: string): Promise<void> {
  const db = await getDb()
  await db.messages.where('sessionId').equals(sessionId).delete()
}

/**
 * 缓存淘汰策略：每个会话最多保留 maxCount 条消息（保留最新的）
 * @param sessionId 会话ID
 * @param maxCount 最大保留数量
 */
export async function trimSessionMessages(sessionId: string, maxCount = 500): Promise<number> {
  const db = await getDb()
  const { minKey, maxKey } = await getDexieKeys()
  const count = await db.messages.where('sessionId').equals(sessionId).count()

  if (count <= maxCount) return 0

  // 获取需要删除的最旧消息
  const toDelete = await db.messages
    .where('[sessionId+sendTime]')
    .between([sessionId, minKey], [sessionId, maxKey])
    .limit(count - maxCount)
    .toArray()

  const idsToDelete = toDelete.map((msg: StoredMessage) => msg.id)
  await db.messages.bulkDelete(idsToDelete)

  return idsToDelete.length
}

/**
 * 清理所有已发送的旧消息（保留最近N天的）
 * @param keepDays 保留天数
 */
export async function trimOldMessages(keepDays = 30): Promise<number> {
  const db = await getDb()
  const cutoffTime = Date.now() - keepDays * 24 * 60 * 60 * 1000

  const oldMessages = await db.messages
    .where('sendTime')
    .below(cutoffTime)
    .and((msg: any) => msg.status === 'sent') // 只删除已发送的
    .toArray()

  const idsToDelete = oldMessages.map((msg: any) => msg.id)
  await db.messages.bulkDelete(idsToDelete)

  return idsToDelete.length
}

// ==================== 统计和诊断 ====================

/**
 * 获取缓存统计信息
 */
export async function getCacheStats(): Promise<{
  totalMessages: number
  totalSessions: number
  sendingCount: number
  failedCount: number
}> {
  const db = await getDb()
  const allMessages = await db.messages.toArray()
  const sessionIds = new Set(allMessages.map((m: any) => m.sessionId))

  return {
    totalMessages: allMessages.length,
    totalSessions: sessionIds.size,
    sendingCount: allMessages.filter((m: any) => m.status === 'sending').length,
    failedCount: allMessages.filter((m: any) => m.status === 'failed').length,
  }
}

/**
 * 导出某个会话的所有消息（用于备份或迁移）
 * @param sessionId 会话ID
 */
export async function exportSessionMessages(sessionId: string): Promise<StoredMessage[]> {
  const db = await getDb()
  return db.messages.where('sessionId').equals(sessionId).sortBy('sendTime')
}

/**
 * 导入消息到缓存
 * @param sessionId 会话ID
 * @param messages 消息数组
 */
export async function importSessionMessages(
  sessionId: string,
  messages: StoredMessage[],
): Promise<void> {
  const db = await getDb()
  const messagesWithSession = messages.map((m) => ({
    ...m,
    sessionId,
  }))
  await db.messages.bulkPut(messagesWithSession)
}
