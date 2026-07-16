package logic

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthLogic_Health_OK(t *testing.T) {
	t.Parallel()

	svcCtx := &svc.ServiceContext{
		ConversationRepo: repository.NewInMemoryConversationRepo(),
		EventPublisher:   events.NewInMemoryEventPublisher(),
	}
	l := NewHealthLogic(context.Background(), svcCtx)

	resp, err := l.Health()
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok", resp.Status)
	assert.True(t, resp.DbOK)
	assert.True(t, resp.KafkaOK)
}

func TestHealthLogic_Health_NoKafka_Degraded(t *testing.T) {
	t.Parallel()

	svcCtx := &svc.ServiceContext{
		ConversationRepo: repository.NewInMemoryConversationRepo(),
		// EventPublisher 故意为空
	}
	l := NewHealthLogic(context.Background(), svcCtx)

	resp, err := l.Health()
	require.NoError(t, err)
	assert.Equal(t, "degraded", resp.Status)
	assert.True(t, resp.DbOK)
	assert.False(t, resp.KafkaOK)
}