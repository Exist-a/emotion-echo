# Emotion-Echo 前端设计文档

> 版本：v1.0
> 日期：2026-05-02
> 用途：多 Agent AI 系统对接指南

---

## 一、系统架构

### 1.1 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 框架 | Vue 3 + Nuxt 4 | SSR/CSR 同构框架 |
| UI 库 | Element Plus | 组件库 |
| 状态管理 | Pinia | 全局状态管理 |
| 图表 | ECharts + vue-echarts | 数据可视化 |
| 语音 | Web Speech API | 语音输入（预留） |
| 本地存储 | IndexedDB (Dexie) | 离线消息缓存 |
| 构建工具 | Vite | 开发构建 |

### 1.2 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         前端应用层                               │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                     Pages（页面）                        │  │
│  │  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐           │  │
│  │  │  Login │ │  Chat  │ │Dashboard│ │Survey │           │  │
│  │  └────────┘ └────────┘ └────────┘ └────────┘           │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                  Components（组件）                       │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐               │  │
│  │  │ NavBar   │ │ Charts   │ │ Reports  │               │  │
│  │  └──────────┘ └──────────┘ └──────────┘               │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                  Composables（可组合函数）               │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐               │  │
│  │  │ useApi   │ │useAIStream│ │ usePrompt │               │  │
│  │  └──────────┘ └──────────┘ └──────────┘               │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                    Stores（状态管理）                    │  │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐               │  │
│  │  │   User   │ │Conversation│ │ Message  │               │  │
│  │  └──────────┘ └──────────┘ └──────────┘               │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                        API 对接层                               │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                   useApi（请求封装）                     │  │
│  │  • 自动附加 AccessToken                                  │  │
│  │  • 401 自动刷新 Token                                    │  │
│  │  • 429 限流重试                                          │  │
│  │  • 错误统一处理                                          │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐  │
│  │                  useAIStream（流式对话）                  │  │
│  │  • SSE 流式请求                                          │  │
│  │  • 打字机效果                                            │  │
│  │  • 请求中断                                              │  │
│  └─────────────────────────────────────────────────────────┘  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                               │
                               ▼
┌─────────────────────────────────────────────────────────────────┐
│                      后端 API 服务                              │
│                                                                 │
│   Auth │ User │ Conversation │ Message │ AI │ Survey │ Report  │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### 1.3 页面结构

```
/                           # 首页（重定向到 /chat）
│
├── /login                  # 登录模块
│   ├── /login/index        # 登录页
│   ├── /login/forget       # 忘记密码
│   │   ├── /forget/verify  # 验证身份
│   │   ├── /forget/modify  # 修改密码
│   │   └── /forget/success # 成功
│
├── /chat                   # 聊天模块（需登录）
│   ├── /chat/conversation  # 会话列表
│   │   ├── /chat/conversation/new    # 新建会话
│   │   └── /chat/conversation/:id    # 会话详情
│   ├── /chat/user          # 用户中心
│   ├── /chat/setting       # 设置页
│   └── /chat/dashboard     # 数据报表
│       ├── /chat/dashboard/dailyReport   # 日报
│       ├── /chat/dashboard/weeklyReport  # 周报
│       ├── /chat/dashboard/monthlyReport # 月报
│       └── /chat/dashboard/annualReport  # 年报
│
└── /question               # 心理测验
    ├── /question/index     # 量表列表
    └── /question/:id      # 量表详情/答题页
```

### 1.4 项目目录结构

