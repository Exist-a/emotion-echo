package llm

import (
	"context"
	"fmt"
	"strings"

	"emotion-echo-gin/internal/config"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

type Chain struct {
	llm llms.Model
}

func NewChain(cfg *config.Config) (*Chain, error) {
	apiKey := cfg.AI.Kimi.APIKey
	baseURL := cfg.AI.Kimi.BaseURL
	if baseURL == "" {
		baseURL = "https://api.moonshot.cn/v1"
	}
	model := cfg.AI.Kimi.Model
	if model == "" {
		model = "moonshot-v1-8k"
	}

	llm, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL(baseURL),
		openai.WithModel(model),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM: %w", err)
	}

	return &Chain{
		llm: llm,
	}, nil
}

func (c *Chain) GetLLM() llms.Model {
	return c.llm
}

type ChatMessage struct {
	Role    string
	Content string
}

func (c *Chain) Call(ctx context.Context, systemPrompt string, userInput string, intent string, history []ChatMessage) (string, error) {
	fullPrompt := FormatPrompt(systemPrompt, userInput, intent, history)

	completion, err := c.llm.Call(ctx, fullPrompt,
		llms.WithTemperature(0.7),
		llms.WithMaxTokens(500),
	)
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return completion, nil
}

func (c *Chain) CallWithLLMChat(ctx context.Context, systemPrompt string, llmHistory []llms.ChatMessage, intent string) (string, error) {
	var messages []llms.MessageContent

	messages = append(messages, llms.TextParts(llms.ChatMessageTypeSystem, systemPrompt))

	for _, m := range llmHistory {
		switch m.GetType() {
		case llms.ChatMessageTypeHuman:
			messages = append(messages, llms.TextParts(m.GetType(), m.GetContent()))
		case llms.ChatMessageTypeAI:
			messages = append(messages, llms.TextParts(m.GetType(), m.GetContent()))
		}
	}

	var stylePrompt = ""
	switch intent {
	case "emotional_support":
		stylePrompt = `回复要求：像朋友一样自然对话，语气温暖共情，不要用序号或列表，保持流畅自然，适当换行分段，体现理解和支持`
	case "study_help":
		stylePrompt = `回复要求：1. 用清晰的步骤说明（1. 2. 3.） 2. 关键点可以加粗 3. 结尾加鼓励的话 4. 保持温和耐心`
	case "tech_help":
		stylePrompt = `回复要求：步骤清晰，用序号列出，需要代码时，用代码块包裹，语言简洁专业，适当使用无序列表`
	case "career_help":
		stylePrompt = `回复要求：分点给出建议（1. 2. 3.），逻辑清晰，条理分明，关键建议加粗标注`
	}

	if stylePrompt != "" {
		messages = append(messages, llms.TextParts(llms.ChatMessageTypeSystem, stylePrompt))
	}

	fmt.Println("│                     发送消息详情                                  │")
	for i, msg := range messages {
		for _, part := range msg.Parts {
			if textContent, ok := part.(llms.TextContent); ok {
				contentPreview := textContent.Text
				if len(contentPreview) > 100 {
					contentPreview = contentPreview[:100] + "..."
				}
				fmt.Printf("│ [%d] %s\n", i, contentPreview)
			}
		}
	}
	fmt.Println("│                     发送消息完毕                                  │")

	completion, err := c.llm.GenerateContent(ctx, messages, llms.WithTemperature(0.7), llms.WithMaxTokens(500))
	if err != nil {
		return "", fmt.Errorf("LLM GenerateContent failed: %w", err)
	}

	fmt.Println("│                       LLM 返回详情                                  │")
	if len(completion.Choices) > 0 {
		fmt.Printf("│ %s\n", completion.Choices[0].Content)
		fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
		return completion.Choices[0].Content, nil
	}
	fmt.Printf("│ [警告] LLM 返回为空\n")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	return "", nil
}

func FormatPrompt(systemPrompt, userInput, intent string, history []ChatMessage) string {
	var promptBuilder strings.Builder

	promptBuilder.WriteString(systemPrompt)
	promptBuilder.WriteString("\n\n")

	switch intent {
	case "emotional_support":
		promptBuilder.WriteString(`回复要求：
1. 像朋友一样自然对话，语气温暖共情
2. 不要用序号或列表，保持流畅自然
3. 适当使用换行分段
4. 体现理解和支持
`)
	case "study_help":
		promptBuilder.WriteString(`回复要求：
1. 用清晰的步骤说明（1. 2. 3.）
2. 关键点可以加粗（**重点**）
3. 结尾可以加鼓励的话
4. 保持温和耐心
`)
	case "tech_help":
		promptBuilder.WriteString("回复要求：\n1. 步骤清晰，用序号列出\n2. 需要代码时，用代码块包裹（```语言\\n代码\\n```）\n3. 语言简洁专业\n4. 适当使用无序列表\n")
	case "career_help":
		promptBuilder.WriteString(`回复要求：
1. 分点给出建议（1. 2. 3.）
2. 逻辑清晰，条理分明
3. 关键建议加粗标注
`)
	default:
		promptBuilder.WriteString(`回复要求：
1. 简洁直接
2. 适当用列表或分段
`)
	}

	promptBuilder.WriteString("\n")

	if len(history) > 0 {
		promptBuilder.WriteString("对话历史：\n")
		for _, msg := range history {
			promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Role, msg.Content))
		}
		promptBuilder.WriteString("\n")
	}

	promptBuilder.WriteString(fmt.Sprintf("用户: %s\n", userInput))
	promptBuilder.WriteString("AI: ")

	return promptBuilder.String()
}

func MessageToHistory(role, content string) ChatMessage {
	return ChatMessage{
		Role:    role,
		Content: content,
	}
}

func (c *Chain) GenerateTitle(ctx context.Context, userMessage string) (string, error) {
	prompt := fmt.Sprintf(`请根据用户的首条消息生成一个简短的会话标题。

要求：
1. 不超过10个中文字符
2. 能准确概括用户意图
3. 不要使用引号包裹
4. 直接输出标题，不要添加解释

用户消息：%s`, userMessage)

	completion, err := c.llm.Call(ctx, prompt,
		llms.WithTemperature(0.3),
		llms.WithMaxTokens(50),
	)
	if err != nil {
		return "", fmt.Errorf("GenerateTitle LLM call failed: %w", err)
	}

	title := strings.TrimSpace(completion)
	title = strings.Trim(title, "\"'-")

	if len([]rune(title)) > 10 {
		runes := []rune(title)
		title = string(runes[:10])
	}

	return title, nil
}

func TruncateString(s string, maxLen int) string {
	if len([]rune(s)) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen])
}
