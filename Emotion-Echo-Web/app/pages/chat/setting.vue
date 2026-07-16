<template>
  <div class="setting-container">
    <div class="setting-item font-size-edit">
      <span :style="{ fontSize: userConfig.fontSize }" class="content"
        >测试字体</span
      >
      <div style="align-items: center;">
        <span class="setting-title">选择字体大小</span>
        <el-dropdown
          v-model="userConfig.fontSize"
          @command="handleFontSizeChange"
        >
          <span class="el-dropdown-link">
            {{ fontSizeLabel }}
            <el-icon class="el-icon--right">
              <arrow-down />
            </el-icon>
          </span>
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item command="small">14px (小)</el-dropdown-item>
              <el-dropdown-item command="medium">16px (中)</el-dropdown-item>
              <el-dropdown-item command="large">18px (大)</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
      </div>
    </div>
    <div class="setting-item theme-edit">
      <span class="setting-title">选择主题</span>
      <el-radio-group v-model="userConfig.theme" @change="handleThemeChange">
        <el-radio-button label="light">
          <el-icon><Sunny /></el-icon> 浅色
        </el-radio-button>
        <el-radio-button label="dark">
          <el-icon><Moon /></el-icon> 深色
        </el-radio-button>
        <el-radio-button label="auto">
          <el-icon><Monitor /></el-icon> 跟随系统
        </el-radio-button>
      </el-radio-group>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ArrowDown, Sunny, Moon, Monitor } from "@element-plus/icons-vue";
import type { themeType } from "~/types/userConfig/userConfigType";
const userStore = useUserStore();
const userConfig = ref(userStore.getUserConfig());
const fontSizeMap: Record<string, string> = {
  small: "14px (小)",
  medium: "16px (中)",
  large: "18px (大)",
};
const fontSizeLabel = computed(() => fontSizeMap[userConfig.value.fontSize] || userConfig.value.fontSize);
const handleFontSizeChange = (fontSize: "small" | "medium" | "large") => {
  userConfig.value.fontSize = fontSize;
  userStore.setFontSize(fontSize);
};
const handleThemeChange = (theme: any) => {
  const t = theme as themeType;
  userConfig.value.theme = t;
  userStore.setTheme(t);
};
</script>

<style scoped lang="scss">
.setting-container {
  background-color: #fff;
  border-radius: 12px;
  padding: 20px;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.08);
  width: 100%;
  box-sizing: border-box;

  .setting-item {
    display: flex;
    align-items: center;
    background-color: #f5f7fa;
    border-radius: 12px;
    padding: 10px;
    margin-bottom: 16px;
    transition: background-color 0.2s ease;

    // 鼠标悬浮轻微变色
    &:hover {
      background-color: #f0f2f5;
    }

    // 最后一个项去掉底部间距
    &:last-child {
      margin-bottom: 0;
    }
  }

  .font-size-edit {
    justify-content: space-between;
    flex-wrap: wrap; // 响应式换行，避免小屏幕溢出
    gap: 16px; // 弹性布局间距，替代margin

    .setting-title {
      font-size: 14px;
      color: #303133;
      font-weight: 500; // 轻微加粗，突出标题
      margin-right: 12px;
      white-space: nowrap; // 防止标题换行
    }

    .content {
      background-color: #07c160;
      color: #fff;
      // 统一圆角，更美观
      border-radius: 12px;
      display: inline-block;
      margin: 0; // 去掉默认margin，用父级gap控制
      padding: 10px 12px; // 优化内边距，更舒适
      line-height: 1.6; // 优化行高，提升可读性
      max-width: 60%; // 调整宽度，避免占比过大
      word-wrap: break-word;
      word-break: break-all;
      box-sizing: border-box;
      box-shadow: 0 2px 8px rgba(7, 193, 96, 0.2); // 轻微阴影，突出测试文本
    }

    // 下拉菜单触发区样式优化
    .el-dropdown-link {
      display: inline-flex;
      align-items: center;
      color: #409eff; // Element Plus 主色，突出可点击
      cursor: pointer;
      padding: 4px 8px;
      border-radius: 4px;
      transition: background-color 0.2s ease;

      // 悬浮高亮
      &:hover {
        background-color: rgba(64, 158, 255, 0.1);
      }

      // 图标间距优化
      .el-icon {
        margin-left: 4px;
        font-size: 12px;
      }
    }
  }

  .theme-edit {
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 16px;

    .setting-title {
      font-size: 14px;
      color: #303133;
      font-weight: 500;
      margin-right: 12px;
      white-space: nowrap;
    }

    .el-radio-group {
      .el-radio-button__inner {
        display: inline-flex;
        align-items: center;
        gap: 4px;
      }
    }
  }
}

// 响应式适配：小屏幕下测试文本占比提升
@media (max-width: 768px) {
  .setting-container .font-size-edit .content {
    max-width: 100%;
    margin-bottom: 8px;
  }
}
</style>