```
Emotion-Echo-Web/
├── app/
│   ├── pages/                    # 页面
│   │   ├── chat/
│   │   │   ├── conversation/
│   │   │   │   ├── [id].vue      # 会话详情页
│   │   │   │   ├── index.vue     # 会话列表
│   │   │   │   └── new.vue       # 新建会话
│   │   │   ├── dashboard/
│   │   │   │   ├── index.vue     # 报表首页
│   │   │   │   ├── dailyReport.vue
│   │   │   │   ├── weeklyReport.vue
│   │   │   │   ├── monthlyReport.vue
│   │   │   │   └── annualReport.vue
│   │   │   ├── user/index.vue    # 用户中心
│   │   │   ├── setting.vue       # 设置页
│   │   │   └── index.vue         # 聊天首页
│   │   ├── login/
│   │   │   ├── index.vue         # 登录页
│   │   │   └── forget/           # 忘记密码流程
│   │   ├── question/
│   │   │   ├── index.vue         # 量表列表
│   │   │   └── [id].vue          # 量表详情/答题
│   │   └── index.vue             # 首页
│   │
│   ├── components/               # 组件
│   │   ├── charts/              # 图表组件
│   │   │   ├── barChart.vue
│   │   │   ├── lineChart.vue
│   │   │   ├── pieChart.vue
│   │   │   └── radarChart.vue
│   │   └── report/
│   │       └── chartsCard.vue
│   │
│   ├── composables/              # 可组合函数
│   │   ├── useApi.ts             # API 请求封装
│   │   ├── useAIStream.ts         # AI 流式对话
│   │   ├── useConversationSender.ts  # 会话发送
│   │   ├── usePrompt.ts          # 提示词管理
│   │   └── verificationCodeCountDown.ts  # 验证码倒计时
│   │
│   ├── stores/                   # Pinia 状态管理
│   │   ├── user.ts               # 用户状态
│   │   ├── conversation.ts       # 会话状态
│   │   └── message.ts            # 消息状态
│   │
│   ├── types/                    # TypeScript 类型定义
│   │   ├── api.ts                # API 类型
│   │   ├── commonType.ts         # 通用类型
│   │   ├── userConfig/           # 用户配置类型
│   │   ├── conversation/         # 会话类型
│   │   ├── charts/               # 图表类型
│   │   ├── report/               # 报表类型
│   │   └── prompt/               # 提示词类型
│   │
│   ├── configs/                  # 配置
│   │   ├── userConfig/           # 用户配置
│   │   └── chartConfig/          # 图表配置
│   │
│   ├── layouts/                  # 布局
│   │   ├── default.vue
│   │   └── nav.vue
│   │
│   ├── middleware/               # 中间件
│   │   ├── auth.global.ts        # 全局鉴权
│   │   └── forgetPwd.ts
│   │
│   ├── plugins/                  # 插件
│   │   ├── init.ts               # 初始化
│   │   └── vueInject.ts          # 依赖注入
│   │
│   ├── assets/                   # 静态资源
│   │   ├── icons/                # SVG 图标
│   │   └── scss/                 # 样式
│   │
│   ├── utils/                    # 工具函数
│   │   ├── index.ts
│   │   ├── db.ts                 # IndexedDB
│   │   └── messageCache.ts       # 消息缓存
│   │
│   ├── router.options.ts         # 路由配置
│   └── app.vue                   # 根组件
│
├── public/                       # 公共资源
│   ├── fonts/                    # 字体
│   └── imgs/                     # 图片
│
├── docs/                         # 文档
│   └── DESIGN.md                 # 本文档
│
├── nuxt.config.ts               # Nuxt 配置
├── package.json
└── tsconfig.json
```

---

## 二、API 对接说明

### 2.1 请求封装 (useApi)

```typescript
// composables/useApi.ts

import type { ApiResponse } from '~/types/api';
import { useUserStore } from '~/stores/user';

const BASE_URL = 'http://localhost:8081/api/v1';

/**
 * 获取 AccessToken
 */
function getAccessToken(): string | null {
  const userStore = useUserStore();
  return userStore.getAccessToken || null;
}

/**
 * 刷新 Token
 */
async function refreshToken(): Promise<string | null> {
  // 实现见 2.3 Token 刷新流程
}

/**
 * 通用 GET 请求
 */
async function get<T = any>(
  endpoint: string,
  params?: Record<string, any>
): Promise<ApiResponse<T>> {
  const url = new URL(`${BASE_URL}${endpoint}`);
  if (params) {
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== null) {
        url.searchParams.append(k, String(v));
      }
    });
  }

  const token = getAccessToken();

  const res = await fetch(url.toString(), {
    headers: {
      'Authorization': token ? `Bearer ${token}` : '',
      'Content-Type': 'application/json',
    },
    credentials: 'include',
  });

  return handleResponse(res);
}

/**
 * 通用 POST 请求
 */
async function post<T = any>(
  endpoint: string,
  body?: Record<string, any>
): Promise<ApiResponse<T>> {
  const token = getAccessToken();

  const res = await fetch(`${BASE_URL}${endpoint}`, {
    method: 'POST',
    headers: {
      'Authorization': token ? `Bearer ${token}` : '',
      'Content-Type': 'application/json',
    },
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'include',
  });

  return handleResponse(res);
}
```

