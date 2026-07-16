package logic

import (
	"context"
	"testing"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestHealthLogic(t *testing.T) *HealthLogic {
	t.Helper()
	svcCtx := &svc.ServiceContext{Config: config.Config{}}
	return NewHealthLogic(context.Background(), svcCtx)
}

func TestHealthLogic_Health_ReturnsOkStatus(t *testing.T) {
	t.Parallel()

	l := newTestHealthLogic(t)
	resp, err := l.Health()

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "emotion-echo-assessment-svc", resp.Service)
	assert.NotEmpty(t, resp.Version)
	assert.Greater(t, resp.Time, int64(0))
}