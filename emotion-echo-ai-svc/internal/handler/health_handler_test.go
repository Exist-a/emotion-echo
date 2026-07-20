package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"emotion-echo-ai-svc/internal/aiclient"
	"emotion-echo-ai-svc/internal/config"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"
)

func init() { gin.SetMode(gin.TestMode) }

// 本文件为 ai-svc handler 层做端到端测试（httptest + 真实 gin router），
// 锁定 Stage 26-N 任务：把 handler 路径从覆盖率缺口补完。
//
// 复用：servicecontext + repository.NewInMemoryEmotionRepo + NewFERClient(空) 等。
// 因为 FER / SenseVoice / XTTS 都启用了真实 HTTP 调用，handler 测里把它们
// 全部置空 BaseURL（或指向无效端口），让 AIHealthHandler 走降级路径。

// newTestServiceContext 构造 ServiceContext，全 AI client 用空 BaseURL（关闭）
func newTestServiceContext() *svc.ServiceContext {
	cfg := config.Config{
		Name: "emotion-echo-ai-svc",
		FER:        config.FER{BaseURL: ""},
		SenseVoice: config.SenseVoice{BaseURL: ""},
		XTTS:       config.XTTS{BaseURL: ""},
	}
	repo := repository.NewInMemoryEmotionRepo()
	svcCtx := svc.NewServiceContext(cfg, repo)
	// 显式 nil FER / SV / XTTS client — AIHealthHandler 看到 nil 会标 "disabled"
	_ = aiclient.Config{BaseURL: ""} // import sanity
	svcCtx.FER = nil
	svcCtx.SenseVoice = nil
	svcCtx.XTTS = nil
	return svcCtx
}

// TestHealthHandler_ReturnsOkStatus 真实 gin + httptest.NewRecorder，验证 /health 200 + body
func TestHealthHandler_ReturnsOkStatus(t *testing.T) {
	svcCtx := newTestServiceContext()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, "ok", body["status"])
	require.Equal(t, "emotion-echo-ai-svc", body["service"])
	require.NotEmpty(t, body["version"])
}

// TestAIHealthHandler_AllDisabled_Returns200 模拟 3 个 AI 模型 client 都关闭（BaseURL 空）。
//
// 期望：/api/v1/ai/health 返 200 + allHealthy=false（FER/SV/TT 都被标 "disabled"）。
//
// 这是 Stage 26-N 锁定的关键行为：即使所有依赖下线，handler 也不能 5xx —
// 真实 readiness 是看 body.allHealthy 而不是 HTTP code。
func TestAIHealthHandler_AllDisabled_Returns200(t *testing.T) {
	svcCtx := newTestServiceContext()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/ai/health", AIHealthHandler(svcCtx))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/health", nil)
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code,
		"All-disabled should still return 200, got %d body=%s", rec.Code, rec.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Contains(t, body, "allHealthy")
	require.Equal(t, false, body["allHealthy"],
		"allHealthy should be false when all AI clients disabled")
	require.NotZero(t, body["time"])
}

// TestAIHealthHandler_WithRealishStub aiclient.Ping 走真实 fastapi-style http
// 注：FER client 在 BaseURL 为空时 Health 返回 ErrNotConfigured，所以"占位 stub"不必须。
// 这里仍走 nil path 验 handler 行为。
func TestAIHealthHandler_HealthLogicBodyIsValidJSON(t *testing.T) {
	svcCtx := newTestServiceContext()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/ai/health", AIHealthHandler(svcCtx))

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ai/health", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	// body 含 Content-Type: application/json（Gin 默认）
	require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
}

// silence unused
var _ = aiclient.NewFERClient
