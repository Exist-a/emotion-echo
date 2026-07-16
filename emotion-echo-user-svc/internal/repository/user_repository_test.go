package repository

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UserRepo 的契约测试

func strPtr(s string) *string { return &s }

func TestUserRepo_InMemory_GetByID_Existing_ReturnsUser(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "alice",
		Phone:    strPtr("13800138000"),
		Nickname: strPtr("Alice"),
	}))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(1), got.ID)
	assert.Equal(t, "alice", got.Username)
	require.NotNil(t, got.Phone)
	assert.Equal(t, "13800138000", *got.Phone)
}

func TestUserRepo_InMemory_GetByID_NotFound_ReturnsNil(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	got, err := repo.GetByID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got, "missing user should return nil, not error")
}

func TestUserRepo_InMemory_GetByUsername_Existing(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       42,
		Username: "bob",
	}))

	got, err := repo.GetByUsername(context.Background(), "bob")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(42), got.ID)
}

func TestUserRepo_InMemory_GetByPhone_Existing(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:    7,
		Phone: strPtr("13900139000"),
	}))

	got, err := repo.GetByPhone(context.Background(), "13900139000")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(7), got.ID)
}

func TestUserRepo_InMemory_Ping_OK(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	err := repo.Ping(context.Background())
	require.NoError(t, err)
}

func TestUserRepo_InMemory_UpdateProfile_Existing_UpdatesFields(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "alice",
		Nickname: strPtr("Old Nick"),
	}))

	gender := int16(1)
	newNick := "New Nick"
	require.NoError(t, repo.UpdateProfile(context.Background(), 1, &newNick, &gender, nil, nil))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Nickname)
	assert.Equal(t, "New Nick", *got.Nickname)
	assert.Equal(t, int16(1), got.Gender)
}

func TestUserRepo_InMemory_UpdateProfile_NotFound_ReturnsErrNotFound(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	err := repo.UpdateProfile(context.Background(), 999, strPtr("X"), nil, nil, nil)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUserRepo_InMemory_UpdateProfile_NilFields_DoesNotTouch(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "alice",
		Nickname: strPtr("Alice"),
	}))

	// 全部 nil 不应报错也不应改任何字段
	require.NoError(t, repo.UpdateProfile(context.Background(), 1, nil, nil, nil, nil))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got.Nickname)
	assert.Equal(t, "Alice", *got.Nickname)
}