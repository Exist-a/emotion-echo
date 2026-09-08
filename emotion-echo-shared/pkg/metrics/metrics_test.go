// Package metrics 测试（Stage 25-E）
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestGinMetricsMiddleware_IncrementsCounter(t *testing.T) {
	const svc = "test-svc"
	before := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
		"service": svc, "method": "GET", "path": "/test_inc", "status": "200",
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMetricsMiddleware(svc))
	r.GET("/test_inc", func(c *gin.Context) { c.String(200, "ok") })

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test_inc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("want 200, got %d", w.Code)
		}
	}

	after := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
		"service": svc, "method": "GET", "path": "/test_inc", "status": "200",
	})

	if after-before < 3 {
		t.Errorf("counter increment < 3: before=%v after=%v", before, after)
	}
}

func TestPromHTTPHandler_ServesMetrics(t *testing.T) {
	const svc = "test-svc-metrics"

	gin.SetMode(gin.TestMode)
	// 触发 1 次 HTTP 请求让 counter 出现
	r2 := gin.New()
	r2.Use(GinMetricsMiddleware(svc))
	r2.GET("/api/v1/health", func(c *gin.Context) { c.String(200, "ok") })
	r2.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/health", nil))

	// 拉取 /metrics
	r := gin.New()
	r.GET("/metrics", gin.WrapH(PromHTTPHandler()))

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "emotion_echo_http_requests_total") {
		t.Errorf("metrics output missing emotion_echo_http_requests_total:\n%s", body)
	}
	if !strings.Contains(body, `service="`+svc+`"`) {
		t.Errorf("metrics output missing service label %q:\n%s", svc, body)
	}
}

func TestGinMetricsMiddleware_SkipsMetricsRoute(t *testing.T) {
	const svc = "test-svc-skip"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMetricsMiddleware(svc))
	r.GET("/metrics", gin.WrapH(PromHTTPHandler()))

	before := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
		"service": svc, "method": "GET", "path": "/metrics", "status": "200",
	})

	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	after := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
		"service": svc, "method": "GET", "path": "/metrics", "status": "200",
	})

	if after != before {
		t.Errorf("/metrics should not be counted, but counter changed: before=%v after=%v", before, after)
	}
}

