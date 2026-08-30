// Package logic — userbehavior_depth_logic_test.go
//
// Sibling test for userbehavior_depth_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 2 RED: cover GET /api/v1/user-behavior/depth.
//
// Each test file owns its own stubEventRepo helper (per AGENTS §1.1
// '不要把多个 _test.go 合并' — sibling tests don't share helpers
// across files; duplication is intentional).
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

func newDepthSvcCtx(repo repository.EventRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, EventRepo: repo}
}

func TestUserBehaviorDepthLogic_HappyPath(t *testing.T) {
	t.Parallel()
	want := &repository.InteractionDepth{
		TotalMessages:         42,
		TotalConversations:    5,
		AvgMessagesPerConv:    8.4,
		LongestConversationMs: 3600000,
	}
	repo := &depthStubEventRepo{depth: want}
	l := NewUserBehaviorDepthLogic(context.Background(), newDepthSvcCtx(repo))

	resp, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Depth)
	assert.Equal(t, int64(42), resp.Depth.TotalMessages)
	assert.InDelta(t, 8.4, resp.Depth.AvgMessagesPerConv, 0.01)
}

func TestUserBehaviorDepthLogic_ZeroConversations_DividesByZero(t *testing.T) {
	t.Parallel()
	repo := &depthStubEventRepo{depth: &repository.InteractionDepth{
		TotalMessages: 0, TotalConversations: 0,
		AvgMessagesPerConv: 0, LongestConversationMs: 0,
	}}
	l := NewUserBehaviorDepthLogic(context.Background(), newDepthSvcCtx(repo))
	resp, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp.Depth.AvgMessagesPerConv)
}

func TestUserBehaviorDepthLogic_RepoError(t *testing.T) {
	t.Parallel()
	boom := errors.New("connection reset")
	repo := &depthStubEventRepo{depthErr: boom}
	l := NewUserBehaviorDepthLogic(context.Background(), newDepthSvcCtx(repo))
	_, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestUserBehaviorDepthLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &depthStubEventRepo{}
	l := NewUserBehaviorDepthLogic(context.Background(), newDepthSvcCtx(repo))
	_, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: -1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}

// depthStubEventRepo 仅覆盖 depth 测试需要的 EventRepo 方法。
type depthStubEventRepo struct {
	depth    *repository.InteractionDepth
	depthErr error
}

func (s *depthStubEventRepo) GetInteractionDepth(_ context.Context, _ int64, _, _ time.Time) (*repository.InteractionDepth, error) {
	return s.depth, s.depthErr
}

// 其他 EventRepo 方法 stub（不被 depth 测试调用）
func (s *depthStubEventRepo) GetDayNightPattern(_ context.Context, _ int64, _, _ time.Time) (map[int]int64, error) {
	return nil, nil
}

func (s *depthStubEventRepo) GetFrequencyTrend(_ context.Context, _ int64, _, _ time.Time) ([]repository.DailyCount, error) {
	return nil, nil
}

func (s *depthStubEventRepo) GetByID(_ context.Context, _ int64) (*model.UserBehaviorEvent, error) {
	return nil, nil
}

func (s *depthStubEventRepo) Create(_ context.Context, _ *model.UserBehaviorEvent) error { return nil }

func (s *depthStubEventRepo) Ping(_ context.Context) error { return nil }