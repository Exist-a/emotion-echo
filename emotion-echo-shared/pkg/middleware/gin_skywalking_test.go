package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGinSkywalkingMiddleware_SetsTracerOnContext 业务路径应把 tracer 挂到 gin ctx
func TestGinSkywalkingMiddleware_SetsTracerOnContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var (
		gotTracer bool
		gotNil    bool
	)
	r.GET("/api/v1/foo", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		v, ok := c.Get("skywalking_tracer")
		gotTracer = ok
		if !ok {
			gotNil = true
		}
		_ = v
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if !gotTracer {
		t.Fatalf("expected skywalking_tracer key on ctx")
	}
	_ = gotNil
}

// TestGinSkywalkingMiddleware_SkipsHealth /health 直接放行不挂 tracer
func TestGinSkywalkingMiddleware_SkipsHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var sawKey bool
	r.GET("/health", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		_, sawKey = c.Get("skywalking_tracer")
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if sawKey {
		t.Fatalf("/health should not attach tracer key")
	}
}

// TestGinSkywalkingMiddleware_SkipsInternalPrefix /internal/* 全部跳过
func TestGinSkywalkingMiddleware_SkipsInternalPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, p := range []string{"/internal/sw-status", "/internal/debug", "/internal/"} {
		sawKey := false
		r := gin.New()
		r.GET(p, GinSkywalkingMiddleware(nil), func(c *gin.Context) {
			_, sawKey = c.Get("skywalking_tracer")
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status want=200 got=%d", p, rec.Code)
		}
		if sawKey {
			t.Fatalf("%s should not attach tracer key, but did", p)
		}
	}
}

// TestGinSkywalkingMiddleware_AttachesForPath 表驱动：所有非白名单路径应挂 tracer
func TestGinSkywalkingMiddleware_AttachesForPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, p := range []string{"/", "/api/v1/chat", "/api/v1/ai/health", "/api/v1/tts/synthesize"} {
		sawKey := false
		r := gin.New()
		r.GET(p, GinSkywalkingMiddleware(nil), func(c *gin.Context) {
			_, sawKey = c.Get("skywalking_tracer")
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		r.ServeHTTP(rec, req)
		if !sawKey {
			t.Fatalf("%s should attach tracer key, but did not (status=%d)", p, rec.Code)
		}
	}
}

// TestGinSkywalkingMiddleware_NilTracerPassedThrough 传 nil tracer 应不崩且仍走 next
func TestGinSkywalkingMiddleware_NilTracerPassedThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	called := false
	r.GET("/api/v1/ok", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ok", nil)
	r.ServeHTTP(rec, req)
	if !called {
		t.Fatalf("next handler should run even with nil tracer")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
}
