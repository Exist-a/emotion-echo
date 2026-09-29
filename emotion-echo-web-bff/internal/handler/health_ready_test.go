package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 A 组 · 测试点 #4：D-29 决议下 BFF 的 liveness/readiness 分离。
//
// 🔴 计划期实测（plan §0 F-b）：BFF 已能算出 status="degraded"
// （health_handler.go:102-107），但最后一行恒 `c.JSON(http.StatusOK, resp)`
// ⇒ **说 degraded 却返 200**。而 compose healthcheck 用 wget 只看状态码，
// 于是"某个下游挂了"完全不影响容器健康判定。
//
// 这是 F-137 悖论"BFF /health 200 与网关 503 可同时成立"的机制解释：
// 进程活着 + 自查通过，但**它自己在 Nacos 里没有实例** ⇒ 网关找不到节点。

// TestHealthHandler_D29LivenessAlwaysOK 锁定 liveness：下游全挂也返 200。
func TestHealthHandler_D29LivenessAlwaysOK(t *testing.T) {
	bad := healthMockSrv(t, `{"status":"error"}`, http.StatusInternalServerError)
	targets := []DownstreamTarget{{Name: "user", BaseURL: bad}}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", NewHealthHandler(targets, time.Second))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, w.Code,
		"/health 是 liveness，下游挂掉也必须 200 —— 存量消费方按 200 判定")
	var resp healthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp.Status,
		"liveness 的响应体仍须如实报 degraded（差异只在 HTTP 码）")
}

// TestHealthReadyHandler_D29ReadinessReflectsDownstream 锁定 readiness：下游挂 ⇒ 503。
func TestHealthReadyHandler_D29ReadinessReflectsDownstream(t *testing.T) {
	ok := healthMockSrv(t, `{"status":"ok"}`, http.StatusOK)
	bad := healthMockSrv(t, `{"status":"error"}`, http.StatusInternalServerError)

	tests := []struct {
		name       string
		targets    []DownstreamTarget
		wantCode   int
		wantStatus string
	}{
		{
			name:       "all downstream healthy returns 200",
			targets:    []DownstreamTarget{{Name: "user", BaseURL: ok}, {Name: "chat", BaseURL: ok}},
			wantCode:   http.StatusOK,
			wantStatus: "ok",
		},
		{
			// 🔴 核心断言：修复前恒返 200。
			name:       "one downstream down returns 503",
			targets:    []DownstreamTarget{{Name: "user", BaseURL: ok}, {Name: "chat", BaseURL: bad}},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "degraded",
		},
		{
			name:       "all downstream down returns 503",
			targets:    []DownstreamTarget{{Name: "user", BaseURL: bad}},
			wantCode:   http.StatusServiceUnavailable,
			wantStatus: "degraded",
		},
		{
			name:       "no targets configured returns 200",
			targets:    nil,
			wantCode:   http.StatusOK,
			wantStatus: "ok",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.GET("/health/ready", NewHealthReadyHandler(tt.targets, time.Second))

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

			assert.Equal(t, tt.wantCode, w.Code,
				"readiness 必须用状态码表达下游健康，compose 探针只看这个")
			var resp healthResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			assert.Equal(t, tt.wantStatus, resp.Status)
		})
	}
}

// TestHealthHandlers_D29AgreeOnBody 锁定两个端点响应体一致。
func TestHealthHandlers_D29AgreeOnBody(t *testing.T) {
	bad := healthMockSrv(t, `{"status":"error"}`, http.StatusInternalServerError)
	targets := []DownstreamTarget{{Name: "user", BaseURL: bad}}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", NewHealthHandler(targets, time.Second))
	r.GET("/health/ready", NewHealthReadyHandler(targets, time.Second))

	liveRec := httptest.NewRecorder()
	r.ServeHTTP(liveRec, httptest.NewRequest(http.MethodGet, "/health", nil))
	readyRec := httptest.NewRecorder()
	r.ServeHTTP(readyRec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	var liveBody, readyBody map[string]any
	require.NoError(t, json.Unmarshal(liveRec.Body.Bytes(), &liveBody))
	require.NoError(t, json.Unmarshal(readyRec.Body.Bytes(), &readyBody))

	assert.Equal(t, liveBody["status"], readyBody["status"],
		"两端点 status 必须一致 —— 差别只在 HTTP 码，否则同一状态有两个说法")
	assert.Equal(t, liveBody["version"], readyBody["version"], "version 必须一致")
}
