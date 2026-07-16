package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"emotion-echo-gin/internal/workflow/graph"
)

const (
	keyKeywords = "keywords"
)

type KeywordExtractionNode struct {
	id        string
	llmCaller func(ctx context.Context, prompt string) (string, error)
}

func NewKeywordExtractionNode(llmCaller func(ctx context.Context, prompt string) (string, error)) *KeywordExtractionNode {
	return &KeywordExtractionNode{
		id:        "keyword_extraction",
		llmCaller: llmCaller,
	}
}

func (n *KeywordExtractionNode) GetID() string {
	return n.id
}

func (n *KeywordExtractionNode) Execute(ctx context.Context, state graph.State) (graph.State, error) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   [KEYWORD EXTRACTION NODE]                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
	
	content := buildRawContent(state)
	fmt.Printf("  [INPUT] Content length: %d\n", len(content))
	
	if content == "" {
		fmt.Println("  [ERROR] No content to extract keywords from!")
		fmt.Println("  [OUTPUT] keywords=[]")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                [KEYWORD EXTRACTION NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyKeywords, []string{})
		return state, nil
	}

	fmt.Println("  [ACTION] Calling LLM for keyword extraction...")
	
	prompt := fmt.Sprintf(`请从以下对话内容中提取5-10个关键词，这些关键词应该能够反映用户的主要关注点和情绪状态。

对话内容：
%s

请以JSON格式返回，格式如下：
{"keywords":["关键词1","关键词2",...]}

只返回JSON，不要其他内容。`, content)

	response, err := n.llmCaller(ctx, prompt)
	if err != nil {
		fmt.Printf("  [WARNING] LLM call failed: %v\n", err)
		fmt.Println("  [OUTPUT] keywords=[] (fallback)")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                [KEYWORD EXTRACTION NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyKeywords, []string{})
		return state, nil
	}

	fmt.Printf("  [LLM RESPONSE] %s\n", response)
	
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result struct {
		Keywords []string `json:"keywords"`
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		fmt.Printf("  [WARNING] Failed to parse LLM response: %v\n", err)
		fmt.Println("  [OUTPUT] keywords=[] (fallback)")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                [KEYWORD EXTRACTION NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyKeywords, []string{})
		return state, nil
	}

	if result.Keywords == nil {
		result.Keywords = []string{}
	}

	state.Set(keyKeywords, result.Keywords)

	fmt.Printf("  [OUTPUT] keywords=%v\n", result.Keywords)
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                [KEYWORD EXTRACTION NODE - END]                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	return state, nil
}
