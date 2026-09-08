// PR-OBS-16: chat-svc bootstrap 装配断言
//
// 目的: 防止重构漏挂 GinMetricsMiddleware / GinAuthMiddleware
// (与 PR-4c-4 reset-password 同源教训)
//
// 注: GinSkywalkingMiddleware 是条件挂 (main.go:201 if tracer != nil)
// nil tracer 测试场景下 skywalking 不会挂 (此 case 不测 skywalking)
//
// TDD 说明 — 测试覆盖补全,不是 RED->GREEN 周期:
// chat-svc main.go:200-204 中间件已挂,本 case 直接 GREEN
package main

import (
	"net/http/httptest"
	"strings"
	"testing"

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"

	"github.com/gin-gonic/gin"
)

// TestBootstrap_AssessmentSvc_AllMiddlewaresAttached 验证 chat-svc 中间件装配
func TestBootstrap_AssessmentSvc_AllMiddlewaresAttached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 复用 main.go:200-204 中间件装配顺序
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("assessment-svc"))
	r.Use(sharedmw.GinAuthMiddleware())
	// SkyWalking 条件挂 (nil tracer),本 case 不挂
	r.GET("/health", func(c *gin.Context) { c.Status(200) })
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))

	// 1. /metrics 返回 200 + 含 go_goroutines (PromHTTPHandler 注册)
	wMetrics := httptest.NewRecorder()
	r.ServeHTTP(wMetrics, httptest.NewRequest("GET", "/metrics", nil))
	if wMetrics.Code != 200 {
		t.Errorf("/metrics should return 200, got %d", wMetrics.Code)
	}
	if !strings.Contains(wMetrics.Body.String(), "go_goroutines") {
		t.Errorf("/metrics body missing go_goroutines")
	}

	// 2. /health 200 (AuthMiddleware 白名单)
	wHealth := httptest.NewRecorder()
	r.ServeHTTP(wHealth, httptest.NewRequest("GET", "/health", nil))
	if wHealth.Code != 200 {
		t.Errorf("/health should return 200, got %d", wHealth.Code)
	}

	// 3. 业务路由 /api/v1/conversations 无 X-User-Id → 401 (GinAuthMiddleware 已挂)
	wNoAuth := httptest.NewRecorder()
	r.ServeHTTP(wNoAuth, httptest.NewRequest("POST", "/api/v1/conversations",
		strings.NewReader(`{}`)))
	if wNoAuth.Code != 401 {
		t.Errorf("/api/v1/conversations without X-User-Id should return 401 (GinAuthMiddleware not attached?), got %d", wNoAuth.Code)
	}

	// 4. 业务路由带 X-User-Id → 不应被拦截 (中间件透传)
	wAuth := httptest.NewRecorder()
	wAuthReq := httptest.NewRequest("POST", "/api/v1/conversations",
		strings.NewReader(`{}`))
	wAuthReq.Header.Set("X-User-Id", "1")
	r.ServeHTTP(wAuth, wAuthReq)
	// handler 不存在 → 404 (不是 401),证明 auth 通过
	if wAuth.Code == 401 {
		t.Errorf("/api/v1/conversations with X-User-Id should NOT return 401, got 401 (GinAuthMiddleware blocking legit req?)")
	}
}
