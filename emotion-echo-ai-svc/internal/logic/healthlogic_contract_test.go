package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-ai-svc/internal/config"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 A 组 · 测试点 #3（ai 分支）：/health 的 status 必须说真话。
//
// 🔴 计划期实测（plan §0 F-a）：`healthlogic.go:40` 把 Status 写死为 "ok"。

// pingFailEmotionRepo 嵌入 InMemory 实现，只覆写 Ping。
type pingFailEmotionRepo struct {
	*repository.InMemoryEmotionRepo
	pingErr error
}

func (f *pingFailEmotionRepo) Ping(ctx context.Context) error { return f.pingErr }

var _ repository.EmotionRepo = (*pingFailEmotionRepo)(nil)

func TestHealthLogic_Health_StatusReflectsDatabaseState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus string
		wantDbOK   bool
	}{
		{
			name:       "db reachable returns ok",
			pingErr:    nil,
			wantStatus: "ok",
			wantDbOK:   true,
		},
		{
			name:       "db unreachable returns degraded not ok",
			pingErr:    errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"),
			wantStatus: "degraded",
			wantDbOK:   false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			repo := &pingFailEmotionRepo{
				InMemoryEmotionRepo: repository.NewInMemoryEmotionRepo(),
				pingErr:             tt.pingErr,
			}
			l := NewHealthLogic(context.Background(), &svc.ServiceContext{
				Config:      config.Config{},
				EmotionRepo: repo,
			})

			resp, err := l.Health()

			require.NoError(t, err, "依赖不可达不应是 handler 级错误，应由 status 字段表达")
			require.NotNil(t, resp)
			assert.Equal(t, tt.wantStatus, resp.Status,
				"status 必须反映依赖真实状态（依赖挂了还报 ok = 探针说假话）")
			assert.Equal(t, tt.wantDbOK, resp.DbOK, "DbOK 必须如实反映 ping 结果")
			assert.NotEmpty(t, resp.Service, "service 字段必须有值")
			assert.NotEmpty(t, resp.Version, "version 字段必须有值")
		})
	}
}

// 反面对照：status 与 DbOK 不得脱钩。
func TestHealthLogic_Health_StatusAndDbOKNeverDisagree(t *testing.T) {
	t.Parallel()

	repo := &pingFailEmotionRepo{
		InMemoryEmotionRepo: repository.NewInMemoryEmotionRepo(),
		pingErr:             errors.New("boom"),
	}
	l := NewHealthLogic(context.Background(), &svc.ServiceContext{
		Config:      config.Config{},
		EmotionRepo: repo,
	})

	resp, err := l.Health()
	require.NoError(t, err)

	assert.False(t, resp.DbOK, "DbOK 应为 false")
	assert.NotEqual(t, "ok", resp.Status,
		"DbOK=false 时 status 绝不能是 ok —— 响应体不得自相矛盾")
}
