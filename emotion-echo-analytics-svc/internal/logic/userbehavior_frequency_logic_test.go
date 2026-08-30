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

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserBehaviorFrequencyLogic_HappyPath(t *testing.T) {
	t.Parallel()
	want := []repository.DailyCount{
		{Date: "2026-07-01", Count: 5},
		{Date: "2026-07-02", Count: 3},
		{Date: "2026-07-03", Count: 8},
	}
	repo := &stubEventRepo{freq: want}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newDayNightSvcCtx(repo))

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
	// 用户无事件 → counts 必须是空 slice（JSON [] 而非 null）
	repo := &stubEventRepo{freq: []repository.DailyCount{}}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newDayNightSvcCtx(repo))
	resp, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.NoError(t, err)
	assert.NotNil(t, resp.Counts)
	assert.Empty(t, resp.Counts)
}

func TestUserBehaviorFrequencyLogic_WindowLimitOver90Days(t *testing.T) {
	t.Parallel()
	// 按 backlog §二 / stage-30-A §四 implementation note：30d 日计数。
	// 暂不强制 90 天上限（与 day-night/depth 行为一致）；保留扩展点。
	repo := &stubEventRepo{freq: []repository.DailyCount{}}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-01-01", EndDate: "2026-12-31",
	})
	require.NoError(t, err, "无 90 天上限时，1 年窗口应该被接受")
}

func TestUserBehaviorFrequencyLogic_RepoError(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{freqErr: errors.New("kafka consumer lag > 1h")}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka")
}

func TestUserBehaviorFrequencyLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{}
	l := NewUserBehaviorFrequencyLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetFrequencyTrend(&types.GetFrequencyTrendReq{
		UserID: 0, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}