---
status: landed
superseded-by: stage-30-C-browser-testing.md
original-path: .trae/specs/ai-stream-cancel-fix/{spec,checklist,tasks}.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
note: 三份(spec/checklist/tasks)合并为一份
---

# AI 回复系统问题修复（合并自 spec + checklist + tasks）

## 原始 spec.md

# AI 回复系统问题修复 Spec

## Why
1. 用户取消 AI 回复时，前端错误地显示"流式返回失败"，应该显示"取消成功"
2. AI 能记住用户名字"老陈"，但当用户问"之前聊了什么"时，AI 说没聊过——系统提示词没有指导 AI 记住和引用对话历史

## What Changes

### 问题1修复：取消成功提示
- 前端 `sendAIStream` 需要区分"用户主动取消"和"服务端错误"
- AbortController abort 时设置一个标志位
- onError 回调检查该标志位，如果是被取消的请求，显示"已取消"而不是"失败"

### 问题2修复：系统提示词增强
- 修改 `DefaultPrompt`，添加"记住对话历史"的指令
- 使 AI 能够正确引用之前的对话内容

## Impact
- Affected specs: AI 流式对话功能
- Affected code:
  - `Emotion-Echo-Web/app/stores/message.ts` - sendAIStream 取消处理
  - `Emotion-Echo-Gin/internal/service/ai_service.go` - DefaultPrompt

---

## ADDED Requirements

### Requirement: 取消操作成功提示
当用户主动取消 AI 回复时，系统应该显示友好的"已取消"提示，而不是错误提示。

#### Scenario: 用户取消 AI 回复
- **WHEN** 用户点击红色停止按钮取消正在生成的 AI 回复
- **THEN** 前端显示"已取消，当前回复未保存"或类似提示
- **AND** 消息气泡不显示（因为是截断且部分内容未保存）

### Requirement: AI 记住对话历史
AI 应该在回复中能够正确引用之前的对话内容，当用户询问"之前聊了什么"时，能准确回忆。

#### Scenario: 用户询问对话历史
- **WHEN** 用户说"我们之前聊了什么"或类似问题
- **THEN** AI 能够根据上下文历史给出相关回答
- **AND** AI 不会说"我们之前没聊过"

---

## MODIFIED Requirements

### Requirement: 系统提示词
**Original**: `你是一位专业的心理健康助手。耐心倾听用户的倾诉，给予适当的回应和建议。语气友好、专业。`

**Modified**: 在提示词中增加"记住对话历史"的要求，使 AI 能够：
1. 记住用户的基本信息（如名字）
2. 在回复中适当引用之前的对话内容
3. 当用户询问历史时，能够准确描述之前的交流

---

## REMOVED Requirements
无

---

## Technical Details

### 问题1解决方案
在 `message.ts` 中：
1. 添加 `isCancelled` 标志位
2. `cancelAIStream()` 时设置 `isCancelled = true`
3. 在 `catch` 块和 `error` 事件中检查 `isCancelled`
4. 如果是取消的请求，显示取消提示而不是错误提示

### 问题2解决方案
修改 `ai_service.go` 中的 `DefaultPrompt`：
```
你是一位专业的心理健康助手。耐心倾听用户的倾诉，给予适当的回应和建议。
语气友好、专业，回复控制在200字以内。

重要：你正在和用户进行连续的对话。请记住用户之前分享的信息
（如名字、经历、问题等），在后续对话中适当引用，这有助于提供
更连贯、更个性化的支持。如果用户询问之前聊过的内容，你应该能够
准确描述。
```

---

## 原始 checklist.md

# Checklist

## 问题1：取消成功提示
- [x] `isCancelled` 标志位已添加
- [x] `cancelAIStream` 设置 `isCancelled = true`
- [x] `catch` 块检查 `isCancelled` 显示取消提示
- [x] `error` 事件处理检查 `isCancelled`

## 问题2：AI 记忆对话历史
- [x] `DefaultPrompt` 已修改，增加了"记住对话历史"指令
- [ ] AI 能正确引用之前的对话内容

## 编译验证
- [x] 后端代码编译通过

---

## 原始 tasks.md

# Tasks

## 任务1：修复取消操作的成功提示
- [x] Task 1.1: 在 `sendAIStream` 中添加 `isCancelled` 标志位
- [x] Task 1.2: 修改 `cancelAIStream` 函数，设置 `isCancelled = true`
- [x] Task 1.3: 在 `catch` 块中检查 `isCancelled`，显示取消提示而不是错误提示
- [x] Task 1.4: 在 `error` 事件处理中也检查 `isCancelled` 标志

## 任务2：修复 AI 记忆对话历史功能
- [x] Task 2.1: 修改 `DefaultPrompt`，添加"记住对话历史"的指令
- [ ] Task 2.2: 测试 AI 是否能正确引用之前的对话内容

## 任务3：验证测试
- [ ] Task 3.1: 验证取消操作显示正确提示
- [ ] Task 3.2: 验证 AI 能正确回忆对话历史
- [x] Task 3.3: 后端编译验证
