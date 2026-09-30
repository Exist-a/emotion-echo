package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// E2E-23 A 组 · D-29 决议配套：/health/ready 必须免鉴权。
//
// 🔴 陷阱来源：两处鉴权中间件都用**精确匹配** `path == "/health"`
// （gin_auth.go:112、jwt_auth.go:53）。D-29 决议新增 `/health/ready` 后，
// 若不同步扩展白名单，该端点会被 401 拦掉 —— 而 compose healthcheck
// 是从容器内 wget、不带 X-User-Id 头 ⇒ 探针恒失败、容器永远不 healthy。
//
// 这类缺陷靠"加完路由手工 curl 一下"抓不到（curl 会带上任意 header 或
// 走本地），必须钉成自动化测试。

// TestGinAuthMiddleware_SkipsHealthEndpoints 锁定 gin 版白名单。
//
// 必须注册**真实路由**后再过中间件：CreateTestContext 不跑 handler 链，
// 中间件内部 c.Next() 之后没有 handler，状态码无法反映"是否被放行"，
// 会把未覆盖与已覆盖都读成 200（假绿）。
//
// 两类断言分开写：
//   - 免鉴权端点：**不带** X-User-Id 也必须放行（这才是"白名单覆盖"的证据）
//   - 受保护端点：**不带** X-User-Id 必须 401（带了就放行是设计如此，不该断言）
func TestGinAuthMiddleware_SkipsHealthEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		path     string
		wantPass bool
		reason   string
	}{
		{path: "/health", wantPass: true, reason: "存量端点，免鉴权"},
		{path: "/health/ready", wantPass: true, reason: "D-29 新增 readiness 端点，必须免鉴权（compose 探针不带 X-User-Id）"},
		{path: "/metrics", wantPass: true, reason: "存量端点，免鉴权"},
		{path: "/health/live", wantPass: false, reason: "未定义的子路径不得因前缀相似而被放行"},
		{path: "/api/v1/x", wantPass: false, reason: "业务端点必须鉴权"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			router := gin.New()
			router.Use(GinAuthMiddlewareWithOpts(AuthOpts{RequireAPISIXIP: false}))
			// 真实 handler：被放行才会执行到这里并回 "reached"。
			router.GET(tt.path, func(c *gin.Context) { c.String(http.StatusOK, "reached") })

			rec := httptest.NewRecorder()
			// 一律不带 X-User-Id：免鉴权端点应照样放行，受保护端点应 401。
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			router.ServeHTTP(rec, req)

			if tt.wantPass {
				if rec.Code != http.StatusOK || rec.Body.String() != "reached" {
					t.Fatalf("%s: 无 X-User-Id 期望放行到 handler，实际 %d —— 白名单未覆盖该端点（%s）",
						tt.path, rec.Code, tt.reason)
				}
			} else {
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("%s: 无 X-User-Id 期望 401 实际 %d —— 白名单过宽，鉴权被绕过（%s）",
						tt.path, rec.Code, tt.reason)
				}
			}
		})
	}
}

// TestGinAuthMiddleware_ProtectedPathsStillAcceptTrustedCaller 反向保障：
// 受保护端点在带合法 X-User-Id 时**仍要放行** —— 白名单扩了不等于把业务端点也关了。
func TestGinAuthMiddleware_ProtectedPathsStillAcceptTrustedCaller(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, path := range []string{"/health/live", "/api/v1/x"} {
		path := path
		t.Run(path, func(t *testing.T) {
			router := gin.New()
			router.Use(GinAuthMiddlewareWithOpts(AuthOpts{RequireAPISIXIP: false}))
			router.GET(path, func(c *gin.Context) { c.String(http.StatusOK, "reached") })

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set(XUserIDHeader, "1")
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("%s: 带合法 X-User-Id 期望 200，实际 %d —— 白名单扩展误伤了业务路径", path, rec.Code)
			}
		})
	}
}

// TestAuthMiddleware_SkipsHealthEndpoints 锁定 net/http 版白名单。
// 该版本虽非服务实际使用的（服务用 gin 版），但两处白名单必须同步，
// 否则后续有人切回 net/http 栈时 /health/ready 静默 401。
func TestAuthMiddleware_SkipsHealthEndpoints(t *testing.T) {
	tests := []struct {
		path     string
		wantPass bool
	}{
		{path: "/health", wantPass: true},
		{path: "/health/ready", wantPass: true},
		{path: "/metrics", wantPass: true},
		{path: "/health/live", wantPass: false},
		{path: "/api/v1/x", wantPass: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			// 一律不带 X-User-Id：免鉴权端点应照样放行，受保护端点应 401。
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)

			nextCalled := false
			AuthMiddleware()(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(http.StatusOK)
			}).ServeHTTP(rec, req)

			if tt.wantPass && !nextCalled {
				t.Fatalf("%s: 期望放行但未进入 next —— 白名单未覆盖", tt.path)
			}
			if !tt.wantPass && nextCalled {
				t.Fatalf("%s: 期望 401 但进入了 next —— 白名单过宽", tt.path)
			}
		})
	}
}