func TestGinMetricsMiddleware_DifferentServicesIndependent(t *testing.T) {
	const svcA = "test-svc-A"
	const svcB = "test-svc-B"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(GinMetricsMiddleware(svcA))
	r.Use(GinMetricsMiddleware(svcB)) // 串联两层
	r.GET("/api/v1/health", func(c *gin.Context) { c.String(200, "ok") })

	for i := 0; i < 5; i++ {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/health", nil))
	}

	a := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
		"service": svcA, "method": "GET", "path": "/api/v1/health", "status": "200",
	})
	b := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
		"service": svcB, "method": "GET", "path": "/api/v1/health", "status": "200",
	})

	// ===== PR-OBS-9 RED: 4 个 metrics 契约增强 case =====
	// 目的: 防止后续重构漏挂 /metrics + Content-Type 漂移 + histogram bucket 缺失
	// (与 PR-4c-4 reset-password 同源教训: 单测绿 ≠ 端到端通)
	//
	// 期望覆盖:
	// 1. PromHTTPHandler Content-Type 是 text/plain;version=0.0.4 (Prometheus 文本格式)
	// 2. /metrics 端点暴露关键 series: http_requests_total + http_request_duration_seconds + go_goroutines
	// 3. histogram bucket series 存在 (emotion_echo_http_request_duration_seconds_bucket)
	// 4. /metrics 自循环 N 次请求不增 counter (强化现有 SkipsMetricsRoute)

	// PR-OBS-9 case 1: PromHTTPHandler Content-Type = text/plain;version=0.0.4
	// (现有 TestPromHTTPHandler_ServesMetrics 只断言 200 + body 含 series 名,未验 Content-Type)
	// 期望 Content-Type = "text/plain; version=0.0.4; charset=utf-8" (promhttp 默认)
	t.Run("Content-Type is Prometheus text format", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.GET("/metrics", gin.WrapH(PromHTTPHandler()))

		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("want 200, got %d", w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "text/plain") {
			t.Errorf("Content-Type should start with text/plain (Prometheus exposition format), got %q", ct)
		}
		if !strings.Contains(ct, "version=0.0.4") {
			t.Errorf("Content-Type should contain Prometheus version=0.0.4, got %q", ct)
		}
	})

	// PR-OBS-9 case 2: /metrics 暴露 3 个关键 series (请求总数 / 延迟直方图 / goroutines)
	t.Run("exposes all 3 critical series", func(t *testing.T) {
		const svc = "test-svc-critical-series"
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinMetricsMiddleware(svc))
		r.GET("/health", func(c *gin.Context) { c.String(200, "ok") })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))

		r2 := gin.New()
		r2.GET("/metrics", gin.WrapH(PromHTTPHandler()))
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		body := w.Body.String()

		mustContain := []string{
			"emotion_echo_http_requests_total",                     // counter
			"emotion_echo_http_request_duration_seconds",            // histogram (sum/count/bucket)
			"go_goroutines",                                          // 进程级 metric
		}
		for _, m := range mustContain {
			if !strings.Contains(body, m) {
				t.Errorf("/metrics missing critical series %q", m)
			}
		}
	})

	// PR-OBS-9 case 3: histogram bucket series 存在 (emotion_echo_http_request_duration_seconds_bucket)
	t.Run("histogram bucket series present", func(t *testing.T) {
		const svc = "test-svc-bucket"
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinMetricsMiddleware(svc))
		r.GET("/api/v1/test", func(c *gin.Context) { c.String(200, "ok") })
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/test", nil))

		r2 := gin.New()
		r2.GET("/metrics", gin.WrapH(PromHTTPHandler()))
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
		body := w.Body.String()

		// histogram bucket 形如: emotion_echo_http_request_duration_seconds_bucket{...,le="0.005"}
		// 必须含 _bucket 后缀 + le label
		if !strings.Contains(body, "emotion_echo_http_request_duration_seconds_bucket") {
			t.Errorf("/metrics missing histogram bucket series, body_len=%d", len(body))
		}
		if !strings.Contains(body, `le="`) {
			t.Errorf("/metrics missing histogram le label")
		}
		// 至少要有 11 个 bucket (与 metrics.go:50 buckets 定义对齐)
		bucketCount := strings.Count(body, "_bucket{")
		if bucketCount < 11 {
			t.Errorf("histogram bucket count = %d, want >= 11 (per metrics.go Buckets def)", bucketCount)
		}
	})

	// PR-OBS-9 case 4: /metrics 自循环 N 次 (强化现有 SkipsMetricsRoute)
	// 现有 case 只测 1 次,无法发现\"中间件逻辑漂移\"(如新增 path 检查但漏写)
	t.Run("self-loop 100x does not increment counter", func(t *testing.T) {
		const svc = "test-svc-self-loop"
		gin.SetMode(gin.TestMode)
		r := gin.New()
		r.Use(GinMetricsMiddleware(svc))
		r.GET("/metrics", gin.WrapH(PromHTTPHandler()))

		before := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
			"service": svc, "method": "GET", "path": "/metrics", "status": "200",
		})

		for i := 0; i < 100; i++ {
			r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/metrics", nil))
		}

		after := readCounter(t, "emotion_echo_http_requests_total", map[string]string{
			"service": svc, "method": "GET", "path": "/metrics", "status": "200",
		})
		if after != before {
			t.Errorf("100x /metrics should NOT increment counter, but before=%v after=%v", before, after)
		}
	})

	if a < 5 {
		t.Errorf("svcA counter = %v, want >= 5", a)
	}
	if b < 5 {
		t.Errorf("svcB counter = %v, want >= 5", b)
	}
}

// readCounter 拿 prometheus counter 当前值
func readCounter(t *testing.T, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matchLabelPairs(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func matchLabelPairs(got []*dto.LabelPair, want map[string]string) bool {
	gotMap := make(map[string]string, len(got))
	for _, lp := range got {
		gotMap[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if gotMap[k] != v {
			return false
		}
	}
	return true
}