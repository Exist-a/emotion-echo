package memory

import (
	"context"
	"fmt"
	"sync"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/models"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/memory"
	"github.com/tmc/langchaingo/schema"
)

type ChatMessageImpl struct {
	msgType llms.ChatMessageType
	content string
}

func (c ChatMessageImpl) GetType() llms.ChatMessageType {
	return c.msgType
}

func (c ChatMessageImpl) GetContent() string {
	return c.content
}

type ConversationMemory struct {
	cfg     *config.Config
	mem     schema.Memory
	llm     llms.Model
	history []llms.ChatMessage
	mu      sync.RWMutex
}

func NewConversationMemory(cfg *config.Config, llm llms.Model) (*ConversationMemory, error) {
	var mem schema.Memory
	opts := []memory.ConversationBufferOption{
		memory.WithReturnMessages(true),
	}

	switch cfg.AI.Context.Type {
	case "token":
		fmt.Printf("[Memory] Using ConversationTokenBuffer, MaxTokens: %d\n", cfg.AI.Context.MaxTokens)
		mem = memory.NewConversationTokenBuffer(llm, cfg.AI.Context.MaxTokens, opts...)
	case "window":
		fmt.Printf("[Memory] Using ConversationWindowBuffer, WindowSize: %d\n", cfg.AI.Context.WindowSize)
		mem = memory.NewConversationWindowBuffer(cfg.AI.Context.WindowSize, opts...)
	default:
		fmt.Println("[Memory] Using ConversationBuffer (default)")
		mem = memory.NewConversationBuffer(opts...)
	}

	return &ConversationMemory{
		cfg:     cfg,
		mem:     mem,
		llm:     llm,
		history: make([]llms.ChatMessage, 0),
	}, nil
}

func (m *ConversationMemory) LoadMessages(ctx context.Context) ([]llms.ChatMessage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	fmt.Printf("│         [Memory] ===== 加载消息 - 历史记录有 %d 条 =====\n", len(m.history))
	for i, msg := range m.history {
		contentPreview := msg.GetContent()
		if len(contentPreview) > 60 {
			contentPreview = contentPreview[:60] + "..."
		}
		msgType := "用户"
		if msg.GetType() == llms.ChatMessageTypeAI {
			msgType = "AI  "
		}
		fmt.Printf("│         [%2d] [%s] %s\n", i, msgType, contentPreview)
	}
	fmt.Printf("│         [Memory] ===== 加载消息结束 =====\n")

	return m.history, nil
}

func (m *ConversationMemory) LoadMessagesFromModels(
	ctx context.Context, messages []*models.Message,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	fmt.Printf("│         [Memory] 从数据库加载: 开始加载 %d 条消息\n", len(messages))
	m.history = make([]llms.ChatMessage, 0)

	// **重要修复**：数据库查询使用 send_time DESC（降序），最新消息在前
	// 但我们需要时间顺序（最旧在前），所以要反转数组
	// 例如: [新3, 新2, 新1] -> [新1, 新2, 新3]
	reversed := make([]*models.Message, len(messages))
	for i, msg := range messages {
		reversed[len(messages)-1-i] = msg
	}

	for _, msg := range reversed {
		switch msg.Sender {
		case "user":
			m.history = append(m.history, ChatMessageImpl{msgType: llms.ChatMessageTypeHuman, content: msg.Content})
		case "ai", "assistant":
			m.history = append(m.history, ChatMessageImpl{msgType: llms.ChatMessageTypeAI, content: msg.Content})
		default:
		}
	}

	fmt.Printf("│         [Memory] 从数据库加载: 完成, 历史记录现有 %d 条消息\n", len(m.history))
	return nil
}

func (m *ConversationMemory) SaveContext(ctx context.Context, userInput, aiOutput string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	userPreview := userInput
	aiPreview := aiOutput
	if len(userPreview) > 30 {
		userPreview = userPreview[:30] + "..."
	}
	if len(aiPreview) > 30 {
		aiPreview = aiPreview[:30] + "..."
	}

	fmt.Printf("│         [Memory] 保存上下文: 用户='%s', AI='%s'\n", userPreview, aiPreview)

	m.history = append(m.history, ChatMessageImpl{msgType: llms.ChatMessageTypeHuman, content: userInput})
	m.history = append(m.history, ChatMessageImpl{msgType: llms.ChatMessageTypeAI, content: aiOutput})

	fmt.Printf("│         [Memory] 保存上下文: 完成, 历史记录现有 %d 条消息\n", len(m.history))

	inputValues := map[string]any{"input": userInput}
	outputValues := map[string]any{"output": aiOutput}
	if err := m.mem.SaveContext(ctx, inputValues, outputValues); err != nil {
		fmt.Printf("│         [Memory] [警告] 内部内存保存失败: %v\n", err)
		return err
	}

	return nil
}

func (m *ConversationMemory) Clear(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.history = make([]llms.ChatMessage, 0)
	return m.mem.Clear(ctx)
}
