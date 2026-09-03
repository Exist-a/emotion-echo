---
status: landed
superseded-by: stage-30-A + stage-30-C
original-path: .trae/documents/上下文管理和消息存储问题检查与修复计划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 上下文管理和消息存储问题检查与修复计划

## 问题分析

从用户反馈和日志分析，发现以下问题：
1. 数据库中出现重复消息（如连续两条"你好"）
2. LLM 回复基于旧上下文（如第二条消息"你是谁"仍然回复"你好"相关的内容）
3. 前端可能存在重复请求调用

## 根本原因分析

### 1. Memory 模块问题（已修复 ✓）
**问题**：`LoadMessagesFromModels` 每次调用都重新创建内存实例，导致之前保存的消息丢失

**修复**：改为直接向现有内存 `SaveContext`，不再重新创建

### 2. 需要检查的其他潜在问题

#### 2.1 前端重复调用检测
- 检查 `sendAIStream` 是否有并发调用
- 检查是否有多个地方同时调用消息发送
- 检查 `skipUserMessage` 逻辑是否正确

#### 2.2 后端数据库写入重复
- 检查 `StreamChat` 是否在某些情况下重复保存用户消息
- 检查 `SaveAIResponse` 是否可能被多次调用
- 检查数据库是否有唯一约束防止重复

#### 2.3 消息列表查询逻辑
- 检查 `ListByConversationID` 查询是否正确
- 检查是否有数据排序问题

## 检查清单

### 前端检查项
- [ ] `sendAIStream` 函数是否在组件卸载时被正确取消
- [ ] 是否有并发调用 `sendAIStream` 的地方
- [ ] `triggerAIWithVoiceContext` 是否可能导致重复调用
- [ ] 消息列表加载逻辑是否会导致重复渲染

### 后端检查项
- [ ] `StreamChat` 中用户消息保存逻辑是否有重复
- [ ] `SaveAIResponse` 是否可能被多次调用
- [ ] 数据库事务是否正确处理
- [ ] Memory 缓存与会话生命周期的关系

### 数据库检查项
- [ ] 检查 messages 表是否有唯一约束
- [ ] 检查是否有遗留测试数据

## 修复步骤

### 步骤 1：前端去重和重复调用防护
1. 在 `sendAIStream` 中增强 `isStreaming` 标志检查
2. 确保组件卸载时正确取消请求
3. 在 `triggerAIWithVoiceContext` 中添加防护逻辑

### 步骤 2：后端消息保存去重
1. 在 `StreamChat` 中添加消息去重逻辑
2. 在数据库层面添加消息唯一标识

### 步骤 3：验证和测试
1. 重启后端服务
2. 测试连续发送消息的场景
3. 检查后端日志确认消息流程正确

## 影响范围

### 需要修改的文件
1. `Emotion-Echo-Web/app/stores/message.ts` - 前端消息发送逻辑
2. `Emotion-Echo-Gin/internal/service/ai_service.go` - 后端消息保存逻辑
3. `Emotion-Echo-Gin/internal/pkg/memory/memory.go` - Memory 模块（已修复）

### 风险评估
- 前端修改风险：低（主要是添加状态检查）
- 后端修改风险：中（涉及消息保存逻辑）
- 数据库修改风险：中（如果添加约束）

## 实施顺序
1. 先检查前端和后端的重复调用问题
2. 添加必要的去重逻辑
3. 测试验证修复效果
4. 如有需要再调整数据库结构
