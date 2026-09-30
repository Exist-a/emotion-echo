package logic

import (
	"context"
	"testing"

	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthLogic_Health_ReturnsOkStatus(t *testing.T) {
	t.Parallel()

	// 2026-09-30 更正：原先是空 ServiceContext，这个 happy path 实际跑的是
	// "repo == nil"分支，只因旧实现对 nil 报 ok 才碰巧通过。
	svcCtx := &svc.ServiceContext{EmotionRepo: repository.NewInMemoryEmotionRepo()}
	l := NewHealthLogic(context.Background(), svcCtx)

	resp, err := l.Health()
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "emotion-echo-ai-svc", resp.Service)
	assert.NotEmpty(t, resp.Version)
	assert.Greater(t, resp.Time, int64(0))
}
