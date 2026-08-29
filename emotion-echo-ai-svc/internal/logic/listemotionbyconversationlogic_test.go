// Package logic — listemotionbyconversationlogic_test.go
//
// Sibling test for listemotionbyconversationlogic.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover the missing
// listemotionbyconversationlogic test surface. The pre-existing 2 tests
// in querylogic_test.go are migrated here and extended:
//
//   - happy path (multiple analyses in one conv + cross-conv filter) —
//     returns only the in-conversation rows in repo order.
//   - empty conversation — returns empty (non-nil) slice.
//   - repo error — propagated verbatim.
//   - single-conv single-row — boundary case.
//   - cross-conversation filter sanity — a row in convID=60 must NOT
//     appear when querying convID=50.
//
// InMemoryEmotionRepo is the live test double from
// emotion_repository.go — no snapshot-copy of internals.
package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListEmotionByConversationLogic_HappyPath_FiltersAndOrders
// covers the canonical read path: 3 rows across 2 conversations,
// query for convID=50 → 2 rows in repo insertion order; the row in
// convID=60 must NOT appear.
func TestListEmotionByConversationLogic_HappyPath_FiltersAndOrders(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 2, ConversationID: 50, PrimaryEmotion: "anxious",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 3, ConversationID: 60, PrimaryEmotion: "calm", // different conv
	}))

	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewListEmotionByConversationLogic(context.Background(), svcCtx)

	resp, err := l.ListEmotionByConversation(&types.ListEmotionByConversationReq{ConversationId: 50})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Emotions, 2)
	assert.Equal(t, "happy", resp.Emotions[0].PrimaryEmotion)
	assert.Equal(t, "anxious", resp.Emotions[1].PrimaryEmotion)
	// Cross-conv filter sanity
	for _, e := range resp.Emotions {
		assert.Equal(t, int64(50), e.ConversationId)
	}
}

// TestListEmotionByConversationLogic_EmptyConv_ReturnsEmptySlice
// pins the contract: caller can safely range over the result even
// when there are no rows. The response slice MUST be non-nil.
func TestListEmotionByConversationLogic_EmptyConv_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewListEmotionByConversationLogic(context.Background(), svcCtx)

	resp, err := l.ListEmotionByConversation(&types.ListEmotionByConversationReq{ConversationId: 999})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.NotNil(t, resp.Emotions, "should return empty slice, not nil")
	assert.Empty(t, resp.Emotions)
}

// TestListEmotionByConversationLogic_RepoError_PropagatesAsIs
// covers the "DB transient error" branch using a wrapped repo.
func TestListEmotionByConversationLogic_RepoError_PropagatesAsIs(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	wrapped := &failingListEmotionRepo{
		EmotionRepo: repo,
		listErr:     errors.New("postgres: deadlock detected"),
	}
	svcCtx := &svc.ServiceContext{EmotionRepo: wrapped}
	l := NewListEmotionByConversationLogic(context.Background(), svcCtx)

	resp, err := l.ListEmotionByConversation(&types.ListEmotionByConversationReq{ConversationId: 50})
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadlock")
}

// TestListEmotionByConversationLogic_SingleRow_OneConv covers the
// boundary: exactly one matching row, exactly one element returned.
func TestListEmotionByConversationLogic_SingleRow_OneConv(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 99, ConversationID: 77, PrimaryEmotion: "neutral",
		Confidence: 0.5, SentimentScore: 0.0, Model: "keyword-v1",
	}))

	svcCtx := &svc.ServiceContext{EmotionRepo: repo}
	l := NewListEmotionByConversationLogic(context.Background(), svcCtx)

	resp, err := l.ListEmotionByConversation(&types.ListEmotionByConversationReq{ConversationId: 77})
	require.NoError(t, err)
	require.Len(t, resp.Emotions, 1)
	assert.Equal(t, "neutral", resp.Emotions[0].PrimaryEmotion)
	assert.Equal(t, int64(99), resp.Emotions[0].MessageId)
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// failingListEmotionRepo forces ListByConversationID to return listErr.
type failingListEmotionRepo struct {
	repository.EmotionRepo
	listErr error
}

func (r *failingListEmotionRepo) ListByConversationID(ctx context.Context, conversationID int64) ([]model.EmotionAnalysis, error) {
	return nil, r.listErr
}

var _ repository.EmotionRepo = (*failingListEmotionRepo)(nil)