package repository

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-12：UserRepo.UpdateProfile 必须支持 config 字段。
func TestUserRepo_InMemory_UpdateProfile_WithConfig_PersistsConfig(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "echo",
		Nickname: strPtr("Echo"),
	}))

	config := model.JSONMap{"fontSize": "18px", "theme": "dark"}
	require.NoError(t, repo.UpdateProfile(context.Background(), 1, nil, nil, nil, nil, &config))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Config, "Config 应被持久化")
	assert.Equal(t, "18px", got.Config["fontSize"])
	assert.Equal(t, "dark", got.Config["theme"])
}

// E2E-12：nil config 不应覆盖已有 config。
func TestUserRepo_InMemory_UpdateProfile_NilConfig_PreservesExisting(t *testing.T) {
	t.Parallel()

	repo := NewInMemoryUserRepo()
	existingConfig := model.JSONMap{"fontSize": "small", "theme": "auto"}
	require.NoError(t, repo.Create(context.Background(), &model.User{
		ID:       1,
		Username: "echo",
		Config:   existingConfig,
	}))

	// 只改 nickname，不动 config
	newNick := "New Echo"
	require.NoError(t, repo.UpdateProfile(context.Background(), 1, &newNick, nil, nil, nil, nil))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.Config, "nil config 不应清除已有 config")
	assert.Equal(t, "small", got.Config["fontSize"])
	assert.Equal(t, "auto", got.Config["theme"])
}