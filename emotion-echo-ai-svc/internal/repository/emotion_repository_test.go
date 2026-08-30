package repository

import (
	"context"
	"testing"

	"emotion-echo-ai-svc/internal/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmotionRepo_InMemory_CreateAndGet(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID:      100,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
	}))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

func TestEmotionRepo_InMemory_GetByMessageID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 100, UserID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 101, UserID: 1, ConversationID: 50, PrimaryEmotion: "sad",
	}))

	got, err := repo.GetByMessageID(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
}

func TestEmotionRepo_InMemory_GetByMessageID_NotFound(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	got, err := repo.GetByMessageID(context.Background(), 999)
	require.NoError(t, err)
	assert.Nil(t, got)
}

func TestEmotionRepo_InMemory_ListByConversationID(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	// conv=50: 2 条
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 2, ConversationID: 50, PrimaryEmotion: "anxious",
	}))
	// conv=60: 1 条
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 3, ConversationID: 60, PrimaryEmotion: "calm",
	}))

	got, err := repo.ListByConversationID(context.Background(), 50)
	require.NoError(t, err)
	assert.Len(t, got, 2)

	got60, err := repo.ListByConversationID(context.Background(), 60)
	require.NoError(t, err)
	assert.Len(t, got60, 1)

	// 不存在的会话
	got999, err := repo.ListByConversationID(context.Background(), 999)
	require.NoError(t, err)
	assert.Empty(t, got999)
}

func TestEmotionRepo_InMemory_Ping_OK(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Ping(context.Background()))
}

// TestEmotionRepo_InMemory_DuplicateEventID_InsertsOnce 验证 Stage 30-C A1 幂等去重：
// 同一 EventID 第二次 Create 必须不产生新行（既不报错，也不重复落库）。
func TestEmotionRepo_InMemory_DuplicateEventID_InsertsOnce(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	first := &model.EmotionAnalysis{
		EventID:        "evt-dup-1",
		MessageID:      100,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
	}
	second := &model.EmotionAnalysis{
		EventID:        "evt-dup-1",
		MessageID:      100,
		UserID:         1,
		ConversationID: 50,
		PrimaryEmotion: "happy",
	}

	require.NoError(t, repo.Create(context.Background(), first))
	require.NoError(t, repo.Create(context.Background(), second), "重复 event_id 的 Create 应返回 nil（去重语义）")

	// 只能查到一行：ID 为 first.ID（second 未分配新 ID）
	got, err := repo.GetByID(context.Background(), first.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "evt-dup-1", got.EventID)

	// 关键断言：表中只有一个 id（first.ID），不存在 second 生成的 id
	probeID := first.ID + 1
	gotProbe, err := repo.GetByID(context.Background(), probeID)
	require.NoError(t, err)
	assert.Nil(t, gotProbe, "重复 event_id 不应分配新 ID / 不应产生新行")
}

// TestEmotionRepo_InMemory_DistinctEventIDs_InsertBoth 验证反例：不同 EventID 各自落一行。
func TestEmotionRepo_InMemory_DistinctEventIDs_InsertBoth(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		EventID:   "evt-a",
		MessageID: 1, PrimaryEmotion: "happy",
	}))
	require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
		EventID:   "evt-b",
		MessageID: 2, PrimaryEmotion: "sad",
	}))

	gotA, err := repo.GetByMessageID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, gotA)
	assert.Equal(t, "evt-a", gotA.EventID)

	gotB, err := repo.GetByMessageID(context.Background(), 2)
	require.NoError(t, err)
	require.NotNil(t, gotB)
	assert.Equal(t, "evt-b", gotB.EventID)
}

// TestEmotionRepo_InMemory_EmptyEventID_NoDedupe 验证 gRPC 同步分析路径：
// EventID 为空时不走去重，多次 Create 各落一行（不报错）。
func TestEmotionRepo_InMemory_EmptyEventID_NoDedupe(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryEmotionRepo()
	for i := 0; i < 3; i++ {
		require.NoError(t, repo.Create(context.Background(), &model.EmotionAnalysis{
			EventID:        "", // gRPC 同步路径无 event_id
			MessageID:      int64(100 + i),
			PrimaryEmotion: "happy",
		}))
	}
	got1, err := repo.GetByMessageID(context.Background(), 100)
	require.NoError(t, err)
	require.NotNil(t, got1)
	got2, err := repo.GetByMessageID(context.Background(), 101)
	require.NoError(t, err)
	require.NotNil(t, got2)
	got3, err := repo.GetByMessageID(context.Background(), 102)
	require.NoError(t, err)
	require.NotNil(t, got3)
}