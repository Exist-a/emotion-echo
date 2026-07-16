// 会话列表项数据类型（兼容后端 ConversationItem）
export type conversationListItemDataType = {
  id: string;
  title: string;
  isTop: boolean;
  // 后端返回的字段，前端可能用到
  userId?: string;
  lastMessage?: string | null;
  lastMessageTime?: number | null;
  createdAt?: string;
  updatedAt?: string;
};

export type conversationListLabelType =
  | "今天"
  | "一周内"
  | "三十天内"
  | "更早"
  | "置顶";

export interface conversationListItemType {
  label: conversationListLabelType;
  data: conversationListItemDataType[] | null;
}
