// Package session — passthrough_test.go
//
// Stage 32 PR-16: session passthrough helper 测试（X-User-Id 模式）。
package session

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWithRequestAuth_ExtractsXUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	c.Request.Header.Set(XUserIDHeader, "1001")

	ctx := WithRequestAuth(c)
	uid, ok := downstream.UserIDFromContext(ctx)
	assert.True(t, ok, "应提取 X-User-Id")
	assert.Equal(t, int64(1001), uid, "uid 应为 1001")
}

func TestWithRequestAuth_NoHeader_KeepsCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)

	ctx := WithRequestAuth(c)
	_, ok := downstream.UserIDFromContext(ctx)
	assert.False(t, ok, "无 X-User-Id → ctx 无 user_id")
}

func TestWithRequestAuth_InvalidXUserID_KeepsCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set(XUserIDHeader, "not-a-number")

	ctx := WithRequestAuth(c)
	_, ok := downstream.UserIDFromContext(ctx)
	assert.False(t, ok, "非法 X-User-Id → ctx 无 user_id（不污染）")
}

func TestAuthorizationFromContext_AlwaysEmptyAfterStage32(t *testing.T) {
	// Stage 32 PR-16: BFF 不再透传 Authorization，AuthorizationFromContext 总返回空
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set("Authorization", "Bearer my-jwt")

	ctx := WithRequestAuth(c)
	assert.Equal(t, "", AuthorizationFromContext(ctx),
		"Stage 32 后 BFF 不再透传 Authorization，函数总返回空（保留签名兼容）")
}

func TestAuthorizationFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", AuthorizationFromContext(ctx))
}
