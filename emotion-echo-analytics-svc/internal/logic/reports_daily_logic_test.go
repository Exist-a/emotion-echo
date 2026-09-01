// Package logic — reports_daily_logic_test.go
//
// Sibling test for reports_daily_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 1 RED commit: cover the daily report logic
// contract. Logic signature:
//
//   func NewReportsDailyLogic(ctx, svcCtx) *ReportsDailyLogic
//   func (l *ReportsDailyLogic) GetDailyReport(req *types.GetDailyReportReq)
//       (*types.GetDailyReportResp, error)
//
// Coverage matrix:
//
//   - happy path: req has userID + date → resp.Report populated
//   - date default: empty date → today
//   - bad date format → validation error
//   - repo error propagates verbatim
//   - user with no events → empty EmotionCounts + zero counts (NOT nil)
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

func newReportsDailySvcCtx(repo repository.ReportRepo) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:    config.Config{},
		EventRepo: nil, // ReportRepo 独立字段（Round 1 扩展 ServiceContext）
		ReportRepo: repo,
	}
}

func TestReportsDailyLogic_HappyPath_PopulatesReport(t *testing.T) {
	t.Parallel()
	want := &repository.DailyReport{
		UserID:            42,
		Date:              "2026-07-15",
		EmotionCounts:     map[string]int64{"happy": 3, "sad": 1},
		MessageCount:      10,
		ConversationCount: 2,
		AssessmentCount:   1,
		AvgSentiment:      0.42,
		AvgConfidence:     0.85,
	}
	repo := &fakeReportRepo{getDailyReport: want}
	l := NewReportsDailyLogic(context.Background(), newReportsDailySvcCtx(repo))

	resp, err := l.GetDailyReport(&types.GetDailyReportReq{
		UserID: 42,
		Date:   "2026-07-15",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Report)
	assert.Equal(t, int64(42), resp.Report.UserID)
	assert.Equal(t, "2026-07-15", resp.Report.Date)
	assert.Equal(t, int64(3), resp.Report.EmotionCounts["happy"])
	assert.Equal(t, int64(10), resp.Report.MessageCount)
	assert.InDelta(t, 0.42, resp.Report.AvgSentiment, 0.001)
}

// TestReportsDailyLogic_PopulatesEmotionDistributionByModality 验证 Stage 34 新字段。
//
// 期望 logic 调 ModalityReportRepo.GetDailyEmotionByModality 并把结果填进
// DailyReport.EmotionDistributionByModality。当前实现未做 → 必须失败（RED）。
func TestReportsDailyLogic_PopulatesEmotionDistributionByModality(t *testing.T) {
	t.Parallel()

	wantModality := &repository.ModalityEmotionDistribution{
		Text:  map[string]int64{"happy": 3, "sad": 1},
		Face:  map[string]int64{"neutral": 2},
		Voice: map[string]int64{"happy": 1},
	}

	reportRepo := &fakeReportRepo{
		getDailyReport: &repository.DailyReport{
			UserID: 42, Date: "2026-09-01",
			EmotionCounts: map[string]int64{"happy": 4, "sad": 1},
		},
	}
	modalityRepo := &fakeModalityReportRepo{
		getDailyEmotionByModality: wantModality,
	}

	ctx := &svc.ServiceContext{
		Config:           config.Config{},
		ReportRepo:       reportRepo,
		ModalityReportRepo: modalityRepo,
	}
	l := NewReportsDailyLogic(context.Background(), ctx)
	resp, err := l.GetDailyReport(&types.GetDailyReportReq{
		UserID: 42,
		Date:   "2026-09-01",
	})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)

	require.NotNil(t, resp.Report.EmotionDistributionByModality,
		"EmotionDistributionByModality must be populated by logic")
	assert.Equal(t, map[string]int64{"happy": 3, "sad": 1}, resp.Report.EmotionDistributionByModality.Text)
	assert.Equal(t, map[string]int64{"neutral": 2}, resp.Report.EmotionDistributionByModality.Face)
	assert.Equal(t, map[string]int64{"happy": 1}, resp.Report.EmotionDistributionByModality.Voice)
}

// fakeModalityReportRepo 满足 ModalityReportRepo 接口
type fakeModalityReportRepo struct {
	repository.ModalityReportRepo // embedded interface for forward-compat

	getDailyEmotionByModality *repository.ModalityEmotionDistribution
	getDailyEmotionByModalityErr error
}

