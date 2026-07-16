package service

import (
	"context"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/assessment"
	"emotion-echo-gin/internal/workflow/graph"
)

// MentalHealthService 心理健康服务
type MentalHealthService struct {
	assessmentRepo     *repository.MentalHealthRepository
	msgRepo            *repository.MessageRepository
	convRepo           *repository.ConversationRepository
	surveyResultRepo   *repository.SurveyResultRepository
	analysisRepo       *repository.EmotionAnalysisRepository
	workflow           *assessment.Workflow
}

// NewMentalHealthService 创建心理健康服务
func NewMentalHealthService(
	assessmentRepo *repository.MentalHealthRepository,
	msgRepo *repository.MessageRepository,
	convRepo *repository.ConversationRepository,
	surveyResultRepo *repository.SurveyResultRepository,
	analysisRepo *repository.EmotionAnalysisRepository,
	llmCaller func(ctx context.Context, prompt string) (string, error),
	checkpointer graph.Checkpointer,
) *MentalHealthService {
	return &MentalHealthService{
		assessmentRepo:   assessmentRepo,
		msgRepo:          msgRepo,
		convRepo:         convRepo,
		surveyResultRepo: surveyResultRepo,
		analysisRepo:     analysisRepo,
		workflow:         assessment.NewWorkflow(llmCaller, msgRepo, convRepo, surveyResultRepo, analysisRepo, checkpointer),
	}
}

// GetLatestAssessment 获取最新评估
func (s *MentalHealthService) GetLatestAssessment(ctx context.Context, userID int64, assessmentType string) (*models.MentalHealthAssessment, error) {
	return s.assessmentRepo.GetLatestByUserID(ctx, userID, assessmentType)
}

// GetAssessmentHistory 获取评估历史
func (s *MentalHealthService) GetAssessmentHistory(ctx context.Context, userID int64, assessmentType string, limit int, cursor int64) ([]*models.MentalHealthAssessment, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.assessmentRepo.ListByUserID(ctx, userID, assessmentType, limit, cursor)
}

// TriggerAssessment 手动触发评估
func (s *MentalHealthService) TriggerAssessment(ctx context.Context, userID int64, req *TriggerAssessmentRequest) (*TriggerAssessmentResponse, error) {
	// 检查是否已有进行中的评估（今天）
	hasToday, err := s.assessmentRepo.HasAssessmentToday(ctx, userID, req.Type)
	if err != nil {
		return nil, err
	}
	if hasToday {
		return nil, errors.New(errors.ErrInvalidParams, "今日已生成评估，请明天再来")
	}

	// 异步执行评估（使用传入的 ctx，而非 context.Background()）
	go func() {
		periodDays := 1
		if req.Type == "weekly" {
			periodDays = 7
		} else if req.Type == "comprehensive" {
			periodDays = req.PeriodDays
			if periodDays <= 0 || periodDays > 30 {
				periodDays = 7
			}
		}

		result, err := s.workflow.Execute(ctx, userID, req.Type, periodDays)
		if err != nil {
			// 记录错误日志
			return
		}

		// 保存评估结果
		if err := s.assessmentRepo.Create(ctx, result); err != nil {
			// 记录错误日志
		}
	}()

	return &TriggerAssessmentResponse{
		Status:        "processing",
		EstimatedTime: "30s",
	}, nil
}

// GetTrendData 获取趋势数据
func (s *MentalHealthService) GetTrendData(ctx context.Context, userID int64, trendType string) (*TrendData, error) {
	return GetTrendData(ctx, userID, trendType, s.assessmentRepo)
}

// BatchDailyAssessment 批量每日评估（使用 goroutine pool 并行）
func (s *MentalHealthService) BatchDailyAssessment(ctx context.Context) error {
	return BatchDailyAssessment(ctx, s.assessmentRepo, s.workflow)
}


