package repository

import (
	"context"
	"testing"

	"emotion-echo-analytics-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventRepo_InMemory_CreateAndGet(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEventRepo()
	require.NoError(t, repo.Create(context.Background(), &model.UserBehaviorEvent{
		UserID:    100,
		EventType: "page_view",
		Target:    "/home",
	}))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "page_view", got.EventType)
	assert.Equal(t, "/home", got.Target)
}

func TestEventRepo_InMemory_GetByID_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEventRepo()
	got, err := repo.GetByID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEventRepo_InMemory_Ping_OK(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEventRepo()
	require.NoError(t, repo.Ping(context.Background()))
}