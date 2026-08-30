// Package logic — userbehavior_frequency_logic_test.go
//
// Sibling test for userbehavior_frequency_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 2 RED: cover GET /api/v1/user-behavior/frequency.
package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/model"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFreqSvcCtx(repo repository.EventRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, EventRepo: repo}
}

func TestUserBehaviorFrequencyLogic_HappyPath(t *testing.T) {
	t.Parallel()
	want := []repository.DailyCount{
		{Date: "2026-07-01", Count: 5},
		{Date: "2026-07-02", Count: 3},
		{Date: "2026-07-03", Count: 8},
	}
	repo := &freqStubEventRepo{freq: want}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newFreqSvcCtx(repo))

	resp, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Counts, 3)
	assert.Equal(t, int64(5), resp.Counts[0].Count)
}

func TestUserBehaviorFrequencyLogic_EmptyCountsNeverNil(t *testing.T) {
	t.Parallel()
	repo := &freqStubEventRepo{freq: []repository.DailyCount{}}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newFreqSvcCtx(repo))
	resp, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp.Counts)
	assert.Empty(t, resp.Counts)
}

func TestUserBehaviorFrequencyLogic_OneYearWindow_Accepted(t *testing.T) {
	t.Parallel()
	// backlog 没强制 90 天上限。1 年窗口应被接受。
	repo := &freqStubEventRepo{freq: []repository.DailyCount{}}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newFreqSvcCtx(repo))
	_, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-01-01", EndDate: "2026-12-31",
	})
	require.NoError(t, err)
}

func TestUserBehaviorFrequencyLogic_RepoError(t *testing.T) {
	t.Parallel()
	repo := &freqStubEventRepo{freqErr: errors.New("kafka consumer lag > 1h")}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newFreqSvcCtx(repo))
	_, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka")
}

func TestUserBehaviorFrequencyLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &freqStubEventRepo{}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newFreqSvcCtx(repo))
	_, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 0, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}

// freqStubEventRepo 仅覆盖 frequency 测试需要的方法。
type freqStubEventRepo struct {
	freq    []repository.DailyCount
	freqErr error
}

func (s *freqStubEventRepo) GetFrequencyTrend(_ context.Context, _ int64, _, _ time.Time) ([]repository.DailyCount, error) {
	return s.freq, s.freqErr
}

// 其他 EventRepo 方法 stub
func (s *freqStubEventRepo) GetDayNightPattern(_ context.Context, _ int64, _, _ time.Time) (map[int]int64, error) {
	return nil, nil
}

func (s *freqStubEventRepo) GetInteractionDepth(_ context.Context, _ int64, _, _ time.Time) (*repository.InteractionDepth, error) {
	return nil, nil
}

func (s *freqStubEventRepo) GetByID(_ context.Context, _ int64) (*model.UserBehaviorEvent, error) {
	return nil, nil
}

func (s *freqStubEventRepo) Create(_ context.Context, _ *model.UserBehaviorEvent) error { return nil }

func (s *freqStubEventRepo) Ping(_ context.Context) error { return nil }