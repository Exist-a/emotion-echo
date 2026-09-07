// PR-OBS-10: chat-svc metrics endpoint contract test
//
// 目的：防止重构时漏挂 metrics 中间件（PR-4c-4 reset-password 同源教训）
// 复用 main.go 中已挂的路由（/health, /metrics），通过 main_test.go 启动 test server
// 但 chat-svc main 函数需 Nacos + DB 初始化，无法直接调
//
// 策略：单独构造最小 router（用 main.go 同样的中间件挂载 + 真实已注册路径）
// 验证 /metrics 端点暴露关键 series + service="chat-svc" label
package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	"github.com/gin-gonic/gin"
)

// readCounterFromBody 解析 Prometheus 文本格式,提取指定 labels 的 counter 值
func readCounterFromBody(t *testing.T, body, name string, labels map[string]string) float64 {
	t.Helper()
	var total float64
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			continue
		}
		if strings.HasPrefix(line, name) {
			rest := line[len(name):]
			if len(rest) > 0 && rest[0] == '{' {
				end := strings.Index(rest, "}")
				if end < 0 {
					continue
				}
				labelStr := rest[1:end]
				if !matchLabels(labelStr, labels) {
					continue
				}
				parts := strings.Fields(rest[end+1:])
				if len(parts) >= 1 {
					if v, err := strconv.ParseFloat(parts[0], 64); err == nil {
						total = v
					}
				}
			}
		}
	}
	return total
}

func matchLabels(labelStr string, want map[string]string) bool {
	for k, v := range want {
		expected := k + `="` + v + `"`
		if !strings.Contains(labelStr, expected) {
			return false
		}
	}
	return true
}

func TestMetricsEndpoint_ChatSvc(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(sharedmetrics.GinMetricsMiddleware("chat-svc"))
	// 复用 main.go:207-208 已注册路径
	r.GET("/health", func(c *gin.Context) { c.String(200, "ok") })
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))

	// 触发 1 次 /health
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))

	// 断言 1: /metrics 返回 200
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 200 {
		t.Fatalf("/metrics returns 200, got %d", w.Code)
	}

	// 断言 2: Content-Type
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") || !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("/metrics Content-Type should be Prometheus text format, got %q", ct)
	}

	// 断言 3: 关键 series
	body := w.Body.String()
	for _, m := range []string{
		"emotion_echo_http_requests_total",
		"emotion_echo_http_request_duration_seconds",
	} {
		if !strings.Contains(body, m) {
			t.Errorf("/metrics missing critical series %q", m)
		}
	}

	// 断言 4: svc label
	if !strings.Contains(body, `service="chat-svc"`) {
		t.Errorf("/metrics missing service label 'chat-svc'")
	}

	// 断言 5: 触发后 counter 增 1
	healthCounter := readCounterFromBody(t, body, "emotion_echo_http_requests_total", map[string]string{
		"service": "chat-svc", "method": "GET", "path": "/health", "status": "200",
	})
	if healthCounter < 1 {
		t.Errorf("emotion_echo_http_requests_total{service=chat-svc, path=/health} = %v, want >= 1", healthCounter)
	}
}
