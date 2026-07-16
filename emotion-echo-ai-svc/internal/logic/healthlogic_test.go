package logic

import (
	"context"
	"testing"

	"emotion-echo-ai-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthLogic_Health_ReturnsOkStatus(t *testing.T) {
	t.Parallel()

	svcCtx := &svc.ServiceContext{}
	l := NewHealthLogic(context.Background(), svcCtx)

	resp, err := l.Health()
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "emotion-echo-ai-svc", resp.Service)
	assert.NotEmpty(t, resp.Version)
	assert.Greater(t, resp.Time, int64(0))
}