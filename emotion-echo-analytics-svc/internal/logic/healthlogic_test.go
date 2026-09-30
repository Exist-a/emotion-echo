package logic

import (
	"context"
	"testing"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthLogic_Health_ReturnsOkStatus(t *testing.T) {
	t.Parallel()

	// 2026-09-30 更正：原先这里是空的 ServiceContext，于是这个名义上的 happy path
	// 实际跑的是"repo == nil"分支，只因旧实现对 nil 报 ok 才碰巧通过。
	// 探针语义改成 nil ⇒ degraded（E2E-23 F-96 实测）后，这里必须真的提供依赖。
	svcCtx := &svc.ServiceContext{EventRepo: repository.NewInMemoryEventRepo()}
	l := NewHealthLogic(context.Background(), svcCtx)

	resp, err := l.Health()
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "emotion-echo-analytics-svc", resp.Service)
	assert.NotEmpty(t, resp.Version)
	assert.Greater(t, resp.Time, int64(0))
}
