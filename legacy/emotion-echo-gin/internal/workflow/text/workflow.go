package text

import (
	"context"
	"fmt"

	"emotion-echo-gin/internal/workflow/graph"
	"emotion-echo-gin/internal/workflow/text/nodes"
)

type LLMCaller func(ctx context.Context, prompt string) (string, error)

func NewOnlineWorkflow(llmCaller LLMCaller) *graph.Graph {
	g := graph.NewGraph("text_online", nil)

	intentNode := nodes.NewIntentRecognitionNode(llmCaller)
	emotionNode := nodes.NewEmotionAnalysisNode(llmCaller)
	promptNode := nodes.NewPromptSelectorNode()

	g.AddNode(intentNode)
	g.AddNode(emotionNode)
	g.AddNode(promptNode)

	g.AddEdge("intent_recognition", "emotion_analysis", nil)
	g.AddEdge("emotion_analysis", "prompt_selector", nil)

	return g
}

func NewOfflineWorkflow(llmCaller LLMCaller) *graph.Graph {
	g := graph.NewGraph("text_offline", nil)

	emotionNode := nodes.NewEmotionAnalysisNode(llmCaller)
	keywordNode := nodes.NewKeywordExtractionNode(llmCaller)
	summaryNode := nodes.NewSummaryGenerationNode(llmCaller)

	g.AddNode(emotionNode)
	g.AddNode(keywordNode)
	g.AddNode(summaryNode)

	g.AddEdge("emotion_analysis", "keyword_extraction", nil)
	g.AddEdge("keyword_extraction", "summary_generation", nil)

	return g
}

func RunOnlineWorkflow(ctx context.Context, llmCaller LLMCaller, state *TextState) (*TextState, error) {
	g := NewOnlineWorkflow(llmCaller)
	
	intentState, err := executeSingleNode(ctx, g, "intent_recognition", state)
	if err != nil {
		return state, err
	}
	
	intent := intentState.GetIntent()
	
	if intent != "emotional_support" {
		fmt.Println("  [WORKFLOW] Intent is not emotional_support, setting neutral emotion")
		intentState.SetEmotion("neutral")
		intentState.SetSystemPrompt(getDefaultPrompt())
		return intentState, nil
	}
	
	finalState, err := executeRemainingNodes(ctx, g, intentState)
	if err != nil {
		return intentState, err
	}
	
	return finalState, nil
}

func executeSingleNode(ctx context.Context, g *graph.Graph, nodeID string, state *TextState) (*TextState, error) {
	nodes := g.GetNodes()
	for _, node := range nodes {
		if node.GetID() == nodeID {
			result, err := node.Execute(ctx, state)
			if err != nil {
				return state, err
			}
			return result.(*TextState), nil
		}
	}
	return state, nil
}

func executeRemainingNodes(ctx context.Context, g *graph.Graph, state *TextState) (*TextState, error) {
	nodes := g.GetNodes()
	currentState := state
	
	for _, node := range nodes {
		nodeID := node.GetID()
		if nodeID == "intent_recognition" {
			continue
		}
		
		result, err := node.Execute(ctx, currentState)
		if err != nil {
			return currentState, err
		}
		currentState = result.(*TextState)
	}
	
	return currentState, nil
}

func getDefaultPrompt() string {
	return `你是一位专业的智能助手。请友好、专业地回答用户的问题。

要求：
1. 语气友好专业
2. 提供准确有用的信息
3. 回复长度控制在200字以内
4. 禁止编造信息，若无法提供有效帮助，请明确告知用户。

重要：你正在和用户进行连续的对话。请记住用户之前分享的信息
（如名字、经历、问题等），在后续对话中适当引用，这有助于提供
更连贯、更个性化的支持。如果用户询问之前聊过的内容，你应该能够
准确描述。`
}

func RunOfflineWorkflow(ctx context.Context, llmCaller LLMCaller, state *TextState) (*TextState, error) {
	g := NewOfflineWorkflow(llmCaller)
	finalState, err := g.Execute(ctx, "offline_run", state)
	if err != nil {
		return state, err
	}
	return finalState.(*TextState), nil
}
