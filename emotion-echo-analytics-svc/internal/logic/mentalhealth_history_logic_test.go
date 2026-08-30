// Package logic — mentalhealth_history_logic_test.go
//
// Sibling test for mentalhealth_history_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 3 part 1 RED: cover GET /api/v1/mental-health/history.
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

func TestMentalHealthHistoryLogic_HappyPath_Pagination(t *testing.T) {
	t.Parallel()
	want := []repository.AssessmentHistoryItem{
		{
			ID: 3, UserID: 1, AssessmentType: "PHQ-9",
			OverallScore: 22, RiskLevel: "low",
			SubmittedAt: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC),
		},
		{
			ID: 2, UserID: 1, AssessmentType: "PHQ-9",
			OverallScore: 18, RiskLevel: "low",
			SubmittedAt: time.Date(2026, 7, 8, 10, 0, 0, 0, time.UTC),
		},
	}
	repo := &mhStubRepo{history: want, historyNextCur: "next-page-token-xyz"}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))

	resp, err := l.ListHistory(&types.GetMentalHealthHistoryReq{
		UserID: 1, Type: "PHQ-9", Limit: 20,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Items, 2)
	assert.Equal(t, "next-page-token-xyz", resp.NextCursor)
}

func TestMentalHealthHistoryLogic_LimitZero_ClampsToDefault(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	repo := &mhStubRepo{
		onListHistory: func(_ int64, _, _ string, limit int) {
			capturedLimit = limit
		},
		history: []repository.AssessmentHistoryItem{},
	}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 1, Limit: 0})
	require.NoError(t, err)
	assert.Equal(t, 20, capturedLimit, "limit <= 0 应 clamp 到 20")
}

func TestMentalHealthHistoryLogic_LimitOver100_ClampsTo100(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	repo := &mhStubRepo{
		onListHistory: func(_ int64, _, _ string, limit int) {
			capturedLimit = limit
		},
		history: []repository.AssessmentHistoryItem{},
	}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 1, Limit: 500})
	require.NoError(t, err)
	assert.Equal(t, 100, capturedLimit)
}

func TestMentalHealthHistoryLogic_LimitInRange_PassThrough(t *testing.T) {
	t.Parallel()
	var capturedLimit int
	repo := &mhStubRepo{
		onListHistory: func(_ int64, _, _ string, limit int) {
			capturedLimit = limit
		},
		history: []repository.AssessmentHistoryItem{},
	}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 1, Limit: 50})
	require.NoError(t, err)
	assert.Equal(t, 50, capturedLimit)
}

func TestMentalHealthHistoryLogic_CursorForwarded(t *testing.T) {
	t.Parallel()
	var capturedCursor string
	repo := &mhStubRepo{
		onListHistory: func(_ int64, _, cursor string, _ int) {
			capturedCursor = cursor
		},
		history: []repository.AssessmentHistoryItem{},
	}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 1, Cursor: "abc123"})
	require.NoError(t, err)
	assert.Equal(t, "abc123", capturedCursor)
}

func TestMentalHealthHistoryLogic_EmptyHistory_NeverNil(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{history: nil}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	resp, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 1, Limit: 20})
	require.NoError(t, err)
	assert.NotNil(t, resp.Items, "空历史时 items 不能是 nil（JSON [] 而非 null）")
	assert.Empty(t, resp.Items)
}

func TestMentalHealthHistoryLogic_RepoError(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{historyErr: errors.New("kafka lag")}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 1, Limit: 20})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kafka")
}

func TestMentalHealthHistoryLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	repo := &mhStubRepo{}
	l := NewMentalHealthHistoryLogic(context.Background(), newAssessmentSvcCtx(repo))
	_, err := l.ListHistory(&types.GetMentalHealthHistoryReq{UserID: 0, Limit: 20})
	require.Error(t, err)
}