// Package handler — auth_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.32 RED: auth handler 契约测试
package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/auth"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAuthRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	mgr, err := auth.NewManager("test-secret", 3600)
	require.NoError(t, err)
	router := gin.New()
	router.POST("/api/v1/auth/:action", NewAuthHandler(mgr))
	return router
}

func TestAuthHandler_Login_ReturnsToken(t *testing.T) {
	router := newAuthRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"alice@test.com","password":"x","rememberMe":true}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var data LoginData
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	assert.NotEmpty(t, data.AccessToken, "应签发 accessToken")
	assert.Greater(t, data.ExpiresIn, int64(0))
	assert.NotZero(t, data.User.UserID, "user.userId 应非零")
	assert.Equal(t, "alice@test.com", data.User.Account)
}

func TestAuthHandler_Login_StableUserID(t *testing.T) {
	router := newAuthRouter(t)
	// 同一 username 两次登录 → 同一 user_id
	body := func() LoginData {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			bytes.NewReader([]byte(`{"username":"bob@test.com","password":"x"}`)))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var data LoginData
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
		return data
	}
	d1, d2 := body(), body()
	assert.Equal(t, d1.User.UserID, d2.User.UserID, "同一账号应映射到同一 user_id")
}

func TestAuthHandler_Login_EmptyCredentials_Returns400(t *testing.T) {
	router := newAuthRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"","password":""}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "username and password are required")
}

func TestAuthHandler_Register_ReturnsToken(t *testing.T) {
	router := newAuthRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register",
		bytes.NewReader([]byte(`{"username":"new@test.com","password":"x","verificationCode":"1234"}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var data LoginData
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	assert.NotEmpty(t, data.AccessToken)
}

func TestAuthHandler_Logout_ReturnsSuccess(t *testing.T) {
	router := newAuthRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"success":true`)
}

func TestAuthHandler_VerificationCode_ReturnsSuccess(t *testing.T) {
	router := newAuthRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verification-code",
		bytes.NewReader([]byte(`{"username":"a@b.com"}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthHandler_TokenCanBeParsed(t *testing.T) {
	// 登录签发 → auth.Manager 能解析 → user_id 稳定
	mgr, _ := auth.NewManager("test-secret", 3600)
	router := gin.New()
	router.POST("/api/v1/auth/:action", NewAuthHandler(mgr))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"carol@test.com","password":"x"}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var data LoginData
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &data))
	uid, err := mgr.Parse(data.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, data.User.UserID, uid, "token 内 user_id 应等于响应 user.userId")
}
