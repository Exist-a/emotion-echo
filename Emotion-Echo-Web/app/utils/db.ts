import Dexie, { type EntityTable } from "dexie";
import type { StoredMessage } from "~/types/conversation/conversationMessagesType";

export const db = new Dexie('EmotionEcho') as Dexie & {
  messages: EntityTable<
    StoredMessage,
    'id'               // 主键（消息ID）
  >;
};


db.version(1).stores({
  messages: 'id, sessionId, [sessionId+sendTime]' // 复合索引用于高效查询
});