### 2.2 错误处理

```typescript
// types/api.ts

export interface ApiResponse<T = any> {
  code: number;
  message: string;
  data: T;
}

export class ApiError extends Error {
  code: number;

  constructor(code: number, message: string) {
    super(message);
    this.code = code;
    this.name = 'ApiError';
  }
}

// 错误码
export const ErrorCodes = {
  PARAM_ERROR: 10001,
  TOKEN_EXPIRED: 10002,
  TOKEN_INVALID: 10003,
  USER_NOT_EXIST: 20001,
  PASSWORD_ERROR: 20002,
  VERIFY_CODE_ERROR: 20003,
  USER_EXISTS: 20004,
  CODE_TOO_FREQUENT: 20005,
  CONVERSATION_NOT_EXIST: 30001,
  NOT_CONVERSATION_OWNER: 30002,
  AI_SERVICE_ERROR: 50001,
} as const;
```

### 2.3 Token 刷新流程

```
请求失败 (401)
    │
    ▼
┌─────────────────┐
│ 检查 refreshPromise │──▶ 等待中 ──▶ 返回 Promise
└────────┬────────┘
         │ 不存在
         ▼
┌─────────────────┐
│ 创建刷新请求     │
└────────┬────────┘
         │
         ▼
    ┌────────┐
    │ 调用    │
    │ POST    │
    │ /auth/  │
    │ refresh │
    └────┬───┘
         │
    ┌────┴────┐
    │         │
  成功      失败
    │         │
    ▼         ▼
 更新Token  清除登录
    │         │
    └────┬────┘
         │
         ▼
    重试原请求
```

---

## 三、AI 流式对话

### 3.1 核心 Hook (useAIStream)

```typescript
// composables/useAIStream.ts

import { ref, onUnmounted } from 'vue';

interface StreamEvent {
  type: 'start' | 'delta' | 'finish' | 'error';
  conversationId?: string;
  content?: string;
  messageId?: string;
  code?: number;
  message?: string;
}

export function useAIStream() {
  const streamContent = ref('');
  const loading = ref(false);
  const error = ref('');
  let abortController: AbortController | null = null;

  /**
   * 发起 AI 流式对话
   */
  const fetchAIStream = async (params: {
    conversationId?: string;
    message: string;
    emotion?: 'sad' | 'angry' | 'anxious';
    model?: string;
  }) => {
    loading.value = true;
    error.value = '';
    streamContent.value = '';
    abortController = new AbortController();

    try {
      const response = await $fetch(`${BASE_URL}/ai/stream`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${localStorage.getItem('access_token')}`,
        },
        body: params,
        responseType: 'stream',
        signal: abortController.signal,
      }) as Response;

      const reader = response.body?.getReader();
      const decoder = new TextDecoder('utf-8');

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        const chunk = decoder.decode(value, { stream: true });
        const lines = chunk.split('\n').filter(Boolean);

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            const data = JSON.parse(line.slice(6)) as StreamEvent;
            handleStreamEvent(data);
          }
        }
      }
    } catch (e: any) {
      if (e.name === 'AbortError') {
        error.value = '请求已取消';
      } else {
        error.value = e.message;
      }
    } finally {
      loading.value = false;
    }
  };

  /**
   * 处理流式事件
   */
  const handleStreamEvent = (event: StreamEvent) => {
    switch (event.type) {
      case 'start':
        console.log('对话开始:', event.conversationId);
        break;
      case 'delta':
        streamContent.value += event.content;
        break;
      case 'finish':
        console.log('对话结束:', event.messageId);
        break;
      case 'error':
        error.value = event.message || 'AI 服务错误';
        break;
    }
  };

  /**
   * 中断请求
   */
  const abort = () => {
    abortController?.abort();
  };

  onUnmounted(() => {
    abort();
  });

  return {
    streamContent,
    loading,
    error,
    fetchAIStream,
    abort,
  };
}
```

### 3.2 SSE 事件格式

| 事件类型 | 数据格式 | 说明 |
|----------|----------|------|
| start | `{"type":"start","conversationId":"conv_xxx"}` | 对话开始，返回会话 ID |
| delta | `{"type":"delta","content":"你好"}` | 内容增量（打字机效果） |
| finish | `{"type":"finish","messageId":"msg_xxx"}` | 对话结束，返回消息 ID |
| error | `{"type":"error","code":50001,"message":"AI服务繁忙"}` | 错误信息 |

### 3.3 多 AI 模型切换

```typescript
// 请求示例
await fetchAIStream({
  conversationId: 'conv_xxx',
  message: '我最近工作压力大',
  emotion: 'anxious',
  model: 'kimi'  // 可选: 'kimi' | 'openai' | 'local'
});
```

---

## 四、状态管理

### 4.1 用户状态 (user.ts)

```typescript
// stores/user.ts

