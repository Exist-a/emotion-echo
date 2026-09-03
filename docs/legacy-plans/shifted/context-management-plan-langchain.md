---
status: shifted
superseded-by: stage-35-llm-fusion-hardening.md + adr-2026-09-llm-fusion-hardening.md
original-path: .trae/documents/context_management_plan.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# 企业级上下文管理实现方案（基于 tmc/langchaingo

## 一、现状分析

### 当前实现（存在问题）
1. **简单全量携带**：在 `ai_service.go:244` 直接取最近 **20 条**历史消息
2. **无Token限制**：没有Token计算和截断机制，可能超出模型上下文窗口
3. **无消息压缩**：直接使用原始消息内容，没有摘要或压缩
4. **无配置化**：参数硬编码，无法灵活调整

### 好消息！（已有的完美工具！
我们已经有 `tmc/langchaingo v0.1.14` 依赖！它已经提供了完整的上下文管理模块！

| 组件 | 功能说明 |
|-------|---------|
| `memory.ConversationTokenBuffer` | 基于 **精确Token管理（我们的核心需求！） |
| `memory.ConversationWindowBuffer` | 基于窗口大小管理 |
| `memory.ConversationBuffer` | 简单全量历史 |
| `memory.ChatMessageHistory` | 消息历史底层存储 |

---

## 二、整体方案设计

### 核心方案：直接使用 langchain 内存模块！
这将大大减少手写代码！几乎不需要自己写Token管理、截断等逻辑！

### 选择的策略
1. **优先使用 ConversationTokenBuffer（基于Token）
2. **配置化：支持Token限制
3. **集成到现有AIService

---

## 三、详细实现步骤

### 1. 配置层修改
**文件：** `internal/config/config.go`

**新增配置项：**
```go
type AIConfig struct {
    // ... 现有配置 ...
    Context ContextConfig `mapstructure:"context"`
}

type ContextConfig struct {
    Type           string `mapstructure:"type"` // "token" or "window" or "buffer"
    MaxTokens      int    `mapstructure:"max_tokens"`
    WindowSize     int    `mapstructure:"window_size"`
}
```

**默认值设置：**
```go
viper.SetDefault("ai.context.type", "token")
viper.SetDefault("ai.context.max_tokens", 5000)
viper.SetDefault("ai.context.window_size", 20)
```

---

### 2. 创建内存管理封装模块
**文件：** `internal/pkg/memory/memory.go`

**功能：**
- 封装 langchain memory
- 支持配置化创建
- 把 model.Message 转换为 llms.ChatMessage

**实现：
```go
type ConversationMemory struct {
    mem schema.Memory
    cfg *config.Config
}

func NewConversationMemory(cfg *config.Config, llm llms.Model) (*ConversationMemory, error)
func (m *ConversationMemory) Load(ctx context.Context) ([]llms.ChatMessage, error)
func (m *ConversationMemory) Save(ctx context.Context, userInput, aiOutput string) error
func (m *ConversationMemory) Clear(ctx context.Context) error
```

---

### 3. 修改Chain集成
**文件：** `internal/pkg/llm/chain.go`

**修改方法：**
```go
func (c *Chain) Invoke(
    ctx context.Context,
    state map[string]interface{},
    stateKey string,
    memory schema.Memory, // 新增：传入 memory
) (*ChainResponse, error)
```

---

### 4. 修改AIService
**文件：** `internal/service/ai_service.go`

**修改点：**
- 初始化时创建 ConversationMemory
- 加载历史消息到 ChatMessageHistory
- 使用 Memory.SaveContext() 保存每对对话
- 使用 Memory.LoadMemoryVariables() 获取历史
- 添加详细日志

---

## 四、预期效果

✅ 精确Token管理
✅ 配置灵活
✅ 使用成熟库，减少手写代码

---

## 五、涉及文件清单

| 文件路径 | 修改类型 | 说明 |
|---------|---------|------|
| `internal/config/config.go` | 修改 | 添加Context配置 |
| `internal/pkg/memory/memory.go` | 新增 | 封装langchain memory |
| `internal/pkg/llm/chain.go` | 修改 | 集成memory |
| `internal/service/ai_service.go` | 修改 | 使用memory |
| `configs/config.yaml` | 修改 | 配置示例 |
