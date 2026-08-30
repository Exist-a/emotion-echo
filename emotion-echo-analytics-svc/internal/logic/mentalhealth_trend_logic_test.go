// Package logic — mentalhealth_trend_logic_test.go
//
// Sibling test for mentalhealth_trend_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 3 part 2 RED: cover GET /api/v1/mental-health/trend.
//
// Coverage matrix:
//
//   - HappyPathWeekly
//   - HappyPathMonthly
//   - InvalidType (must be weekly|monthly)
//   - BadDateFormat
//   - InvertedDateRange
//   - EmptyTrend_NeverNil
//   - RepoError_Propagates
//   - InvalidUserID
package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMentalHealthTrendLogic_HappyPathWeekly(t *testing.T) {
	t.Parallel()
	want := []repository.TrendPoint{
		{Date: "2026-07-01", PrimaryEmotion: "happy", AvgSentiment: 0.5, AvgConfidence: 0.9, Count: 5},
		{Date: "2026-07-08", PrimaryEmotion: "calm", AvgSentiment: 0.7, AvgConfidence: 0.85, Count: 3},
	}
	repo := &mhTrendStubRepo{trend: want}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))

	resp, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "weekly", StartDate: "2026-07-01", EndDate: "2026-07-15",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)
	assert.Len(t, resp.Report.Points, 2)
}

func TestMentalHealthTrendLogic_HappyPathMonthly(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{trend: []repository.TrendPoint{
		{Date: "2026-07-01", PrimaryEmotion: "neutral", AvgSentiment: 0.0, AvgConfidence: 0.8, Count: 10},
	}}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	resp, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "monthly", StartDate: "2026-07-01", EndDate: "2026-07-31",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Report.Points, 1)
}

func TestMentalHealthTrendLogic_InvalidType(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "daily", StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type")
}

func TestMentalHealthTrendLogic_BadDateFormat(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "weekly", StartDate: "garbage", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}

func TestMentalHealthTrendLogic_InvertedDateRange(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "weekly", StartDate: "2026-07-30", EndDate: "2026-07-01",
	})
	require.Error(t, err)
}

func TestMentalHealthTrendLogic_EmptyTrend_NeverNil(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{trend: nil}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	resp, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "weekly", StartDate: "2026-07-01", EndDate: "2026-07-07",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp.Report.Points)
	assert.Empty(t, resp.Report.Points)
}

func TestMentalHealthTrendLogic_RepoError(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{trendErr: errors.New("postgres unavailable")}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "weekly", StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}

func TestMentalHealthTrendLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &mhTrendStubRepo{}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 0, Type: "weekly", StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}

func TestMentalHealthTrendLogic_TimeWindowPassed(t *testing.T) {
	t.Parallel()
	var gotStart, gotEnd time.Time
	repo := &mhTrendStubRepo{
		onGetTrend: func(_ int64, _ string, start, end time.Time) {
			gotStart, gotEnd = start, end
		},
		trend: []repository.TrendPoint{},
	}
	l := NewMentalHealthTrendLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.GetTrend(&types.GetMentalHealthTrendReq{
		UserID: 1, Type: "monthly", StartDate: "2026-07-01", EndDate: "2026-07-31",
	})
	require.NoError(t, err)
	assert.Equal(t, 2026, gotStart.Year())
	assert.Equal(t, time.July, gotStart.Month())
	assert.Equal(t, 31, gotEnd.Day())
}

// mhStubRepo 没 onGetTrend 字段——补
// (其实已经在 assessment_test.go 里定义过；这里要么 reuse 要么 redefine)
// 由于不同 _test.go 不共享 helper，触发需要在 trend_test 里独立定义。
// 但 mhStubRepo 是个 struct 类型——同名类型在不同 _test.go 文件里
// 是 Go 的 test binary 内部的两个独立类型（每个 _test.go 编译进
// 同一个 test binary），所以两者冲突。需要在 trend_test 用别的
// stub 名或共享。共享得用 _test.go 跨文件 helper，这违反 sibling。
// 改名：trend_test 用 mhTrendStubRepo（独立类型）。
type mhTrendStubRepo struct {
	assessment      *repository.MentalAssessment
	assessmentErr   error

	history         []repository.AssessmentHistoryItem
	historyNextCur  string
	historyErr      error
	onListHistory   func(userID int64, atype, cursor string, limit int)

	trend           []repository.TrendPoint
	trendErr        error
	onGetTrend      func(userID int64, trendType string, start, end time.Time)
}

func (s *mhTrendStubRepo) GetLatestAssessment(_ context.Context, _ int64, _ repository.AssessmentType) (*repository.MentalAssessment, error) {
	return s.assessment, s.assessmentErr
}

func (s *mhTrendStubRepo) ListAssessmentHistory(_ context.Context, _ int64, _, _ string, _ int) ([]repository.AssessmentHistoryItem, string, error) {
	return s.history, s.historyNextCur, s.historyErr
}

func (s *mhTrendStubRepo) GetTrendData(_ context.Context, userID int64, trendType string, start, end time.Time) ([]repository.TrendPoint, error) {
	if s.onGetTrend != nil {
		s.onGetTrend(userID, trendType, start, end)
	}
	return s.trend, s.trendErr
}

func (s *mhTrendStubRepo) Ping(_ context.Context) error { return nil }