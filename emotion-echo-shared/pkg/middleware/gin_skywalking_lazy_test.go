package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
)

// CL-F-14 回归钉：中间件在**构造期** tracer 为 nil、之后全局补挂时，
// 仍必须为后续请求建入口 span。
//
// 真实时序（2026-10-14 起栈实测）：svc 先于 OAP 启动 ⇒
// `GinSkywalkingMiddleware(NewGo2SkyTracer(nil))`；OAP 起来后后台补挂 SetTracer。
// 旧实现把 nil 快照进闭包 ⇒ 该服务整段生命周期零入口 span（SkyWalking 服务列表缺席）。
func TestGinSkywalkingMiddleware_ReadsGlobalTracer_WhenConstructedWithNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	grpcinterceptor.SetGlobalTracer(nil)
	t.Cleanup(func() { grpcinterceptor.SetGlobalTracer(nil) })

	r := gin.New()
	r.Use(GinSkywalkingMiddleware(nil)) // 构造期无 tracer
	r.GET("/api/v1/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	// 后台补挂发生在构造之后、请求之前
	stub := &stubTracer{}
	grpcinterceptor.SetGlobalTracer(stub)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Len(t, stub.startEntryCalls, 1,
		"补挂的全局 tracer 必须被中间件读到（旧实现快照 nil ⇒ 此处为 0）")
}

// 负向对照：构造期 nil + 全局也 nil ⇒ 照旧放行，不建 span、不 panic。
func TestGinSkywalkingMiddleware_StillNoopWhenGlobalAlsoNil(t *testing.T) {
	gin.SetMode(gin.TestMode)
	grpcinterceptor.SetGlobalTracer(nil)
	t.Cleanup(func() { grpcinterceptor.SetGlobalTracer(nil) })

	r := gin.New()
	r.Use(GinSkywalkingMiddleware(nil))
	r.GET("/api/v1/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

// 显式注入优先：构造期已有 tracer 时，不得被全局覆盖。
func TestGinSkywalkingMiddleware_ExplicitTracerWinsOverGlobal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	grpcinterceptor.SetGlobalTracer(nil)
	t.Cleanup(func() { grpcinterceptor.SetGlobalTracer(nil) })

	explicit := &stubTracer{}
	other := &stubTracer{}
	grpcinterceptor.SetGlobalTracer(other)

	r := gin.New()
	r.Use(GinSkywalkingMiddleware(explicit))
	r.GET("/api/v1/probe", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/probe", nil))

	assert.Len(t, explicit.startEntryCalls, 1, "显式注入的 tracer 应被使用")
	assert.Len(t, other.startEntryCalls, 0, "全局 tracer 不应抢占显式注入")
}
