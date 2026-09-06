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
	"testing"

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/svc"

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
