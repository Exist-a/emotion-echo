package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 测试点 #5 / #6：/health 必须覆盖 **Redis** 与 **Nacos 注册状态**。
//
// 🔴 缺陷形态（plan §0 F-c）：改动前 BFF 的 /health 只探 6 个下游，
// **既不查 Redis、也不查自己是否已注册到 Nacos**。于是出现账本 F-137
// 记录的那个悖论："BFF /health 200 与网关 503 可同时成立"
// —— 进程活着 + 下游都通 = 自查健康，但它**自己在 Nacos 里没有实例**
// ⇒ APISIX 解析不到节点 ⇒ 全站 502。
//
// #5 的前置事实：dev 下 BFF 确有 Redis（E2E-20 的 LoginLockStore 在用），
// 但 client 藏在 buildAuthLockStore() 内部、没进 ServiceContext，
// 健康检查**结构上无法**探它 —— 不是"忘了写"，是"没有载体"。
//
// 本测试锁定两个新维度，并要求 readiness 一并纳入判定。

// fakePinger 是 Redis 探针替身。
type fakePinger struct{ err error }

func (f *fakePinger) Ping(ctx context.Context) error { return f.err }

// depsRouter 构造带健康依赖的探针路由（liveness + readiness）。
func depsRouter(t *testing.T, deps HealthDeps) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	okSrv := healthMockSrv(t, `{"status":"ok"}`, http.StatusOK)
	targets := []DownstreamTarget{{Name: "user", BaseURL: okSrv}}

	r := gin.New()
	r.GET("/health", NewHealthHandlerWithDeps(targets, time.Second, BuildInfo{}, deps))
	r.GET("/health/ready", NewHealthReadyHandlerWithDeps(targets, time.Second, deps))
	return r
}

func getHealth(t *testing.T, r *gin.Engine, path string) (*httptest.ResponseRecorder, healthResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	var resp healthResponse
	if w.Code == http.StatusOK || w.Code == http.StatusServiceUnavailable {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp),
			"响应须是合法 JSON：%s", w.Body.String())
	}
	return w, resp
}

func TestHealthDeps_RedisDown_ReportsDegraded(t *testing.T) {
	t.Parallel()

	r := depsRouter(t, HealthDeps{
		Redis:           &fakePinger{err: errors.New("dial tcp: connection refused")},
		NacosRegistered: func() bool { return true },
	})

	w, resp := getHealth(t, r, "/health")
	assert.Equal(t, http.StatusOK, w.Code, "liveness 恒 200（D-29）")
	assert.Equal(t, "degraded", resp.Status, "Redis 不通 ⇒ 必须报 degraded")
	require.Contains(t, resp.Deps, "redis", "响应须含 redis 维度")
	assert.Equal(t, "unhealthy", resp.Deps["redis"].Status)
	assert.Contains(t, resp.Deps["redis"].Detail, "connection refused")
}

func TestHealthDeps_NotRegisteredInNacos_ReportsDegraded(t *testing.T) {
	t.Parallel()

	r := depsRouter(t, HealthDeps{
		Redis:           &fakePinger{},
		NacosRegistered: func() bool { return false },
	})

	_, resp := getHealth(t, r, "/health")
	assert.Equal(t, "degraded", resp.Status,
		"未注册到 Nacos ⇒ 必须报 degraded（F-137 悖论：进程活着但网关找不到它）")
	require.Contains(t, resp.Deps, "nacos")
	assert.Equal(t, "not_registered", resp.Deps["nacos"].Status)
}

func TestHealthDeps_AllHealthy_ReportsOK(t *testing.T) {
	t.Parallel()

	r := depsRouter(t, HealthDeps{
		Redis:           &fakePinger{},
		NacosRegistered: func() bool { return true },
	})

	w, resp := getHealth(t, r, "/health/ready")
	assert.Equal(t, http.StatusOK, w.Code, "全绿 ⇒ readiness 200")
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "ok", resp.Deps["redis"].Status)
	assert.Equal(t, "ok", resp.Deps["nacos"].Status)
}

// TestHealthDeps_ReadinessCoversNewDimensions 是 #5/#6 的核心：
// readiness 必须把新维度纳入判定，否则 compose 探针看不到降级。
func TestHealthDeps_ReadinessCoversNewDimensions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		deps HealthDeps
	}{
		{
			name: "redis down yields 503",
			deps: HealthDeps{Redis: &fakePinger{err: errors.New("down")}, NacosRegistered: func() bool { return true }},
		},
		{
			name: "not registered yields 503",
			deps: HealthDeps{Redis: &fakePinger{}, NacosRegistered: func() bool { return false }},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			r := depsRouter(t, tt.deps)
			w, _ := getHealth(t, r, "/health/ready")
			assert.Equal(t, http.StatusServiceUnavailable, w.Code,
				"降级维度必须传导到 readiness 的 HTTP 码，否则探针形同虚设")
		})
	}
}

// TestHealthDeps_NotInjected_NoFalseDegrade 向后兼容：老部署无 Redis 时
// 不应凭空报降级（否则接入本改动会把没配 Redis 的环境全判死）。
func TestHealthDeps_NotInjected_NoFalseDegrade(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	okSrv := healthMockSrv(t, `{"status":"ok"}`, http.StatusOK)
	r := gin.New()
	r.GET("/health/ready", NewHealthReadyHandlerWithDeps(
		[]DownstreamTarget{{Name: "user", BaseURL: okSrv}}, time.Second, HealthDeps{}))

	w, resp := getHealth(t, r, "/health/ready")
	assert.Equal(t, http.StatusOK, w.Code, "未注入依赖探针 ⇒ 不得误报降级")
	assert.Equal(t, "ok", resp.Status)
	assert.Empty(t, resp.Deps, "未注入时不凭空造 deps 字段")
}
