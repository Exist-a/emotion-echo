<template>
  <div class="forget-container" :class="{ 'mobile-container': $device.isMobile }">
    <!-- 步骤条：根据设备调整间距和尺寸 -->
    <el-steps
      class="forget-steps"
      :class="{ 'mobile-steps': $device.isMobile }"
      :active="active"
      finish-status="success"
      :space="$device.isMobile ? 'auto' : 300"
      simple
    >
      <el-step title="确认账户" :icon="Lock" />
      <el-step title="修改密码" :icon="Edit" />
      <el-step title="修改成功" :icon="Check" />
    </el-steps>
    <main class="forget-main" :class="{ 'mobile-main': $device.isMobile }">
      <NuxtPage @changeActive="changeActive" />
    </main>
  </div>
</template>

<script setup lang="ts">
import { Lock, Edit, Check } from "@element-plus/icons-vue";
const route = useRoute();
const router = useRouter();

// 若需要在script中使用设备判断，补充useDevice（模板中直接用$device即可）
const { isMobile } = useDevice();

// 核心：定义「路由路径」与「步骤值」的映射表（根据你的实际路由路径调整）
const routeStepMap = new Map([
  ["/login/forget/verify", 0], // 确认账户/验证码 → 步骤0
  ["/login/forget/modify", 1], // 修改密码 → 步骤1
  ["/login/forget/success", 2], // 修改成功 → 步骤2
]);

// 初始化 active 值：根据当前路由匹配
const active = ref(routeStepMap.get(route.path) || 0);

// 监听路由变化：浏览器后退/前进时，同步更新 active
watch(
  () => route.path, // 监听路由路径变化
  (newPath) => {
    active.value = routeStepMap.get(newPath) || 0;
  },
  { immediate: true } // 初始化时执行一次
);

// 处理子组件的“下一步”事件：跳转到对应路由
const changeActive = () => {
  const currentStep = active.value;
  // 根据当前步骤，获取下一个路由
  const nextRoute = Array.from(routeStepMap.entries()).find(
    ([_, step]) => step === currentStep + 1
  )?.[0];

  if (nextRoute) {
    router.push(nextRoute); // 跳转到下一个路由（路由变化会自动更新 active）
  }
};
</script>

<style scoped lang="scss">
// 全局基础重置
* {
  box-sizing: border-box;
}

// 通用容器样式
.forget-container {
  padding: 80px 10vw;
  min-height: 100vh; // 修正原100vw错误，改为最小高度100视口高度
  height: 100%; // 自适应高度
  background-color: $bg-color;
  max-width: 100%;
  margin: 0 auto;
}

// 通用步骤条样式
.forget-steps {
  max-width: 80vw;
  margin: 0 auto 40px; // 步骤条居中，与主内容拉开间距
}

// 通用主内容样式
.forget-main {
  max-width: 800px;
  margin: 0 auto;
  width: 100%;
}

// ------------------------ 移动端适配 ------------------------
.mobile-container {
  padding: 40px 5vw !important; // 减少上下/左右内边距
  min-height: 100vh !important;
}

// 移动端步骤条
.mobile-steps {
  max-width: 95vw !important; // 占满屏幕宽度
  margin: 0 auto 20px !important; // 减少底部间距
  font-size: 14px !important; // 缩小字体

  // 移动端步骤条图标大小
  :deep(.el-step__icon) {
    width: 24px !important;
    height: 24px !important;
    font-size: 14px !important;
  }

  // 移动端步骤条文字
  :deep(.el-step__title) {
    font-size: 13px !important;
    margin-top: 5px !important;
  }
}

// 移动端主内容区
.mobile-main {
  max-width: 95vw !important; // 占满屏幕宽度
  padding: 0 10px !important;
}

// 兜底：小屏PC适配
@media (max-width: 1024px) and (min-width: 768px) {
  .forget-container {
    padding: 60px 8vw;
  }
  .forget-steps {
    max-width: 90vw;
  }
}
</style>