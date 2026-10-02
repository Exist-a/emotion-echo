// E2E-26 测试点 #3：GinSkywalkingMiddleware 必须把 StartEntry 返回的承载 ctx
// 装回 c.Request —— 否则 handler 里发起的下游 gRPC/Kafka exit span 续不上
// HTTP 入口 span，每个下游调用各起新 trace（OAP 实测孤岛 trace 的第二层根因）。
package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type ginSpanCtxKey struct{}

// TestGinSkywalkingMiddleware_InstallsSpanCtxIntoRequest handler 必须能从
// c.Request.Context() 读到 StartEntry 注入的 span 承载 ctx。
func TestGinSkywalkingMiddleware_InstallsSpanCtxIntoRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)

	marked := context.WithValue(context.Background(), ginSpanCtxKey{}, "span-ctx")
	stub := &stubTracer{ctxToReturn: marked}

	var gotCtx context.Context
	r := gin.New()
	r.Use(GinSkywalkingMiddleware(stub))
	r.GET("/api/ctx-probe", func(c *gin.Context) {
		gotCtx = c.Request.Context()
		c.Status(200)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/ctx-probe", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, 200, w.Code)
	require.NotNil(t, gotCtx, "handler 应取到 request ctx")
	assert.Equal(t, "span-ctx", gotCtx.Value(ginSpanCtxKey{}),
		"中间件必须把 StartEntry 返回的承载 ctx 装回 c.Request——"+
			"当前 `_, span = ...` 丢弃 ctx ⇒ 下游 exit span 全部脱离入口 trace（E2E-26 #3 第二层根因）")
}
