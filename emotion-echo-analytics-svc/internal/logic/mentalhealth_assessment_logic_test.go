// Package logic — mentalhealth_assessment_logic_test.go
//
// Sibling test for mentalhealth_assessment_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 3 part 1 RED: cover GET /api/v1/mental-health/assessment.
//
// Coverage matrix:
//
//   - HappyPath_Daily: full MentalAssessment mapping
//   - HappyPath_Weekly: type=weekly
//   - HappyPath_Comprehensive: type=comprehensive
//   - InvalidType_ReturnsValidationError
//   - InvalidUserID_ReturnsValidationError
//   - NoAssessment_ReturnsNil (not error, caller handles nil)
//   - RepoError_Propagates
package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAssessmentSvcCtx(repo repository.MentalHealthRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, MentalHealthRepo: repo}
}

func TestMentalHealthAssessmentLogic_HappyPath_Daily(t *testing.T) {
	t.Parallel()
	now := time.Now()
	want := &repository.MentalAssessment{
		UserID:       42,
		Type:         "daily",
		WindowStart:  "2026-07-15",
		WindowEnd:    "2026-07-15",
		OverallScore: 35.5,
		RiskLevel:    "moderate",
		Dimensions: []repository.DimensionScore{
			{Name: "depression", Score: 25, RiskLevel: "low", Count: 1},
			{Name: "anxiety", Score: 46, RiskLevel: "moderate", Count: 1},
		},
		GeneratedAt: now,
	}
	repo := &mhStubRepo{assessment: want}
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))

	resp, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{
		UserID: 42, Type: "daily",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Assessment)
	assert.Equal(t, int64(42), resp.Assessment.UserID)
	assert.Equal(t, "daily", resp.Assessment.Type)
	assert.Equal(t, "moderate", resp.Assessment.RiskLevel)
	assert.Len(t, resp.Assessment.Dimensions, 2)
}

func TestMentalHealthAssessmentLogic_HappyPath_Weekly(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{
		assessment: &repository.MentalAssessment{
			UserID: 1, Type: "weekly",
			WindowStart: "2026-07-09", WindowEnd: "2026-07-15",
			OverallScore: 28, RiskLevel: "low",
			Dimensions: []repository.DimensionScore{
				{Name: "depression", Score: 10, RiskLevel: "low", Count: 2},
			},
		},
	}
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))
	resp, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{UserID: 1, Type: "weekly"})
	require.NoError(t, err)
	assert.Equal(t, "weekly", resp.Assessment.Type)
}

func TestMentalHealthAssessmentLogic_HappyPath_Comprehensive(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{
		assessment: &repository.MentalAssessment{
			UserID: 1, Type: "comprehensive",
			OverallScore: 65, RiskLevel: "high",
			Dimensions: []repository.DimensionScore{
				{Name: "depression", Score: 60, RiskLevel: "high", Count: 3},
				{Name: "anxiety", Score: 70, RiskLevel: "high", Count: 2},
				{Name: "sleep", Score: 65, RiskLevel: "high", Count: 1},
			},
		},
	}
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))
	resp, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{UserID: 1, Type: "comprehensive"})
	require.NoError(t, err)
	assert.Len(t, resp.Assessment.Dimensions, 3)
}

func TestMentalHealthAssessmentLogic_InvalidType_ReturnsValidationError(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{}
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{UserID: 1, Type: "garbage"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type")
}

func TestMentalHealthAssessmentLogic_InvalidUserID_ReturnsValidationError(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{}
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{UserID: 0, Type: "daily"})
	require.Error(t, err)
}

func TestMentalHealthAssessmentLogic_NoAssessment_ReturnsNilNotError(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{assessment: nil} // 用户无评估记录
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))
	resp, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{UserID: 1, Type: "daily"})
	require.NoError(t, err)
	assert.Nil(t, resp.Assessment, "无评估时 Logic 不返 error，caller 检查 nil")
}

func TestMentalHealthAssessmentLogic_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("postgres unavailable")
	repo := &mhStubRepo{assessmentErr: boom}
	l := NewMentalHealthAssessmentLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetLatestAssessment(&types.GetMentalAssessmentReq{UserID: 1, Type: "daily"})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// mhStubRepo 满足 MentalHealthRepo 接口（仅实现测试用方法）。
type mhStubRepo struct {
	assessment      *repository.MentalAssessment
	assessmentErr   error

	history         []repository.AssessmentHistoryItem
	historyNextCur  string
	historyErr      error
	onListHistory   func(userID int64, atype, cursor string, limit int)

	trend           []repository.TrendPoint
	trendErr        error
}

func (s *mhStubRepo) GetLatestAssessment(_ context.Context, _ int64, _ repository.AssessmentType) (*repository.MentalAssessment, error) {
	return s.assessment, s.assessmentErr
}

func (s *mhStubRepo) ListAssessmentHistory(_ context.Context, userID int64, atype, cursor string, limit int) ([]repository.AssessmentHistoryItem, string, error) {
	if s.onListHistory != nil {
		s.onListHistory(userID, atype, cursor, limit)
	}
	return s.history, s.historyNextCur, s.historyErr
}

func (s *mhStubRepo) GetTrendData(_ context.Context, _ int64, _ string, _, _ time.Time) ([]repository.TrendPoint, error) {
	return s.trend, s.trendErr
}

func (s *mhStubRepo) Ping(_ context.Context) error { return nil }