---
status: landed
superseded-by: stage-30-A-analytics-business.md + stage-30-A-landing.md
original-path: .trae/documents/voice-message-feature/{spec,checklist,tasks}.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
note: 三份(spec/checklist/tasks)合并为一份，原始三份见 .trae/documents/voice-message-feature/
---

# 语音消息功能（合并自 spec + checklist + tasks）

> 本文件由 `.trae/documents/voice-message-feature/` 下三份文档合并而来，保留全部内容。
> 原始文件：
> - spec.md
> - checklist.md
> - tasks.md

---

## 原始 spec.md

# 语音消息功能 Spec

## Why
用户需要通过语音输入消息，系统自动识别语音内容并分析情绪，提供更自然便捷的交互方式。

## What Changes
- 新增语音消息类型，支持录音上传、语音识别、情绪分析
- 复用现有文字工作流，在 Prompt 中描述语音场景
- 前端新增语音录制 UI 和播放组件
- 后端新增语音处理服务和模型调用封装

## Impact
- Affected specs: 消息模块、AI 对话模块、用户界面
- Affected code:
  - 后端: `internal/service/voice_service.go`、`internal/pkg/llm/`、路由配置
  - 前端: `app/pages/chat/conversation/[id].vue`、`app/stores/message.ts`、新增组件

## ADDED Requirements

### Requirement: 语音消息录制
用户可通过点击录音按钮录制语音消息。

#### Scenario: 开始录音
- **WHEN** 用户点击录音按钮
- **THEN** 输入框被禁用，显示录音状态，开始采集音频

#### Scenario: 结束录音
- **WHEN** 用户再次点击录音按钮
- **THEN** 停止录音，将音频上传到后端，显示语音条和转写文字

### Requirement: 语音消息存储
语音消息需要同时存储音频文件和转写文本。

#### Scenario: 语音消息保存
- **WHEN** 语音识别完成后
- **THEN** 系统保存：音频文件路径、转写文本、情绪标签到数据库

#### Scenario: 语音消息展示
- **WHEN** 渲染语音消息时
- **THEN** 显示语音条（播放按钮、时长）和转写文字

### Requirement: 语音消息 AI 处理
语音识别完成后，复用现有文字工作流处理用户消息。

#### Scenario: 语音消息 AI 回复
- **WHEN** 语音识别完成并保存后
- **THEN** 构造 Prompt：`用户发来一段语音，从语音中听出：{情绪}。语音内容：{转写文本}。`
- **THEN** 调用现有文字工作流获取 AI 回复

### Requirement: 模型服务
语音模型部署为独立 HTTP 服务。

#### Scenario: 模型服务部署
- **WHEN** 系统启动时
- **THEN** Qwen3-ASR 运行在 `localhost:8001`，SenseVoice 运行在 `localhost:8002`

## MODIFIED Requirements

### Requirement: 消息类型扩展
**原类型**: text、image 等
**新增类型**: audio（语音消息）

### Requirement: AI Prompt 扩展
**原 Prompt**: 用户消息直接使用文本内容
**新 Prompt**: 语音消息使用 `用户发来一段语音，从语音中听出：{情绪}。语音内容：{转写文本}。`

## 语音消息数据结构

```go
// VoiceMessage 语音消息
type VoiceMessage struct {
    AudioURL    string `json:"audioUrl"`    // 音频文件路径
    Transcript  string `json:"transcript"`  // 转写文本
    Emotion     string `json:"emotion"`     // 情绪标签：happy, sad, angry, anxious, neutral
    Duration    int    `json:"duration"`    // 时长（秒）
}
```

## API 设计

### 1. 语音上传接口
```
POST /api/v1/voice/upload
Content-Type: multipart/form-data

参数:
- file: 音频文件（webm/opus格式）
- conversationId: 会话ID（可选）

响应:
{
  "messageId": "msg_xxx",
  "audioUrl": "/uploads/voice/msg_xxx.webm",
  "duration": 5
}
```

### 2. 语音处理（内部）
```
后端内部处理流程:
1. 接收音频文件
2. 并发调用:
   - ASR服务: POST http://localhost:8001/v1/audio/transcriptions
   - 情绪识别服务: POST http://localhost:8002/analyze
3. 等待结果返回
4. 保存消息到数据库
5. 触发 AI 回复工作流
6. SSE 流式推送转写进度和结果
```

## 前端 UI 设计

```
┌─────────────────────────────────┐
│  [🔴 录音中 00:05]              │  ← 录音状态条
├─────────────────────────────────┤
│  [▶️ 0:05] 用户发来一段语音...  │  ← 语音条 + 转写文字
└─────────────────────────────────┘
```

## 情绪标签映射

| SenseVoice 标签 | 前端显示 |
|-----------------|---------|
| happy | 开心 |
| sad | 悲伤 |
| angry | 愤怒 |
| anxious | 焦虑 |
| neutral | 中性 |

## 降级策略
- 如果 ASR 服务不可用，返回错误提示用户
- 如果情绪识别失败，使用 neutral 作为默认值

---

## 原始 checklist.md

# Checklist

## 模型服务部署

- [ ] 模型服务部署文档已创建
- [ ] Qwen3-ASR 服务可正常启动（localhost:8001）
- [ ] SenseVoice 服务可正常启动（localhost:8002）

## 后端实现

### 语音消息存储
- [ ] 数据库表结构已更新支持音频字段
- [ ] `Message` 模型包含 `AudioURL`、`Transcript`、`Emotion`、`Duration` 字段

