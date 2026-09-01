// Package metrics 提供 Emotion-Echo 各微服务共用的 Prometheus 指标收集中间件
//
// 设计：
//   - 通用 HTTP metrics（每个 svc 共享同一组指标名）
//   - 通过 `service` label 区分不同 svc（避免指标冲突）
//   - 使用 promauto 自注册到 default registry
//
// 指标：
//   - http_requests_total{service, method, path, status}      HTTP 请求计数
//   - http_request_duration_seconds{service, method, path}    HTTP 请求耗时 histogram
//
// 用法：
//   import "github.com/emotion-echo/shared/pkg/metrics"
//   r.Use(metrics.GinMetricsMiddleware("chat-svc"))                 // gin middleware
//   r.GET("/metrics", gin.WrapH(metrics.PromHTTPHandler()))         // 暴露端点
//
// 命名约定：
//   service label 值建议：<svc-name> (e.g. "chat-svc", "ai-svc", "analytics-svc")
//
// Stage 25-E：从 ai-svc/internal/metrics 抽取为共享包，供 5 个 svc 共用。
// Stage 35 PR-6：新增 fusion_metrics.go（4 个 fusion collector）+ RegistryGatherCounter 测试 helper。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
)

// HTTPRequestsTotal HTTP 请求总数（按 service/method/path/status 区分）
var HTTPRequestsTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_http_requests_total",
		Help: "Total number of HTTP requests processed, labeled by service, method, path, status.",
	},
	[]string{"service", "method", "path", "status"},
)

// HTTPRequestDuration HTTP 请求耗时 histogram（按 service/method/path 区分）
var HTTPRequestDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "emotion_echo_http_request_duration_seconds",
		Help:    "Histogram of HTTP request latency in seconds.",
		Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
	},
	[]string{"service", "method", "path"},
)

// PromHTTPHandler 返回 promhttp 的 http.Handler（用于 gin.WrapH 注册 /metrics）
func PromHTTPHandler() http.Handler {
	return promhttp.Handler()
}

// GinMetricsMiddleware 返回 gin HTTP metrics 收集中间件
//
// 参数 serviceName：用于给所有指标打 `service` label（区分多 svc）
//
// 行为：
//   - 跳过 /metrics 自身（避免指标采集产生的指标自循环）
//   - path 使用 c.FullPath()（路由模板，避免高基数）
//   - 记录 status / method / 耗时
func GinMetricsMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next() // 处理请求

		// 跳过 /metrics 自身
		if c.Request.URL.Path == "/metrics" {
			return
		}

		path := c.FullPath() // 路由模板（如 /api/v1/conversations/:id/messages），避免高基数
		if path == "" {
			path = "unmatched"
		}
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		HTTPRequestsTotal.WithLabelValues(serviceName, method, path, status).Inc()
		HTTPRequestDuration.WithLabelValues(serviceName, method, path).Observe(time.Since(start).Seconds())
	}
}
// RegistryGatherCounter 读取 default registry 中指定 name + labels 的 Counter 当前值。
//
// 用于测试断言：counter.Inc() 后读出来的值应大于 Inc 之前的值。
// 返回 0 + nil 当 metric 不存在（首次注册前）。
func RegistryGatherCounter(name string, labels map[string]string) (float64, error) {
	mfs, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		return 0, err
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if !labelsMatch(m.GetLabel(), labels) {
				continue
			}
			if m.GetCounter() == nil {
				continue
			}
			return m.GetCounter().GetValue(), nil
		}
	}
	return 0, nil
}

// labelsMatch 判定 proto label list 是否包含且等于查询 labels。
func labelsMatch(have []*dto.LabelPair, want map[string]string) bool {
	if len(have) != len(want) {
		return false
	}
	for _, lp := range have {
		v, found := want[lp.GetName()]
		if !found || lp.GetValue() != v {
			return false
		}
	}
	return true
}
