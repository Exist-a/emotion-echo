package assessment

import (
	"context"
	"fmt"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/graph"
)

// Workflow 心理健康评估工作流
type Workflow struct {
	graph            *graph.Graph
	llmCaller        func(ctx context.Context, prompt string) (string, error)
	msgRepo          *repository.MessageRepository
	convRepo         *repository.ConversationRepository
	surveyResultRepo *repository.SurveyResultRepository
	analysisRepo     *repository.EmotionAnalysisRepository
}

// NewWorkflow 创建评估工作流
func NewWorkflow(
	llmCaller func(ctx context.Context, prompt string) (string, error),
	msgRepo *repository.MessageRepository,
	convRepo *repository.ConversationRepository,
	surveyResultRepo *repository.SurveyResultRepository,
	analysisRepo *repository.EmotionAnalysisRepository,
	checkpointer graph.Checkpointer,
) *Workflow {
	return &Workflow{
		graph:            BuildAssessmentWorkflow(msgRepo, convRepo, surveyResultRepo, analysisRepo, llmCaller, checkpointer),
		llmCaller:        llmCaller,
		msgRepo:          msgRepo,
		convRepo:         convRepo,
		surveyResultRepo: surveyResultRepo,
		analysisRepo:     analysisRepo,
	}
}

// Execute 执行评估
func (w *Workflow) Execute(ctx context.Context, userID int64, assessmentType string, periodDays int) (*models.MentalHealthAssessment, error) {
	// 计算时间范围
	now := time.Now()
	periodEnd := now
	periodStart := now.AddDate(0, 0, -periodDays)

	// 生成运行 ID
	runID := fmt.Sprintf("%d_%s_%d", userID, assessmentType, now.Unix())

	// 初始化状态
	state := graph.NewMemoryStateFromMap(map[string]interface{}{
		"user_id":         userID,
		"assessment_type": assessmentType,
		"period_start":    periodStart.Format(time.RFC3339),
		"period_end":      periodEnd.Format(time.RFC3339),
	})

	// 执行工作流（带 5 分钟超时）
	finalState, err := w.graph.ExecuteWithTimeout(ctx, runID, state, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	// 构建评估结果（五维，已移除睡眠）
	assessment := &models.MentalHealthAssessment{
		UserID:          userID,
		AssessmentType:  assessmentType,
		PeriodStart:     periodStart,
		PeriodEnd:       periodEnd,
		EmotionScore:    finalState.GetInt("emotion_score"),
		DepressionScore: finalState.GetInt("depression_score"),
		AnxietyScore:    finalState.GetInt("anxiety_score"),
		StressScore:     finalState.GetInt("stress_score"),
		SocialScore:     finalState.GetInt("social_score"),
		OverallScore:    finalState.GetInt("overall_score"),
		RiskLevel:       finalState.GetString("risk_level"),
		RiskFactors:     finalState.GetStringSlice("risk_factors"),
		WarningFlags:    finalState.GetStringSlice("warning_flags"),
		Summary:         finalState.GetString("summary"),
		CreatedAt:       now,
	}

	// 解析建议
	if suggestions, ok := finalState.Get("suggestions"); ok {
		if sgs, ok := suggestions.([]models.Suggestion); ok {
			assessment.Suggestions = sgs
		}
	}

	return assessment, nil
}

// ExecuteForConversation 针对单会话执行评估
func (w *Workflow) ExecuteForConversation(ctx context.Context, userID int64, conversationID string) (*models.MentalHealthAssessment, error) {
	// 初始化状态（单会话模式）
	now := time.Now()
	runID := fmt.Sprintf("%d_conv_%s_%d", userID, conversationID, now.Unix())

	state := graph.NewMemoryStateFromMap(map[string]interface{}{
		"user_id":          userID,
		"conversation_id":  conversationID,
		"assessment_type":  "daily",
		"period_start":     now.AddDate(0, 0, -1).Format(time.RFC3339),
		"period_end":       now.Format(time.RFC3339),
	})

	// 执行工作流（带 5 分钟超时）
	finalState, err := w.graph.ExecuteWithTimeout(ctx, runID, state, 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("workflow execution failed: %w", err)
	}

	// 构建评估结果（五维，已移除睡眠）
	assessment := &models.MentalHealthAssessment{
		UserID:          userID,
		AssessmentType:  "daily",
		PeriodStart:     now.AddDate(0, 0, -1),
		PeriodEnd:       now,
		EmotionScore:    finalState.GetInt("emotion_score"),
		DepressionScore: finalState.GetInt("depression_score"),
		AnxietyScore:    finalState.GetInt("anxiety_score"),
		StressScore:     finalState.GetInt("stress_score"),
		SocialScore:     finalState.GetInt("social_score"),
		OverallScore:    finalState.GetInt("overall_score"),
		RiskLevel:       finalState.GetString("risk_level"),
		RiskFactors:     finalState.GetStringSlice("risk_factors"),
		WarningFlags:    finalState.GetStringSlice("warning_flags"),
		Summary:         finalState.GetString("summary"),
		CreatedAt:       now,
	}

	// 解析建议
	if suggestions, ok := finalState.Get("suggestions"); ok {
		if sgs, ok := suggestions.([]models.Suggestion); ok {
			assessment.Suggestions = sgs
		}
	}

	return assessment, nil
}
