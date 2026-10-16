import type { EntityTable } from 'dexie'
import type { StoredMessage } from '~/types/conversation/conversationMessagesType'

// SSR-safe lazy singleton: Dexie accesses indexedDB which doesn't exist on the server.
// First call to getDb() on the client dynamically imports and caches the instance.
let _db: any = null
let _initPromise: Promise<any> | null = null

export async function getDb() {
  if (_db) return _db
  if (!import.meta.client) {
    throw new Error('[db] IndexedDB is not available on the server')
  }
  if (!_initPromise) {
    _initPromise = (async () => {
      const { default: Dexie } = await import('dexie')
      const db = new Dexie('EmotionEcho') as InstanceType<typeof Dexie> & {
        messages: EntityTable<StoredMessage, 'id'>
      }
      db.version(1).stores({
        messages: 'id, sessionId, [sessionId+sendTime]',
      })
      // CL-05 Phase 2 · T-9：v1 漏建了缓存层**真实在用**的两个索引，补上。
      //   · sendTime            —— trimOldMessages / exportSessionMessages 直接按时间查/排序，
      //                            缺索引时 `where('sendTime')` 抛
      //                            SchemaError（实测：KeyPath sendTime ... is not indexed）；
      //   · [sessionId+status]  —— getPendingMessages / getFailedMessages 的 where({sessionId,status})。
      //                            ⚠️ 实测**不抛**（Dexie 用 sessionId 索引 + 内存过滤兜住），
      //                            补它是为了走索引而非全会话扫描（效率/契约，不是修崩溃）。
      // 加法式升级：v1 声明保留，Dexie 自动迁移、不删旧 store、不丢用户本地缓存。
      db.version(2).stores({
        messages: 'id, sessionId, sendTime, [sessionId+sendTime], [sessionId+status]',
      })
      // CL-F-20：缓存行补 `userId`（归属用户）并建索引。
      //   · userId           —— 按用户隔离查询（换账号后不误补上一账号的消息）；
      //   · [userId+status]  —— getAllFailedMessages(userId) 走索引而非全表扫描。
      // 加法式升级：v1/v2 声明均保留，Dexie 自动迁移；旧行没有 userId ⇒ 查不到，
      // 语义 = "归属未知的行不自动补发"（宁可不发，不拿错令牌发）。
      db.version(3).stores({
        messages:
          'id, sessionId, userId, sendTime, [sessionId+sendTime], [sessionId+status], [userId+status]',
      })
      _db = db
      return db
    })()
  }
  return _initPromise
}

/**
 * Synchronous accessor — returns the cached instance if already initialized,
 * otherwise throws. Use `await getDb()` for the safe async path.
 */
export function getDbSync() {
  if (!_db) throw new Error('[db] Not initialized — call await getDb() first')
  return _db
}
