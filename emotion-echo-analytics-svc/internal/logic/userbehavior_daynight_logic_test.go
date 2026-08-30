// Package logic — userbehavior_daynight_logic_test.go
//
// Sibling test for userbehavior_daynight_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 2 RED commit: cover the GET /api/v1/user-behavior/day-night
// endpoint contract.
//
// Coverage matrix:
//
//   - HappyPath: 24 个 hour 桶（缺失 hour 计为 0）
//   - Validation: UserID <= 0
//   - Empty data: 全 0 map（但 len == 24）
//   - Repo error propagates
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

func newDayNightSvcCtx(repo repository.EventRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, EventRepo: repo}
}

func TestUserBehaviorDayNightLogic_HappyPath_All24Buckets(t *testing.T) {
	t.Parallel()
	want := map[int]int64{
		0: 5, 1: 3, 8: 12, 14: 20, 22: 8,
	}
	repo := &stubEventRepo{dayNight: want}
	l := NewUserBehaviorDayNightLogic(context.Background(), newDayNightSvcCtx(repo))

	resp, err := l.GetDayNightPattern(&types.GetDayNightPatternReq{
		UserID:    42,
		StartDate: "2026-07-01",
		EndDate:   "2026-07-30",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Pattern, 24)
	assert.Equal(t, int64(5), resp.Pattern[0])
	assert.Equal(t, int64(20), resp.Pattern[14])
	assert.Equal(t, int64(0), resp.Pattern[12], "缺失 hour 应补 0")
}

func TestUserBehaviorDayNightLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{}
	l := NewUserBehaviorDayNightLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetDayNightPattern(&types.GetDayNightPatternReq{
		UserID:    0,
		StartDate: "2026-07-01",
		EndDate:   "2026-07-30",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}

func TestUserBehaviorDayNightLogic_BadDateFormat(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{}
	l := NewUserBehaviorDayNightLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetDayNightPattern(&types.GetDayNightPatternReq{
		UserID:    1,
		StartDate: "garbage",
		EndDate:   "2026-07-30",
	})
	require.Error(t, err)
}

func TestUserBehaviorDayNightLogic_RepoError(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{dayNightErr: errors.New("kafka lag too high")}
	l := NewUserBehaviorDayNightLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetDayNightPattern(&types.GetDayNightPatternReq{
		UserID:    1,
		StartDate: "2026-07-01",
		EndDate:   "2026-07-30",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka lag")
}

func TestUserBehaviorDayNightLogic_EmptyData_All24BucketsAreZero(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{dayNight: map[int]int64{}} // 无任何事件
	l := NewUserBehaviorDayNightLogic(context.Background(), newDayNightSvcCtx(repo))

	resp, err := l.GetDayNightPattern(&types.GetDayNightPatternReq{
		UserID:    1,
		StartDate: "2026-07-01",
		EndDate:   "2026-07-30",
	})
	require.NoError(t, err)
	assert.Len(t, resp.Pattern, 24)
	for h := 0; h < 24; h++ {
		assert.Equal(t, int64(0), resp.Pattern[h])
	}
}

// stubEventRepo 仅覆盖 Round 2 测试需要的 EventRepo 方法。
// 嵌入 ReportRepo 让未实现方法 panic（如果误调），同时让本类型
// 满足 repository.EventRepo 接口。
type stubEventRepo struct {
	repository.ReportRepo

	dayNight    map[int]int64
	dayNightErr error

	depth    *repository.InteractionDepth
	depthErr error

	freq    []repository.DailyCount
	freqErr error
}

func (s *stubEventRepo) GetDayNightPattern(_ context.Context, _ int64, _, _ time.Time) (map[int]int64, error) {
	return s.dayNight, s.dayNightErr
}

func (s *stubEventRepo) GetInteractionDepth(_ context.Context, _ int64, _, _ time.Time) (*repository.InteractionDepth, error) {
	return s.depth, s.depthErr
}

func (s *stubEventRepo) GetFrequencyTrend(_ context.Context, _ int64, _, _ time.Time) ([]repository.DailyCount, error) {
	return s.freq, s.freqErr
}