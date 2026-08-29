// Package handler — health_handler_test.go
//
// Sibling test for health_handler.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.8: cover the analytics-svc health handler
// (20 LOC). The handler delegates to logic.NewHealthLogic(...).Health()
// and maps the result to a JSON response. Coverage:
//
//   - DB up → 200 + JSON with DbOK=true
//   - DB down (Ping returns error) → 200 + JSON with DbOK=false
//     (current behavior — DB errors degrade gracefully, not 5xx)
//   - nil EventRepo → 200 + JSON with DbOK=true (skipped probe)
//   - logic-layer panic recovery: out of scope; covered by gin's
//     Recovery middleware in main.go (see analytics-svc/main.go).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// newAnalyticsHandlerSvc returns a minimal *svc.ServiceContext with
// a configurable EventRepo. Tests inject failing-repo variants to
// exercise the DB-down branch.
func newAnalyticsHandlerSvc(t *testing.T, repo repository.EventRepo) *svc.ServiceContext {
	t.Helper()
	if repo == nil {
		repo = repository.NewInMemoryEventRepo()
	}
	return svc.NewServiceContext(config.Config{Name: "emotion-echo-analytics-svc"}, repo)
}

// newHealthRouter builds a gin engine with /health registered.
func newHealthRouter(svcCtx *svc.ServiceContext) *gin.Engine {
	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))
	return r
}

// TestHealthHandler_DBUp_ReturnsOK covers the canonical happy path:
// EventRepo.Ping returns nil → 200 + DbOK=true.
func TestHealthHandler_DBUp_ReturnsOK(t *testing.T) {
	t.Parallel()

	svcCtx := newAnalyticsHandlerSvc(t, nil)
	r := newHealthRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"healthy should be 200, got %d body=%s", rec.Code, rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, true, body["dbOk"])
	assert.Equal(t, "emotion-echo-analytics-svc", body["service"])
}

// TestHealthHandler_DBDown_ReturnsOKWithDbOKFalse pins the current
// behavior — DB errors DO NOT 5xx; instead DbOK becomes false and
// the HTTP status stays 200. This is the standard "alive but
// degraded" pattern. Tightening this to 503 would be a deliberate
// change to HealthHandler (and would affect K8s liveness probes).
func TestHealthHandler_DBDown_ReturnsOKWithDbOKFalse(t *testing.T) {
	t.Parallel()

	repo := repository.NewInMemoryEventRepo()
	wrapped := &failingPingRepo{
		EventRepo: repo,
		pingErr:   errors.New("postgres: connection refused"),
	}
	svcCtx := newAnalyticsHandlerSvc(t, wrapped)
	r := newHealthRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"DB-down currently returns 200 + DbOK=false; got %d body=%s",
		rec.Code, rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, false, body["dbOk"], "DbOK must be false on DB error")
}

// TestHealthHandler_NilEventRepo_ReturnsOKDbOKTrue covers the
// "no repo wired" branch: the logic skips the Ping probe and
// reports DbOK=true (because we have no evidence of failure).
func TestHealthHandler_NilEventRepo_ReturnsOKDbOKTrue(t *testing.T) {
	t.Parallel()

	svcCtx := newAnalyticsHandlerSvc(t, repository.NewInMemoryEventRepo())
	// Force EventRepo to nil — bypassing NewServiceContext's wiring.
	svcCtx.EventRepo = nil

	r := newHealthRouter(svcCtx)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, true, body["dbOk"],
		"with nil EventRepo the probe is skipped; DbOK stays true (no evidence of failure)")
}

// ─────────────────────────────────────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────────────────────────────────────

// failingPingRepo wraps an EventRepo and forces Ping to return pingErr.
type failingPingRepo struct {
	repository.EventRepo
	pingErr error
}

func (r *failingPingRepo) Ping(ctx context.Context) error {
	return r.pingErr
}

var _ repository.EventRepo = (*failingPingRepo)(nil)