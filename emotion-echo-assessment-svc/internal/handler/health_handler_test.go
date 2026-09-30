// Package handler — health_handler_test.go
//
// Sibling test for health_handler.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 coverage: HTTP-boundary contract for /health.
//
// Coverage matrix:
//
//   - happy path: 200 + JSON {status, time, service, version, dbOk}
//   - svcCtx with no SurveyRepo: still 200, dbOk=true (nil-safe)
//   - unknown path on router: 404 (proves the handler is scoped)
//
// Note: the handler's `if err != nil { 500 }` branch is unreachable
// today because logic.Health() never returns a non-nil error (it
// surfaces DB issues via the resp.DbOK flag, not via err). We
// deliberately don't fabricate a way to make the handler return 500
// — that would require changing logic.Health's contract or
// snapshot-copying its internals, both of which AGENTS §四 bans.
// If a future change makes logic.Health() return an error, add a
// regression test here.
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
)

func TestHealthHandler_HappyPath_Returns200WithJSON(t *testing.T) {
	t.Parallel()
	// 2026-09-30 更正：原先不注入 repo，这个名义上的 happy path 实际跑的是
	// "repo == nil"分支，只因旧实现对 nil 报 ok 才碰巧通过。
	svcCtx := &svc.ServiceContext{
		Config:     config.Config{Name: "emotion-echo-assessment-svc"},
		SurveyRepo: repository.NewInMemorySurveyRepo(),
	}

	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "ok", got["status"])
	assert.Equal(t, "emotion-echo-assessment-svc", got["service"])
	assert.Contains(t, got, "time")
	assert.Contains(t, got, "version")
}

// 🔴 契约反转（2026-09-30，E2E-23 F-96 实测驱动）：原名 `..._NoRepo_Still200`
// 断言 `dbOk=true`。该推理把"这个部署本来不接 DB"与"main.go 单次连接失败后的
// 降级启动"混为一谈 —— 后者在生产真实发生，实测会让 `/health/ready` 返 200、
// 容器判 healthy、APISIX 照常路由而后端全挂，**零告警**。
// 注意 `/health` 仍返 200（D-29：liveness 恒 200），**变的是 body 里的 dbOk/status**。
func TestHealthHandler_NoRepoReportsDegraded(t *testing.T) {
	t.Parallel()
	svcCtx := &svc.ServiceContext{Config: config.Config{}} // SurveyRepo == nil

	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, false, got["dbOk"],
		"repo 为 nil（生产降级启动形态）时 dbOk 必须是 false")
	assert.Equal(t, "degraded", got["status"],
		"repo 为 nil 时 status 必须是 degraded —— 报 ok 会让容器被判 healthy")
}

func TestHealthHandler_UnknownPath_Returns404(t *testing.T) {
	t.Parallel()
	svcCtx := &svc.ServiceContext{Config: config.Config{}}
	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))

	req := httptest.NewRequest(http.MethodGet, "/not-health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}
