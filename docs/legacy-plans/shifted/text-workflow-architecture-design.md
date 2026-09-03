---
status: shifted
superseded-by: text-workflow-refactor.md（4 节点方案，取代本设计的 3 节点）
original-path: .trae/documents/文字工作流架构设计.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-A
---

# 文字工作流架构重构方案

## 用户愿景

- **当前**: 仅文字传输
- **未来**: 扩展语音情绪识别
- **目标**: 建立可复用的工作流组件体系

---

## 核心理念：可组合工作流

```
┌─────────────────────────────────────────────────────────────┐
│                     统一入口（文字工作流）                     │
│                                                             │
│   输入: 文字消息                                             │
│   输出: AI 回复 + 情绪标签 + 上下文                           │
└─────────────────────────────────────────────────────────────┘
                            │
          ┌─────────────────┼─────────────────┐
          ▼                 ▼                 ▼
   ┌─────────────┐  ┌─────────────┐  ┌─────────────┐
   │ 情绪检测节点 │  │ Prompt选择节点│  │ 关键词提取节点│
   │ (可复用)    │  │ (可复用)     │  │ (可复用)     │
   └─────────────┘  └─────────────┘  └─────────────┘
          │                 │                 │
          └─────────────────┼─────────────────┘
                            ▼
                   ┌─────────────────┐
                   │ 响应生成节点     │
                   │ (AI对话)        │
                   └─────────────────┘
```

---

## 建议的工作流组件化

### 1. 情绪分析（Emotion Analysis）
- **输入**: 原始文字
- **输出**: 情绪标签 + 置信度
- **用途**:
  - `chat` 工作流（实时）
  - `assessment` 工作流（离线）
  - 未来语音情绪识别也可复用

### 2. Prompt 选择（Prompt Selection）
- **输入**: 情绪标签
- **输出**: 系统提示词
- **用途**: 仅 `chat` 工作流使用

### 3. 关键词提取（Keyword Extraction）
- **输入**: 文字
- **输出**: 关键词列表
- **用途**:
  - `chat` 工作流
  - `assessment` 工作流

### 4. 摘要生成（Summary Generation）
- **输入**: 消息列表
- **输出**: 摘要文本
- **用途**:
  - `EmotionWorker` 批量处理
  - `assessment` 工作流

---

## 重构后的工作流边界

### 层级清晰

```
┌─────────────────────────────────────────────────────────────┐
│  文字工作流（TextWorkflow）                                    │
│  ├── 情绪分析（复用）                                         │
│  ├── Prompt选择（仅在线）                                     │
│  ├── 关键词提取（复用）                                       │
│  └── 响应生成                                                 │
└─────────────────────────────────────────────────────────────┘
          │
          ├── 在线: AIService.StreamChat
          │         └── TextWorkflow（快速模式）
          │
          ├── 离线轻量: EmotionWorker
          │         └── TextWorkflow（批处理模式，不含Prompt选择）
          │
          └── 离线深度: MentalHealthService
                    └── TextWorkflow（分析模式）+ AssessmentWorkflow
```

### 语音扩展预留

```
┌─────────────────────────────────────────────────────────────┐
│                      多模态工作流                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│   ┌─────────────┐     ┌─────────────┐                       │
│   │  语音输入    │────▶│  ASR 转文字  │────┐                │
│   └─────────────┘     └─────────────┘     │                │
│                                            ▼                │
│                                     ┌───────────┐           │
│                                     │  TextWorkflow │◄── 复用  │
│                                     └───────────┘           │
│                                            │                 │
│                                            ▼                 │
│                                     ┌───────────┐           │
│                                     │ 语音情感识别 │◄── 新增  │
│                                     │ (未来扩展)  │           │
│                                     └───────────┘           │
└─────────────────────────────────────────────────────────────┘
```

---

## 具体实现建议

### 组件节点设计

```go
// 情绪分析节点（可复用）
type EmotionAnalysisNode struct{}

func (n *EmotionAnalysisNode) Execute(ctx context.Context, state State) (State, error) {
    // 提取文字
    text := state.GetString("text")
    // 调用 LLM 分析情绪
    emotion := analyzeEmotion(text)
    // 输出
    state.Set("emotion", emotion.Label)
    state.Set("confidence", emotion.Confidence)
    return state, nil
}

// Prompt 选择节点（可复用）
type PromptSelectorNode struct{}

func (n *PromptSelectorNode) Execute(ctx context.Context, state State) (State, error) {
    emotion := state.GetString("emotion")
    prompt := selectPrompt(emotion)
    state.Set("system_prompt", prompt)
    return state, nil
}
```

### 工作流组装

```go
// 文字工作流（在线模式：情绪分析 + Prompt选择 + 响应生成）
func BuildTextWorkflowOnline(llmCaller LLMCaller) *Graph {
    g := NewGraph("text_online")
    g.AddNode(NewEmotionAnalysisNode(llmCaller))    // 复用
    g.AddNode(NewPromptSelectorNode())               // 复用
    g.AddNode(NewResponseGenerationNode(llmCaller))  // 新增
    g.AddEdge("emotion_analysis", "prompt_selector")
    g.AddEdge("prompt_selector", "response_generation")
    return g
}

// 文字工作流（离线模式：情绪分析 + 关键词提取 + 摘要）
func BuildTextWorkflowOffline(llmCaller LLMCaller) *Graph {
    g := NewGraph("text_offline")
    g.AddNode(NewEmotionAnalysisNode(llmCaller))    // 复用
    g.AddNode(NewKeywordExtractionNode(llmCaller))  // 复用
    g.AddNode(NewSummaryGenerationNode(llmCaller))   // 复用
    g.AddEdge("emotion_analysis", "keyword_extraction")
    g.AddEdge("keyword_extraction", "summary_generation")
    return g
}
```

---

## 总结

| 原则 | 说明 |
|------|------|
| **组件化** | 情绪分析、Prompt选择、关键词提取、摘要生成都作为独立可复用节点 |
| **分层** | 在线工作流、离线轻量工作流、离线深度工作流，各自按需组装 |
| **扩展性** | 未来语音输入时，只需在 TextWorkflow 之前增加 ASR 节点 |
| **统一入口** | TextWorkflow 作为文字处理的统一入口，对外屏蔽内部复杂度 |

这个设计思路清晰，完全可行。需要我进一步输出具体的代码结构设计吗？
