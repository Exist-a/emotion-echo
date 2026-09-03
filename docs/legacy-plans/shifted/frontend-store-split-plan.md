---
status: shifted
superseded-by: stage-26-O-frontend-redesign.md
original-path: .trae/documents/前端项目拆分规划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# Emotion-Echo-Web 前端项目拆分规划

## 一、问题分析

### 1.1 主要耦合度过高的文件

| 文件 | 行数 | 主要问题 |
|------|------|----------|
| `stores/message.ts` | ~520 | 混合了消息状态、流式处理、语音录制、TTS回调 |
| `stores/conversation.ts` | ~300 | 混合了会话状态、时间分组逻辑、API调用 |
| `pages/chat/conversation/[id].vue` | ~500 | 混合了UI、语音录制、AI流处理、TTS、数字人控制 |
| `utils/index.ts` | ~300 | 混合了多种不相关的工具函数 |
| `composables/useConversationSender.ts` | ~135 | TTS/AI流回调逻辑重复 |

### 1.2 核心耦合问题

```
[message_store.ts] 问题:
├─ 混合了 SSE 流式处理逻辑
├─ 混合了 MediaRecorder 语音录制
├─ 混合了 TTS 回调处理
└─ sendAIStream 函数过于庞大 (~240行)

[conversation/[id].vue] 问题:
├─ startRecording/stopRecording (~165行) 嵌入页面
├─ TTS 防抖逻辑重复 (accumulatedDeltaText, ttsDebounceTimer)
├─ AI 流回调处理重复
└─ 数字人控制耦合在页面中

[utils/index.ts] 问题:
├─ 日期格式化、相对时间
├─ 深拷贝、防抖、节流
├─ 情绪标签映射
├─ 文件操作、剪贴板
└─ URL参数处理
```

---

## 二、拆分方案

### 2.1 utils/index.ts 拆分 (简单)

**目标**: 将混合的工具函数按功能拆分为独立文件

**拆分结果**:
```
utils/
├── index.ts           # 导出入口
├── date.ts            # formatDate, formatRelativeTime
├── function.ts        # deepClone, debounce, throttle, generateId, sleep, isEmpty
├── emotion.ts         # EMOTION_LABEL_MAP, getEmotionLabel, EmotionLabel
├── file.ts            # formatFileSize, downloadFile, copyToClipboard
├── url.ts             # parseQueryParams, buildQueryString
└── safe.ts            # safeGet
```

**修改点**:
- 创建 `utils/date.ts`, `utils/function.ts`, `utils/emotion.ts`, `utils/file.ts`, `utils/url.ts`, `utils/safe.ts`
- `utils/index.ts` 改为导出所有工具
- 全局搜索 `import { formatDate, formatRelativeTime, ... }` 并更新导入路径

---

### 2.2 stores/conversation.ts 拆分 (中等)

**目标**: 将会话状态与分组逻辑分离

**拆分结果**:
```
composables/
└── useConversationGrouper.ts   # 提取分组逻辑
```

**现有逻辑提取**:
- `groupedConversations` computed 逻辑 → `useConversationGrouper.ts`
- 保留 `fetchConversations`, `createConversation` 等 API 相关逻辑在 store

**修改点**:
- 创建 `composables/useConversationGrouper.ts`
- 接收 `conversationList` 作为参数，返回 `groupedConversations`
- 页面中调用 `const { groupedConversations } = useConversationGrouper(conversationStore.conversationList)`

---

### 2.3 stores/message.ts 拆分 (复杂 - 核心)

**目标**: 将消息状态、流式处理、语音录制、TTS回调分离

**拆分结果**:
```
composables/
├── useAIStreamHandler.ts      # AI 流式响应处理
├── useVoiceRecorder.ts        # 语音录制逻辑
└── useTTSManager.ts            # TTS 管理器

stores/
└── message.ts                 # 仅保留消息状态
```

#### 2.3.1 useVoiceRecorder.ts 新建

**职责**: 管理 MediaRecorder 生命周期和状态

**接口设计**:
```typescript
export interface UseVoiceRecorderOptions {
  onRecordingStart?: () => void
  onRecordingStop?: (blob: Blob, duration: number) => void
  onError?: (error: string) => void
}

export const useVoiceRecorder = (options: UseVoiceRecorderOptions) => {
  // 状态
  const isRecording = ref(false)
  const isUploading = ref(false)
  const duration = ref(0)

  // 方法
  const startRecording = () => Promise<void>
  const stopRecording = () => void

  return { isRecording, isUploading, duration, startRecording, stopRecording }
}
```

**修改点**:
- 提取 `pages/chat/conversation/[id].vue` 中的 `startRecording`, `stopRecording` 逻辑
- 提取 `stores/message.ts` 中的 `uploadVoiceMessage` 相关逻辑

#### 2.3.2 useAIStreamHandler.ts 新建

**职责**: 处理 SSE 流式响应，解析 Server-Sent Events

