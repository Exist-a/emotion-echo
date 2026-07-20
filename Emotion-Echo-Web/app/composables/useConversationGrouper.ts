/**
 * 会话分组 Composable
 * 提取自 stores/conversation.ts 的分组逻辑
 */
import type { ConversationItem } from '~/types/api'
import { computed } from 'vue'

export interface GroupedConversation {
  label: string
  data: ConversationItem[]
}

export function useConversationGrouper(conversationList: Ref<ConversationItem[]>) {
  const groupedConversations = computed(() => {
    const groups: Record<string, ConversationItem[]> = {
      "置顶": [],
      "今天": [],
      "一周内": [],
      "三十天内": [],
      "更早": [],
    };

    const getGroup = (label: string): ConversationItem[] => groups[label] || [];

    const now = new Date();
    const todayStartLocal = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    const todayUTC = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()));
    const todayStartUTC = todayUTC.getTime();

    conversationList.value.forEach((item) => {
      if (item.isTop) {
        getGroup("置顶").push(item);
        return;
      }

      const itemDate = new Date(item.updatedAt);
      const itemTime = itemDate.getTime();

      const itemYear = itemDate.getFullYear();
      const itemMonth = itemDate.getMonth();
      const itemDateValue = itemDate.getDate();

      if (itemYear === now.getFullYear() && itemMonth === now.getMonth() && itemDateValue === now.getDate()) {
        getGroup("今天").push(item);
      } else {
        const diff = now.getTime() - itemTime;
        if (diff <= 7 * 24 * 60 * 60 * 1000) {
          getGroup("一周内").push(item);
        } else if (diff <= 30 * 24 * 60 * 60 * 1000) {
          getGroup("三十天内").push(item);
        } else {
          getGroup("更早").push(item);
        }
      }
    });

    const labelOrder = ["置顶", "今天", "一周内", "三十天内", "更早"];
    return labelOrder
      .filter((label) => getGroup(label).length > 0)
      .map((label) => ({
        label,
        data: getGroup(label),
      }));
  });

  return {
    groupedConversations
  }
}