import { defineStore } from 'pinia';

export const useUserStore = defineStore('user', () => {
  // State
  const userInfo = ref<UserInfo | null>(null);
  const accessToken = ref<string>('');
  const tokenExpiry = ref<number>(0);

  // Getters
  const isAuthenticated = computed(() => !!accessToken.value && !!userInfo.value?.id);
  const getNickname = computed(() => userInfo.value?.nickname || '用户');
  const getAvatarPath = computed(() => userInfo.value?.avatar || '/imgs/default-avatar.webp');

  // Actions
  const setAccessToken = (token: string, expiresIn: number, rememberMe?: boolean) => {
    accessToken.value = token;
    tokenExpiry.value = Date.now() + expiresIn * 1000;

    if (import.meta.client) {
      if (rememberMe) {
        localStorage.setItem('access_token', token);
      } else {
        sessionStorage.setItem('access_token', token);
      }
    }
  };

  const clearToken = () => {
    accessToken.value = '';
    tokenExpiry.value = 0;

    if (import.meta.client) {
      localStorage.removeItem('access_token');
      sessionStorage.removeItem('access_token');
    }
  };

  return {
    userInfo,
    accessToken,
    tokenExpiry,
    isAuthenticated,
    getNickname,
    getAvatarPath,
    setAccessToken,
    clearToken,
  };
});
```

### 4.2 会话状态 (conversation.ts)

```typescript
// stores/conversation.ts

import { defineStore } from 'pinia';

export const useConversationStore = defineStore('conversation', () => {
  // State
  const conversationList = ref<ConversationItem[]>([]);
  const currentConversationId = ref<string | null>(null);

  // Getters
  const currentConversation = computed(() => {
    return conversationList.value.find(c => c.id === currentConversationId.value);
  });

  // 按时间分组
  const groupedConversations = computed(() => {
    const groups: Record<string, any[]> = {
      '置顶': [],
      '今天': [],
      '一周内': [],
      '三十天内': [],
      '更早': [],
    };
    // 分组逻辑...
    return labelOrder.filter(label => groups[label].length > 0).map(label => ({
      label,
      data: groups[label],
    }));
  });

  // Actions
  const fetchConversations = async () => { /* ... */ };
  const createConversation = async (title?: string) => { /* ... */ };
  const deleteConversation = async (id: string) => { /* ... */ };
  const pinConversation = async (id: string, isTop: boolean) => { /* ... */ };

  return {
    conversationList,
    currentConversationId,
    currentConversation,
    groupedConversations,
    fetchConversations,
    createConversation,
    deleteConversation,
    pinConversation,
  };
});
```

### 4.3 消息状态 (message.ts)

```typescript
// stores/message.ts

import { defineStore } from 'pinia';

export const useMessageStore = defineStore('message', () => {
  // State
  const messageMap = ref<Record<string, MessageWithStatus[]>>({});

  // Getters
  const getMessages = (conversationId: string) => {
    return messageMap.value[conversationId] || [];
  };

  // Actions
  const fetchMessages = async (conversationId: string) => { /* ... */ };
  const addMessage = (conversationId: string, message: MessageWithStatus) => { /* ... */ };
  const updateMessage = (conversationId: string, messageId: string, updates: Partial<MessageWithStatus>) => { /* ... */ };

  return {
    messageMap,
    getMessages,
    fetchMessages,
    addMessage,
    updateMessage,
  };
});
```

---

## 五、类型定义

### 5.1 API 类型 (api.ts)

```typescript
// types/api.ts

