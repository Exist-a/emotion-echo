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