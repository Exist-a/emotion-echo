// Package session — passthrough_test.go
//
// Stage 30 / stage-30-web-bff.md T3.31: session passthrough helper 测试
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

func TestWithRequestAuth_ExtractsBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	c.Request.Header.Set("Authorization", "Bearer my-jwt")

	ctx := WithRequestAuth(c)
	assert.Equal(t, "my-jwt", downstream.JWTFromContext(ctx), "应提取 Bearer 后的 token")
}

func TestWithRequestAuth_NoAuth_KeepsCtx(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/health", nil)

	ctx := WithRequestAuth(c)
	assert.Equal(t, "", downstream.JWTFromContext(ctx), "无 Authorization → JWT 空")
}

func TestWithRequestAuth_NonBearer_PassesThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set("Authorization", "Token abc123")

	ctx := WithRequestAuth(c)
	assert.Equal(t, "Token abc123", downstream.JWTFromContext(ctx), "非 Bearer 原样传")
}

func TestAuthorizationFromContext_RoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	c.Request.Header.Set("Authorization", "Bearer my-jwt")

	ctx := WithRequestAuth(c)
	assert.Equal(t, "Bearer my-jwt", AuthorizationFromContext(ctx), "AuthorizationFromContext 应还原完整头值")
}

func TestAuthorizationFromContext_Empty(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", AuthorizationFromContext(ctx), "无 JWT → 空串")
}
