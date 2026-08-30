// Package handler — health_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.59 RED: health handler 聚合探测契约测试
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

// healthMockSrv 起一个返回固定 status 的下游 mock
func healthMockSrv(t *testing.T, body string, statusCode int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestHealthHandler_AllHealthy_ReturnsOK(t *testing.T) {
	ok := healthMockSrv(t, `{"status":"ok"}`, http.StatusOK)
	targets := []DownstreamTarget{
		{Name: "user", BaseURL: ok},
		{Name: "chat", BaseURL: ok},
		{Name: "ai", BaseURL: ok},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", NewHealthHandler(targets, time.Second))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	var resp healthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "ok", resp.Status)
	assert.Equal(t, "ok", resp.Downstream["user"].Status)
	assert.Len(t, resp.Downstream, 3)
}

func TestHealthHandler_OneUnhealthy_ReturnsDegraded(t *testing.T) {
	ok := healthMockSrv(t, `{"status":"ok"}`, http.StatusOK)
	bad := healthMockSrv(t, `{"status":"error"}`, http.StatusInternalServerError)
	targets := []DownstreamTarget{
		{Name: "user", BaseURL: ok},
		{Name: "chat", BaseURL: bad},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", NewHealthHandler(targets, time.Second))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	var resp healthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "unhealthy", resp.Downstream["chat"].Status)
	assert.Equal(t, "ok", resp.Downstream["user"].Status)
}

func TestHealthHandler_DownstreamUnreachable_ReturnsUnhealthy(t *testing.T) {
	targets := []DownstreamTarget{
		{Name: "down-svc", BaseURL: "http://127.0.0.1:1", Timeout: 100 * time.Millisecond}, // 不可达端口
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", NewHealthHandler(targets, 100*time.Millisecond))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	var resp healthResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "unhealthy", resp.Downstream["down-svc"].Status)
	assert.NotEmpty(t, resp.Downstream["down-svc"].Detail)
}

func TestHealthHandler_EmptyTargets_ReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", NewHealthHandler(nil, time.Second))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"ok"`)
}
