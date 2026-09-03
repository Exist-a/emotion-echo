---
status: landed
superseded-by: stage-35-llm-fusion-hardening.md + adr-2026-09-llm-fusion-hardening.md
original-path: .trae/documents/上下文管理问题分析和修复方案.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 上下文管理问题分析和修复方案

## 问题现象

用户发送消息时：
- 第1条消息 → AI正常回复
- 第2条消息 → AI回复内容是第1条的（"你是谁"回复了"你好"相关）
- 第3条消息 → AI回复内容是第2条的
- 以此类推...

**关键观察**：第N条消息的回复总是第N-1条消息的内容。

## 问题根源分析

### 1. langchaingo Memory 的工作原理

langchaingo 的 `ConversationBuffer` 实现中，`LoadMemoryVariables()` 和 `SaveContext()` 共享同一个内部存储。

**关键问题**：`LoadMemoryVariables()` 每次调用都会清空内部 buffer，然后重新加载历史！

```go
// langchaingo memory.ConversationBuffer 内部逻辑（推测）
func (b *ConversationBuffer) LoadMemoryVariables(ctx context.Context, _ InputValues) (OutputValues, error) {
    // 问题在这里：先清空！
    b.messages = []schema.ChatMessage{}  // 清空！

    // 然后重新加载所有消息
    for _, msg := range b.history {
        b.messages = append(b.messages, msg)
    }

    return OutputValues{b.memoryKey: b.messages}, nil
}
```

### 2. 当前代码流程分析

**第1次请求（用户发送"你好"）：**
```
1. StreamChat: 用户消息"你好"保存到数据库
2. goroutine: 创建内存，加载0条历史
3. goroutine: LoadMessages() -> 返回[]（空）
4. goroutine: 调用LLM，生成回复
5. goroutine: SaveContext("你好", "你好！很高兴为你服务")
   → 内存现在: [用户:你好, AI:第一次回复]
```

**第2次请求（用户发送"你是谁"）：**
```
1. StreamChat: 用户消息"你是谁"保存到数据库
2. goroutine: 内存已存在，直接使用
3. goroutine: LoadMessages()
   → 关键问题触发！
   → LoadMemoryVariables() 清空内存
   → 然后... 加载什么？加载的是内存中之前保存的 [用户:你好, AI:第一次回复]
   → 返回: [用户:你好, AI:第一次回复]
4. goroutine: 调用LLM，应该看到: 用户:你好, AI:第一次回复, 用户:你是谁
5. goroutine: SaveContext("你是谁", "你是AI助手")
   → 内存现在: [用户:你好, AI:第一次回复, 用户:你是谁, AI:第二次回复]
```

**但问题是**：`LoadMessagesFromModels()` 只在**第一次创建内存时**调用！

### 3. 真正的Bug位置

**Bug在 `LoadMessages` 函数**：它调用 `LoadMemoryVariables()` 会清空内存，但此时还没有加载新消息！

```go
func (m *ConversationMemory) LoadMessages(ctx context.Context) ([]llms.ChatMessage, error) {
    vars, err := m.mem.LoadMemoryVariables(ctx, nil)  // ← 这里会清空内存！
    // ...处理返回值
}
```

当第2次请求时：
1. `exists := s.convMemoryCache[convID]` → true（内存已存在）
2. 进入 else 分支：**不会调用 `LoadMessagesFromModels`**
3. 调用 `convMem.LoadMessages(ctx)` → **但 LoadMessages 会清空内存！**
4. 返回的是内存中**当时**保存的消息，但时序有问题

### 4. 真正的根本原因

**内存管理器和调用时序的问题**：
1. `SaveContext` 保存用户+AI对
2. `LoadMessages` 调用 `LoadMemoryVariables` 清空并重新加载
3. 但第2次请求时，如果 `exists=true`，不会先加载数据库，而是直接使用内存
4. 此时内存中的消息顺序和内容可能不正确

## 修复方案

### 方案1：自定义内存管理（推荐）

不再依赖 langchaingo 的 `LoadMemoryVariables`，而是维护自己的消息列表：

```go
type ConversationMemory struct {
    cfg     *config.Config
    mem     schema.Memory
    llm     llms.Model
    history []llms.ChatMessage  // 自己维护历史
}

func (m *ConversationMemory) LoadMessages(ctx context.Context) ([]llms.ChatMessage, error) {
    // 直接返回自己维护的历史，不调用 LoadMemoryVariables
    return m.history, nil
}

func (m *ConversationMemory) SaveContext(ctx context.Context, userInput, aiOutput string) error {
    // 保存到自己的历史列表
    m.history = append(m.history, llms.TextParts(llms.ChatMessageTypeHuman, userInput))
    m.history = append(m.history, llms.TextParts(llms.ChatMessageTypeAI, aiOutput))

    // 同时保存到 langchaingo（为了兼容其他功能）
    inputValues := map[string]any{"input": userInput}
    outputValues := map[string]any{"output": aiOutput}
    return m.mem.SaveContext(ctx, inputValues, outputValues)
}
```

### 方案2：修改加载逻辑

确保每次使用内存前，先从数据库加载完整历史，再应用内存中的新消息。

### 方案3：简化内存管理

完全不使用 langchaingo 的内存功能，直接在服务层管理消息历史。

## 推荐实施步骤

1. **立即实施**：使用方案1，自定义内存管理
2. **修改文件**：`Emotion-Echo-Gin/internal/pkg/memory/memory.go`
3. **测试验证**：发送连续消息，确认上下文正确
4. **监控日志**：确认每次 LLM 调用都看到正确的完整历史

## 影响范围

- 修改文件：`internal/pkg/memory/memory.go`
- 影响功能：AI 对话上下文管理
- 风险等级：中（涉及核心功能，需要充分测试）
