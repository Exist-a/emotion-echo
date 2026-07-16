package service

import (
	"context"
	"fmt"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/workflow/text"
)

// EmotionAnalysisResult 情绪分析结果
type EmotionAnalysisResult struct {
	SystemPrompt string
	Emotion     string
	Confidence  float64
	Intent      string
	AllScores   map[string]float64
}

// AnalyzeEmotion 执行情绪分析
func (s *AIService) AnalyzeEmotion(ctx context.Context, req *StreamRequest) *EmotionAnalysisResult {
	result := &EmotionAnalysisResult{
		SystemPrompt: DefaultPrompt,
	}

	if s.emotionWorkflow == nil || s.llmCaller == nil {
		fmt.Printf("│ [警告] 情绪工作流未配置，使用默认提示词\n")
		return result
	}

	textState := text.NewTextState()
	textState.SetText(req.Message)

	if req.VoiceEmotion != "" {
		textState.SetAudioContext(req.Message, req.VoiceEmotion)
	}

	resultState, err := text.RunOnlineWorkflow(ctx, s.llmCaller, textState)
	if err != nil {
		fmt.Printf("│ [警告] 情绪分析失败: %v\n", err)
		return result
	}

	result.SystemPrompt = resultState.GetSystemPrompt()
	result.Emotion = resultState.GetEmotion()
	result.Confidence = resultState.GetConfidence()
	result.Intent = resultState.GetIntent()
	result.AllScores = resultState.GetAllScores()

	fmt.Printf("│ 检测到情绪: %s (置信度: %.2f)\n", result.Emotion, result.Confidence)
	fmt.Printf("│ 检测到意图: %s\n", result.Intent)

	return result
}

// UpdateMessageEmotion 更新消息的情绪标签
func (s *AIService) UpdateMessageEmotion(ctx context.Context, userMsgID, emotion, intentType string) {
	if emotion == "" || userMsgID == "" {
		return
	}
	_ = s.msgService.UpdateEmotionTag(ctx, userMsgID, &emotion)
	_ = s.msgService.UpdateIntentType(ctx, userMsgID, intentType)
}

// SaveEmotionAnalysisAsync 异步保存情绪分析结果
func (s *AIService) SaveEmotionAnalysisAsync(ctx context.Context, userID int64, convID string, emotion string, confidence float64, allScores map[string]float64) {
	if emotion == "" || s.analysisRepo == nil {
		return
	}

	emotionScores := models.EmotionScoresMap{}
	if allScores != nil {
		emotionScores = allScores
	} else {
		emotionScores = map[string]float64{
			emotion: confidence,
		}
	}

	analysis := &models.EmotionAnalysis{
		UserID:          userID,
		ConversationID:  convID,
		AnalyzedAt:      time.Now(),
		DominantEmotion: emotion,
		EmotionScores:   emotionScores,
	}

	if err := s.analysisRepo.Create(ctx, analysis); err != nil {
		fmt.Printf("[ERROR] Failed to save emotion analysis: %v\n", err)
	}
}

// BuildSurveyContext 构建量表上下文
func (s *AIService) BuildSurveyContext(ctx context.Context, userID int64, emotion string) string {
	if s.surveyService == nil {
		return ""
	}
	return ""
}

// ShouldAddSurveyContext 判断是否需要添加量表上下文
func ShouldAddSurveyContext(emotion string) bool {
	return emotion == "sad" || emotion == "angry" || emotion == "anxious"
}
