// Package metrics 提供 ai-svc 的 Prometheus 指标（Stage 20-5）
//
// 暴露 /metrics 端点（promhttp.Handler）供 Prometheus server 抓取。
//
// 当前指标：
//   - ai_svc_http_requests_total{method, path, status}      HTTP 请求计数
//   - ai_svc_http_request_duration_seconds{method, path}    HTTP 请求耗时（histogram）
//   - ai_svc_analyzer_total{analyzer, status}               Analyzer 调用（primary/secondary/fallback）
//
// 注册方式：
//   import "emotion-echo-ai-svc/internal/metrics"
//   r.Use(metrics.GinMetricsMiddleware())                       // gin HTTP metrics
//   r.GET("/metrics", gin.WrapH(metrics.PromHTTPHandler()))    // 暴露端点
//   metrics.AnalyzerCalls.WithLabelValues("primary", "ok").Inc()
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// 业务指标（自注册到 default registry）
var (
	// HTTPRequestsTotal HTTP 请求总数
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_svc_http_requests_total",
			Help: "Total number of HTTP requests processed, labeled by method, path, status.",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration HTTP 请求耗时直方图
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "ai_svc_http_request_duration_seconds",
			Help:    "Histogram of HTTP request latency in seconds.",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)

	// AnalyzerCalls Analyzer 调用统计
	// analyzer: primary | secondary | keyword
	// status:   ok | err
	AnalyzerCalls = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "ai_svc_analyzer_total",
			Help: "Total number of analyzer invocations, labeled by analyzer name and status.",
		},
		[]string{"analyzer", "status"},
	)
)

// PromHTTPHandler 返回 promhttp 的 http.Handler（用于 gin.WrapH 注册 /metrics）
func PromHTTPHandler() http.Handler {
	return promhttp.Handler()
}

// GinMetricsMiddleware gin HTTP metrics 收集中间件
//
// 记录：
//   - HTTPRequestsTotal{method, path, status}
//   - HTTPRequestDuration{method, path}（histogram）
func GinMetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next() // 处理请求

		// 跳过 /metrics 自身（避免指标采集产生的指标自循环）
		if c.Request.URL.Path == "/metrics" {
			return
		}

		path := c.FullPath() // 路由模板（如 /api/v1/emotion/message/:messageId），避免高基数
		if path == "" {
			path = "unmatched"
		}
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(method, path).Observe(time.Since(start).Seconds())
	}
}
