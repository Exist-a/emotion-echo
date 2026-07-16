package nodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"emotion-echo-gin/internal/workflow/graph"
)

const (
	keyText         = "text"
	keyMessages     = "messages"
	keyEmotion      = "emotion"
	keyConfidence  = "confidence"
	keyAllScores   = "all_scores"
	keySystemPrompt = "system_prompt"
	keyMessage      = "message" // 兼容旧的字段名
)

type EmotionAnalysisNode struct {
	id        string
	llmCaller func(ctx context.Context, prompt string) (string, error)
}

func NewEmotionAnalysisNode(llmCaller func(ctx context.Context, prompt string) (string, error)) *EmotionAnalysisNode {
	return &EmotionAnalysisNode{
		id:        "emotion_analysis",
		llmCaller: llmCaller,
	}
}

func (n *EmotionAnalysisNode) GetID() string {
	return n.id
}

func getTextFromState(state graph.State) string {
	if val, exists := state.Get(keyText); exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	if val, exists := state.Get(keyMessage); exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getMessagesFromState(state graph.State) string {
	if val, exists := state.Get(keyMessages); exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func buildRawContent(state graph.State) string {
	text := getTextFromState(state)
	if text != "" {
		return text
	}
	return getMessagesFromState(state)
}

func (n *EmotionAnalysisNode) Execute(ctx context.Context, state graph.State) (graph.State, error) {
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                      [EMOTION ANALYSIS NODE]                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
	
	content := buildRawContent(state)
	fmt.Printf("  [INPUT] Content length: %d\n", len(content))
	if len(content) > 0 {
		fmt.Printf("  [INPUT] Content preview: %s\n", truncateString(content, 100))
	}
	
	// 优先使用语音情绪（如果存在且有效）
	if audioCtxVal, exists := state.Get("audio_context"); exists {
		if audioCtx, ok := audioCtxVal.(map[string]string); ok {
			voiceEmotion := audioCtx["emotion"]
			// 检查语音情绪是否有效（不是空、unk、unknown）
			validVoiceEmotions := map[string]bool{"happy": true, "sad": true, "angry": true, "anxious": true, "neutral": true}
			if voiceEmotion != "" && validVoiceEmotions[voiceEmotion] && voiceEmotion != "unk" && voiceEmotion != "unknown" {
				fmt.Printf("  [VOICE] Voice emotion detected: %s (confidence=1.0)\n", voiceEmotion)
				state.Set(keyEmotion, voiceEmotion)
				state.Set(keyConfidence, 1.0)

				fmt.Println("  [DECISION] Voice emotion is valid, using it directly")
				fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
				fmt.Println("║                   [EMOTION ANALYSIS NODE - END]                        ║")
				fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
				return state, nil
			} else {
				fmt.Printf("  [VOICE] Voice emotion is invalid or empty: \"%s\", will use LLM analysis\n", voiceEmotion)
			}
		}
	}
	
	if content == "" {
		fmt.Println("  [ERROR] No content to analyze!")
		fmt.Println("  [OUTPUT] emotion=neutral, confidence=0.5")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                   [EMOTION ANALYSIS NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyEmotion, "neutral")
		state.Set(keyConfidence, 0.5)
		return state, fmt.Errorf("no content to analyze")
	}

	fmt.Println("  [ACTION] Calling LLM for emotion analysis...")
	
	prompt := fmt.Sprintf(`请分析以下用户输入的情绪状态，返回主导情绪标签及其置信度，以及所有情绪的置信度分布。
支持的情绪标签：
- happy：开心
- sad：悲伤
- angry：愤怒
- anxious：焦虑
- neutral：中性
- unk：未知（无法识别情绪）

用户输入：%s

请以JSON格式返回，格式如下：
{
  "emotion": "主导情绪标签",
  "confidence": 主导情绪的置信度(0-1之间),
  "all_scores": {
    "happy": 0.1,
    "sad": 0.9,
    "angry": 0.0,
    "anxious": 0.3,
    "neutral": 0.2,
    "unk": 0.0
  }
}
all_scores 中每个情绪的置信度都必须是0-1之间的浮点数，且所有分数之和应接近1.0。

只返回JSON，不要其他内容。`, content)

	response, err := n.llmCaller(ctx, prompt)
	if err != nil {
		fmt.Printf("  [WARNING] LLM call failed: %v\n", err)
		fmt.Println("  [OUTPUT] emotion=neutral, confidence=0.5 (fallback)")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                   [EMOTION ANALYSIS NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyEmotion, "neutral")
		state.Set(keyConfidence, 0.5)
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
		Emotion    string             `json:"emotion"`
		Confidence float64            `json:"confidence"`
		AllScores  map[string]float64 `json:"all_scores"`
	}

	if err := json.Unmarshal([]byte(cleanedResponse), &result); err != nil {
		fmt.Printf("  [WARNING] Failed to parse LLM response: %v\n", err)
		fmt.Println("  [OUTPUT] emotion=neutral, confidence=0.5 (fallback)")
		fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
		fmt.Println("║                   [EMOTION ANALYSIS NODE - END]                        ║")
		fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")
		state.Set(keyEmotion, "neutral")
		state.Set(keyConfidence, 0.5)
		return state, nil
	}

	validEmotions := map[string]bool{"happy": true, "sad": true, "angry": true, "anxious": true, "neutral": true, "unk": true, "unknown": true}
	if !validEmotions[result.Emotion] {
		fmt.Printf("  [WARNING] Invalid emotion '%s', using 'neutral'\n", result.Emotion)
		result.Emotion = "neutral"
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		fmt.Printf("  [WARNING] Invalid confidence %.2f, using 0.5\n", result.Confidence)
		result.Confidence = 0.5
	}

	validAllScores := map[string]bool{"happy": true, "sad": true, "angry": true, "anxious": true, "neutral": true, "unk": true}
	if result.AllScores == nil {
		result.AllScores = make(map[string]float64)
	}
	for emotion := range result.AllScores {
		if !validAllScores[emotion] {
			delete(result.AllScores, emotion)
		}
	}

	state.Set(keyEmotion, result.Emotion)
	state.Set(keyConfidence, result.Confidence)
	state.Set(keyAllScores, result.AllScores)

	fmt.Printf("  [OUTPUT] emotion=%s, confidence=%.4f\n", result.Emotion, result.Confidence)
	if result.AllScores != nil {
		fmt.Printf("  [OUTPUT] all_scores=%v\n", result.AllScores)
	}
	fmt.Println("╔══════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                   [EMOTION ANALYSIS NODE - END]                        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════════════════╝")

	return state, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
