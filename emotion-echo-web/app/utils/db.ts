import type { EntityTable } from "dexie";
import type { StoredMessage } from "~/types/conversation/conversationMessagesType";

// SSR-safe lazy singleton: Dexie accesses indexedDB which doesn't exist on the server.
// First call to getDb() on the client dynamically imports and caches the instance.
let _db: any = null;
let _initPromise: Promise<any> | null = null;

export async function getDb() {
  if (_db) return _db;
  if (!import.meta.client) {
    throw new Error("[db] IndexedDB is not available on the server");
  }
  if (!_initPromise) {
    _initPromise = (async () => {
      const { default: Dexie } = await import("dexie");
      const db = new Dexie('EmotionEcho') as Dexie & {
        messages: EntityTable<StoredMessage, 'id'>;
      };
      db.version(1).stores({
        messages: 'id, sessionId, [sessionId+sendTime]',
      });
      _db = db;
      return db;
    })();
  }
  return _initPromise;
}

/**
 * Synchronous accessor — returns the cached instance if already initialized,
 * otherwise throws. Use `await getDb()` for the safe async path.
 */
export function getDbSync() {
  if (!_db) throw new Error("[db] Not initialized — call await getDb() first");
  return _db;
}
