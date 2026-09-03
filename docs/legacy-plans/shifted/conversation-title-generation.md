---
status: shifted
superseded-by: stage-30-C-browser-testing.md + stage-32-landing.md（前端会话列表显示由 BFF 阶段实现，AI 生成标题部分未完整收口）
original-path: .trae/documents/conversation-title-generation/{spec,checklist,tasks}.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-B
note: 三份(spec/checklist/tasks)合并为一份
---

# 智能会话标题生成（合并自 spec + checklist + tasks）

## 原始 spec.md

# 智能会话标题生成 Spec

## Why
新对话创建时，用户的第一条消息被用作默认标题，但这种方式不够智能。需要根据用户消息内容AI生成一个简短、精准的标题。

## What Changes
- 后端新增标题生成节点，在AI回复完成后生成会话标题
- 前端会话列表显示AI生成的标题
- 标题生成失败时降级为截取消息前10个字符

## Impact
- Affected specs: 会话管理、消息模块
- Affected code:
  - 后端: `internal/service/conversation_service.go`、`internal/service/ai_service.go`
  - 前端: `app/stores/conversation.ts`、会话列表组件

## ADDED Requirements

### Requirement: 智能标题生成
系统应在用户发送第一条消息且AI回复完成后，自动生成会话标题。

#### Scenario: AI回复完成后生成标题
- **WHEN** 用户发送新消息创建新对话，且AI完成回复
- **THEN** 系统调用LLM生成不超过10个字符的标题，并更新会话记录

#### Scenario: AI生成失败时降级处理
- **WHEN** LLM调用失败或超时
- **THEN** 系统使用消息内容前10个字符作为标题

### Requirement: 标题展示
前端会话列表应显示AI生成的标题。

#### Scenario: 会话列表显示
- **WHEN** 用户打开会话列表
- **THEN** 每个会话项显示该会话的标题（而非默认的"新对话"或首消息前20字）

## MODIFIED Requirements

### Requirement: 会话创建流程
**原流程**: 创建会话时使用用户首消息前20字符作为标题
**新流程**: 创建会话时不立即设置标题，等待AI回复完成后由AI生成标题

## 标题生成Prompt
```
请根据用户的首条消息生成一个简短的会话标题。

要求：
1. 不超过10个中文字符
2. 能准确概括用户意图
3. 不要使用引号包裹
4. 直接输出标题，不要添加解释

用户消息：{user_message}
```

## 降级策略
- 如果LLM调用失败（超时、API错误等）
- 自动使用消息内容前10个字符作为标题
- 如果消息不足10个字符，则使用全部内容

---

## 原始 checklist.md

# Checklist

## 后端实现

- [ ] `GenerateTitle` 方法已创建并包含Prompt模板
- [ ] `GenerateTitle` 实现了降级逻辑（截取前10字符）
- [ ] `UpdateTitle` 方法已创建
- [ ] `ai_service.go` 在AI回复完成后调用标题生成
- [ ] 会话标题正确更新到数据库

## 前端实现

- [ ] 会话列表组件正确显示标题字段
- [ ] 标题更新后列表能实时刷新显示

## 功能测试

- [ ] 新对话标题生成正常（AI生成）
- [ ] AI生成失败时降级为截取字符
- [ ] 标题在会话列表中正确显示

---

## 原始 tasks.md

# Tasks

## Task 1: 后端 - 添加标题生成方法
- [ ] 创建 `GenerateTitle` 方法：封装LLM调用生成标题的逻辑
- [ ] 设计标题生成Prompt模板
- [ ] 实现降级处理逻辑（失败时截取前10字符）
- [ ] 依赖：无

## Task 2: 后端 - 修改AI回复流程，在回复完成后更新会话标题
- [ ] 在 `ai_service.go` 的 `StreamChat` 方法中，AI回复完成后调用标题生成
- [ ] 调用 `convService.UpdateTitle` 更新会话标题
- [ ] 依赖：Task 1

## Task 3: 后端 - 添加UpdateTitle接口（如不存在）
- [ ] 在 `ConversationService` 中添加 `UpdateTitle` 方法
- [ ] 在 `ConversationRepository` 中添加更新标题的数据库操作
- [ ] 依赖：无

## Task 4: 前端 - 会话列表显示真实标题
- [ ] 确认后端返回的会话数据包含标题字段
- [ ] 检查前端会话列表组件是否正确渲染标题
- [ ] 依赖：Task 2（需要后端API返回标题）

## Task 5: 测试验证
- [ ] 创建新对话，验证标题生成
- [ ] 验证标题正确显示在会话列表
- [ ] 验证AI生成失败时的降级处理
