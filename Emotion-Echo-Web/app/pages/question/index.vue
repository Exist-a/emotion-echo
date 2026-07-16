<template>
  <div class="question-container">
    <div class="bar">
      <el-button type="primary" @click="navigateTo({ name: 'chat-user' })"
        >返回用户页面</el-button
      >
    </div>
    <h1 class="title">心理测验量表</h1>
    <p class="description">我们将根据您的答题情况分析您的心理状况</p>

    <!-- 加载状态 -->
    <el-skeleton v-if="isLoading" :rows="5" animated />

    <!-- 空状态 -->
    <el-empty v-else-if="tableData.length === 0" description="暂无量表数据" />

    <el-table
      v-else
      :data="tableData"
      class="table"
      :row-class-name="tableRowClassName"
      :fit="true"
      border
    >
      <!-- 量表名称列：设置最小宽度+自适应 -->
      <el-table-column
        prop="title"
        label="量表名称"
        min-width="200"
        flex="2"
      />
      <!-- 预计用时列 -->
      <el-table-column
        prop="estimatedTime"
        label="预计用时"
        min-width="120"
        flex="1"
      />
      <!-- 是否完成列：最小宽度+自适应 -->
      <el-table-column
        prop="statusText"
        label="是否完成"
        min-width="120"
        flex="1"
      />
      <!-- 操作列：固定右侧，设置宽度，优化对齐 -->
      <el-table-column fixed="right" label="操作" width="180" align="center">
        <template #default="scope">
          <el-button
            link
            type="primary"
            size="small"
            @click="doQuestion(scope.row)"
          >
            {{ scope.row.status === "completed" ? "重新做题" : "去做题" }}
          </el-button>
          <el-button
            link
            type="primary"
            size="small"
            @click="checkRes(scope.row)"
            :disabled="scope.row.status !== 'completed'"
          >
            查看结果
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>

  <!-- 查看结果弹窗 -->
  <el-dialog v-model="dialogVisible" title="测试结果" width="500">
    <div v-if="currentResult" class="result-content">
      <p><strong>总分：</strong>{{ currentResult.totalScore }}</p>
      <p><strong>等级：</strong>{{ currentResult.level }}</p>
      <p><strong>建议：</strong>{{ currentResult.suggestion }}</p>
    </div>
    <div v-else>加载结果中...</div>
    <template #footer>
      <div class="dialog-footer">
        <el-button type="primary" @click="dialogVisible = false">
          确认
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { SurveyItem, SurveyResult } from "~/types/api";
import { get } from "~/composables/useApi";

definePageMeta({
  layout: "default",
});

const dialogVisible = ref(false);
const currentResult = ref<SurveyResult | null>(null);
const isLoading = ref(true);

interface TableRow extends SurveyItem {
  statusText: string;
}

const tableData = ref<TableRow[]>([]);

const fetchSurveys = async () => {
  isLoading.value = true;
  try {
    const data = await get<{ list: SurveyItem[] }>("/surveys");
    tableData.value = data.list.map((item) => ({
      ...item,
      statusText: item.status === "completed" ? "已完成" : "未开始",
    }));
  } catch (error: any) {
    ElNotification({
      type: "error",
      message: error.message || "获取量表列表失败",
    });
  } finally {
    isLoading.value = false;
  }
};

onMounted(() => {
  fetchSurveys();
});

const tableRowClassName = ({
  row,
  rowIndex,
}: {
  row: TableRow;
  rowIndex: number;
}) => {
  if (row.status === "not_started") {
    return "warning-row";
  } else if (row.status === "completed") {
    return "success-row";
  }
  return "";
};

const doQuestion = (data: TableRow) => {
  navigateTo({ name: "question-detail", params: { id: data.id } });
};

const checkRes = async (data: TableRow) => {
  if (!data.resultId) return;
  dialogVisible.value = true;
  currentResult.value = null;
  try {
    const result = await get<SurveyResult>(`/surveys/result/${data.resultId}`);
    currentResult.value = result;
  } catch (error: any) {
    ElNotification({
      type: "error",
      message: error.message || "获取结果失败",
    });
  }
};
</script>

<style scoped lang="scss">
.question-container {
  margin: 0 auto; // 整体居中

  .bar {
    padding: 10px;
    background-color: #fff;
    border-radius: 4px;
    box-shadow: 0 2px 12px 0 rgba(0, 0, 0, 0.04);
  }

  .title {
    text-align: center;
    margin-top: 60px; // 优化margin写法，避免上下左右都40px
    font-size: 40px;
    color: #303133;
    text-shadow: $box-shadow;
  }
  .description {
    text-align: center;
    margin-bottom: 60px;
    font-size: 14px;
    text-shadow: $box-shadow;
    color: #606266;
  }
  .table {
    width: 95%; // 改为100%，外层max-width控制整体宽度
    margin: 30px auto;
    border-radius: $radius-lg;
    box-shadow: $box-shadow;
    --el-table-header-text-color: #606266;
    --el-table-row-hover-bg-color: #f5f7fa;
  }
}

.result-content {
  p {
    margin-bottom: 12px;
    line-height: 1.6;
    color: #303133;
  }
}

// 关键：用:deep()穿透scoped，确保行样式生效
:deep(.el-table .warning-row) {
  --el-table-tr-bg-color: var(--el-color-warning-light-9);
}
:deep(.el-table .success-row) {
  --el-table-tr-bg-color: var(--el-color-success-light-9);
}

// 响应式优化：小屏适配
@media (max-width: 768px) {
  .question-container {
    .title {
      margin: 20px 0;
      font-size: 20px;
    }

    .table {
      --el-table-font-size: 14px;
    }

    // 小屏隐藏固定列，改为普通列（避免挤压）
    :deep(.el-table-column--fixed-right) {
      position: static !important;
      width: 100% !important;
    }
  }
}
</style>
