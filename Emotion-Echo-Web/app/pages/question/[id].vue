<template>
  <el-button
    @click="goBackDialogVisible = true"
    style="position: fixed; left: 5px; top: 5px"
    type="primary"
    >返回题目列表</el-button
  >
  <!-- 加载占位：避免数据未加载时白屏/报错 -->
  <div v-if="isLoading" class="loading-container">
    <el-skeleton :rows="5" animated />
  </div>

  <!-- 空状态 -->
  <el-empty
    v-else-if="!survey.id"
    description="量表不存在或已下架"
    style="margin-top: 80px"
  />

  <!-- 答题页面：数据加载完成后显示 -->
  <div v-else class="question-container">
    <h1 class="title">{{ survey.title }}</h1>
    <p class="description" v-if="survey.description">
      {{ survey.description }}
    </p>
    <!-- 遍历题目，绑定每个题目的选中值 -->
    <div
      v-for="question in survey.questions"
      :key="question.id"
      class="question-block"
    >
      <h3 class="question-text">{{ question.id }}. {{ question.title }}</h3>
      <!-- 核心：绑定v-model收集每个题目的选中值 -->
      <el-radio-group v-model="answerMap[question.id]" class="radio-group">
        <el-radio
          v-for="option in question.options"
          :key="option.id"
          :value="option.id"
          class="radio-option"
        >
          {{ option.text }}
        </el-radio>
      </el-radio-group>
    </div>

    <!-- 提交按钮：收集所有答题数据 -->
    <div class="submit-btn-wrap">
      <el-button
        type="primary"
        size="large"
        @click="handleSubmit"
        :disabled="!hasAnsweredAll"
        :loading="isSubmitting"
      >
        提交答案
      </el-button>
    </div>
  </div>

  <el-dialog v-model="goBackDialogVisible" title="提示" width="500">
    <span>是否返回？答题情况不会保存</span>
    <template #footer>
      <div class="dialog-footer">
        <el-button @click="goBackDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="navigateTo({ name: 'question' })">
          确认
        </el-button>
      </div>
    </template>
  </el-dialog>

  <!-- 提交结果弹窗 -->
  <el-dialog v-model="resultDialogVisible" title="测试结果" width="500">
    <div v-if="submitResult" class="result-content">
      <p><strong>总分：</strong>{{ submitResult.totalScore }}</p>
      <p><strong>等级：</strong>{{ submitResult.level }}</p>
      <p><strong>建议：</strong>{{ submitResult.suggestion }}</p>
    </div>
    <template #footer>
      <div class="dialog-footer">
        <el-button type="primary" @click="handleResultConfirm">
          确认
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { SurveyDetail, SurveyResult } from "~/types/api";
import { get, post } from "~/composables/useApi";

const route = useRoute();
const isLoading = ref(true);
const isSubmitting = ref(false);
const survey = ref<SurveyDetail>({
  id: 0,
  title: "",
  description: "",
  estimatedTime: "",
  questions: [],
});
const goBackDialogVisible = ref(false);
const resultDialogVisible = ref(false);
const submitResult = ref<SurveyResult | null>(null);

// 收集每个题目的选中值
const answerMap = ref<Record<number, number>>({});

// 根据路由id获取题目详情
const getQuestionDetail = async (id: string | number) => {
  isLoading.value = true;
  try {
    const data = await get<SurveyDetail>(`/surveys/${id}`);
    survey.value = data;
    // 初始化答题映射
    answerMap.value = {};
    data.questions.forEach((q) => {
      answerMap.value[q.id] = 0;
    });
  } catch (err: any) {
    ElMessage.error(err.message || "获取题目失败");
  } finally {
    isLoading.value = false;
  }
};

// 计算属性：判断是否所有题目都已作答（控制提交按钮禁用）
const hasAnsweredAll = computed(() => {
  const questions = survey.value.questions;
  if (questions.length === 0) return false;
  return questions.every((q) => answerMap.value[q.id]! > 0);
});

// 提交答题数据逻辑
const handleSubmit = async () => {
  if (!hasAnsweredAll.value) return;

  const answers = Object.entries(answerMap.value)
    .filter(([_, optionId]) => optionId > 0)
    .map(([questionId, optionId]) => ({
      questionId: Number(questionId),
      optionId,
    }));

  isSubmitting.value = true;
  try {
    const result = await post<SurveyResult>(
      `/surveys/${survey.value.id}/submit`,
      { answers }
    );
    submitResult.value = result;
    resultDialogVisible.value = true;
    ElMessage.success("答题提交成功！");
  } catch (err: any) {
    ElMessage.error(err.message || "提交失败");
  } finally {
    isSubmitting.value = false;
  }
};

const handleResultConfirm = () => {
  resultDialogVisible.value = false;
  navigateTo({ name: "question" });
};

// 页面挂载时获取题目详情
onMounted(() => {
  const questionId = route.params.id as string | number;
  if (questionId) {
    getQuestionDetail(questionId);
  } else {
    ElMessage.warning("缺少题目ID");
    isLoading.value = false;
  }
});
</script>

<style scoped lang="scss">
.question-container {
  width: 70%;
  margin: 0 auto;
  background-color: #fff;
  padding: 40px;
  min-height: 80vh;
  border-radius: $radius-lg;
  box-shadow: $box-shadow;
  .title {
    text-align: center;
    margin-bottom: 40px;
    font-size: 30px;
    color: #303133;
  }
  .description {
    text-align: center;
    margin-bottom: 30px;
    font-size: 16px;
    color: #909399;
  }
  .question-block {
    margin-bottom: 30px;
    padding-bottom: 20px;
    border-bottom: 1px solid #e6e6e6;

    .question-text {
      margin-bottom: 15px;
      font-size: 18px;
      color: #606266;
    }

    .radio-group {
      display: flex; // 选项横向排列，更美观
      gap: 20px; // 选项间距
      flex-wrap: wrap; // 小屏自动换行

      .radio-option {
        margin: 0 !important; // 重置Element默认间距
        font-size: 16px;
      }
    }
  }

  .submit-btn-wrap {
    text-align: center;
    margin-top: 40px;
  }
}

.result-content {
  p {
    margin-bottom: 12px;
    line-height: 1.6;
    color: #303133;
  }
}

.loading-container {
  width: 70%;
  margin: 40px auto;
  padding: 20px;
}
</style>
