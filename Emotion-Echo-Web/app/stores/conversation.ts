import { defineStore } from "pinia";
import { ElNotification } from "element-plus";
import type { returnMsgType } from "~/types/commonType";
import type { ConversationItem, CreateConversationParams } from "~/types/api";
import { get, post, put, del } from "~/composables/useApi";

export const useConversationStore = defineStore("conversation", () => {
  // ==================== State ====================

  const conversationList = ref<ConversationItem[]>([])
  const currentConversationId = ref<string | null>(null)
  const isLoading = ref(false)
  const hasMore = ref(true)
  const cursor = ref<string>("")
  let fetchPromise: Promise<any> | null = null;
  
  // ==================== Getters ====================
  
  const currentConversation = computed(() => {
    return conversationList.value.find(c => c.id === currentConversationId.value) || null;
  });
  
  /**
   * 按时间分组的会话列表
   */
  const groupedConversations = computed(() => {
    const groups: Record<string, any[]> = {
      "置顶": [],
      "今天": [],
      "昨日": [],
      "一周内": [],
      "三十天内": [],
      "更早": [],
    };
    
    // 使用类型断言确保 groups[label] 不会为 undefined
    const getGroup = (label: string): any[] => groups[label] || [];
    
    const now = new Date();
    
    // 计算本地时区今天0点的时间戳
    const todayStartLocal = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
    
    // 将UTC的0点转换为本地时区，再转为时间戳
    const todayUTC = new Date(Date.UTC(now.getFullYear(), now.getMonth(), now.getDate()));
    const todayStartUTC = todayUTC.getTime();
    
    console.log('[Conversation] 开始分组，当前时间:', now, '今天0点(本地):', todayStartLocal, '今天0点(UTC):', todayStartUTC);
    
    conversationList.value.forEach((item) => {
      if (item.isTop) {
        getGroup("置顶").push(item);
        return;
      }
      
      const itemDate = new Date(item.updatedAt);
      const itemTime = itemDate.getTime();
      
      console.log(`[Conversation] 会话 ${item.id}(${item.title}): updatedAt=${item.updatedAt}, itemTime=${itemTime}`);
      
      // 将时间转换为本地日期进行比较
      const itemYear = itemDate.getFullYear();
      const itemMonth = itemDate.getMonth();
      const itemDateValue = itemDate.getDate();
      
      if (itemYear === now.getFullYear() && itemMonth === now.getMonth() && itemDateValue === now.getDate()) {
        // 今天（同一年月日）
        getGroup("今天").push(item);
      } else {
        // 不是今天，计算相差天数
        const diffDays = Math.floor((now.getTime() - itemTime) / (24 * 60 * 60 * 1000));
        if (diffDays === 1) {
          getGroup("昨日").push(item);
        } else if (diffDays <= 7) {
          getGroup("一周内").push(item);
        } else if (diffDays <= 30) {
          getGroup("三十天内").push(item);
        } else {
          getGroup("更早").push(item);
        }
      }
    });
    
    const labelOrder = ["置顶", "今天", "昨日", "一周内", "三十天内", "更早"];
    return labelOrder
      .filter((label) => getGroup(label).length > 0)
      .map((label) => ({
        label,
        data: getGroup(label),
      }));
  });
  
  // ==================== Actions ====================
  
  /**
   * 获取会话列表
   */
  const fetchConversations = async (limit: number = 20): Promise<returnMsgType> => {
    // 防止并发请求
    if (fetchPromise) {
      return fetchPromise;
    }

    isLoading.value = true;
    fetchPromise = (async () => {
      try {
        const params: any = { limit };
        if (cursor.value) {
          params.cursor = cursor.value;
        }
        
        const data = await get<{ list: ConversationItem[]; hasMore: boolean }>(
          "/conversations",
          params
        );
        
        console.log('[Conversation] 从后端获取的会话列表:', data.list.map(item => ({
          id: item.id,
          title: item.title,
          updatedAt: item.updatedAt,
          updatedAtType: typeof item.updatedAt,
          parsedTime: new Date(item.updatedAt).getTime()
        })));
        
        if (cursor.value) {
          // 加载更多，追加到列表
          conversationList.value.push(...data.list);
        } else {
          // 首次加载，替换列表
          conversationList.value = data.list;
        }
        
        hasMore.value = data.hasMore;
        // 更新 cursor 为最后一条的 updatedAt
        if (data.list.length > 0) {
          const lastItem = data.list[data.list.length - 1];
          if (lastItem) {
            cursor.value = lastItem.updatedAt;
          }
        }
        
        return { isOk: true, msg: "获取成功" };
      } catch (error: any) {
        return { isOk: false, msg: error.message || "获取失败" };
      } finally {
        isLoading.value = false;
        fetchPromise = null;
      }
    })();
    
    return fetchPromise;
  };
  
  /**
   * 加载更多会话
   */
  const loadMore = async (): Promise<returnMsgType> => {
    if (!hasMore.value || isLoading.value) {
      return { isOk: true, msg: "没有更多数据" };
    }
    return fetchConversations();
  };
  
  /**
   * 创建新会话
   */
  const createConversation = async (title?: string): Promise<{ isOk: boolean; msg: string; id?: string }> => {
    try {
      const params: CreateConversationParams = {};
      if (title) params.title = title;
      
      const data = await post<ConversationItem>("/conversations", params);
      
      // 添加到列表头部
      conversationList.value.unshift(data);
      
      return { isOk: true, msg: "创建成功", id: data.id };
    } catch (error: any) {
      return { isOk: false, msg: error.message || "创建失败" };
    }
  };
  
  /**
   * 更新会话标题
   */
  const updateConversationTitle = async (id: string, title: string): Promise<returnMsgType> => {
    try {
      await put(`/conversations/${id}`, { title });
      
      // 更新本地数据
      const conversation = conversationList.value.find(c => c.id === id);
      if (conversation) {
        conversation.title = title;
        conversation.updatedAt = new Date().toISOString();
      }
      
      return { isOk: true, msg: "修改成功" };
    } catch (error: any) {
      return { isOk: false, msg: error.message || "修改失败" };
    }
  };
  
  /**
   * 置顶/取消置顶会话
   */
  const togglePinConversation = async (id: string, isTop?: boolean): Promise<returnMsgType> => {
    try {
      const conversation = conversationList.value.find(c => c.id === id);
      if (!conversation) {
        return { isOk: false, msg: "会话不存在" };
      }
      
      const newIsTop = isTop !== undefined ? isTop : !conversation.isTop;
      
      await post(`/conversations/${id}/pin`, { isTop: newIsTop });
      
      // 更新本地数据
      conversation.isTop = newIsTop;
      conversation.updatedAt = new Date().toISOString();
      
      // 重新排序：置顶的在前面
      conversationList.value.sort((a, b) => {
        if (a.isTop === b.isTop) {
          return new Date(b.updatedAt).getTime() - new Date(a.updatedAt).getTime();
        }
        return a.isTop ? -1 : 1;
      });
      
      ElNotification({
        type: "success",
        message: newIsTop ? "置顶成功" : "取消置顶成功",
      });
      
      return { isOk: true, msg: newIsTop ? "置顶成功" : "取消置顶成功" };
    } catch (error: any) {
      return { isOk: false, msg: error.message || "操作失败" };
    }
  };
  
  /**
   * 删除会话
   */
  const deleteConversation = async (id: string): Promise<returnMsgType> => {
    try {
      await del(`/conversations/${id}`);
      
      // 从列表中移除
      const index = conversationList.value.findIndex(c => c.id === id);
      if (index !== -1) {
        conversationList.value.splice(index, 1);
      }
      
      // 如果删除的是当前会话，清空当前会话ID
      if (currentConversationId.value === id) {
        currentConversationId.value = null;
      }
      
      ElNotification({
        type: "success",
        message: "删除成功",
      });
      
      return { isOk: true, msg: "删除成功" };
    } catch (error: any) {
      return { isOk: false, msg: error.message || "删除失败" };
    }
  };
  
  /**
   * 设置当前会话
   */
  const setCurrentConversation = (id: string | null) => {
    currentConversationId.value = id;
  };
  
  /**
   * 初始化
   */
  const init = async () => {
    cursor.value = "";
    hasMore.value = true;
    await fetchConversations();
  };
  
  return {
    // State
    conversationList,
    currentConversationId,
    isLoading,
    hasMore,
    // Getters
    currentConversation,
    groupedConversations,
    // Actions
    fetchConversations,
    loadMore,
    createConversation,
    updateConversationTitle,
    togglePinConversation,
    deleteConversation,
    setCurrentConversation,
    init,
  };
});
