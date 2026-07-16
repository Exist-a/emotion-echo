package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-user-svc/internal/middleware"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func contextWithUserID(ctx context.Context, uid int64) context.Context {
	return context.WithValue(ctx, middleware.CtxUserIDKey{}, uid)
}

func sp(s string) *string { return &s }

func newTestGetMeLogic(repo repository.UserRepo, ctx context.Context) *GetMeLogic {
	svcCtx := &svc.ServiceContext{UserRepo: repo}
	return NewGetMeLogic(ctx, svcCtx)
}

func TestGetMeLogic_ExistingUser_ReturnsUserInfo(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       5,
		Username: "alice",
		Phone:    sp("13800"),
		Nickname: sp("Alice"),
	}))

	l := newTestGetMeLogic(repo, contextWithUserID(context.Background(), 5))
	resp, err := l.GetMe(&types.GetMeReq{})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(5), resp.User.UserId)
	assert.Equal(t, "alice", resp.User.Account)
}

func TestGetMeLogic_NoUserIDInContext_Returns401(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestGetMeLogic(repo, context.Background())
	resp, err := l.GetMe(&types.GetMeReq{})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestGetMeLogic_UserNotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestGetMeLogic(repo, contextWithUserID(context.Background(), 99999))
	resp, err := l.GetMe(&types.GetMeReq{})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errors.Is(err, repository.ErrNotFound))
}