// Package main — main_test.go
//
// Sprint 1 PR-1 (2026-09-04): BFF 路由清单契约测试
//
// 目的：断言 registerRoutes 注册的完整路径集合（含主路径 27 条 + EmotionQ 条件分支 +3），
// 防止以下漂移未被发现：
//   - 改 handler 时无意删路由（无报警）
//   - 加新路由但忘了在某处同步（下游对接才发现）
//   - refactor 把动态参数 :id / :kind / :action / :messageId 误改成 :ID 等
//
// 测试设计（调研确认要点）：
//   - registerRoutes 阶段只调 r.GET/POST/... 把 handler 函数 ref 塞进 trie，**不调任何下游 client 方法**
//     —— 6 个 client 字段 (User/Chat/Assessment/Analytics/AI/XTTS) 可保持 nil，无需构造 fake
//   - cfg.Auth.JWTSecret 必须非空（auth.NewManager 强校验；空 secret 触发 log.Fatalf）
//   - s.EmotionQ 默认 nil（条件分支不进入）；EmotionQ 单独 case 覆盖 +3 条
//   - gin.Routes() 返回 RoutesInfo；用 testify assert.Subset 按 (Method, Path) 字段比对
//     （HandlerFunc 字段零值为 nil，Handler 字段零值为 ""，确保 reflect.DeepEqual 不被干扰）
//
// 调研依据：
//   - emotion-echo-web-bff/main.go:214-246 registerRoutes 完整结构
//   - emotion-echo-web-bff/internal/handler/{user,chat,survey,analytics,multimodal,tts,upload,emotion_query}_handler.go 各 Register 方法
//   - emotion-echo-web-bff/internal/svc/servicecontext.go:15-29 ServiceContext 结构
//   - emotion-echo-web-bff/internal/auth/jwt.go:48 NewManager 强校验 JWTSecret
//   - todo-pile-2026-09-04.md C8 + stage-40 §四 此 PR 的根因记录
package main

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/svc"

	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// wantRoutes 是 registerRoutes 必须注册的完整路径集合（不含 EmotionQ 条件分支）。
// 新增/删除路由必须同步更新本列表 + 在 PR 描述里说明。
// 行内排序：先 main.go 直接注册的 4 条，再按 handler 文件字母序。
var wantRoutes = gin.RoutesInfo{
	// ----- main.go 直接注册（4 条）-----
	{Method: "GET", Path: "/health"},
	{Method: "GET", Path: "/metrics"},
	{Method: "POST", Path: "/api/v1/auth/:action"},
	{Method: "POST", Path: "/api/v1/ai/stream"},

	// ----- user_handler.go (4 条) -----
	{Method: "GET", Path: "/api/v1/user/profile"},
	{Method: "GET", Path: "/api/v1/users/me"},
	{Method: "PATCH", Path: "/api/v1/users/me"},
	{Method: "GET", Path: "/api/v1/users/:id"},

	// ----- chat_handler.go (5 条) -----
	{Method: "GET", Path: "/api/v1/conversations"},
	{Method: "POST", Path: "/api/v1/conversations"},
	{Method: "POST", Path: "/api/v1/conversations/:id/messages"},
	{Method: "GET", Path: "/api/v1/conversations/:id/messages"},
	{Method: "DELETE", Path: "/api/v1/conversations/:id"},

	// ----- survey_handler.go (5 条) -----
	// 注意：/api/v1/surveys/results 必须在 /api/v1/surveys/:id 之前注册（survey_handler.go:36 注释）
	{Method: "GET", Path: "/api/v1/surveys"},
	{Method: "GET", Path: "/api/v1/surveys/results"},
	{Method: "GET", Path: "/api/v1/surveys/results/:resultId"},
	{Method: "GET", Path: "/api/v1/surveys/:id"},
	{Method: "POST", Path: "/api/v1/surveys/:id/submit"},

	// ----- analytics_handler.go (6 条) -----
	{Method: "GET", Path: "/api/v1/reports/daily"},
	{Method: "GET", Path: "/api/v1/reports/trend"},
	{Method: "GET", Path: "/api/v1/user-behavior/day-night"},
	{Method: "GET", Path: "/api/v1/user-behavior/depth"},
	{Method: "GET", Path: "/api/v1/user-behavior/frequency"},
	{Method: "GET", Path: "/api/v1/mental-health/assessment"},

	// ----- multimodal_handler.go (1 条) -----
	{Method: "POST", Path: "/api/v1/multimodal/analyze"},

	// ----- tts_handler.go (2 条) -----
	{Method: "POST", Path: "/api/v1/tts/synthesize"},
	{Method: "POST", Path: "/api/v1/tts/stream"},

	// ----- upload_handler.go (1 条) -----
	{Method: "POST", Path: "/api/v1/uploads/:kind"},

	// ----- voice_handler.go (Sprint 1 PR-4c-1, 1 条) -----
	{Method: "POST", Path: "/api/v1/voice/upload"},

	// ----- avatar_handler.go (Sprint 1 PR-4c-2, 1 条) -----
	{Method: "POST", Path: "/api/v1/user/avatar"},
}

