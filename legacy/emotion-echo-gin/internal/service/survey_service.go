package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/nanoid"
	"emotion-echo-gin/internal/repository"
)

// SurveyService 测验服务
type SurveyService struct {
	surveyRepo *repository.SurveyRepository
	resultRepo *repository.SurveyResultRepository
}

// NewSurveyService 创建测验服务
func NewSurveyService(surveyRepo *repository.SurveyRepository, resultRepo *repository.SurveyResultRepository) *SurveyService {
	return &SurveyService{
		surveyRepo: surveyRepo,
		resultRepo: resultRepo,
	}
}

// List 获取量表列表
func (s *SurveyService) List(ctx context.Context, userID int64) ([]*SurveyListItem, error) {
	surveys, err := s.surveyRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	// 获取用户已完成的状态
	results, err := s.resultRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	completedMap := make(map[int]string)
	for _, r := range results {
		completedMap[r.SurveyID] = r.ID
	}

	// 构建列表
	items := make([]*SurveyListItem, 0, len(surveys))
	for _, survey := range surveys {
		item := &SurveyListItem{
			ID:            survey.ID,
			Title:         survey.Title,
			Description:   survey.Description,
			EstimatedTime: survey.EstimatedTime,
			Status:        "not_started",
		}
		if resultID, ok := completedMap[survey.ID]; ok {
			item.Status = "completed"
			item.ResultID = resultID
			// 获取完成时间
			for _, r := range results {
				if r.ID == resultID {
					item.CompletedAt = &r.CreatedAt
					break
				}
			}
		}
		items = append(items, item)
	}

	return items, nil
}

// Get 获取量表详情
func (s *SurveyService) Get(ctx context.Context, surveyID int) (*models.Survey, error) {
	survey, err := s.surveyRepo.GetByID(ctx, surveyID)
	if err != nil {
		return nil, err
	}
	if survey == nil {
		return nil, errors.New(errors.ErrInvalidParams, "量表不存在")
	}
	return survey, nil
}

// Submit 提交答案
func (s *SurveyService) Submit(ctx context.Context, userID int64, surveyID int, req *SubmitRequest) (*SubmitResult, error) {
	// 1. 获取量表
	survey, err := s.surveyRepo.GetByID(ctx, surveyID)
	if err != nil {
		return nil, err
	}
	if survey == nil {
		return nil, errors.New(errors.ErrInvalidParams, "量表不存在")
	}

	// 2. 验证答案完整性
	if len(req.Answers) != len(survey.Questions) {
		return nil, errors.New(errors.ErrInvalidParams, "答案不完整")
	}

	// 3. 使用评分引擎计算得分
	scorer := NewSurveyScorer(surveyID)
	scoreResult, err := scorer.Calculate(req.Answers, survey.Questions)
	if err != nil {
		return nil, errors.New(errors.ErrInvalidParams, err.Error())
	}

	// 4. 保存结果（TotalScore 存储标准分）
	result := &models.SurveyResult{
		ID:         nanoid.GenerateWithPrefix("res"),
		UserID:     userID,
		SurveyID:   surveyID,
		Answers:    req.Answers,
		TotalScore: scoreResult.StandardScore,
		Level:      scoreResult.Level,
		Suggestion: scoreResult.Suggestion,
		CreatedAt:  time.Now(),
	}

	if err := s.resultRepo.Create(ctx, result); err != nil {
		return nil, err
	}

	return &SubmitResult{
		ResultID:   result.ID,
		RawScore:   scoreResult.RawScore,
		TotalScore: scoreResult.StandardScore,
		Level:      scoreResult.Level,
		Suggestion: scoreResult.Suggestion,
	}, nil
}

// GetResult 获取测验结果详情
func (s *SurveyService) GetResult(ctx context.Context, resultID string) (*SubmitResult, error) {
	result, err := s.resultRepo.GetByID(ctx, resultID)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New(errors.ErrInvalidParams, "测验结果不存在")
	}

	return &SubmitResult{
		ResultID:   result.ID,
		TotalScore: result.TotalScore,
		Level:      result.Level,
		Suggestion: result.Suggestion,
	}, nil
}

// GetLatestPsychProfile 获取用户最新心理画像
func (s *SurveyService) GetLatestPsychProfile(ctx context.Context, userID int64) *PsychProfile {
	return GetLatestPsychProfile(ctx, userID, s.resultRepo)
}
