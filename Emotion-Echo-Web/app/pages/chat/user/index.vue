<template>
  <div class="user-container">
    <div class="user-info-card card">
      <el-avatar :src="avatarPath" class="avatar" />
      <div class="user-info">
        <p ref="nicknameRef" class="nickname">
          {{ nickname }}
        </p>
        <p class="age">年龄:{{ age }}</p>
        <p class="id">ID:{{ id }}</p>
      </div>
      <div class="btn-group">
        <el-button type="primary" class="btn" @click="editInfo"
          >修改信息</el-button
        >
        <br />
        <el-button
          type="danger"
          class="btn"
          @click="loginoutDialogVisible = true"
          >退出登录</el-button
        >
        <br />
        <el-button
          type="success"
          class="btn"
          @click="navigateTo({ name: 'question'})"
          >进行心理测验</el-button
        >
      </div>
    </div>
    <div class="user-data-card card">
      <div class="chart-item" v-for="item in chartData" :key="item.title">
        <pieChart
          v-if="item.chartType === 'pie'"
          :data="item.data"
          :height="chartItemHeight"
          :title="item.title"
        />
        <lineChart
          v-if="item.chartType === 'line'"
          :height="chartItemHeight"
          :XData="item.XData"
          :YData="item.YData"
          :title="item.title"
        />
        <barChart
          v-if="item.chartType === 'bar'"
          :height="chartItemHeight"
          :XData="item.XData"
          :YData="item.YData"
          :title="item.title"
        />
      </div>
    </div>
  </div>

  <el-dialog v-model="dialogFormVisible" title="修改信息" width="500">
    <el-form :model="form" style="padding: 20px">
      <el-form-item label="头像">
        <el-upload
          class="avatar-uploader"
          action=""
          :show-file-list="false"
          :on-success="handleAvatarSuccess"
          :before-upload="beforeAvatarUpload"
          style="width: 100px"
        >
          <img v-if="form.avatarPath" :src="form.avatarPath" class="avatar" />
          <el-icon v-else class="avatar-uploader-icon"><Plus /></el-icon>
        </el-upload>
      </el-form-item>
      <el-form-item label="昵称">
        <el-input v-model="form.nickname" autocomplete="off" />
      </el-form-item>
      <el-form-item label="年龄">
        <el-input v-model="form.age" autocomplete="off" type="number" />
      </el-form-item>
    </el-form>
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="dialogFormVisible = false">取消</el-button>
        <el-button type="primary" @click="saveInfo"> 确认 </el-button>
      </div>
    </template>
  </el-dialog>

  <el-dialog v-model="loginoutDialogVisible" title="提示" width="500">
    <span>是否退出登录？</span>
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="loginoutDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleLogout"> 确认 </el-button>
      </div>
    </template>
  </el-dialog>
  <NuxtPage />
</template>

<script setup lang="ts">
import pieChart from "~/components/charts/pieChart.vue";
import lineChart from "~/components/charts/lineChart.vue";
import barChart from "~/components/charts/barChart.vue";
import type { ChartItem } from "~/types/charts/common";
import type { UploadProps } from "element-plus";
import { Plus } from "@element-plus/icons-vue";
import { ref, onMounted } from "vue";
import { ElNotification, ElMessage } from "element-plus";
import { get, post } from "~/composables/useApi";

const userStore = useUserStore();
const nickname = userStore.getNickname;       // computed ref
const avatarPath = userStore.getAvatarPath;   // computed ref
const age = userStore.getAge;                 // computed ref
const id = userStore.getId;                   // computed ref
const dialogFormVisible = ref(false);
const chartItemHeight = ref(0);
const loginoutDialogVisible = ref(false);

const form = ref<{
  nickname: string;
  avatarPath: string;
  age: number;
}>({
  nickname: nickname as unknown as string,
  avatarPath: avatarPath as unknown as string,
  age: age as unknown as number,
});