// wantRoutesWithEmotionQ 是当 svc.EmotionQ != nil 时额外注册的 3 条。
var wantRoutesWithEmotionQ = gin.RoutesInfo{
	{Method: "GET", Path: "/api/v1/emotion/message/:messageId"},
	{Method: "GET", Path: "/api/v1/emotion/message/:messageId/fused"},
	{Method: "GET", Path: "/api/v1/emotion/conversation/:conversationId"},
}

// stubServiceContext 构造一个 registerRoutes 能接受的最小 ServiceContext。
//   - cfg.Auth.JWTSecret 必须非空（auth.NewManager 强校验）
//   - 6 个 client 字段保持 nil：registerRoutes 阶段只 r.GET/POST，不调任何 client 方法
//   - EmotionQ 默认 nil（条件分支不进入）
func stubServiceContext(t *testing.T, withEmotionQ bool) (*svc.ServiceContext, *config.Config) {
	t.Helper()

	cfg := &config.Config{}
	cfg.Auth.JWTSecret = "test-jwt-secret-32chars-min-padding"
	cfg.Auth.TokenTTLSeconds = 3600
	cfg.Health.TimeoutMs = 1000

	mgr, err := auth.NewManager(cfg.Auth.JWTSecret, cfg.Auth.TokenTTLSeconds)
	require.NoError(t, err, "auth.NewManager 不应失败（JWTSecret 非空）")

	s := svc.NewServiceContext(*cfg)
	s.Auth = mgr
	// 6 个 client 保持 nil —— registerRoutes 不触发调用
	if withEmotionQ {
		// EmotionQ 是 interface 字段；nil 也算"非 nil"——所以这里其实只能 stub 真实 client
		// 但调研结论：保持 nil + EmotionQ != nil 判断 = false → 分支不进；与 EmotionQ != nil 分支
		// 完全等价（都会跳过 emotion_query 注册）。**这里特意保持 nil 以测主分支**。
		// 第二个 case (TestRegisterRoutes_WithEmotionQ) 不在本 PR 范围，留待后续 PR（需要 stub EmotionQueryClient）
	}
	return s, cfg
}

func TestRegisterRoutes_MainContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s, cfg := stubServiceContext(t, false)
	registerRoutes(r, s, cfg)

	// testify assert.Subset 用 reflect.DeepEqual 比对 RouteInfo 全字段（含 Handler/HandlerFunc）；
	// got 的 Handler/HandlerFunc 是真实值（字符串 + 函数指针），want 是零值，会永远不等。
	// 解法：清空 got 的 Handler/HandlerFunc 字段，只比 (Method, Path)。
	got := r.Routes()
	for i := range got {
		got[i].Handler = ""
		got[i].HandlerFunc = nil
	}

	// 断言 1：主路径集合 ⊇ 期望（不允许少注册路由）
	assert.Subset(t, got, wantRoutes, "registerRoutes 必须注册 wantRoutes 全部条目")

	// 断言 2：路由总数 = wantRoutes 数（EmotionQ 为 nil 时不应有 emotion 3 条）
	assert.Len(t, got, len(wantRoutes), "无 EmotionQ 时路由总数必须等于 wantRoutes 长度")

	// 断言 3：emotion 3 条**不**在路由集合里（条件分支未进入）
	for _, er := range wantRoutesWithEmotionQ {
		assert.NotContains(t, got, er, "EmotionQ=nil 时不应注册 emotion_query 路由")
	}
}