**接口设计**:
```typescript
export interface UseAIStreamHandlerOptions {
  onStart?: (data: { conversationId?: string; userMessageId?: string }) => void
  onDelta?: (content: string) => void
  onFinish?: (data: { messageId: string; emotion?: string }) => void
  onError?: (error: string) => void
  onTruncated?: (data: { content: string }) => void
  onTitleUpdated?: (data: { conversationId: string; title: string }) => void
}

export const useAIStreamHandler = (options: UseAIStreamHandlerOptions) => {
  const sendAIStream = async (params: AIStreamParams) => Promise<AIStreamResult>
  const cancelAIStream = () => void
  const isStreaming = ref(false)

  return { sendAIStream, cancelAIStream, isStreaming }
}
```

**修改点**:
- 提取 `stores/message.ts` 中的 `sendAIStream` 函数的 SSE 处理逻辑 (~180行)
- 提取 `pages/chat/conversation/[id].vue` 中的 `handleSubmit` 回调逻辑

#### 2.3.3 useTTSManager.ts 新建

**职责**: 管理 TTS 播放和文本缓冲

**接口设计**:
```typescript
export interface UseTTSManagerOptions {
  onLipShapeChange?: (shape: string) => void
  onEmotionChange?: (emotion: string) => void
}

export const useTTSManager = (options: UseTTSManagerOptions) => {
  // 状态
  const isPlaying = ref(false)

  // 方法
  const playText = (text: string) => void
  const flushRemaining = () => void
  const stop = () => void
  const setEnabled = (enabled: boolean) => void

  return { isPlaying, playText, flushRemaining, stop, setEnabled }
}
```

**修改点**:
- 提取 `pages/chat/conversation/[id].vue` 中的 TTS 相关逻辑 (~20行)
- 注意: `composables/useDigitalHumanTTS.ts` 已存在，可考虑合并或复用

#### 2.3.4 stores/message.ts 简化

**简化后职责**:
- 消息列表状态管理 (`currentMessages`, `sortedMessages`)
- 消息发送状态 (`isSending`, `isStreaming`)
- 加载更多历史消息 (`loadMoreMessages`)
- 对外暴露组合后的方法

**修改后代码量**: ~150行

---

### 2.4 pages/chat/conversation/[id].vue 拆分 (复杂)

**目标**: 将语音录制、TTS、AI流处理、数字人控制分离

**拆分结果**:
```
pages/chat/conversation/[id].vue   # 仅保留 UI 组合和布局
components/
└── chat/
    ├── ChatInput.vue              # 输入框组件
    ├── VoiceRecordButton.vue      # 语音录制按钮
    ├── StreamingBubble.vue        # 流式消息气泡
    └── ChatHeader.vue             # 聊天头部
```

**修改点**:

#### 2.4.1 ChatInput.vue 提取
- 从 `conversation/[id].vue` 提取输入框相关 UI 和逻辑
- 使用 `useVoiceRecorder` 和 `useTTSManager`

#### 2.4.2 VoiceRecordButton.vue 提取
- 语音录制按钮组件
- 包含录音状态 UI (录音环动画)

#### 2.4.3 页面简化
- 移除 `startRecording`, `stopRecording` 逻辑 (~165行)
- 移除 TTS 相关状态和回调 (~20行)
- 移除 AI 流处理回调 (~80行)
- 仅保留 UI 组合和基础事件处理

---

## 三、实施步骤

### Phase 1: utils 拆分 (第1步)
1. 创建 `utils/date.ts`
2. 创建 `utils/function.ts`
3. 创建 `utils/emotion.ts`
4. 创建 `utils/file.ts`
5. 创建 `utils/url.ts`
6. 创建 `utils/safe.ts`
7. 更新 `utils/index.ts` 导出
8. 全局搜索并更新其他文件的导入路径

### Phase 2: composables 新建 (第2步)
1. 创建 `composables/useVoiceRecorder.ts`
2. 创建 `composables/useAIStreamHandler.ts`
3. 创建 `composables/useTTSManager.ts`
4. 创建 `composables/useConversationGrouper.ts`

### Phase 3: stores/message.ts 简化 (第3步)
1. 移除语音录制逻辑，委托给 `useVoiceRecorder`
2. 移除 SSE 处理逻辑，委托给 `useAIStreamHandler`
3. 移除 TTS 回调逻辑，委托给 `useTTSManager`
4. 保留消息状态和 API 调用

### Phase 4: 页面拆分 (第4步)
1. 创建 `components/chat/ChatInput.vue`
2. 创建 `components/chat/VoiceRecordButton.vue`
3. 简化 `conversation/[id].vue`

### Phase 5: 测试和修复 (第5步)
1. 确保所有功能正常工作
2. 修复类型错误
3. 验证导入路径正确

---

## 四、预期效果

| 指标 | 拆分前 | 拆分后 |
|------|--------|--------|
| stores/message.ts | ~520行 | ~150行 |
| conversation/[id].vue | ~500行 | ~300行 |
| utils/index.ts | ~300行 | ~50行 |
| 代码职责单一性 | 低 | 高 |
| 可测试性 | 低 | 高 |
| 可复用性 | 低 | 高 |

---

## 五、注意事项

1. **向后兼容**: 拆分过程中保持 API 接口不变
2. **类型安全**: 确保 TypeScript 类型正确传递
3. **渐进式**: 每步完成后进行测试
4. **导入路径**: 使用 `~/utils/...` 别名避免相对路径问题
