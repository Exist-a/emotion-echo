---
status: historical
superseded-by: text-workflow-refactor.md（仅作决策回顾保留）
original-path: .trae/documents/文字工作流架构设计方案.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-尾声
note: 早期 3 节点方案；最终落地为 text-workflow-refactor 的 4 节点方案
---

# 文字工作流架构设计方案

## 目标

**本次任务**: 仅设计文字工作流架构，不涉及代码重构

**定位**: 为未来代码重构提供清晰的架构蓝图

---

## 一、现状分析

### 现有三个工作流的问题

| 工作流 | 问题 |
|--------|------|
| `chat.EmotionWorkflow` | 情绪分析 + Prompt选择，节点耦合 |
| `workflow.AnalysisWorkflow` | 轻量分析，与 chat 有重复节点 |
| `assessment.Workflow` | 深度分析，5阶段复杂流程 |

**核心问题**: 各工作流节点不可复用，边界模糊

---

## 二、设计方案

### 2.1 组件节点设计（4个核心节点）

| 节点 | 输入 | 输出 | 复用范围 |
|------|------|------|----------|
| **EmotionAnalysis** | text | emotion, confidence | 所有工作流 |
| **PromptSelector** | emotion | system_prompt | 仅在线 |
| **KeywordExtraction** | text/messages | keywords | 离线分析 |
| **SummaryGeneration** | messages | summary | 离线分析 |

### 2.2 工作流组装模式

```
模式1: 在线快速流
  EmotionAnalysis → PromptSelector → ResponseGeneration

模式2: 离线轻量流
  EmotionAnalysis → KeywordExtraction → SummaryGeneration

模式3: 离线深度流
  EmotionAnalysis → KeywordExtraction → SummaryGeneration
           ↓
  (进入 assessment 深度分析阶段)
```

### 2.3 统一状态管理

```go
// TextState 文字工作流统一状态
type TextState struct {
    // 输入
    Text       string   // 原始文字
    Messages   []Message // 消息列表（离线用）

    // 中间状态
    Emotion    string   // 情绪标签
    Confidence float64  // 置信度
    Keywords   []string // 关键词
    Summary   string   // 摘要

    // 输出
    SystemPrompt string   // 系统提示词（仅在线）
    AIResponse  string   // AI回复（仅在线）
}
```

---

## 三、代码结构设计（仅供参考）

### 3.1 包结构

```
internal/workflow/
├── text/                      # 文字工作流（新增）
│   ├── nodes/
│   │   ├── emotion.go         # 情绪分析节点
│   │   ├── prompt.go          # Prompt选择节点
│   │   ├── keyword.go         # 关键词提取节点
│   │   └── summary.go          # 摘要生成节点
│   ├── workflow.go            # 工作流组装
│   └── state.go               # 状态定义
├── chat/                      # 保留（重构后调用 text）
├── assessment/                # 保留（重构后调用 text）
└── graph/                     # 底层引擎（不变）
```

### 3.2 节点接口

```go
type Node interface {
    Name() string
    Execute(ctx context.Context, state *TextState) error
}
```

### 3.3 工作流工厂

```go
// 在线模式
func NewOnlineWorkflow(llmCaller LLMCaller) *Graph

// 离线轻量模式
func NewOfflineWorkflow(llmCaller LLMCaller) *Graph
```

---

## 四、迁移策略（未来）

| 阶段 | 工作 | 影响 |
|------|------|------|
| Phase 1 | 新增 `text` 包，实现4个节点 | 无影响 |
| Phase 2 | `chat` 包改为调用 `text` | 小范围修改 |
| Phase 3 | `assessment` 复用 `text` 节点 | 小范围修改 |
| Phase 4 | 废弃/合并 `workflow` root 包 | 需测试验证 |

---

## 五、总结

| 设计原则 | 说明 |
|----------|------|
| **节点独立** | 每个节点职责单一，可独立测试 |
| **组合灵活** | 不同工作流按需组装节点 |
| **状态统一** | TextState 作为标准输入输出 |
| **平滑迁移** | 新旧并行，逐步切换 |

---

## 下一步

如需将此设计落地为代码重构计划，请告知。
