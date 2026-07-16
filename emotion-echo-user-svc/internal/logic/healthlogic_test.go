package logic

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 🔴 RED：先写测试，描述 HealthLogic 的预期行为
//
// 健康检查接口必须：
//   - 返回 status="ok"
//   - 包含服务名 / 版本号
//   - 提供时间戳

func newTestHealthLogic(t *testing.T) *HealthLogic {
	t.Helper()
	svcCtx := &svc.ServiceContext{
		Config: config.Config{},
	}
	return NewHealthLogic(context.Background(), svcCtx)
}

func TestHealthLogic_Health_StubTestCompiles(t *testing.T) {
	t.Parallel()
	// 这个 stub 用来确保测试结构正确。
	// 在 RED 阶段，它先验证测试本身能编译运行。
	l := newTestHealthLogic(t)
	require.NotNil(t, l)
	assert.NotNil(t, l.ctx)
}

// 🔴 RED：HealthLogic 应当返回 status="ok" 的响应。
// 当前实现返回 (nil, nil)，所以这个测试必须失败。
func TestHealthLogic_Health_ReturnsOkStatus(t *testing.T) {
	t.Parallel()

	l := newTestHealthLogic(t)
	resp, err := l.Health()

	require.NoError(t, err, "Health 不应返回 error")
	require.NotNil(t, resp, "Health 应返回非 nil 响应")
	assert.Equal(t, "ok", resp.Status, "status 必须是 ok")
	assert.NotEmpty(t, resp.Service, "service 字段必须有值")
	assert.NotEmpty(t, resp.Version, "version 字段必须有值")
	assert.Greater(t, resp.Time, int64(0), "time 必须 > 0")
}