const validateInfo = () => {
  form.value.nickname = form.value.nickname.trim();
  if (!nicknameReg.test(form.value.nickname)) {
    ElNotification({
      message:
        "昵称格式错误，长度为2-12个字符，包含中英文（含繁体）、数字（全角/半角）、下划线、横线、中文间隔号",
      type: "warning",
    });
    return false;
  }
  form.value.age = +form.value.age;
  if (form.value.age < 0 || form.value.age > 130) {
    ElNotification({
      message: "年龄格式错误，应在0-130之间",
      type: "warning",
    });
    return false;
  }
  return true;
};

const handleAvatarSuccess: UploadProps["onSuccess"] = (
  response,
  uploadFile
) => {
  // TODO: 实际上传头像到服务器
  // 目前使用本地预览URL
  form.value.avatarPath = URL.createObjectURL(uploadFile.raw!);
  handleUploadAvatar(uploadFile.raw!);
};

/**
 * 上传头像到服务器
 * @param file 头像文件
 */
const handleUploadAvatar = async (file: File) => {
  try {
    const formData = new FormData();
    formData.append("avatar", file);
    const res = await post<{ avatar: string }>("/user/avatar", formData);
    // 更新本地头像显示
    form.value.avatarPath = res.avatar;
    // 同步更新 store
    if (userStore.userInfo) {
      userStore.userInfo.avatar = res.avatar;
    }
    ElNotification({
      type: "success",
      message: "头像上传成功",
    });
  } catch (error: any) {
    ElNotification({
      type: "error",
      message: error.message || "头像上传失败",
    });
  }
};

const beforeAvatarUpload: UploadProps["beforeUpload"] = (rawFile) => {
  if (rawFile.size / 1024 / 1024 > 2) {
    ElMessage.error("Avatar picture size can not exceed 2MB!");
    return false;
  }
  return true;
};

const editInfo = () => {
  dialogFormVisible.value = true;
};

const saveInfo = async () => {
  if (!validateInfo()) return;

  const [nickRes, ageRes] = await Promise.all([
    userStore.editNickname(form.value.nickname),
    userStore.editAge(form.value.age),
  ]);

  if (!nickRes.isOk || !ageRes.isOk) {
    ElNotification({
      message: nickRes.msg || ageRes.msg || "信息修改失败",
      type: "error",
    });
    return;
  }

  ElNotification({
    message: "信息修改成功！",
    type: "success",
  });
  dialogFormVisible.value = false;
};

// 用户行为数据
const behaviorData = ref({
  dayNight: null as any,
  depth: null as any,
  frequency: null as any,
});
const isLoadingBehavior = ref(false);

// 获取用户行为数据
const fetchBehaviorData = async () => {
  isLoadingBehavior.value = true;
  try {
    const [dayNight, depth, frequency] = await Promise.all([
      get("/user-behavior/day-night"),
      get("/user-behavior/depth"),
      get("/user-behavior/frequency"),
    ]);
    behaviorData.value.dayNight = dayNight;
    behaviorData.value.depth = depth;
    behaviorData.value.frequency = frequency;
  } catch (error: any) {
    ElNotification({
      type: "warning",
      message: "获取行为数据失败: " + (error.message || "未知错误"),
    });
  } finally {
    isLoadingBehavior.value = false;
  }
};

