<template>
  <section class="user-container">
    <div class="user-info-card card">
      <img class="avatar" :src="avatarPath" alt="头像">
      <div class="user-info">
        <span class="eyebrow">YOUR SPACE</span>
        <p ref="nicknameRef" class="nickname">{{ nickname }}</p>
        <p class="meta">{{ age }} 岁 · ID {{ id }}</p>
      </div>
      <div class="btn-group">
        <button type="button" class="ee-btn ee-btn-primary btn" @click="editInfo">修改资料</button>
        <button type="button" class="ee-btn btn" @click="navigateTo({ name: 'question' })">开始测验</button>
        <button type="button" class="ee-btn btn danger" @click="loginoutDialogVisible = true">退出登录</button>
      </div>
    </div>
    <div class="section-heading">
      <h2>和自己的相处</h2>
      <p>这些记录帮助你看见最近的节奏，不是评判。</p>
    </div>
    <div class="user-data-card card">
      <div class="chart-item" v-for="item in chartData" :key="item.title">
        <pieChart v-if="item.chartType === 'pie'" :data="item.data" :height="chartItemHeight" :title="item.title" />
        <lineChart v-if="item.chartType === 'line'" :height="chartItemHeight" :XData="item.XData" :YData="item.YData" :title="item.title" />
        <barChart v-if="item.chartType === 'bar'" :height="chartItemHeight" :XData="item.XData" :YData="item.YData" :title="item.title" />
      </div>
      <div class="ee-empty">暂无数据</div>
      <div class="ee-skeleton"></div>
    </div>
  </section>

  <el-dialog v-model="dialogFormVisible" title="修改资料" width="min(500px, calc(100vw - 32px))">
    <form class="profile-form">
      <label class="ee-field" data-label="头像">
        <el-upload class="avatar-uploader" action="" :show-file-list="false" :on-success="handleAvatarSuccess" :before-upload="beforeAvatarUpload">
          <img v-if="form.avatarPath" :src="form.avatarPath" class="avatar" alt="头像预览" />
          <span class="ee-icon" aria-hidden="true"><Plus /></span>
        </el-upload>
      </label>
      <label class="ee-field" data-label="昵称"><input type="text" class="ee-input" autocomplete="off" v-model="form.nickname"></label>
      <label class="ee-field" data-label="年龄"><input type="number" class="ee-input" autocomplete="off" v-model="form.age"></label>
    </form>
    <template #footer>
      <button type="button" class="ee-btn" @click="dialogFormVisible = false">取消</button>
      <button type="button" class="ee-btn ee-btn-primary" @click="saveInfo">保存资料</button>
    </template>
  </el-dialog>

  <el-dialog v-model="loginoutDialogVisible" title="离开这里" width="min(420px, calc(100vw - 32px))">
    <p>确定要退出当前账号吗？</p>
    <template #footer>
      <button type="button" class="ee-btn" @click="loginoutDialogVisible = false">留下</button>
      <button type="button" class="ee-btn ee-btn-primary" @click="handleLogout">确认退出</button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import pieChart from "~/components/charts/pieChart.vue";
import lineChart from "~/components/charts/lineChart.vue";
import barChart from "~/components/charts/barChart.vue";
import type { ChartItem } from "~/types/charts/common";
import { ref, onMounted } from "vue";
import { get, post } from "~/composables/useApi";
import { useNotify } from "~/composables/useNotify";

const { success: notifySuccess, error: notifyError } = useNotify();

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
    notify('', '昵称格式错误，长度为2-12个字符，包含中英文（含繁体）、数字（全角/半角）、下划线、横线、中文间隔号', 'warning', 3000);
    return false;
  }
  form.value.age = +form.value.age;
  if (form.value.age < 0 || form.value.age > 130) {
    notify('', '年龄格式错误，应在0-130之间', 'warning', 3000);
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
    notify('', '头像上传成功', 'success', 3000);
  } catch (error: any) {
    notify('', '', 'error', 3000);
  }
};

const beforeAvatarUpload: UploadProps["beforeUpload"] = (rawFile) => {
  if (rawFile.size / 1024 / 1024 > 2) {
    notify('', 'Avatar picture size can not exceed 2MB!', 'error', 3000);
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
    notify('', '', 'error', 3000);
    return;
  }

  notify('', '信息修改成功！', 'success', 3000);
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
    notify('', '获取行为数据失败: ', 'warning', 3000);
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
    
    notify('', '已退出登录', 'success', 3000);
    
    navigateTo("/login");
  } catch (error) {
    notify('', '退出登录失败', 'error', 3000);
  }
  loginoutDialogVisible.value = false;
};
onMounted(() => {
  chartItemHeight.value = vhToPx(40);
  fetchBehaviorData();
});
</script>

<style scoped lang="scss">
.user-container { width: min(1000px, 100%); margin: 0 auto; }
.card { background: var(--ee-surface); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-lg); }
.user-info-card { display: flex; align-items: center; gap: 24px; padding: clamp(20px, 4vw, 38px); }
.avatar { width: 88px; height: 88px; border: 3px solid var(--ee-primary-soft); }
.user-info { flex: 1; min-width: 0; }
.eyebrow { color: var(--ee-primary); font-size: 10px; font-weight: 700; letter-spacing: .16em; }
.nickname { margin-top: 5px; font-size: clamp(22px, 3vw, 30px); font-weight: 600; letter-spacing: -.04em; }
.meta { margin-top: 5px; color: var(--ee-text-muted); font-size: 13px; }
.btn-group { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 8px; }
.btn { min-height: 38px; border-radius: var(--ee-radius-md); }
.btn.danger { color: var(--ee-accent); border-color: color-mix(in srgb, var(--ee-accent) 35%, var(--ee-border)); }
.section-heading { margin: 34px 0 14px; }
.section-heading h2 { font-size: 20px; letter-spacing: -.03em; }
.section-heading p { margin-top: 4px; color: var(--ee-text-muted); font-size: 13px; }
.user-data-card { display: grid; min-height: 240px; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 14px; padding: 16px; }
.chart-item { min-height: 180px; padding: 10px; background: var(--ee-surface-muted); border: 1px solid var(--ee-border); border-radius: var(--ee-radius-md); }
.profile-form { padding: 10px 20px; }
.avatar-uploader { width: 88px; height: 88px; }
.avatar-uploader .avatar { display: block; width: 88px; height: 88px; object-fit: cover; }
.avatar-uploader-icon { display: grid; width: 88px; height: 88px; place-items: center; color: var(--ee-text-muted); border: 1px dashed var(--ee-border); border-radius: var(--ee-radius-md); font-size: 22px; }
@media (max-width: 700px) { .user-info-card { align-items: flex-start; flex-wrap: wrap; } .user-info { flex-basis: calc(100% - 120px); } .btn-group { width: 100%; justify-content: flex-start; } .btn { flex: 1; } .user-data-card { grid-template-columns: 1fr; } }
</style>