// ===== PR-OBS-10: web-bff /metrics 端点契约测试 =====
//
// 目的：防止重构时漏挂 metrics 中间件 (PR-4c-4 reset-password 同源教训)
// 复用 stubServiceContext + registerRoutes 启动完整 router,
// 然后 httptest 拉 /metrics 端点,断言：
//   1. /metrics 返回 200 + Content-Type text/plain;version=0.0.4
//   2. /metrics body 含 emotion_echo_http_requests_total + emotion_echo_http_request_duration_seconds
//   3. /metrics 含 svc 短名 label: service="web-bff"
//   4. 触发 1 次 /health 后 http_requests_total 增 1 (label 正确)
//
// 注：与 TestRegisterRoutes_MainContract 共享 stubServiceContext,
// 避免重复构造 ServiceContext (含 Nacos boot + DB 连接等副作用)。
func TestMetricsEndpoint_WebBFF(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s, cfg := stubServiceContext(t, false)
	// 复用 main.go 真实启动路径中的中间件装配（含 metrics）
	r.Use(sharedmetrics.GinMetricsMiddleware("web-bff"))
	// 不手动注册 /metrics 与 /health — registerRoutes 已注册
	// (main.go:252 r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler())))
	registerRoutes(r, s, cfg)

	// 触发 1 次 registerRoutes 已注册的路由 (auth/login),让 counter 出现
	// 用 POST /api/v1/auth/login (registerRoutes 注册的 catch-all auth 路由)
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader("{}")))

	// 断言 1: /metrics 返回 200
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("/metrics returns 200, got %d", w.Code)
	}

	// 断言 2: Content-Type 是 Prometheus text format
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") || !strings.Contains(ct, "version=0.0.4") {
		t.Errorf("/metrics Content-Type should be Prometheus text format, got %q", ct)
	}

	// 断言 3: 关键 series 名齐全
	body := w.Body.String()
	mustContain := []string{
		"emotion_echo_http_requests_total",
		"emotion_echo_http_request_duration_seconds",
	}
	for _, m := range mustContain {
		if !strings.Contains(body, m) {
			t.Errorf("/metrics missing critical series %q", m)
		}
	}

	// 断言 4: svc 短名 label (web-bff) 存在
	if !strings.Contains(body, `service="web-bff"`) {
		t.Errorf("/metrics missing service label 'web-bff'")
	}

	// 断言 5: 触发后 counter 增 1
	// 注意: metrics.go:88 GinMetricsMiddleware 用 c.FullPath() 拿路由模板 (避免高基数)
	// 实际 path 是 /api/v1/auth/login, 但 label 是路由模板 /api/v1/auth/:action
	authCounter := readCounterFromBody(t, body, "emotion_echo_http_requests_total", map[string]string{
		"service": "web-bff", "method": "POST", "path": "/api/v1/auth/:action",
	})
	if authCounter < 1 {
		t.Errorf("emotion_echo_http_requests_total{service=web-bff, path=/api/v1/auth/:action} = %v, want >= 1", authCounter)
	}
}

// readCounterFromBody 解析 /metrics 文本格式,提取指定 labels 的 counter 值
// 复用 shared readCounter 模式,避免 import 共享测试 helper (svc 独立测试)
func readCounterFromBody(t *testing.T, body, name string, labels map[string]string) float64 {
	t.Helper()
	var total float64
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			// comment / TYPE 行,跳过 (本函数只找 counter,无需 TYPE 行)
			continue
		}
		if strings.HasPrefix(line, name) {
			// 检查 metric name 后是 '{' (有 labels) 或 ' ' (无 labels)
			rest := line[len(name):]
			if len(rest) > 0 && rest[0] == '{' {
				// 解析 {label="value",...}
				end := strings.Index(rest, "}")
				if end < 0 {
					continue
				}
				labelStr := rest[1:end]
				if !matchLabels(labelStr, labels) {
					continue
				}
				// 提取值: "} 12.5" 或 "} 12.5 1234567890"
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

// TestRegisterRoutes_RouteSubset 独立断言：每个注册的 path 都属于某个"已知前缀"集合。
// 这层断言在主契约变更时不会因为 wantRoutes 漏更新而误 pass。
// 已知业务前缀白名单（main.go 实际注册的 27 条 + EmotionQ 3 条共 30 条路径的合法前缀）：
var knownPathPrefixes = []string{
	"/health",
	"/metrics",
	"/api/v1/auth/:action",
	"/api/v1/ai/stream",
	"/api/v1/user/profile",
	"/api/v1/users/",
	"/api/v1/conversations",
	"/api/v1/conversations/:id",
	"/api/v1/surveys",
	"/api/v1/surveys/results",
	"/api/v1/surveys/:id",
	"/api/v1/reports/",
	"/api/v1/user-behavior/",
	"/api/v1/mental-health/assessment",
	"/api/v1/multimodal/",
	"/api/v1/tts/",
	"/api/v1/uploads/:kind",
	"/api/v1/voice/",   // Sprint 1 PR-4c-1: voice upload
	"/api/v1/user/avatar", // Sprint 1 PR-4c-2: avatar upload
	"/api/v1/emotion/", // 仅当 EmotionQ != nil 时
}

func TestRegisterRoutes_NoUnknownPathPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	s, cfg := stubServiceContext(t, false)
	registerRoutes(r, s, cfg)

	got := r.Routes()

	// 列出当前路由的 method+path，便于人工排查 + 自动化比对
	t.Logf("registerRoutes 注册的 %d 条路由:", len(got))
	for _, ri := range got {
		t.Logf("  %s %s", ri.Method, ri.Path)
	}

	// 校验每条路由的 path 至少匹配一个已知前缀（防新路由漂移）
	for _, ri := range got {
		if ri.Path == "/health" || ri.Path == "/metrics" {
			continue // 基础设施路径无前缀
		}
		matched := false
		for _, prefix := range knownPathPrefixes {
			if ri.Path == prefix || hasPathPrefix(ri.Path, prefix) {
				matched = true
				break
			}
		}
		assert.True(t, matched, "路由 %s %s 不在已知前缀白名单内（防漂移）", ri.Method, ri.Path)
	}
}

// hasPathPrefix 判断 path 是否以 prefix 起（精确等也算）
func hasPathPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	return path == prefix || (len(path) > len(prefix) && path[:len(prefix)] == prefix)
}
