package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"emotion-echo-gin/internal/workflow/graph"
)

const (
	keySummary    = "summary"
	keySuggestion = "suggestion"
)

type SummaryGenerationNode struct {
	id        string
	llmCaller func(ctx context.Context, prompt string) (string, error)
}

func NewSummaryGenerationNode(llmCaller func(ctx context.Context, prompt string) (string, error)) *SummaryGenerationNode {
	return &SummaryGenerationNode{
		id:        "summary_generation",
		llmCaller: llmCaller,
	}
}

func (n *SummaryGenerationNode) GetID() string {
	return n.id
}

func (n *SummaryGenerationNode) Execute(ctx context.Context, state graph.State) (graph.State, error) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   [SUMMARY GENERATION NODE]                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
	
	content := buildRawContent(state)
	fmt.Printf("  [INPUT] Content length: %d\n", len(content))
	
	if content == "" {
		fmt.Println("  [ERROR] No content to generate summary from!")
		fmt.Println("  [OUTPUT] summary='', suggestion=''")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                [SUMMARY GENERATION NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keySummary, "")
		state.Set(keySuggestion, "")
		return state, nil
	}

	fmt.Println("  [ACTION] Calling LLM for summary generation...")
	
	prompt := fmt.Sprintf(`请为以下对话内容生成一段简洁的摘要（100字以内）和一条建议。

对话内容：
%s

请以JSON格式返回，格式如下：
{"summary":"摘要内容","suggestion":"建议内容"}

只返回JSON，不要其他内容。`, content)

	response, err := n.llmCaller(ctx, prompt)
	if err != nil {
		fmt.Printf("  [WARNING] LLM call failed: %v\n", err)
		fmt.Println("  [OUTPUT] summary='', suggestion='' (fallback)")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                [SUMMARY GENERATION NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keySummary, "")
		state.Set(keySuggestion, "")
		return state, nil
	}

	fmt.Printf("  [LLM RESPONSE] %s\n", response)
	
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result struct {
		Summary    string `json:"summary"`
		Suggestion string `json:"suggestion"`
	}

	if err := json.Unmarshal([]byte(response), &result); err != nil {
		fmt.Printf("  [WARNING] Failed to parse LLM response: %v\n", err)
		fmt.Println("  [OUTPUT] summary='', suggestion='' (fallback)")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                [SUMMARY GENERATION NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keySummary, "")
		state.Set(keySuggestion, "")
		return state, nil
	}

	state.Set(keySummary, result.Summary)
	state.Set(keySuggestion, result.Suggestion)

	fmt.Printf("  [OUTPUT] summary=%s\n", truncateString(result.Summary, 80))
	fmt.Printf("  [OUTPUT] suggestion=%s\n", truncateString(result.Suggestion, 80))
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                [SUMMARY GENERATION NODE - END]                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	return state, nil
}
