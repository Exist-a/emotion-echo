package logic

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageCreatedHandler_HappyPath：消费一条 message.created → 跑分析 → 写库
func TestMessageCreatedHandler_HappyPath(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	a := analyzer.NewKeywordAnalyzer()
	h := NewMessageCreatedHandler(repo, a)

	// 构造一条 message.created 事件
	evtData := events.MessageCreatedData{
		MessageID:      100,
		ConversationID: 50,
		UserID:         1,
		Role:           "user",
		Content:        "今天很开心，谢谢你",
		CreatedAt:      time.Now().UnixMilli(),
	}
	body, err := json.Marshal(evtData)
	require.NoError(t, err)

	evt := &events.Event{
		ID:     "test-uuid",
		Type:   events.EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   time.Now(),
		Data:   json.RawMessage(body),
	}

	err = h.Handle(context.Background(), evt)
	require.NoError(t, err)

	// 断言 emotion_analysis 已写入
	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, int64(100), got.MessageID)
	assert.Equal(t, int64(50), got.ConversationID)
	assert.Equal(t, int64(1), got.UserID)
	assert.Greater(t, got.SentimentScore, 0.0)
}

// TestMessageCreatedHandler_SadText：负面文本 → sad
func TestMessageCreatedHandler_SadText(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	a := analyzer.NewKeywordAnalyzer()
	h := NewMessageCreatedHandler(repo, a)

	evtData := events.MessageCreatedData{
		MessageID:      200,
		ConversationID: 60,
		UserID:         2,
		Role:           "user",
		Content:        "我今天很难过，糟糕透了",
		CreatedAt:      time.Now().UnixMilli(),
	}
	body, _ := json.Marshal(evtData)
	evt := &events.Event{
		ID:   "test-uuid-2",
		Type: events.EventTypeMessageCreated,
		Data: json.RawMessage(body),
	}

	require.NoError(t, h.Handle(context.Background(), evt))

	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "sad", got.PrimaryEmotion)
	assert.Less(t, got.SentimentScore, 0.0)
}

// TestMessageCreatedHandler_NonUserRole_Skip：非 user 角色（assistant）不分析
func TestMessageCreatedHandler_NonUserRole_Skip(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	a := analyzer.NewKeywordAnalyzer()
	h := NewMessageCreatedHandler(repo, a)

	evtData := events.MessageCreatedData{
		MessageID: 300, ConversationID: 70, UserID: 3,
		Role: "assistant", Content: "你好！",
	}
	body, _ := json.Marshal(evtData)
	evt := &events.Event{
		ID:   "test-uuid-3",
		Type: events.EventTypeMessageCreated,
		Data: json.RawMessage(body),
	}

	require.NoError(t, h.Handle(context.Background(), evt))

	// assistant 消息不分析 → DB 没新行
	got, err := repo.GetByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestMessageCreatedHandler_WrongType_Skip：事件类型不匹配 → 跳过
func TestMessageCreatedHandler_WrongType_Skip(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	a := analyzer.NewKeywordAnalyzer()
	h := NewMessageCreatedHandler(repo, a)

	evt := &events.Event{
		ID:   "x",
		Type: "other.event", // 不匹配
		Data: json.RawMessage(`{}`),
	}
	require.NoError(t, h.Handle(context.Background(), evt))
	got, _ := repo.GetByID(context.Background(), 1)
	assert.Nil(t, got)
}

// TestMessageCreatedHandler_BadData_ReturnsError：data 不是 JSON → 返回错误（不重试丢弃）
func TestMessageCreatedHandler_BadData_ReturnsError(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEmotionRepo()
	a := analyzer.NewKeywordAnalyzer()
	h := NewMessageCreatedHandler(repo, a)

	evt := &events.Event{
		ID:   "x",
		Type: events.EventTypeMessageCreated,
		Data: json.RawMessage(`not json`),
	}
	err := h.Handle(context.Background(), evt)
	require.Error(t, err)
}