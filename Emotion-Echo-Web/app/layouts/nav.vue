<template>
  <div class="nav-container" :class="$device.isMobile ? 'nav-mobile' : ''">
    <el-menu
      :default-active="activeIndex"
      class="menu"
      :class="$device.isMobile?'menu-mobile':''"
      :mode="$device.isMobile ? 'horizontal' : 'vertical'"
    >
      <NuxtLink to="/chat/conversation">
        <el-menu-item index="1" class="menu-item">
          <el-icon><ChatLineSquare /></el-icon>
          <template #title>聊天</template>
        </el-menu-item>
      </NuxtLink>

      <el-sub-menu index="2" class="menu-item">
        <template #title>
          <el-icon><PieChart /></el-icon>
          <span>统计</span>
        </template>
        <NuxtLink to="/chat/dashboard/dailyReport">
          <el-menu-item index="1-1">日度分析</el-menu-item>
        </NuxtLink>
        <NuxtLink to="/chat/dashboard/weeklyReport">
          <el-menu-item index="1-2">周度分析</el-menu-item>
        </NuxtLink>
        <NuxtLink to="/chat/dashboard/monthlyReport">
          <el-menu-item index="1-3">月度分析</el-menu-item>
        </NuxtLink>
        <NuxtLink to="/chat/dashboard/annualReport">
          <el-menu-item index="1-4">年度分析</el-menu-item>
        </NuxtLink>
      </el-sub-menu>
      <!-- </el-menu-item> -->
      <NuxtLink to="/chat/user">
        <el-menu-item index="3" class="menu-item">
          <el-icon><User /></el-icon>
          <template #title>我的</template>
        </el-menu-item>
      </NuxtLink>
      <NuxtLink to="/chat/setting">
        <el-menu-item index="4" class="menu-item">
          <el-icon><Setting /></el-icon>
          <template #title>设置</template>
        </el-menu-item>
      </NuxtLink>
    </el-menu>
    <main class="main" :class="$device.isMobile?'main-mobile':''"><slot /></main>
  </div>
</template>

<script setup lang="ts">
import {
  ChatLineSquare,
  PieChart,
  Setting,
  User,
} from "@element-plus/icons-vue";
const route = useRoute();
const activeIndex = computed(() => {
  const currentPath = route.path; // 当前路由路径
  // 匹配规则：按路由路径映射到菜单index
  if (currentPath.startsWith("/chat/conversation")) {
    return "1"; // 聊天（包括其子路由）
  } else if (currentPath === "/chat/dashboard/dailyReport") {
    return "1-1"; // 日度分析
  } else if (currentPath === "/chat/dashboard/weeklyReport") {
    return "1-2"; // 周度分析
  } else if (currentPath === "/chat/dashboard/monthlyReport") {
    return "1-3"; // 月度分析
  } else if (currentPath === "/chat/dashboard/annualReport") {
    return "1-4"; // 年度分析
  } else if (currentPath === "/chat/user") {
    return "3"; // 我的
  } else if (currentPath === "/chat/setting") {
    return "4"; // 设置
  }
  return "1"; // 默认激活聊天
});
</script>

<style scoped lang="scss">
.nav-container {
  display: flex;
  overflow: hidden;
  .menu {
    // padding: 5px;
    height: 100vh;
  }
  .menu-mobile{
    height:auto;
    
  }
  .main {
    padding: 20px;
    flex: 1;
    overflow-y: auto;
    height: 100vh;
  }
  .main-mobile{
    height:calc(100vh - 59px);
  }
}
.nav-mobile {
  display: block;
}
</style>
