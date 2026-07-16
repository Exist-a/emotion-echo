package nodes

import (
	"context"
	"fmt"

	"emotion-echo-gin/internal/workflow/graph"
)

type PromptSelectorNode struct {
	id string
}

func NewPromptSelectorNode() *PromptSelectorNode {
	return &PromptSelectorNode{
		id: "prompt_selector",
	}
}

func (n *PromptSelectorNode) GetID() string {
	return n.id
}

func (n *PromptSelectorNode) Execute(ctx context.Context, state graph.State) (graph.State, error) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                     [PROMPT SELECTOR NODE]                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
	
	emotion := ""
	if val, exists := state.Get(keyEmotion); exists {
		if str, ok := val.(string); ok {
			emotion = str
		}
	}
	fmt.Printf("  [INPUT] emotion=%s\n", emotion)
	
	if emotion == "" {
		fmt.Println("  [WARNING] No emotion detected, using 'neutral'")
		emotion = "neutral"
	}

	prompt := selectSystemPrompt(emotion)
	state.Set(keySystemPrompt, prompt)

	fmt.Printf("  [OUTPUT] Selected prompt for emotion '%s'\n", emotion)
	fmt.Printf("  [OUTPUT] Prompt preview: %s\n", truncateString(prompt, 80))
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                  [PROMPT SELECTOR NODE - END]                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	return state, nil
}

func selectSystemPrompt(emotion string) string {
	prompts := map[string]string{
		"happy": `你是一个温暖积极的对话伙伴。用户正处于开心、愉悦的情绪中。
要求：
1. 语气轻松愉快，分享他们的喜悦
2. 可以适当表达祝贺和鼓励
3. 回复长度控制在200字以内
4. 禁止编造信息，若无法提供有效帮助，请明确告知用户。`,

		"sad": `你是一个温柔耐心的倾听者和情绪疏导者。用户正处于低落、悲伤的情绪中。
要求：
1. 语气温柔共情，表达理解和支持
2. 避免使用专业术语
3. 回复长度控制在200字以内
4. 禁止编造信息，若无法提供有效帮助，请明确告知用户。`,

		"angry": `你是一个冷静客观的引导者。用户正处于愤怒、暴躁的情绪中。
要求：
1. 语气平和冷静，帮助用户平复情绪
2. 引导用户理性分析问题
3. 回复长度控制在200字以内
4. 禁止编造信息，若无法提供有效帮助，请明确告知用户。`,

		"anxious": `你是一个舒缓放松的陪伴者。用户正处于焦虑、紧张的情绪中。
要求：
1. 语气舒缓放松，帮助用户放松心情
2. 提供可行的建议和指导
3. 回复长度控制在200字以内
4. 禁止编造信息，若无法提供有效帮助，请明确告知用户。`,

		"neutral": `你是一个专业的心理健康助手。用户情绪平静，可以进行常规的心理健康咨询和建议。
要求：
1. 语气友好专业
2. 提供有价值的心理健康建议
3. 回复长度控制在200字以内
4. 禁止编造信息，若无法提供有效帮助，请明确告知用户。`,
	}

	if prompt, ok := prompts[emotion]; ok {
		return prompt
	}
	return prompts["neutral"]
}
