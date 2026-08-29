// Package logic — listmessageslogic_test.go
//
// Sibling test for listmessageslogic.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.1: cover the missing listmessageslogic test
// surface. Coverage targets:
//
//   - happy path: own conversation + multiple messages → returns
//     MessageView list in repo order.
//   - default limit: req.Limit == 0 → repo receives 50 (default).
//   - custom limit: req.Limit == 5 → repo receives 5.
//   - empty result: own conversation with no messages → empty slice
//     (not nil result).
//   - missing user id in ctx → unauthorized, no repo call observed.
//   - conversation not found → "not found" error.
//   - wrong owner → "forbidden" error.
//   - repo error (e.g. transient DB issue) → propagated as-is.
//
// As with sendmessagelogic_test.go, no snapshot-copy of constants —
// helpers (newTestCtx, ctxWithUserID) come from the existing sibling
// createconversationlogic_test.go.
package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListMessagesLogic_HappyPath_ReturnsMessageViews covers the
// canonical read path: own conversation + N messages → returns the
// same N MessageViews with all fields mapped through.
func TestListMessagesLogic_HappyPath_ReturnsMessageViews(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "test",
	}))
	// Seed 3 messages via AppendMessage (uses InMemoryConversationRepo).
	for _, content := range []string{"hi", "how are you?", "goodbye"} {
		require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
			ConversationID: 1,
			UserID:        100,
			Role:          "user",
			Content:       content,
		}))
	}

	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 50})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Len(t, resp.Messages, 3)

	// Fields mapped through (content order matches repo order).
	assert.Equal(t, "hi", resp.Messages[0].Content)
	assert.Equal(t, "how are you?", resp.Messages[1].Content)
	assert.Equal(t, "goodbye", resp.Messages[2].Content)
	// Common fields.
	for i, m := range resp.Messages {
		assert.Equal(t, int64(1), m.ConversationId)
		assert.Equal(t, int64(100), m.UserId)
		assert.Equal(t, "user", m.Role)
		assert.Greater(t, m.Id, int64(0), "message %d should have a non-zero id", i)
	}
}

// TestListMessagesLogic_ZeroLimit_DefaultsTo50 covers the implicit
// default (req.Limit <= 0 → 50). We assert via a wrapped repo that
// observes the limit value passed by the logic.
func TestListMessagesLogic_ZeroLimit_DefaultsTo50(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "test",
	}))

	var observedLimit int
	wrapped := &limitObservingRepo{
		ConversationRepo: repo,
		hook: func(l int) { observedLimit = l },
	}
	svcCtx := newSvcCtxWithRepo(wrapped)

	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 0})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 50, observedLimit, "logic should pass limit=50 to repo when req.Limit==0")
}

// TestListMessagesLogic_CustomLimit_PropagatedToRepo covers the case
// where the caller specifies a limit; the logic must pass it through.
func TestListMessagesLogic_CustomLimit_PropagatedToRepo(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "test",
	}))

	var observedLimit int
	wrapped := &limitObservingRepo{
		ConversationRepo: repo,
		hook: func(l int) { observedLimit = l },
	}
	svcCtx := newSvcCtxWithRepo(wrapped)

	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 5})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 5, observedLimit)
}

// TestListMessagesLogic_EmptyConversation_ReturnsEmptySlice verifies
// the empty-result branch: an existing conversation with no messages
// yields an empty (not nil) Messages slice. Per Go convention, the
// caller can safely range over the result without nil-check.
func TestListMessagesLogic_EmptyConversation_ReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "empty",
	}))

	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 50})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Messages, "Messages slice should be non-nil so callers can range safely")
	assert.Empty(t, resp.Messages)
}

