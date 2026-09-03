---
status: landed
superseded-by: stage-26-M-coverage.md + stage-26-N-bugfix.md
original-path: .trae/documents/图表渲染与上下文携带修复计划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 分类会话图表渲染 & 上下文携带 问题修复计划

## 问题概述

### 问题1：分类会话图表未渲染
- **原因**：前端报表页面（日报、周报、月报、年报）只渲染了情绪分布饼图和情绪趋势折线图
- **缺失内容**：疏导占比（EmotionalSupportRate）数据没有对应的图表渲染
- **影响**：用户无法直观看到情感疏导类问题在所有问题中的占比

### 问题2：AI对话未携带上下文
- **原因**：在 `ai_service.go` 中生成 AI 回复时，构建 Prompt 时只传入了当前用户消息，没有包含历史对话
- **影响**：每次对话都是独立的，AI 无法理解对话的连续性

---

## 修复方案

### 修复1：添加疏导占比图表

#### 1.1 修改日报页面 (`dailyReport.vue`)
**目标**：在 `chartData` computed 属性中添加疏导占比的柱状图

**修改内容**：
```typescript
// 在 chartData computed 中添加
if (reportData.value.emotionalSupportRate !== undefined) {
  items.push({
    chartType: 'bar',
    title: '疏导占比',
    XData: ['情感疏导', '其他问题'],
    YData: [reportData.value.emotionalSupportRate, 100 - reportData.value.emotionalSupportRate]
  })
}
```

#### 1.2 修改周报/月报/年报页面
**目标**：在趋势报表中添加疏导占比的折线图

**修改内容**：
- 在 `chartData` 中添加一个新的 series，追踪疏导占比的变化趋势

---

### 修复2：使用 LangChain 管理对话上下文（重点）

#### 2.1 现有 LangChain 使用情况
项目已有 `github.com/tmc/langchaingo v0.1.14` 依赖，在 `emotion_worker.go` 中有简单使用

#### 2.2 方案：使用 LangChain 的 Prompt 模板和对话历史管理

**创建 LLM 服务层** (`internal/pkg/llm/chain.go`)：
```go
package llm

import (
    "context"
    "fmt"

    "github.com/tmc/langchaingo/llms"
    "github.com/tmc/langchaingo/prompts"
    "github.com/tmc/langchaingo/schema"
)

// Chain LLM Chain
type Chain struct {
    llm      llms.Model
    prompt   prompts.ChatPromptTemplate
}

// NewChain 创建新的 Chain
func NewChain(cfg *Config) (*Chain, error) {
    // 初始化 LLM（支持 Kimi）
    llm, err := NewClient(cfg)
    if err != nil {
        return nil, err
    }

    // 创建提示词模板
    prompt := prompts.ChatPromptTemplateFromTemplate(`
        {{.system_prompt}}

        对话历史：
        {{- range .history }}
        {{.Role}}: {{.Content}}
        {{- end }}

        当前用户消息：{{.input}}

        请根据以上对话历史和系统提示词回复用户。
    `)

    return &Chain{
        llm:    llm,
        prompt: prompt,
    }, nil
}

// Call 调用 Chain
func (c *Chain) Call(ctx context.Context, systemPrompt, userInput string, history []schema.ChatMessage) (string, error) {
    // 构建提示词
    promptValue, err := c.prompt.Format(map[string]interface{}{
        "system_prompt": systemPrompt,
        "history":       history,
        "input":         userInput,
    })
    if err != nil {
        return "", err
    }

    // 调用 LLM
    resp, err := c.llm.Call(ctx, promptValue.String(),
        llms.WithTemperature(0.7),
        llms.WithMaxTokens(500),
    )
    if err != nil {
        return "", err
    }

    return resp, nil
}
```

#### 2.3 修改 `ai_service.go`

**修改内容**：
1. 注入 LLM Chain 到 AIService
2. 在生成 AI 回复时，使用 Chain 而不是手动拼接 Prompt
3. 传入对话历史

**核心代码变更**：
```go
// AIService 中添加 chain 字段
type AIService struct {
    // ... 其他字段
    chain *llm.Chain
}

// 修改 StreamChat 方法中的 AI 回复生成
go func() {
    // ... 发送 start 事件

    // 使用 LangChain 管理上下文
    var fullResponse string
    if s.chain != nil {
        // 获取历史消息
        messages, _ := s.msgService.List(ctx, userID, convID, 20, 0)

        // 转换为 LangChain 格式
        history := convertToLangChainMessages(messages)

        // 调用 Chain
        fullResponse, err = s.chain.Call(ctx, systemPrompt, req.Message, history)
    } else {
        // 降级处理
        fullResponse = "抱歉，我现在无法回复您。"
    }

    // ... 后续处理
}()
```

#### 2.4 转换函数
```go
func convertToLangChainMessages(messages []*models.Message) []schema.ChatMessage {
    var history []schema.ChatMessage
    for _, msg := range messages {
        if msg.Sender == "user" {
            history = append(history, schema.HumanChatMessage{Content: msg.Content})
        } else {
            history = append(history, schema.AIChatMessage{Content: msg.Content})
        }
    }
    return history
}
```

#### 2.5 修改 main.go 注入 Chain
```go
// 在 main.go 中
llmChain, err := llm.NewChain(cfg)
if err != nil {
    logger.Fatal("Failed to create LLM chain", zap.Error(err))
}

aiService := service.NewAIService(..., llmChain)
```

---

## 实施步骤

### 阶段1：图表渲染修复（优先级高）
1. 修改 `dailyReport.vue` 添加疏导占比柱状图
2. 修改 `weeklyReport.vue` 添加疏导占比折线图
3. 修改 `monthlyReport.vue` 添加疏导占比折线图
4. 修改 `annualReport.vue` 添加疏导占比折线图
5. 测试验证图表渲染正常

### 阶段2：上下文携带修复（使用 LangChain）
1. 创建 `internal/pkg/llm/chain.go`，封装 LangChain 的 Chain
2. 在 `AIService` 中添加 `chain` 字段
3. 修改 `StreamChat` 方法，使用 Chain 管理对话上下文
4. 修改 `main.go`，注入 LLM Chain
5. 测试验证 AI 能理解对话上下文

---

## 技术优势

### 使用 LangChain 的好处
1. **Prompt 管理更规范**：模板化、结构化
2. **对话历史管理更方便**：自动处理历史消息格式
3. **易于扩展**：后续可以添加 Memory、Tool 等组件
4. **减少错误**：避免手动拼接 Prompt 带来的格式问题

---

## 风险评估

### 风险1：LangChain 版本兼容性
- **影响**：v0.1.14 可能存在 bug
- **缓解措施**：参考 emotion_worker.go 中的使用方式

### 风险2：Prompt 过长
- **影响**：历史消息过多可能导致 Token 超出限制
- **缓解措施**：限制历史消息数量为20条，并在 Chain 中设置 MaxTokens

### 风险3：图表数据为空
- **影响**：疏导占比为0时，图表可能显示不友好
- **缓解措施**：当数据为0时，显示默认值或隐藏图表

---

## 预期结果

1. **图表渲染**：用户可以在日报和趋势报表中看到疏导占比的可视化图表
2. **上下文理解**：AI 可以通过 LangChain 的 Chain 自动管理对话历史，提供更连贯的回复
