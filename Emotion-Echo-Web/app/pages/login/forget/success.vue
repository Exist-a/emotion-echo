<template>
  <!-- 加动态类区分移动端 -->
  <div
    class="success-container"
    :class="{ 'mobile-success-container': $device.isMobile }"
  >
    <el-result
      icon="success"
      title="修改成功!"
      sub-title="快去登陆吧!"
      :class="{ 'mobile-result': $device.isMobile }"
    >
      <template #extra>
        <el-button type="primary" @click="gotoLogin">去登陆页</el-button>
      </template>
    </el-result>
  </div>
</template>

<script setup lang="ts">
import { useForgetPwdState } from "~/composables/forgetPwdState"; // 请根据实际路径调整

definePageMeta({
  middleware: "forget-pwd",
});
const gotoLogin = () => {
  const { resetState } = useForgetPwdState();
  resetState();
  navigateTo("/login");
};
</script>

<style scoped lang="scss">
// PC端样式（原有样式保留）
.success-container {
  padding: 50px;
  width: 50vw;
  margin: 100px auto;
  background-color: #fff;
  border-radius: $radius-lg;
  box-shadow: $box-shadow;
  text-align: center;
}

// 移动端适配样式
.mobile-success-container {
  width: 100% !important; // 移动端占90%宽度
  margin: 50px auto !important; // 减少上下间距
  padding: 30px 20px !important; // 减少内边距
  box-sizing: border-box;
}

// 移动端Result组件样式调整
.mobile-result {
  :deep(.el-result__title) {
    font-size: 20px !important; // 缩小标题
  }
  :deep(.el-result__sub-title) {
    font-size: 14px !important; // 缩小副标题
    margin-top: 10px !important;
  }
  :deep(.el-button) {
    height: 44px !important; // 适配移动端触摸
    padding: 0 20px !important;
    font-size: 16px !important;
    border-radius: $radius-mid !important;
  }
}

// 平板端兜底
@media (max-width: 1024px) and (min-width: 768px) {
  .success-container {
    width: 70vw !important;
    padding: 40px 30px !important;
  }
}
</style>