// ==================== 认证模块 ====================

export interface SendVerificationCodeParams {
  username: string;
  type: 'register' | 'login' | 'reset';
}

export interface LoginParams {
  username: string;
  password: string;  // SHA256 哈希后
  rememberMe?: boolean;
}

export interface RegisterParams extends LoginParams {
  verificationCode: string;
}

export interface ResetPasswordParams {
  username: string;
  verificationCode: string;
  newPassword: string;  // SHA256 哈希后
}

export interface LoginData {
  accessToken: string;
  expiresIn: number;
  user: UserInfo;
}

// ==================== 用户模块 ====================

export interface UserInfo {
  id: string;
  username: string;
  nickname: string;
  avatar: string;
  age: number | null;
  config: UserConfig;
  createdAt: string;
  updatedAt?: string;
}

export interface UserConfig {
  fontSize?: 'small' | 'medium' | 'large' | '14px' | '16px' | '18px' | '20px';
  theme?: 'light' | 'dark' | 'auto';
}

export interface UpdateProfileParams {
  nickname?: string;
  age?: number;
  config?: Partial<UserConfig>;
}

// ==================== 会话模块 ====================

export interface ConversationItem {
  id: string;
  userId: string;
  title: string;
  isTop: boolean;
  lastMessage: string | null;
  lastMessageTime: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateConversationParams {
  title?: string;
}

// ==================== 消息模块 ====================

export interface MessageItem {
  id: string;
  conversationId: string;
  sender: 'user' | 'ai';
  content: string;
  contentType: 'text' | 'audio' | 'img';
  emotionTag?: 'sad' | 'angry' | 'anxious';
  sendTime: number;
  createdAt: number;
}

export interface MessageWithStatus extends MessageItem {
  status?: 'sending' | 'sent' | 'failed' | 'streaming';
}

export interface SendMessageParams {
  content: string;
  contentType?: 'text' | 'audio' | 'img';
  emotionTag?: 'sad' | 'angry' | 'anxious';
}

// ==================== AI 模块 ====================

export interface AIStreamParams {
  conversationId?: string;
  message: string;
  emotion?: 'sad' | 'angry' | 'anxious';
  model?: string;
}

export interface StreamChunk {
  type: 'start' | 'delta' | 'finish' | 'error';
  conversationId?: string;
  content?: string;
  messageId?: string;
  code?: number;
  message?: string;
}

// ==================== 测验模块 ====================

export interface SurveyItem {
  id: number;
  title: string;
  description: string;
  estimatedTime: string;
  status: 'completed' | 'not_started';
  completedAt?: string;
  resultId?: string;
}

export interface SurveyDetail {
  id: number;
  title: string;
  description: string;
  estimatedTime: string;
  questions: Question[];
}

export interface Question {
  id: number;
  title: string;
  type: 'radio';
  options: QuestionOption[];
}

export interface QuestionOption {
  id: number;
  text: string;
  score: number;
}

export interface SubmitSurveyParams {
  answers: Array<{
    questionId: number;
    optionId: number;
  }>;
}

export interface SurveyResult {
  resultId: string;
  totalScore: number;
  level: string;
  suggestion: string;
}

// ==================== 报表模块 ====================

export interface EmotionDistribution {
  name: string;
  value: number;
}

export interface DailyReport {
  date: string;
  summary: string;
  emotionDistribution: EmotionDistribution[];
  conversationCount: number;
  messageCount: number;
  wordCount: number;
}

export interface EmotionTrend {
  type: 'daily' | 'weekly' | 'monthly';
  dates: string[];
  series: Array<{
    name: string;
    data: number[];
  }>;
  summary: string;
  emotionDistribution: EmotionDistribution[];
  conversationCount: number;
  messageCount: number;
  wordCount: number;
}
```

### 5.2 通用响应类型

```typescript
// types/commonType.ts

export interface returnMsgType {
  isOk: boolean;
  msg: string;
  code?: number;
}
```

---

## 六、组件说明

### 6.1 图表组件

```typescript
// components/charts/

// BarChart 柱状图
// LineChart 折线图
// PieChart 饼图
// RadarChart 雷达图
```

**使用示例：**

```vue
<template>
  <div>
    <RadarChart :data="radarData" :config="radarConfig" />
  </div>
</template>

<script setup lang="ts">
import RadarChart from '~/components/charts/radarChart.vue';

const radarData = ref({
  indicator: [
    { name: '情绪稳定', max: 100 },
    { name: '抑郁风险', max: 100 },
    { name: '焦虑风险', max: 100 },
  ],
  series: [
    { name: '本周', data: [75, 65, 58] },
  ],
});

const radarConfig = {
  height: '300px',
};
</script>
```

### 6.2 报表卡片组件

```vue
<!-- components/report/chartsCard.vue -->

<template>
  <el-card class="charts-card">
    <template #header>
      <div class="card-header">
        <span>{{ title }}</span>
        <el-button type="primary" link @click="handleMore">
          查看更多
        </el-button>
      </div>
    </template>
    <slot />
  </el-card>
</template>
```

---

## 七、路由与中间件

### 7.1 路由配置

```typescript
// router.options.ts

export default {
  routes: (_routes) => [
    {
      name: 'root',
      path: '/',
      redirect: { name: 'chat' },
    },
    {
      name: 'login',
      path: '/login',
      component: () => import('~/pages/login/index.vue'),
    },
    {
      name: 'chat',
      path: '/chat',
      redirect: '/chat/conversation/new',
      component: () => import('~/pages/chat/index.vue'),
      children: [
        {
          name: 'chat-conversation',
          path: 'conversation',
          redirect: '/chat/conversation/new',
          component: () => import('~/pages/chat/conversation/index.vue'),
          children: [
            {
              name: 'chat-conversation-detail',
              path: ':id',
              component: () => import('~/pages/chat/conversation/[id].vue'),
              props: true,
            },
            {
              name: 'chat-conversation-new',
              path: 'new',
              component: () => import('~/pages/chat/conversation/new.vue'),
            },
          ],
        },
      ],
    },
  ],
};
```

### 7.2 全局鉴权中间件

```typescript
// middleware/auth.global.ts

export default defineNuxtRouteMiddleware((to) => {
  const userStore = useUserStore();

  // 公开路径
  const publicPaths = ['/login', '/forget'];
  if (publicPaths.some(p => to.path.startsWith(p))) {
    return;
  }

  // 检查登录状态
  if (!userStore.isAuthenticated) {
    return navigateTo('/login');
  }
});
```

---

## 八、配置说明

### 8.1 环境变量

```bash
# .env.example
API_BASE_URL=http://localhost:8081/api/v1
AI_STREAM_API=http://localhost:8081/api/v1/ai/stream
LLM_MODEL=kimi
```

### 8.2 Nuxt 配置

```typescript
// nuxt.config.ts

export default defineNuxtConfig({
  modules: [
    '@element-plus/nuxt',
    '@pinia/nuxt',
    '@nuxtjs/device',
    'nuxt-echarts',
  ],

  elementPlus: {
    importStyle: 'scss',
  },

  runtimeConfig: {
    public: {
      API_BASE_URL: process.env.API_BASE_URL || 'http://localhost:8081/api/v1',
      AI_STREAM_API: process.env.AI_STREAM_API || 'http://localhost:8081/api/v1/ai/stream',
      LLM_MODEL: process.env.LLM_MODEL || 'kimi',
    },
  },

  app: {
    head: {
      title: 'Emotion-Echo',
    },
  },
});
```

---

## 九、暗黑模式

### 9.1 主题切换机制

系统支持三种主题模式：浅色（light）、深色（dark）、跟随系统（auto）。

```typescript
// stores/user.ts
const setTheme = (theme: "light" | "dark" | "auto") => {
  // 存储用户偏好
  // 应用主题到 html 元素
  if (theme === "dark" || (theme === "auto" && window.matchMedia("(prefers-color-scheme: dark)").matches)) {
    document.documentElement.classList.add("dark");
  } else {
    document.documentElement.classList.remove("dark");
  }
};
```

### 9.2 暗黑模式适配范围

| 组件/页面 | 适配方式 |
|-----------|----------|
| Element Plus 组件 | 通过 `html.dark` 类 + CSS 变量自动适配 |
| 会话页面 | `global.scss` 中定义 `.chat-page-container` 等样式 |
| 会话列表 | `global.scss` 中定义 `.conversation-container` 等样式 |
| ECharts 图表 | `BaseChart.vue` 中通过 `:theme="isDark ? 'dark' : undefined"` 适配 |
| Markdown 内容 | `global.scss` 中定义代码块、列表等暗黑样式 |
| 用户页面图表 | `global.scss` 中定义 `.user-data-card` 等样式 |

### 9.3 自定义样式示例

```scss
// global.scss
html.dark {
  .custom-component {
    background-color: #242424 !important;
    color: #e5e7eb !important;
    border: 1px solid #374151 !important;
  }
}
```

---

## 十、智能会话标题生成

### 10.1 功能说明

当用户创建新会话并发送第一条消息后，系统会自动生成一个简短的会话标题（不超过10个字符）。

### 10.2 实现流程

```
1. 用户在新建会话页面发送消息
   ↓
2. 前端调用 createNewConversation，传递 shouldGenerateTitle=true
   ↓
3. 后端 AI 回复完成后，调用 LLM 生成标题
   ↓
4. 标题存储到 conversations 表的 title 字段
   ↓
5. 前端刷新会话列表，显示新标题
```

### 10.3 前端关键代码

```typescript
// composables/useConversationSender.ts

const createNewConversation = async (content: string) => {
  // 创建会话
  const createResult = await conversationStore.createConversation();
  
  // 跳转页面
  await navigateTo({ name: 'chat-conversation-detail', params: { id: newSessionId } });
  
  // 设置当前会话
  messageStore.switchSession(newSessionId);
  
  // 触发 AI 回复，传递 shouldGenerateTitle=true
  triggerAIReplyWithOptions(content, { shouldGenerateTitle: true });
};
```

### 10.4 后端关键代码

```go
// internal/service/ai_service.go

// StreamRequest 增加字段
type StreamRequest struct {
  ShouldGenerateTitle bool `json:"shouldGenerateTitle,omitempty"`
}

// AI 回复完成后生成标题
if isNewConversation || req.ShouldGenerateTitle {
  title, err := chain.GenerateTitle(ctx, req.Message)
  if err != nil {
    title = llm.TruncateString(req.Message, 10) // 降级处理
  }
  convService.UpdateTitle(ctx, convID, title)
}
```

---

## 十一、Markdown 渲染

### 11.1 渲染实现

AI 回复内容支持 Markdown 格式，使用 `marked` 库解析，`DOMPurify` 进行 XSS 防护。

```typescript
// pages/chat/conversation/[id].vue

import DOMPurify from "dompurify";
import { marked } from "marked";

const getHtmlContent = (content: string) => {
  if (!content || typeof content !== 'string') return '';
  
  try {
    const result = marked.parse(content);
    if (typeof result === 'string') {
      return DOMPurify.sanitize(result, {
        ALLOWED_TAGS: ['p', 'br', 'strong', 'em', 'u', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 
                       'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'a', 'span', 'div'],
        ALLOWED_ATTR: ['href', 'class', 'target', 'rel']
      });
    }
    return DOMPurify.sanitize(String(result));
  } catch (e) {
    return DOMPurify.sanitize(content);
  }
};
```

### 11.2 模板使用

```vue
<span v-html="getHtmlContent(item.content)"></span>
```

### 11.3 暗黑模式适配

```scss
// global.scss
html.dark {
  .message-content {
    p { color: #e5e7eb !important; }
    code { background-color: #2d2d2d !important; color: #e5e7eb !important; }
    pre { background-color: #1f2937 !important; }
    blockquote { border-left-color: #374151 !important; color: #9ca3af !important; }
    ul, ol { li { color: #e5e7eb !important; } }
  }
}
```
```

---

## 九、开发说明

### 9.1 本地开发

```bash
# 安装依赖
npm install

# 启动开发服务器
npm run dev

# 构建生产版本
npm run build

# 预览生产版本
npm run preview
```

### 9.2 目录规范

- **pages/** - 页面组件，文件名对应路由
- **components/** - 可复用组件，按功能分组
- **composables/** - 可组合函数（hooks）
- **stores/** - Pinia 状态管理
- **types/** - TypeScript 类型定义
- **utils/** - 工具函数
- **configs/** - 配置文件