func (f *fakeModalityReportRepo) GetDailyEmotionByModality(_ context.Context, _ int64, _ time.Time) (*repository.ModalityEmotionDistribution, error) {
	return f.getDailyEmotionByModality, f.getDailyEmotionByModalityErr
}

func (f *fakeModalityReportRepo) Ping(_ context.Context) error { return nil }

func TestReportsDailyLogic_EmptyDate_DefaultsToToday(t *testing.T) {
	t.Parallel()
	var capturedDate time.Time
	repo := &fakeReportRepo{
		getDailyReport: &repository.DailyReport{UserID: 1, Date: "today"},
		onGetDaily: func(userID int64, date time.Time) {
			capturedDate = date
		},
	}
	l := NewReportsDailyLogic(context.Background(), newReportsDailySvcCtx(repo))

	before := time.Now()
	_, err := l.GetDailyReport(&types.GetDailyReportReq{UserID: 1})
	after := time.Now()
	require.NoError(t, err)

	// capturedDate must be today's date (year/month/day match
	// before/after). It may be midnight or any time of day — we
	// only assert the calendar date is "today".
	assert.Equal(t, before.Year(), capturedDate.Year(),
		"year should match today's calendar date")
	assert.Equal(t, before.Month(), capturedDate.Month(),
		"month should match today's calendar date")
	assert.Equal(t, before.Day(), capturedDate.Day(),
		"day should match today's calendar date")
	assert.Equal(t, after.Year(), capturedDate.Year(),
		"year should match today's calendar date")
	assert.Equal(t, after.Month(), capturedDate.Month(),
		"month should match today's calendar date")
	assert.Equal(t, after.Day(), capturedDate.Day(),
		"day should match today's calendar date")
}

func TestReportsDailyLogic_BadDateFormat_ReturnsValidationError(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo{getDailyReport: &repository.DailyReport{}}
	l := NewReportsDailyLogic(context.Background(), newReportsDailySvcCtx(repo))

	_, err := l.GetDailyReport(&types.GetDailyReportReq{
		UserID: 1,
		Date:   "not-a-date",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "date")
}

func TestReportsDailyLogic_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("database connection lost")
	repo := &fakeReportRepo{getDailyReportErr: boom}
	l := NewReportsDailyLogic(context.Background(), newReportsDailySvcCtx(repo))

	_, err := l.GetDailyReport(&types.GetDailyReportReq{UserID: 1, Date: "2026-07-15"})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestReportsDailyLogic_NoData_EmptyEmotionCountsNotNil(t *testing.T) {
	t.Parallel()
	// 用户当天没有任何数据 → emotionCounts 必须是空 map（不是 nil），
	// 这样 JSON 序列化为 {} 而不是 null，前端能正常渲染。
	repo := &fakeReportRepo{
		getDailyReport: &repository.DailyReport{
			UserID:        1,
			Date:          "2026-07-15",
			EmotionCounts: map[string]int64{},
		},
	}
	l := NewReportsDailyLogic(context.Background(), newReportsDailySvcCtx(repo))

	resp, err := l.GetDailyReport(&types.GetDailyReportReq{UserID: 1, Date: "2026-07-15"})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)
	assert.NotNil(t, resp.Report.EmotionCounts)
	assert.Empty(t, resp.Report.EmotionCounts)
}

// fakeReportRepo 满足 repository.ReportRepo 接口；只让 GetDailyReport
// / GetTrendReport 可注入行为，其他方法 panic。
type fakeReportRepo struct {
	repository.EventRepo    // embed for unmocked methods (panic if called)
	repository.ReportRepo   // interface satisfaction (methods below override)

	getDailyReport    *repository.DailyReport
	getDailyReportErr error
	onGetDaily        func(userID int64, date time.Time)

	getTrendReport    *repository.TrendReport
	getTrendReportErr error
	onGetTrend        func(userID int64, trendType string, start, end time.Time)
}

func (f *fakeReportRepo) GetDailyReport(_ context.Context, userID int64, date time.Time) (*repository.DailyReport, error) {
	if f.onGetDaily != nil {
		f.onGetDaily(userID, date)
	}
	return f.getDailyReport, f.getDailyReportErr
}

func (f *fakeReportRepo) GetTrendReport(_ context.Context, userID int64, trendType string, start, end time.Time) (*repository.TrendReport, error) {
	if f.onGetTrend != nil {
		f.onGetTrend(userID, trendType, start, end)
	}
	return f.getTrendReport, f.getTrendReportErr
}

func (f *fakeReportRepo) Ping(_ context.Context) error { return nil }