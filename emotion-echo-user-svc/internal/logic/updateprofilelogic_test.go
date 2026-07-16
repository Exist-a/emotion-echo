package logic

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestUpdateProfileLogic(repo repository.UserRepo, ctx context.Context) *UpdateProfileLogic {
	return NewUpdateProfileLogic(ctx, &svc.ServiceContext{UserRepo: repo})
}

func TestUpdateProfileLogic_HappyPath_UpdatesAndReturnsLatest(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "alice",
		Nickname: sp("Old"),
	}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	gender := int16(1)
	resp, err := l.UpdateProfile(&types.UpdateProfileReq{
		Nickname: sp("New Alice"),
		Gender:   &gender,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "New Alice", resp.User.Nickname)
}

func TestUpdateProfileLogic_NoUserID_Unauthorized(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	l := newTestUpdateProfileLogic(repo, context.Background()) // 没有 user_id

	_, err := l.UpdateProfile(&types.UpdateProfileReq{Nickname: sp("X")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unauthorized")
}

func TestUpdateProfileLogic_NicknameTooLong_ValidationError(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{ID: 1, Username: "alice"}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	long := "a23456789012345678901234567890123" // 33 chars
	_, err := l.UpdateProfile(&types.UpdateProfileReq{Nickname: &long})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nickname")
}

func TestUpdateProfileLogic_GenderOutOfRange_ValidationError(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{ID: 1, Username: "alice"}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	badGender := int16(99)
	_, err := l.UpdateProfile(&types.UpdateProfileReq{Gender: &badGender})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gender")
}

func TestUpdateProfileLogic_BirthdayInvalidFormat_ValidationError(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{ID: 1, Username: "alice"}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	bad := "not-a-date"
	_, err := l.UpdateProfile(&types.UpdateProfileReq{Birthday: &bad})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "birthday")
}

func TestUpdateProfileLogic_BirthdayValid_Parsed(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{ID: 1, Username: "alice"}))

	ctx := contextWithUserID(context.Background(), 1)
	l := newTestUpdateProfileLogic(repo, ctx)

	good := "1990-05-15"
	resp, err := l.UpdateProfile(&types.UpdateProfileReq{Birthday: &good})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestUpdateProfileLogic_UserNotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryUserRepo()
	// 不创建用户
	ctx := contextWithUserID(context.Background(), 999)
	l := newTestUpdateProfileLogic(repo, ctx)

	_, err := l.UpdateProfile(&types.UpdateProfileReq{Nickname: sp("X")})
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrNotFound)
}