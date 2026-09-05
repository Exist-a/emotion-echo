// utils/messageCache.ts - 消息本地缓存管理
import type {
  StoredMessage,
  conversationMessageType,
} from "~/types/conversation/conversationMessagesType";
import type { MessageStatus } from "~/stores/message";
import { db } from "./db";
import Dexie from "dexie";

// 带状态的消息类型
interface MessageWithStatus extends StoredMessage {
  status?: MessageStatus;
  retryCount?: number;
}

// ==================== 基础CRUD操作 ====================

/**
 * 保存单条消息（自动关联当前会话）
 * @param sessionId 会话ID
 * @param message 消息对象（可选包含status）
 */
export async function saveMessage(
  sessionId: string,
  message: conversationMessageType & { status?: MessageStatus; retryCount?: number }
): Promise<void> {
  const stored: MessageWithStatus = {
    ...message,
    sessionId,
    status: message.status || "sent",
    retryCount: message.retryCount || 0,
  };
  await db.messages.put(stored);
}

/**
 * 批量保存消息
 * @param sessionId 会话ID
 * @param messages 消息数组
 */
export async function saveMessages(
  sessionId: string,
  messages: (conversationMessageType & { status?: MessageStatus })[]
): Promise<void> {
  const stored = messages.map((msg) => ({
    ...msg,
    sessionId,
    status: msg.status || "sent",
    retryCount: 0,
  }));
  await db.messages.bulkPut(stored);
}

/**
 * 更新消息状态
 * @param messageId 消息ID
 * @param status 新状态
 */
export async function updateMessageStatus(
  messageId: string | number,
  status: MessageStatus
): Promise<void> {
  await db.messages.update(messageId, { status });
}

/**
 * 批量更新消息状态
 * @param messageIds 消息ID数组
 * @param status 新状态
 */
export async function batchUpdateMessageStatus(
  messageIds: (string | number)[],
  status: MessageStatus
): Promise<void> {
  await db.messages.bulkUpdate(
    messageIds.map((id) => ({ key: id, changes: { status } }))
  );
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
  beforeSendTime?: number
): Promise<MessageWithStatus[]> {
  let query = db.messages
    .where("[sessionId+sendTime]")
    .between(
      [sessionId, Dexie.minKey],
      [sessionId, beforeSendTime ?? Dexie.maxKey]
    );

  const messages = await query
    .reverse()
    .limit(pageSize)
    .toArray();

  return messages;
}

/**
 * 获取某个会话的最新 N 条消息（用于恢复当前会话）
 * @param sessionId 会话ID
 * @param limit 限制数量
 * @returns 消息列表
 */
export async function getLatestMessages(
  sessionId: string,
  limit = 50
): Promise<MessageWithStatus[]> {
  return db.messages
    .where("sessionId")
    .equals(sessionId)
    .reverse()
    .limit(limit)
    .toArray();
}

/**
 * 获取指定ID的消息
 * @param messageId 消息ID
 */
export async function getMessageById(
  messageId: string | number
): Promise<MessageWithStatus | undefined> {
  return db.messages.get(messageId);
}

/**
 * 获取发送中的消息（用于断网重连后重试）
 * @param sessionId 会话ID
 */
export async function getPendingMessages(
  sessionId: string
): Promise<MessageWithStatus[]> {
  return db.messages
    .where({ sessionId, status: "sending" })
    .toArray();
}

/**
 * 获取发送失败的消息
 * @param sessionId 会话ID
 */
export async function getFailedMessages(
  sessionId: string
): Promise<MessageWithStatus[]> {
  return db.messages
    .where({ sessionId, status: "failed" })
    .toArray();
}

// ==================== 删除和清理操作 ====================

/**
 * 删除单条消息
 * @param messageId 消息ID
 */
export async function deleteMessage(
  messageId: string | number
): Promise<void> {
  await db.messages.delete(messageId);
}

/**
 * 清空某个会话的所有缓存
 * @param sessionId 会话ID
 */
export async function clearSessionMessages(sessionId: string): Promise<void> {
  await db.messages.where("sessionId").equals(sessionId).delete();
}

/**
 * 缓存淘汰策略：每个会话最多保留 maxCount 条消息（保留最新的）
 * @param sessionId 会话ID
 * @param maxCount 最大保留数量
 */
export async function trimSessionMessages(
  sessionId: string,
  maxCount = 500
): Promise<number> {
  const count = await db.messages.where("sessionId").equals(sessionId).count();

  if (count <= maxCount) return 0;

  // 获取需要删除的最旧消息
  const toDelete = await db.messages
    .where("[sessionId+sendTime]")
    .between([sessionId, Dexie.minKey], [sessionId, Dexie.maxKey])
    .limit(count - maxCount)
    .toArray();

  const idsToDelete = toDelete.map((msg) => msg.id);
  await db.messages.bulkDelete(idsToDelete);

  return idsToDelete.length;
}

/**
 * 清理所有已发送的旧消息（保留最近N天的）
 * @param keepDays 保留天数
 */
export async function trimOldMessages(keepDays = 30): Promise<number> {
  const cutoffTime = Date.now() - keepDays * 24 * 60 * 60 * 1000;
  
  const oldMessages = await db.messages
    .where("sendTime")
    .below(cutoffTime)
    .and((msg) => msg.status === "sent") // 只删除已发送的
    .toArray();

  const idsToDelete = oldMessages.map((msg) => msg.id);
  await db.messages.bulkDelete(idsToDelete);

  return idsToDelete.length;
}

// ==================== 统计和诊断 ====================

/**
 * 获取缓存统计信息
 */
export async function getCacheStats(): Promise<{
  totalMessages: number;
  totalSessions: number;
  sendingCount: number;
  failedCount: number;
}> {
  const allMessages = await db.messages.toArray();
  const sessionIds = new Set(allMessages.map((m) => m.sessionId));

  return {
    totalMessages: allMessages.length,
    totalSessions: sessionIds.size,
    sendingCount: allMessages.filter((m) => m.status === "sending").length,
    failedCount: allMessages.filter((m) => m.status === "failed").length,
  };
}

/**
 * 导出某个会话的所有消息（用于备份或迁移）
 * @param sessionId 会话ID
 */
export async function exportSessionMessages(
  sessionId: string
): Promise<MessageWithStatus[]> {
  return db.messages
    .where("sessionId")
    .equals(sessionId)
    .sortBy("sendTime");
}

/**
 * 导入消息到缓存
 * @param sessionId 会话ID
 * @param messages 消息数组
 */
export async function importSessionMessages(
  sessionId: string,
  messages: MessageWithStatus[]
): Promise<void> {
  const messagesWithSession = messages.map((m) => ({
    ...m,
    sessionId,
  }));
  await db.messages.bulkPut(messagesWithSession);
}