// 动态构建图表数据
const chartData = computed<ChartItem[]>(() => {
  const items: ChartItem[] = [];

  // 1. 昼夜使用模式（饼图）
  if (behaviorData.value.dayNight?.periods?.length > 0) {
    items.push({
      chartType: "pie",
      title: "昼夜使用模式",
      data: behaviorData.value.dayNight.periods.map((p: any) => ({
        name: `${p.label} (${p.hours})`,
        value: p.value,
      })),
    });
  }

  // 2. 对话频次趋势（折线图）
  if (behaviorData.value.frequency?.dates?.length > 0) {
    items.push({
      chartType: "line",
      title: "近30天对话频次",
      XData: behaviorData.value.frequency.dates,
      YData: behaviorData.value.frequency.messageCount,
    });
  }

  // 3. 互动深度（柱状图）
  if (behaviorData.value.depth) {
    items.push({
      chartType: "bar",
      title: "互动深度指标",
      XData: ["平均轮数", "最长连续(天)", "总会话", "总消息", "日均消息"],
      YData: [
        Math.round(behaviorData.value.depth.avgSessionRounds || 0),
        behaviorData.value.depth.maxConsecutiveDays || 0,
        behaviorData.value.depth.totalConversations || 0,
        behaviorData.value.depth.totalMessages || 0,
        Math.round(behaviorData.value.depth.avgMessagesPerDay || 0),
      ],
    });
  }

  return items;
});

//确认退出登录
const handleLogout = async () => {
  try {
    await userStore.logout();
    
    ElNotification({
      message: "已退出登录",
      type: "success",
    });
    
    navigateTo("/login");
  } catch (error) {
    ElNotification({
      message: "退出登录失败",
      type: "error",
    });
  }
  loginoutDialogVisible.value = false;
};
onMounted(() => {
  chartItemHeight.value = vhToPx(40);
  fetchBehaviorData();
});
</script>

<style scoped lang="scss">
.user-container {
  padding: 0 10px; // 全局容器加左右内边距，避免窄屏内容贴边

  .card {
    width: 100%;
    background-color: #fff;
    box-shadow: $box-shadow;
    border-radius: $radius-lg;
    margin-bottom: 30px;
    box-sizing: border-box;
  }

  .user-info-card {
    display: flex;
    flex-direction: column; // 小屏默认纵向排列
    align-items: center; // 小屏内容居中
    padding: 24px 16px; // 小屏减少内边距
    gap: 20px; // 纵向间距，避免元素挤在一起

    // 大屏断点（≥768px）：横向排列
    @media (min-width: 768px) {
      flex-direction: row; // 横向排列
      justify-content: space-between; // 两端分布
      align-items: center;
      padding: 40px; // 恢复大屏内边距
      gap: 30px; // 横向间距
    }

    // 头像响应式
    .avatar {
      border: #4983df 2px solid;
      cursor: pointer;
      transition: all 0.3s ease;
      width: 8em; // 小屏头像缩小
      height: 8em;

      // 大屏头像恢复原大小
      @media (min-width: 768px) {
        width: 10em;
        height: 10em;
      }

      &:hover {
        transform: scale(1.05);
      }
    }

    // 用户信息块
    .user-info {
      text-align: center; // 小屏文字居中
      @media (min-width: 768px) {
        text-align: left; // 大屏文字左对齐
        flex: 1; // 占满中间空间，让按钮组靠右
      }

      .nickname {
        font-size: 20px; // 小屏字号缩小
        margin: 10px auto;
        @media (min-width: 768px) {
          font-size: 24px;
          margin: 15px auto;
        }
      }

      .age,
      .id {
        font-size: 14px; // 小屏字号缩小
        color: $font-color-light;
        margin: 10px auto;
        @media (min-width: 768px) {
          font-size: 16px;
          margin: 15px auto;
        }
      }
    }

    // 按钮组响应式
    .btn-group {
      .btn {
        width: 120px; // 小屏按钮加宽，更易点击
        height: 40px;
        margin-top: 10px;
        @media (min-width: 768px) {
          width: 100px; // 大屏恢复原宽度
        }
      }
    }
  }

  // -------------------------- 原有图表容器样式（不变） --------------------------
  .user-data-card {
    width: 100%;
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
    gap: 20px;
    align-items: stretch;
    justify-items: stretch;
    overflow-y: auto;
    padding: 16px;
    min-height: 40vh; // 替换固定height，避免窄屏高度不足

    .chart-item {
      height: 100%;
      min-height: 150px;
      background: #f8f9fa;
      border-radius: $radius-lg;
      padding: 10px;
      box-sizing: border-box;
    }
  }
}
</style>
