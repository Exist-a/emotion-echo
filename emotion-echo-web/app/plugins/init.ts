// plugins/init.ts - 应用初始化插件
import { useUserStore } from "~/stores/user";
import { useConversationStore } from "~/stores/conversation";
import { useMessageStore } from "~/stores/message";

/**
 * 应用初始化插件
 * 在 Nuxt 应用启动时执行，负责：
 * 1. 恢复用户登录状态
 * 2. 初始化用户配置
 * 3. 初始化会话列表
 * 4. 网络状态监听
 */
export default defineNuxtPlugin(async (nuxtApp) => {
  // 仅在客户端执行
  if (!import.meta.client) return;

  console.log("🚀 应用初始化开始...");

  // 1. 初始化用户 Store
  const userStore = useUserStore();
  userStore.init();
  
  // 应用用户主题设置
  const config = userStore.getUserConfig();
  if (config && config.theme) {
    userStore.applyTheme(config.theme);
  }

  // 2. 检查登录状态
  if (userStore.isAuthenticated) {
    console.log("✅ 用户已登录");

    // Token 即将过期，提醒刷新
    if (userStore.isTokenExpired()) {
      console.warn("⚠️ Token 即将过期，建议刷新");
      // 这里可以触发自动刷新逻辑
    }

    // 获取最新用户信息
    try {
      await userStore.fetchUserInfo();
      // 获取后端配置后重新应用主题（覆盖之前的默认值）
      const latestConfig = userStore.getUserConfig();
      userStore.applyTheme(latestConfig.theme);
    } catch (error) {
      console.warn("获取用户信息失败", error);
    }

    // 3. 初始化会话列表
    const conversationStore = useConversationStore();
    try {
      await conversationStore.init();
    } catch (error) {
      console.warn("初始化会话列表失败", error);
    }
  } else {
    console.log("👤 用户未登录");
  }

  // 4. 监听网络状态变化
  setupNetworkListener();

  // 5. 监听页面可见性变化（用于处理后台切回前台时的数据同步）
  setupVisibilityListener();

  console.log("✨ 应用初始化完成");
});

/**
 * 设置网络状态监听
 */
function setupNetworkListener() {
  if (typeof window === 'undefined') return;
  if (!navigator.onLine) {
    console.log("📡 当前处于离线状态");
  }

  window.addEventListener("online", () => {
    console.log("📡 网络已恢复");

    // TODO: 实现离线消息重试机制
    // const messageStore = useMessageStore();
    // messageStore.retryFailedMessages?.();
  });

  window.addEventListener("offline", () => {
    console.log("📡 网络已断开");
  });
}

function setupVisibilityListener() {
  if (typeof document === 'undefined') return;
  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") {
      console.log("👁️ 页面重新可见");
      
      // 检查是否需要刷新数据
      const userStore = useUserStore();
      if (userStore.isAuthenticated) {
        // 可以在这里触发数据同步
      }
    }
  });
}
