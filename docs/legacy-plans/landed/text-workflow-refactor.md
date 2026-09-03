---
status: landed
superseded-by: stage-33-p0-fix-bff-purify.md
original-path: .trae/specs/text-workflow-refactor/{spec,checklist,tasks}.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
note: 三份(spec/checklist/tasks)合并为一份
---

# 文字工作流架构重构（合并自 spec + checklist + tasks）

## 原始 spec.md

# 文字工作流架构重构 Spec

## Why

现有 `chat`、`workflow`、`assessment` 三个工作流存在节点重复（如 EmotionAnalysis）、边界模糊的问题。需要重构为可复用的组件体系。

---

## What Changes

### 1. 新增 `text` 包

创建 `internal/workflow/text/` 作为文字工作流统一入口，提供4个可复用节点。

### 2. 节点设计

| 节点 | 输入 | 输出 | 说明 |
|------|------|------|------|
| EmotionAnalysis | text | emotion, confidence | 复用 |
| PromptSelector | emotion | system_prompt | 仅在线 |
| KeywordExtraction | text/messages | keywords | 复用 |
| SummaryGeneration | messages | summary | 复用 |

**注意**: ResponseGeneration 不作为独立节点，AI 回复生成保持在 AIService 层处理（涉及 SSE 流式响应）。

### 3. 工作流组装

```
在线快速流: EmotionAnalysis → PromptSelector
离线分析流: EmotionAnalysis → KeywordExtraction → SummaryGeneration
```

### 4. 迁移策略

- **直接重构**：`chat`、`assessment` 直接改为调用 `text` 包的节点
- **废弃 workflow root**：`EmotionWorker` 改用 `text` 包，`workflow` root 包废弃删除

---

## Impact

### Affected Specs

- 情绪检测能力（复用）
- Prompt 选择能力（统一）
- 关键词/摘要生成（复用）

### Affected Code

| 文件 | 变化 |
|------|------|
| `internal/workflow/text/` | 新增 |
| `internal/workflow/chat/` | 重构，调用 text |
| `internal/workflow/assessment/` | 重构，调用 text |
| `internal/workflow/` | 废弃删除 |
| `internal/worker/emotion_worker.go` | 修改，调用 text |
| `cmd/server/main.go` | 修改初始化逻辑 |

---

## ADDED Requirements

### Requirement: 文字工作流节点可复用

系统 SHALL 提供4个独立的、可组合的文字处理节点，供在线和离线场景复用。

#### Scenario: 在线情绪检测
- **WHEN** 用户发送消息
- **THEN** 执行 EmotionAnalysis → PromptSelector 节点链

#### Scenario: 离线批量分析
- **WHEN** 后台任务触发批量分析
- **THEN** 执行 EmotionAnalysis → KeywordExtraction → SummaryGeneration 节点链

---

## MODIFIED Requirements

### Requirement: AIService 情绪检测

**MODIFIED**: AIService.StreamChat 内部不再自己实现情绪分析，而是调用 text.EmotionAnalysis 节点。

---

## REMOVED Requirements

### Requirement: workflow root 轻量分析

**Reason**: 功能已被 text 包覆盖，且与 chat.EmotionWorkflow 重复
**Migration**: EmotionWorker 改用 text 包

---

## 工作流组装规格

### 5.1 在线快速流（OnlineWorkflow）

```
EmotionAnalysis → PromptSelector
```

**输出**: system_prompt（供 AIService 生成 AI 回复）

### 5.2 离线分析流（OfflineWorkflow）

```
EmotionAnalysis → KeywordExtraction → SummaryGeneration
```

**输出**: keywords, summary（供报告生成使用）

---

## 包结构规格

```
internal/workflow/
├── text/                      # 新增：统一文字工作流
│   ├── nodes/
│   │   ├── emotion.go         # 情绪分析节点
│   │   ├── prompt.go          # Prompt选择节点
│   │   ├── keyword.go         # 关键词提取节点
│   │   └── summary.go          # 摘要生成节点
│   ├── workflow.go            # 工作流组装
│   └── state.go               # 状态定义
├── chat/                      # 重构：调用 text
├── assessment/               # 重构：调用 text
├── graph/                    # 保留：底层引擎
└── (root其他文件)             # 废弃：删除
```

---

## 原始 checklist.md

# Checklist - 文字工作流架构重构

## text 包基础结构

- [x] TextState 结构定义完整
- [x] 节点接口定义清晰
- [x] 工作流组装函数签名确定

## 节点实现

- [x] EmotionAnalysis 节点输入输出正确
- [x] PromptSelector 节点输入输出正确
- [x] KeywordExtraction 节点输入输出正确
- [x] SummaryGeneration 节点输入输出正确
- [x] 4个节点可独立运行测试

## 工作流组装

- [x] OnlineWorkflow 组装正确
- [x] OfflineWorkflow 组装正确
- [x] 节点间数据传递正确

## chat 包重构

- [x] chat 工作流调用 text 包正常
- [x] 情绪检测功能正常
- [x] Prompt 选择功能正常

## assessment 包重构

- [x] assessment 包无直接引用被删除的节点
- [x] assessment 工作流功能正常

## workflow root 废弃

- [x] emotion_worker 改用 text 包正常
- [x] workflow root 包文件已删除
- [x] 无残留引用

## 集成测试

- [x] main.go 编译通过
- [x] go build ./... 成功
- [x] 单元测试通过（如果存在）

---

## 原始 tasks.md

# Tasks - 文字工作流架构重构

## Phase 1: 创建 text 包基础结构

- [x] Task 1.1: 创建 `internal/workflow/text/state.go` - 定义 TextState 结构
- [x] Task 1.2: 创建 `internal/workflow/text/nodes/` - 创建节点目录
- [x] Task 1.3: 创建 `internal/workflow/text/workflow.go` - 定义工作流组装函数

---

## Phase 2: 实现4个核心节点

- [x] Task 2.1: 实现 `emotion.go` - EmotionAnalysis 节点
- [x] Task 2.2: 实现 `prompt.go` - PromptSelector 节点
- [x] Task 2.3: 实现 `keyword.go` - KeywordExtraction 节点
- [x] Task 2.4: 实现 `summary.go` - SummaryGeneration 节点

---

## Phase 3: 创建工作流工厂

- [x] Task 3.1: 实现 `NewOnlineWorkflow()` - 在线快速流组装
- [x] Task 3.2: 实现 `NewOfflineWorkflow()` - 离线分析流组装

---

## Phase 4: 重构 chat 包

- [x] Task 4.1: 修改 `chat/workflow.go` - 改为调用 text 包的节点
- [x] Task 4.2: 修改 `chat/nodes.go` - 删除已迁移的节点代码
- [x] Task 4.3: 测试 chat 工作流功能正常

---

## Phase 5: 重构 assessment 包

- [x] Task 5.1: assessment 包无直接引用被删除的节点，无需修改
- [x] Task 5.2: 测试 assessment 工作流功能正常

---

## Phase 6: 废弃 workflow root

- [x] Task 6.1: 修改 `emotion_worker.go` - 改为调用 text 包
- [x] Task 6.2: 删除 `workflow/` 目录下的 root 包文件（保留 graph/）

---

## Phase 7: 更新 main.go

- [x] Task 7.1: main.go 无需修改，chat.BuildEmotionWorkflow 仍然有效
- [x] Task 7.2: 编译测试整个后端项目 ✓

---

## 重构完成

- [x] text 包创建完成，包含 4 个可复用节点
- [x] chat 包重构完成，调用 text 包
- [x] workflow root 废弃，emotion_worker 改用 text 包
- [x] 全项目编译通过
