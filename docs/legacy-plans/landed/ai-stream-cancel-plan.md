---
status: landed
superseded-by: specs/ai-stream-cancel-fix + stage-30-C-browser-testing.md
original-path: .trae/documents/ai-stream-cancel-feature/plan.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# AI 回复截断功能实现计划

## 功能需求

用户点击发送按钮后，按钮变为**红色停止键**，点击可中断正在流式返回的AI回复，已接收的部分内容将被保存。

---

## 实现方案

### 一、前端修改 ([id].vue](file:///d:\源码\Emotion-Echo\Emotion-Echo-Web\app\pages\chat\conversation\[id].vue))

#### 1. 添加状态变量
- 新增 `isStreaming` 状态追踪AI回复状态
- 计算属性判断按钮显示：流式中显示停止按钮，非流式中显示发送按钮

#### 2. 修改发送按钮
- 发送按钮使用 `el-button`
- 流式状态时：
  - `type="danger"` (红色)
  - `icon="CircleClose"` (停止图标)
  - 点击调用 `cancelAIStream`
- 非流式状态时：
  - `type="primary"` (蓝色)
  - `icon="Promotion"` (发送图标)
  - 点击调用 `handleSubmit`

#### 3. 禁用输入框
- 流式状态时输入框禁用，防止输入新内容

### 二、前端 Store 修改 ([message.ts](file:///d:\源码\Emotion-Echo\Emotion-Echo-Web\app\stores\message.ts))

#### 1. 已有功能确认
- `cancelAIStream()` 已实现，可中断 SSE 请求
- `isStreaming` 状态已存在

#### 2. 修改 onFinish 回调
- 当收到 `finish` 事件时，检查是否是截断的回复
- 部分回复时，状态设为 `truncated`
- 完整回复时，状态设为 `sent`

### 三、后端修改 ([ai_handler.go](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\handler\ai_handler.go))

#### 1. 添加截断事件类型
在 `StreamEvent` 中添加 `truncated` 类型标识

#### 2. 检测客户端断开
- 当 `ctx.Done()` 触发时（客户端断开/取消请求）
- 发送 `truncated` 事件到客户端
- 保存已生成的回复内容到数据库

---

## 实施步骤

### 步骤 1：前端 - 修改发送按钮 UI
1. 导入 `CircleClose` 图标
2. 添加 `isStreaming` 计算属性
3. 修改发送按钮：
   - 使用 `v-if/v-else` 判断显示发送或停止按钮
   - 流式时显示红色停止按钮，点击调用 `messageStore.cancelAIStream()`

### 步骤 2：前端 - 修改输入框禁用状态
1. 流式时禁用输入框
2. 添加 `:disabled="isStreaming"` 到 el-input

### 步骤 3：前端 - 修改 messageStore 的 finish 处理
1. 确认 `onFinish` 回调正确处理截断情况
2. 部分回复的状态标记为 `truncated`

### 步骤 4：后端 - 添加截断事件支持
1. 在 `StreamEvent` 中添加 `truncated` 类型
2. 修改流式循环，检测 `ctx.Done()` 时发送截断事件
3. 在 `StreamChat` 中检测到截断时，保存部分回复

### 步骤 5：后端 - 保存部分回复
1. 当客户端取消时，获取已生成的 `fullResponse`
2. 调用 `SaveAIResponse` 保存部分内容
3. 标记为截断状态（可选）

---

## 关键代码位置

| 文件 | 修改内容 | 关键函数 |
|------|---------|---------|
| `id.vue` | 发送按钮UI | `handleSubmit`, `isStreaming` |
| `message.ts` | 流式状态管理 | `sendAIStream`, `cancelAIStream` |
| `ai_handler.go` | SSE截断处理 | `Stream`, `sendSSEvent` |
| `ai_service.go` | 保存截断回复 | `StreamChat` |

---

## 预期效果

1. **用户发送消息后**：发送按钮变为红色停止按钮
2. **用户点击停止**：
   - 流式中断
   - 已收到的回复内容被保存
   - 按钮恢复为发送按钮
   - 输入框恢复可输入状态
3. **消息显示**：截断的消息带有特殊标记（如状态显示"已截断"）
