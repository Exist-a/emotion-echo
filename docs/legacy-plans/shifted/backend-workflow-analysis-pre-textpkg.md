---
status: shifted
superseded-by: specs/text-workflow-refactor + stage-33-p0-fix-bff-purify.md
original-path: .trae/documents/后端工作流分析.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 后端工作流分析

## 概述

Emotion-Echo 后端存在 **3 个主要工作流**，分别位于不同的 package 下：

---

## 1. 情绪分析工作流（`chat` package）

**文件**: `internal/workflow/chat/workflow.go`

**用途**: 在用户发送消息时，同步分析用户情绪，为 AI 回复选择合适的系统提示词。

**节点链路**:
```
emotion_analysis → prompt_selector
```

**节点说明**:
| 节点 | 输入 | 输出 | 说明 |
|------|------|------|------|
| `emotion_analysis` | `state["message"]` | `state["emotion"]`, `state["confidence"]` | 调用 LLM 分析情绪 |
| `prompt_selector` | `state["emotion"]`, `state["confidence"]` | `state["system_prompt"]` | 根据情绪选择对应提示词 |

**情绪标签**: `happy`, `sad`, `angry`, `anxious`, `neutral`

**使用场景**: `AIService.StreamChat` 中同步执行，耗时约 200-500ms，失败时降级使用默认提示词。

---

## 2. 心理健康评估工作流（`assessment` package）

**文件**: `internal/workflow/assessment/workflow.go`

**用途**: 基于用户的对话历史、量表结果、行为活跃度等进行多维度心理健康评估。

**节点链路**:
```
Phase 1 (并行) → Phase 2 (ReAct循环) → Phase 3 → Phase 4 (条件分支) → Phase 5
```

**阶段说明**:

| Phase | 类型 | 节点 | 说明 |
|-------|------|------|------|
| Phase 1 | 并行 | `collect_messages`, `collect_surveys`, `collect_activity` | 收集消息、量表结果、行为活跃度 |
| Phase 2 | 循环 | `react_analysis` | LLM 多轮反思 + 工具调用进行深度分析 |
| Phase 3 | 顺序 | `calculate_risk` | 六维评分计算 + 综合风险评估 |
| Phase 4 | 条件分支 | `intervention` | 根据风险等级选择干预建议 |
| Phase 5 | 顺序 | `generate_report` | 生成警示标志和摘要 |

**评估维度**: 情绪、抑郁、焦虑、压力、社会支持

**风险等级**: `critical`, `high`, `medium`, `low`

**使用场景**:
- `Workflow.Execute()`: 按时间范围评估
- `Workflow.ExecuteForConversation()`: 针对单会话评估

---

## 3. 通用分析工作流（`workflow` package）

**文件**: `internal/workflow/engine.go`

**用途**: 通用分析流程，包含消息提取、情绪分析、关键词提取、摘要生成。

**节点链路**:
```
ExtractMessagesNode → EmotionAnalysisNode → KeywordExtractionNode → SummaryGenerationNode
```

**节点说明**:
| 节点 | 说明 |
|------|------|
| `ExtractMessagesNode` | 提取对话内容 |
| `EmotionAnalysisNode` | 情绪分析 |
| `KeywordExtractionNode` | 关键词提取 |
| `SummaryGenerationNode` | 生成摘要和建议 |

**使用场景**: 通用分析任务，通过 `AnalysisWorkflow()` 便捷函数调用。

---

## 工作流关系图

```
┌─────────────────────────────────────────────────────────────────┐
│                         AIService.StreamChat                      │
│                              │                                   │
│                              ▼                                   │
│                   ┌─────────────────────┐                        │
│                   │  chat.EmotionWorkflow │ ◄──情绪分析+Prompt选择 │
│                   └─────────────────────┘                        │
│                              │                                   │
│                              ▼                                   │
│                      AI 回复生成                                 │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│              assessment.Workflow (心理健康评估)                   │
│                              │                                   │
│          ┌───────────────────┼───────────────────┐               │
│          ▼                   ▼                   ▼               │
│   collect_messages    collect_surveys    collect_activity        │
│          │                   │                   │               │
│          └───────────────────┼───────────────────┘               │
│                              ▼                                   │
│                      react_analysis (ReAct)                      │
│                              │                                   │
│                              ▼                                   │
│                       calculate_risk                              │
│                              │                                   │
│                              ▼                                   │
│                    intervention (条件分支)                         │
│                              │                                   │
│                              ▼                                   │
│                      generate_report                             │
└─────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────┐
│              workflow.AnalysisWorkflow (通用分析)                │
│                              │                                   │
│          ┌───────────────────┼───────────────────┐               │
│          ▼                   ▼                   ▼               │
│   ExtractMessages   EmotionAnalysis   KeywordExtraction          │
│          │                   │                   │               │
│          └───────────────────┼───────────────────┘               │
│                              ▼                                   │
│                    SummaryGeneration                             │
└─────────────────────────────────────────────────────────────────┘
```

---

## 调用入口

| 工作流 | 入口文件 | 调用位置 |
|--------|----------|----------|
| `chat.EmotionWorkflow` | `cmd/server/main.go` | `aiService` 初始化时构建 |
| `assessment.Workflow` | `cmd/server/main.go` | `emotionWorker` / `scheduledJob` |
| `workflow.AnalysisWorkflow` | `cmd/server/main.go` | `emotionWorker` 初始化 |
