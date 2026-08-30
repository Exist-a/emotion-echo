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

// TestEventRepo_InMemory_DuplicateEventID_InsertsOnce Stage 30-C A1 幂等去重：
// 同 EventID 第二次 Create 必须不产生新行（不报错、不重复落库）。
func TestEventRepo_InMemory_DuplicateEventID_InsertsOnce(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEventRepo()
	first := &model.UserBehaviorEvent{
		EventID:   "evt-dup-1",
		UserID:    42,
		EventType: "message",
		Target:    "evt-dup-1",
	}
	second := &model.UserBehaviorEvent{
		EventID:   "evt-dup-1",
		UserID:    42,
		EventType: "message",
		Target:    "evt-dup-1",
	}

	require.NoError(t, repo.Create(context.Background(), first))
	require.NoError(t, repo.Create(context.Background(), second), "重复 event_id 应返回 nil（去重）")

	// 关键断言：仅 first.ID 有值，second 未分配新 ID
	got, err := repo.GetByID(context.Background(), first.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "evt-dup-1", got.EventID)

	probeID := first.ID + 1
	gotProbe, err := repo.GetByID(context.Background(), probeID)
	require.NoError(t, err)
	assert.Nil(t, gotProbe, "重复 event_id 不应产生新行")
}

// TestEventRepo_InMemory_DistinctEventIDs_InsertBoth 反例：不同 EventID 各落一行。
func TestEventRepo_InMemory_DistinctEventIDs_InsertBoth(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEventRepo()
	require.NoError(t, repo.Create(context.Background(), &model.UserBehaviorEvent{
		EventID: "evt-a", UserID: 1, EventType: "message",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.UserBehaviorEvent{
		EventID: "evt-b", UserID: 2, EventType: "message",
	}))

	gotA, _ := repo.GetByID(context.Background(), 1)
	require.NotNil(t, gotA)
	assert.Equal(t, "evt-a", gotA.EventID)

	gotB, _ := repo.GetByID(context.Background(), 2)
	require.NotNil(t, gotB)
	assert.Equal(t, "evt-b", gotB.EventID)
}