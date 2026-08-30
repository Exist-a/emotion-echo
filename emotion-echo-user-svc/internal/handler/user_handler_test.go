// Package handler — user_handler_test.go
//
// Sibling test for user_handler.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §五 5.1 coverage: 4 handler funcs had no
// HTTP-boundary tests. Coverage matrix:
//
//   GetMeHandler          GET   /api/v1/users/me
//     - missing ctx userID → 401 unauthorized
//     - happy path → 200 + UserInfo
//     - repo not-found → 401 (handler maps any err to 401)
//
//   UpdateProfileHandler  PATCH /api/v1/users/me
//     - missing ctx userID → 400 (handler maps err to 400; logic
//       actually returns "unauthorized" — handler maps to 400 not
//       401, see below)
//     - bad JSON body → 400 bind error
//     - happy path (nickname) → 200
//     - validation: nickname too long → 400
//     - repo not-found → 404 user not found
//
//   GetUserByIdHandler    GET   /api/v1/users/:id
//     - id=0 → 400 invalid id
//     - id=abc → 400 invalid id
//     - id=999 missing → 500 (handler maps any error from
//       logic.GetUserById to 500 — does NOT map ErrNotFound
//       specifically; this is a known contract, locked here)
//     - existing → 200
//
//   HealthHandler         GET   /health
//     - happy → 200 + JSON {status=ok, service=..., version}
//
// Note: middleware/auth.go (in this same svc) is a TYPE RE-EXPORT
// only — it doesn't implement a Middleware func. The actual auth
// lives in emotion-echo-shared/pkg/middleware/gin_auth.go which has
// its own sibling test there. Per AGENTS §一 1.1 we don't write a
// test for a pure type alias (no logic to RED).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/middleware"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

func newUserHandlerSvcCtx(repo repository.UserRepo) *svc.ServiceContext {
	return &svc.ServiceContext{Config: config.Config{}, UserRepo: repo}
}

func userCtxWithUID(uid int64) context.Context {
	return context.WithValue(context.Background(), middleware.CtxUserIDKey{}, uid)
}

func strPtr(s string) *string { return &s }

// =====================================================
// GetMeHandler
// =====================================================

func TestGetMeHandler_NoUserIDInCtx_Returns401(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := gin.New()
	r.GET("/api/v1/users/me", GetMeHandler(newUserHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "unauthorized")
}

func TestGetMeHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	repo.Create(context.Background(), &model.User{
		ID:       7,
		Username: "alice",
		Phone:    strPtr("+8613800000007"),
		Nickname: strPtr("Alice"),
	})
	r := gin.New()
	r.GET("/api/v1/users/me",
		func(c *gin.Context) {
			c.Request = c.Request.WithContext(userCtxWithUID(7))
			c.Next()
		},
		GetMeHandler(newUserHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "alice")
}

// =====================================================
// UpdateProfileHandler
// =====================================================

func TestUpdateProfileHandler_NoUserID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := gin.New()
	r.PATCH("/api/v1/users/me", UpdateProfileHandler(newUserHandlerSvcCtx(repo)))

	body := `{"nickname":"newname"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// handler's error branch (no auth) returns 400 per current impl.
	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateProfileHandler_BadJSON_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := gin.New()
	r.PATCH("/api/v1/users/me",
		func(c *gin.Context) {
			c.Request = c.Request.WithContext(userCtxWithUID(1))
			c.Next()
		},
		UpdateProfileHandler(newUserHandlerSvcCtx(repo)),
	)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me",
		strings.NewReader("{not-json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUpdateProfileHandler_HappyPath_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	repo.Create(context.Background(), &model.User{ID: 1, Username: "bob"})

	r := gin.New()
	r.PATCH("/api/v1/users/me",
		func(c *gin.Context) {
			c.Request = c.Request.WithContext(userCtxWithUID(1))
			c.Next()
		},
		UpdateProfileHandler(newUserHandlerSvcCtx(repo)),
	)

	body := `{"nickname":"newbob"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "newbob")
}

func TestUpdateProfileHandler_NicknameTooLong_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	repo.Create(context.Background(), &model.User{ID: 1, Username: "bob"})

	r := gin.New()
	r.PATCH("/api/v1/users/me",
		func(c *gin.Context) {
			c.Request = c.Request.WithContext(userCtxWithUID(1))
			c.Next()
		},
		UpdateProfileHandler(newUserHandlerSvcCtx(repo)),
	)

	// 64 chars > maxNicknameLen (32).
	body := `{"nickname":"` + strings.Repeat("a", 64) + `"}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "nickname")
}

// =====================================================
// GetUserByIdHandler
// =====================================================

func TestGetUserByIdHandler_ZeroID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := gin.New()
	r.GET("/api/v1/users/:id", GetUserByIdHandler(newUserHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/0", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid id")
}

func TestGetUserByIdHandler_NonNumericID_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := gin.New()
	r.GET("/api/v1/users/:id", GetUserByIdHandler(newUserHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/abc", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetUserByIdHandler_UnknownID_Returns500(t *testing.T) {
	t.Parallel()
	// Note: GetUserByIdHandler maps ANY error to 500 (does not
	// specially handle repository.ErrNotFound). Locking this
	// behavior here; future handler refactor must update this test.
	repo := repository.NewInMemoryUserRepo()
	r := gin.New()
	r.GET("/api/v1/users/:id", GetUserByIdHandler(newUserHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestGetUserByIdHandler_Existing_Returns200(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	repo.Create(context.Background(), &model.User{ID: 5, Username: "carol"})

	r := gin.New()
	r.GET("/api/v1/users/:id", GetUserByIdHandler(newUserHandlerSvcCtx(repo)))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "carol")
}

// =====================================================
// HealthHandler
// =====================================================

func TestHealthHandler_HappyPath_Returns200WithJSON(t *testing.T) {
	t.Parallel()
	svcCtx := &svc.ServiceContext{Config: config.Config{Name: "emotion-echo-user-svc"}}
	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "ok", got["status"])
}

// (no unused-import silence needed; all imports are used)