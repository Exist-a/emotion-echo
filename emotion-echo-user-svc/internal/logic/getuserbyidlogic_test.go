// Package logic — getuserbyidlogic_test.go
//
// Sibling test for getuserbyidlogic.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 5.1 #3: getuserbyidlogic.go (LOC=75) had no
// sibling test. This file covers the GetUserById logic contract:
//
//   - happy path: existing user → full UserInfo mapping (Account from
//     Username, Phone/Nickname from *string with nil-safe deref)
//   - validation: id <= 0 → "invalid user id" before repo call
//   - not-found: InMemoryUserRepo returns (nil, nil) → logic maps to
//     repository.ErrNotFound
//   - repo error: any non-nil error from repo is propagated verbatim
package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newGetUserByIdSvcCtx(repo repository.UserRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, UserRepo: repo}
}

// errUserRepo is a UserRepo stub whose GetByID returns a fixed error
// (for the "repo error propagates" case). Other methods panic if
// called — keeps the test surface tight.
type errUserRepo struct {
	repository.UserRepo
	err error
}

func (r errUserRepo) GetByID(_ context.Context, _ int64) (*model.User, error) {
	return nil, r.err
}

func strPtr(s string) *string { return &s }

func TestGetUserByIdLogic_HappyPath_MapsUserInfo(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	repo.Create(context.Background(), &model.User{
		ID:       42,
		Username: "alice",
		Phone:    strPtr("+8613800000001"),
		Nickname: strPtr("Alice"),
	})

	l := NewGetUserByIdLogic(context.Background(), newGetUserByIdSvcCtx(repo))
	resp, err := l.GetUserById(&types.GetUserByIdReq{Id: 42})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int64(42), resp.User.UserId)
	assert.Equal(t, "alice", resp.User.Account)
	assert.Equal(t, "+8613800000001", resp.User.Phone)
	assert.Equal(t, "Alice", resp.User.Nickname)
}

func TestGetUserByIdLogic_NilPhoneAndNickname_ReturnsEmptyStrings(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	repo.Create(context.Background(), &model.User{ID: 7, Username: "bob"}) // Phone, Nickname == nil

	l := NewGetUserByIdLogic(context.Background(), newGetUserByIdSvcCtx(repo))
	resp, err := l.GetUserById(&types.GetUserByIdReq{Id: 7})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "bob", resp.User.Account)
	assert.Equal(t, "", resp.User.Phone)
	assert.Equal(t, "", resp.User.Nickname)
}

func TestGetUserByIdLogic_ZeroID_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := NewGetUserByIdLogic(context.Background(), newGetUserByIdSvcCtx(repo))

	_, err := l.GetUserById(&types.GetUserByIdReq{Id: 0})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid user id")
}

func TestGetUserByIdLogic_NegativeID_ValidationError(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	l := NewGetUserByIdLogic(context.Background(), newGetUserByIdSvcCtx(repo))

	_, err := l.GetUserById(&types.GetUserByIdReq{Id: -1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid user id")
}

func TestGetUserByIdLogic_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	// InMemoryUserRepo.GetByID returns (nil, nil) for missing IDs;
	// logic maps that to ErrNotFound.
	l := NewGetUserByIdLogic(context.Background(), newGetUserByIdSvcCtx(repo))
	_, err := l.GetUserById(&types.GetUserByIdReq{Id: 9999})
	assert.ErrorIs(t, err, repository.ErrNotFound)
}

func TestGetUserByIdLogic_RepoError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("database connection lost")
	repo := errUserRepo{err: boom}

	l := NewGetUserByIdLogic(context.Background(), newGetUserByIdSvcCtx(repo))
	_, err := l.GetUserById(&types.GetUserByIdReq{Id: 42})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}