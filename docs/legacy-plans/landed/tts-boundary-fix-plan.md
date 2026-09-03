---
status: landed
superseded-by: stage-35-production-hardening.md
original-path: .trae/documents/tts_boundary_fix_plan.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# TTS 边界处理修复计划

## 问题分析

### 1. 静音按钮不工作
**根因**：DigitalHuman组件和[id].vue使用不同的响应式状态，导致静音状态不同步。

**当前流程**：
- DigitalHuman组件：点击静音 → 修改 `digitalHumanStore.voiceEnabled` → 更新组件本地 `voiceEnabled`
- [id].vue：watch `digitalHumanStore.voiceEnabled` → 调用 `setVoiceEnabled()`

**问题**：两个组件的 `useDigitalHumanTTS` 是不同的实例，虽然都从同一个store读取状态，但内部的 `voiceEnabled` ref 可能存在时序问题。

### 2. 发送新消息时不断开当前TTS
**当前流程**：
- 用户发送消息 → `handleSubmit` → `sendAIStream` → AI开始回复 → TTS开始播放
- 如果用户再次发送消息，TTS播放不会中断

**期望流程**：
- 用户发送消息时，应该立即停止当前的TTS播放和AI流接收

### 3. 其他边界情况
- 快速连续发送多条消息
- AI回复过长时的处理
- 网络中断处理

## 修改文件清单

### 1. 修复静音按钮
**文件**: `Emotion-Echo-Web/app/components/digital-human/DigitalHuman.vue`
- 将静音按钮改为触发自定义事件，而不是直接修改store
- 父组件负责处理静音逻辑

### 2. 发送消息时中断TTS
**文件**: `Emotion-Echo-Web/app/pages/chat/conversation/[id].vue`
- 在 `handleSubmit` 中调用 `stop()` 停止当前TTS播放
- 清除累积的文本和定时器

### 3. 降低语速
**文件**: `Emotion-Echo-LLM/XTTS/server.py`
- 将默认 `speed` 从 0.9 改为 0.75（更慢的语速）

## 实现步骤

### Step 1: 修复静音按钮
1. 在 DigitalHuman.vue 中，将 `handleToggleVoice` 改为触发 `voiceToggle` 事件
2. 在 [id].vue 中监听该事件并处理静音逻辑

### Step 2: 发送消息时中断TTS
1. 在 [id].vue 的 `handleSubmit` 开始时调用 `stop()`
2. 清除 `accumulatedDeltaText`、`currentEmotion`、`ttsDebounceTimer`

### Step 3: 调整语速
1. 修改 server.py 中的 `speed` 默认值为 0.75

## 参数调整

| 参数 | 当前值 | 新值 | 说明 |
|------|--------|------|------|
| speed | 0.9 | 0.75 | 更慢的语速 |
| volume | 2.0 | 2.0 | 保持音量不变 |

## 测试要点

1. 点击静音按钮后，AI继续回复时不应该发出声音
2. 发送新消息时，当前的TTS播放应该立即停止
3. 语速应该比之前更慢