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
	"emotion-echo-assessment-svc/internal/svc"
)

func TestHealthHandler_HappyPath_Returns200WithJSON(t *testing.T) {
	t.Parallel()
	svcCtx := &svc.ServiceContext{Config: config.Config{Name: "emotion-echo-assessment-svc"}}

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

func TestHealthHandler_NoRepo_Still200(t *testing.T) {
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
	assert.Equal(t, true, got["dbOk"])
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