// TestListMessagesLogic_NoUserID_ReturnsUnauthorized covers the ctx
// contract — same as sendmessagelogic: missing uid → refuse without
// touching the repo.
func TestListMessagesLogic_NoUserID_ReturnsUnauthorized(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "test",
	}))

	// Repo that records if ListMessages was called.
	wrapped := &callObservingRepo{
		ConversationRepo: repo,
	}
	svcCtx := newSvcCtxWithRepo(wrapped)

	l := NewListMessagesLogic(context.Background(), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 50})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "unauthorized")
	assert.False(t, wrapped.listMessagesCalled, "ListMessages must NOT reach repo when ctx lacks user id")
}

// TestListMessagesLogic_ConversationNotFound_ReturnsNotFoundError
// covers the missing-resource branch: returns a recognizable error,
// not a nil result.
func TestListMessagesLogic_ConversationNotFound_ReturnsNotFoundError(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)

	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 999, Limit: 50})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not found")
}

// TestListMessagesLogic_WrongOwner_ReturnsForbidden covers the
// cross-user read attempt: conversation exists but is owned by
// another user. The implementation must refuse with a forbidden-shaped
// error and NOT leak the message list (which would be an information
// disclosure bug).
func TestListMessagesLogic_WrongOwner_ReturnsForbidden(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)
	// Owned by user 7
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 7, Title: "private",
	}))
	// Seed a message — the test must NOT see it
	require.NoError(t, repo.AppendMessage(context.Background(), &model.Message{
		ConversationID: 1, UserID: 7, Role: "user", Content: "secret",
	}))

	// Caller is user 100
	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 50})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "forbidden")
}

// TestListMessagesLogic_RepoError_PropagatesAsIs covers the "DB
// transient error" branch. The implementation returns the repo error
// verbatim so the handler layer can map it to a 5xx.
func TestListMessagesLogic_RepoError_PropagatesAsIs(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryConversationRepo()
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		UserID: 100, Title: "test",
	}))

	wrapped := &failingListRepo{
		ConversationRepo: repo,
		listErr:          errors.New("postgres: connection refused"),
	}
	svcCtx := newSvcCtxWithRepo(wrapped)

	l := NewListMessagesLogic(ctxWithUserID(context.Background(), 100), svcCtx)
	resp, err := l.ListMessages(&types.ListMessagesReq{Id: 1, Limit: 50})
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "connection refused")
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers (wrappers around InMemoryConversationRepo that observe or
// override behavior for individual tests). All wrappers satisfy the
// repository.ConversationRepo interface by embedding it and overriding only
// the methods under test.
// ─────────────────────────────────────────────────────────────────────────────

// newSvcCtxWithRepo builds a real *svc.ServiceContext with the given repo
// and an in-memory EventPublisher. Used by tests that need a wrapped repo
// without going through the full newTestCtx helper (which seeds its own
// in-memory repo).
func newSvcCtxWithRepo(repo repository.ConversationRepo) *svc.ServiceContext {
	return &svc.ServiceContext{
		ConversationRepo: repo,
		EventPublisher:   events.NewInMemoryEventPublisher(),
	}
}

// limitObservingRepo records the limit argument passed to ListMessages.
type limitObservingRepo struct {
	repository.ConversationRepo
	hook func(limit int)
}

func (r *limitObservingRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error) {
	if r.hook != nil {
		r.hook(limit)
	}
	return r.ConversationRepo.ListMessages(ctx, conversationID, limit)
}

// callObservingRepo records whether ListMessages was called at all.
type callObservingRepo struct {
	repository.ConversationRepo
	listMessagesCalled bool
}

func (r *callObservingRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error) {
	r.listMessagesCalled = true
	return r.ConversationRepo.ListMessages(ctx, conversationID, limit)
}

// failingListRepo forces ListMessages to return listErr.
type failingListRepo struct {
	repository.ConversationRepo
	listErr error
}

func (r *failingListRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error) {
	return nil, r.listErr
}

// Compile-time interface guards (per AGENTS.md §1.1 style: explicit
// interface conformance).
var (
	_ repository.ConversationRepo = (*limitObservingRepo)(nil)
	_ repository.ConversationRepo = (*callObservingRepo)(nil)
	_ repository.ConversationRepo = (*failingListRepo)(nil)
)