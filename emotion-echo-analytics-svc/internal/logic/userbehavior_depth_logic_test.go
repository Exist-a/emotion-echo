// Package logic — userbehavior_depth_logic_test.go
//
// Sibling test for userbehavior_depth_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 2 RED: cover GET /api/v1/user-behavior/depth.
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

func TestUserBehaviorDepthLogic_HappyPath(t *testing.T) {
	t.Parallel()
	want := &repository.InteractionDepth{
		TotalMessages:          42,
		TotalConversations:     5,
		AvgMessagesPerConv:     8.4,
		LongestConversationMs: 3600000,
	}
	repo := &stubEventRepo{depth: want}
	l := NewUserBehaviorDepthLogic(context.Background(), newDayNightSvcCtx(repo))

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
	// 用户无会话 → totalConversations=0 → AvgMessagesPerConv 应该是 0 而非 NaN/panic
	repo := &stubEventRepo{depth: &repository.InteractionDepth{
		TotalMessages: 0, TotalConversations: 0,
		AvgMessagesPerConv: 0, LongestConversationMs: 0,
	}}
	l := NewUserBehaviorDepthLogic(context.Background(), newDayNightSvcCtx(repo))
	resp, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.NoError(t, err)
	assert.Equal(t, float64(0), resp.Depth.AvgMessagesPerConv)
}

func TestUserBehaviorDepthLogic_RepoError(t *testing.T) {
	t.Parallel()
	boom := errors.New("connection reset")
	repo := &stubEventRepo{depthErr: boom}
	l := NewUserBehaviorDepthLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: 1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestUserBehaviorDepthLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &stubEventRepo{}
	l := NewUserBehaviorDepthLogic(context.Background(), newDayNightSvcCtx(repo))
	_, err := l.GetInteractionDepth(&types.GetInteractionDepthReq{
		UserID: -1, StartDate: "2026-07-01", EndDate: "2026-07-30",
	})
	require.Error(t, err)
}