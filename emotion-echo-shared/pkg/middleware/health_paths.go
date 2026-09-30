package middleware

// 免鉴权的健康探针路径。集中在此定义，供 gin 版与 net/http 版两处中间件共用，
// 避免白名单在两处漂移（历史缺陷形态：改了一处漏了另一处，端点静默 401）。

const (
	// HealthPath 是 liveness 端点：进程活着即 200，不受依赖状态影响。
	HealthPath = "/health"
	// HealthReadyPath 是 readiness 端点（D-29 新增）：依赖不可用时返 503。
	// compose healthcheck 指向本端点。
	HealthReadyPath = "/health/ready"
	// MetricsPath 是 Prometheus 抓取端点。
	MetricsPath = "/metrics"
)

// isHealthProbePath 判定请求路径是否属于免鉴权的运维探针面。
//
// 刻意用**精确匹配**而非 strings.HasPrefix：若用前缀 "/health"，
// /health/live、/health/debug 等未定义路径会一并被放行，等于开了鉴权后门。
// 确需新增探针端点时，在此显式登记并同步测试。
func isHealthProbePath(path string) bool {
	switch path {
	case HealthPath, HealthReadyPath, MetricsPath:
		return true
	default:
		return false
	}
}