### LLM 模型调用封装
- [ ] ASR 调用封装完成
- [ ] 情绪识别调用封装完成
- [ ] 并发调用逻辑正确
- [ ] 错误处理和降级策略实现

### 语音处理服务
- [ ] 音频文件上传接口 `/voice/upload` 可用
- [ ] 音频文件正确保存到 `uploads/voice/` 目录
- [ ] 消息正确存储到数据库

### AI 工作流集成
- [ ] 语音消息 Prompt 模板正确
- [ ] AI 回复流程正确触发

### API 路由
- [ ] `/api/v1/voice/upload` 路由配置正确
- [ ] CORS 配置允许音频文件类型

## 前端实现

### 录音功能
- [ ] 点击录音按钮开始录音
- [ ] 再次点击结束录音
- [ ] 录音期间输入框被禁用
- [ ] 录音状态正确显示

### 语音消息组件
- [ ] `VoiceRecorder.vue` 组件正确实现
- [ ] `VoiceMessage.vue` 组件正确实现
- [ ] 播放按钮可正常播放音频
- [ ] 时长显示正确

### 消息页面集成
- [ ] 录音按钮正确显示在输入框旁
- [ ] 语音消息正确渲染（语音条 + 转写文字）
- [ ] SSE 接收转写进度并实时显示

### API 调用
- [ ] 语音文件正确上传到后端
- [ ] 转写进度正确接收
- [ ] 错误处理正确（上传失败、识别失败等）

## 端到端测试

- [ ] 完整录音 → 上传 → 识别 → AI 回复流程跑通
- [ ] 情绪识别结果正确显示
- [ ] 暗黑模式下 UI 正常显示

---

## 原始 tasks.md

# Tasks

## Phase 1: 模型服务部署

### Task 1: 模型服务部署文档
- [ ] 创建模型服务部署指南
- [ ] 包含 Qwen3-ASR 启动命令
- [ ] 包含 SenseVoice 启动命令
- [ ] 包含服务端口配置

## Phase 2: 后端实现

### Task 2: 后端 - 语音消息存储模型
- [ ] 修改 `messages` 表添加音频相关字段（可选存储路径）
- [ ] 或创建 `voice_messages` 表存储语音消息
- [ ] 更新 `Message` 模型结构
- [ ] 依赖：无

### Task 3: 后端 - LLM 包封装模型调用
- [ ] 创建 `internal/pkg/llm/asr.go` - ASR 模型调用封装
- [ ] 创建 `internal/pkg/llm/emotion.go` - 情绪识别模型调用封装
- [ ] 实现并发调用两个模型
- [ ] 实现结果解析和错误处理
- [ ] 依赖：Task 1（模型服务运行）

### Task 4: 后端 - 语音处理服务
- [ ] 创建 `internal/service/voice_service.go`
- [ ] 实现音频文件接收和保存
- [ ] 实现并发调用 ASR 和情绪识别
- [ ] 实现消息存储
- [ ] 依赖：Task 3

### Task 5: 后端 - AI 工作流集成
- [ ] 修改 `ai_service.go` 支持语音消息类型的 Prompt 构造
- [ ] 实现 Prompt 模板：`用户发来一段语音，从语音中听出：{情绪}。语音内容：{转写文本}。`
- [ ] 依赖：Task 4

### Task 6: 后端 - API 路由配置
- [ ] 添加 `/api/v1/voice/upload` 路由
- [ ] 添加 `/api/v1/voice/process` 路由
- [ ] 配置 CORS 允许音频文件类型
- [ ] 依赖：Task 4

## Phase 3: 前端实现

### Task 7: 前端 - 录音状态管理
- [ ] 在 `message.ts` 添加录音状态
- [ ] 实现 `startRecording()` 方法
- [ ] 实现 `stopRecording()` 方法
- [ ] 依赖：无

### Task 8: 前端 - 音频录制组件
- [ ] 创建 `app/components/voice/VoiceRecorder.vue`
- [ ] 实现麦克风权限请求
- [ ] 实现 MediaRecorder API 录音
- [ ] 实现音频数据收集和转换
- [ ] 依赖：Task 7

### Task 9: 前端 - 语音消息 UI
- [ ] 创建 `app/components/voice/VoiceMessage.vue`
- [ ] 实现语音条显示（播放按钮、时长）
- [ ] 实现转写文字显示
- [ ] 实现音频播放功能
- [ ] 依赖：Task 8

### Task 10: 前端 - 消息页面集成
- [ ] 修改 `[id].vue` 集成录音按钮
- [ ] 修改消息列表渲染支持 audio 类型
- [ ] 实现录音时禁用输入框
- [ ] 依赖：Task 8, Task 9

### Task 11: 前端 - 语音上传和进度显示
- [ ] 实现 `uploadVoice()` 方法
- [ ] 实现 SSE 接收转写进度
- [ ] 实现实时显示转写文字
- [ ] 依赖：Task 6（后端 API）

## Phase 4: 测试验证

### Task 12: 端到端测试
- [ ] 测试录音功能
- [ ] 测试语音识别
- [ ] 测试情绪识别
- [ ] 测试 AI 回复
- [ ] 测试消息展示和播放
- [ ] 依赖：Phase 2 + Phase 3 完成

## Task Dependencies
```
Phase 1 (Task 1)
    ↓
Phase 2: Task 3 → Task 4 → Task 5 → Task 6
              ↓         ↓         ↓
         Task 2    (Task 2 可以并行)

Phase 3: Task 7 → Task 8 → Task 9 → Task 10 → Task 11
              ↓         ↓         ↓         ↓
          (可以并行，基于 Task 7 完成)

Phase 4: Task 12 (依赖所有前置任务)
```
