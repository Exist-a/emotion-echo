package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"emotion-echo-gin/internal/pkg/llm"
	"emotion-echo-gin/internal/workflow/graph"
)

const (
	keyIntent          = "intent"
	keyIntentConfidence = "intent_confidence"
)

type IntentRecognitionNode struct {
	id        string
	llmCaller func(ctx context.Context, prompt string) (string, error)
}

func NewIntentRecognitionNode(llmCaller func(ctx context.Context, prompt string) (string, error)) *IntentRecognitionNode {
	return &IntentRecognitionNode{
		id:        "intent_recognition",
		llmCaller: llmCaller,
	}
}

func (n *IntentRecognitionNode) GetID() string {
	return n.id
}

func (n *IntentRecognitionNode) Execute(ctx context.Context, state graph.State) (graph.State, error) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    [INTENT RECOGNITION NODE]                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	content := buildRawContent(state)
	fmt.Printf("  [INPUT] User message: %s\n", truncateString(content, 100))

	var voicePromptPrefix string

	if audioCtxVal, exists := state.Get("audio_context"); exists {
		if audioCtx, ok := audioCtxVal.(map[string]string); ok {
			transcript := audioCtx["transcript"]
			emotion := audioCtx["emotion"]
			voicePromptPrefix = fmt.Sprintf("用户发来一段语音，从语音中听出：%s。语音内容：%s。\n\n",
				llm.GetEmotionLabel(emotion), transcript)
			fmt.Printf("  [VOICE] Detected voice message, emotion=%s\n", emotion)
		}
	}

	if content == "" {
		fmt.Println("  [ERROR] No content to analyze!")
		fmt.Println("  [OUTPUT] intent=other, confidence=0.5 (fallback)")
		fmt.Println("  [DECISION] Skip emotion analysis")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║              [INTENT RECOGNITION NODE - END]                          ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyIntent, "other")
		state.Set(keyIntentConfidence, 0.5)
		return state, nil
	}

	fmt.Println("  [ACTION] Calling LLM for intent classification...")

	prompt := fmt.Sprintf(`请判断以下用户输入的意图类型。

%s请以JSON格式返回判断结果，格式如下：
{"intent":"意图类型","confidence":置信度(0-1)}

意图类型选择：
- emotional_support（情感疏导）：用户表达情绪困扰、心理问题、需要情感支持或心理咨询
- study_help（学习问题）：作业、学习方法、考试焦虑、学业规划
- tech_help（技术问题）：代码问题、工具使用、技术选型、软件操作
- career_help（职业问题）：职业规划、工作压力、职场人际关系、求职咨询
- lifestyle（生活问题）：日常建议、兴趣爱好、娱乐资讯、健康咨询
- other（其他）：无法归类、闲聊、测试消息、指令性操作

只返回JSON，不要其他内容。`, voicePromptPrefix+content)

	response, err := n.llmCaller(ctx, prompt)
	if err != nil {
		fmt.Printf("  [WARNING] LLM call failed: %v\n", err)
		fmt.Println("  [OUTPUT] intent=other, confidence=0.5 (fallback)")
		fmt.Println("  [DECISION] Skip emotion analysis")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║              [INTENT RECOGNITION NODE - END]                          ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyIntent, "other")
		state.Set(keyIntentConfidence, 0.5)
		return state, nil
	}

	fmt.Printf("  [LLM RESPONSE] %s\n", response)
	
	// 清理 LLM 响应中的 markdown 标记
	cleanedResponse := strings.TrimSpace(response)
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```json")
	cleanedResponse = strings.TrimPrefix(cleanedResponse, "```")
	cleanedResponse = strings.TrimSuffix(cleanedResponse, "```")
	cleanedResponse = strings.TrimSpace(cleanedResponse)
	
	var result struct {
		Intent     string  `json:"intent"`
		Confidence float64 `json:"confidence"`
	}

	if err := json.Unmarshal([]byte(cleanedResponse), &result); err != nil {
		fmt.Printf("  [WARNING] Failed to parse LLM response: %v\n", err)
		fmt.Println("  [OUTPUT] intent=other, confidence=0.5 (fallback)")
		fmt.Println("  [DECISION] Skip emotion analysis")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║              [INTENT RECOGNITION NODE - END]                          ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyIntent, "other")
		state.Set(keyIntentConfidence, 0.5)
		return state, nil
	}

	validIntents := map[string]bool{
		"emotional_support": true,
		"study_help":        true,
		"tech_help":         true,
		"career_help":       true,
		"lifestyle":         true,
		"other":             true,
	}
	if !validIntents[result.Intent] {
		fmt.Printf("  [WARNING] Invalid intent '%s', using 'other'\n", result.Intent)
		result.Intent = "other"
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		fmt.Printf("  [WARNING] Invalid confidence %.2f, using 0.5\n", result.Confidence)
		result.Confidence = 0.5
	}

	state.Set(keyIntent, result.Intent)
	state.Set(keyIntentConfidence, result.Confidence)

	fmt.Printf("  [OUTPUT] intent=%s, confidence=%.4f\n", result.Intent, result.Confidence)
	
	if result.Intent == "emotional_support" {
		fmt.Println("  [DECISION] Proceed to emotion analysis")
	} else {
		fmt.Println("  [DECISION] Skip emotion analysis, use default prompt")
	}
	
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              [INTENT RECOGNITION NODE - END]                          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	return state, nil
}
