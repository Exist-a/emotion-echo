// Package logic — getemotionbymessagelogic_test.go
//
// Sibling test for getemotionbymessagelogic.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover the missing getemotionbymessagelogic
// test surface. The pre-existing 2 tests in querylogic_test.go are
// migrated here and extended:
//
//   - happy path (existing analysis) — full EmotionView field mapping.
//   - not found — "not found" error, nil response.
//   - repo error (transient DB issue) — propagated verbatim.
//   - zero-value message ID — repo still called; returns nil/NotFound
//     (callers are expected to validate >0 upstream; this documents the
//     current behavior so any future validation is a deliberate change).
//
// InMemoryEmotionRepo is the live test double from
// emotion_repository.go — no snapshot-copy of internals.
package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetEmotionByMessageLogic_HappyPath_FullFieldMapping covers the
// canonical read path: existing analysis → returns full EmotionView
// with every field populated from the model.
func TestGetEmotionByMessageLogic_HappyPath_FullFieldMapping(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	now := time.Now()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID:      100,
		UserID:         7,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.6,
		Confidence:     0.85,
		Model:          "keyword-v1",
		CreatedAt:      now,
	}))

	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewGetEmotionByMessageLogic(context.Background(), svcCtx)

	resp, err := l.GetEmotionByMessage(&types.GetEmotionByMessageReq{MessageId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Emotion)
	assert.Equal(t, "happy", resp.Emotion.PrimaryEmotion)
	assert.Equal(t, int64(100), resp.Emotion.MessageId)
	assert.Equal(t, int64(50), resp.Emotion.ConversationId)
	assert.Equal(t, int64(7), resp.Emotion.UserId)
	assert.InDelta(t, 0.6, resp.Emotion.SentimentScore, 0.001)
	assert.InDelta(t, 0.85, resp.Emotion.Confidence, 0.001)
	assert.Equal(t, "keyword-v1", resp.Emotion.Model)
	assert.Equal(t, now.UnixMilli(), resp.Emotion.CreatedAt)
	assert.Greater(t, resp.Emotion.Id, int64(0))
}

// TestGetEmotionByMessageLogic_NotFound_ReturnsNotFoundError is the
// documented behavior: when the message has no analysis, the logic
// returns nil + a "not found" error (NOT repository.ErrNotFound — the
// implementation uses an inline error message; we assert the contract
// surface, not the literal sentinel).
func TestGetEmotionByMessageLogic_NotFound_ReturnsNotFoundError(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewGetEmotionByMessageLogic(context.Background(), svcCtx)

	resp, err := l.GetEmotionByMessage(&types.GetEmotionByMessageReq{MessageId: 999})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// TestGetEmotionByMessageLogic_RepoError_PropagatesAsIs covers the
// "DB transient error" branch: repo returns an error, logic propagates
// it verbatim so the handler can map it to a 5xx. We use a wrapper
// repo that always errors.
func TestGetEmotionByMessageLogic_RepoError_PropagatesAsIs(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	wrapped := &failingEmotionRepo{
		EmotionRepo: repo,
		getByMsgErr: errors.New("postgres: connection reset"),
	}
	svcCtx := &svc.ServiceContext{EmotionRepo: wrapped}
	l := NewGetEmotionByMessageLogic(context.Background(), svcCtx)

	resp, err := l.GetEmotionByMessage(&types.GetEmotionByMessageReq{MessageId: 100})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset")
}

// TestGetEmotionByMessageLogic_ZeroMessageID_ReturnsNotFound pins the
// current behavior for the boundary req.MessageId == 0. The InMemory
// repo has no entry for id=0, so this matches the not-found path.
// This documents behavior — a future tightening (validation) is
// a deliberate change, not a regression.
func TestGetEmotionByMessageLogic_ZeroMessageID_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewGetEmotionByMessageLogic(context.Background(), svcCtx)

	resp, err := l.GetEmotionByMessage(&types.GetEmotionByMessageReq{MessageId: 0})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// failingEmotionRepo forces GetByMessageID to return getByMsgErr.
type failingEmotionRepo struct {
	repository.EmotionRepo
	getByMsgErr error
}

func (r *failingEmotionRepo) GetByMessageID(ctx context.Context, messageID int64) (*model.EmotionAnalysis, error) {
	return nil, r.getByMsgErr
}

var _ repository.EmotionRepo = (*failingEmotionRepo)(nil)