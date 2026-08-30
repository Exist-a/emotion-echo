// Package logic — reports_trend_logic_test.go
//
// Sibling test for reports_trend_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 1 RED commit: cover the trend report logic
// contract. Coverage matrix:
//
//   - happy path weekly: 2 points returned in time order
//   - trend type monthly / yearly (same code path, different label)
//   - invalid trend type → validation error
//   - invalid date range (start > end) → validation error
//   - bad date format → validation error
//   - repo error propagates
package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReportsTrendLogic_HappyPathWeekly_TwoPoints(t *testing.T) {
	t.Parallel()
	want := &repository.TrendReport{
		UserID:    7,
		Type:      "weekly",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-14",
		Points: []repository.TrendPoint{
			{Date: "2026-07-01", PrimaryEmotion: "happy", AvgSentiment: 0.5, AvgConfidence: 0.9, Count: 3},
			{Date: "2026-07-08", PrimaryEmotion: "calm", AvgSentiment: 0.7, AvgConfidence: 0.85, Count: 5},
		},
	}
	repo := &fakeReportRepo{getTrendReport: want}
	l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))

	resp, err := l.GetTrendReport(&types.GetTrendReportReq{
		UserID:    7,
		Type:      "weekly",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-14",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Report)
	assert.Equal(t, "weekly", resp.Report.Type)
	assert.Len(t, resp.Report.Points, 2)
	assert.Equal(t, "happy", resp.Report.Points[0].PrimaryEmotion)
}

func TestReportsTrendLogic_InvalidTrendType_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo{}
	l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))

	_, err := l.GetTrendReport(&types.GetTrendReportReq{
		UserID:    1,
		Type:      "bogus-type",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-14",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type")
}

func TestReportsTrendLogic_InvertedDateRange_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo{}
	l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))

	_, err := l.GetTrendReport(&types.GetTrendReportReq{
		UserID:    1,
		Type:      "weekly",
		StartDate: "2026-07-14",
		EndDate:   "2026-07-01",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start")
}

func TestReportsTrendLogic_BadDateFormat_ReturnsError(t *testing.T) {
	t.Parallel()
	repo := &fakeReportRepo{}
	l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))

	_, err := l.GetTrendReport(&types.GetTrendReportReq{
		UserID:    1,
		Type:      "weekly",
		StartDate: "garbage",
		EndDate:   "2026-07-14",
	})
	require.Error(t, err)
}

func TestReportsTrendLogic_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("postgres query timeout")
	repo := &fakeReportRepo{getTrendReportErr: boom}
	l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))

	_, err := l.GetTrendReport(&types.GetTrendReportReq{
		UserID:    1,
		Type:      "weekly",
		StartDate: "2026-07-01",
		EndDate:   "2026-07-14",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestReportsTrendLogic_AllThreeTrendTypes_Accepted(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"weekly", "monthly", "yearly"} {
		typ := typ
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			repo := &fakeReportRepo{
				getTrendReport: &repository.TrendReport{Type: typ, Points: []repository.TrendPoint{}},
			}
			l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))
			_, err := l.GetTrendReport(&types.GetTrendReportReq{
				UserID:    1,
				Type:      typ,
				StartDate: "2026-01-01",
				EndDate:   "2026-12-31",
			})
			require.NoError(t, err)
		})
	}
}

func TestReportsTrendLogic_EmptyPoints_NeverNil(t *testing.T) {
	t.Parallel()
	// 用户在区间内无数据 → Points 必须是非 nil 空 slice，
	// JSON 序列化为 [] 而不是 null。
	repo := &fakeReportRepo{
		getTrendReport: &repository.TrendReport{
			UserID:    1,
			Type:      "weekly",
			StartDate: "2026-07-01",
			EndDate:   "2026-07-07",
			Points:    []repository.TrendPoint{},
		},
	}
	l := NewReportsTrendLogic(context.Background(), newReportsDailySvcCtx(repo))
	resp, err := l.GetTrendReport(&types.GetTrendReportReq{
		UserID: 1, Type: "weekly", StartDate: "2026-07-01", EndDate: "2026-07-07",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp.Report.Points)
	assert.Empty(t, resp.Report.Points)
}