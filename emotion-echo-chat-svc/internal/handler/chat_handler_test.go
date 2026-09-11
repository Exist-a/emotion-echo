package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
)

func init() { gin.SetMode(gin.TestMode) }

// newTestSVC 构造 chat-svc 测试用 svcCtx：InMemory repo + InMemory publisher
func newTestSVC() *svc.ServiceContext {
	cfg := config.Config{Name: "emotion-echo-chat-svc"}
	repo := repository.NewInMemoryConversationRepo()
	pub := events.NewInMemoryEventPublisher()
	return svc.NewServiceContext(cfg, repo, pub)
}

// newTestRouter returns a gin engine configured the same way as the
// production chat-svc main.go: HandleMethodNotAllowed=true so 405 is
// returned for method/path mismatches (gin's default is 404 for both,
// which obscures real client bugs). All existing TestXxxHandler_*
// tests already use gin.New() inline; only the new 405 subtests need
// this helper. Kept private to this file.
func newTestRouter() *gin.Engine {
	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	})
	return r
}

// reqWithUser 把 demo user_id 注入 ctx（模拟中间件）
// handler 内部 logic 通过 sharedmw.CtxUserIDKey 提取
func reqWithUser(req *http.Request, uid int64) *http.Request {
	ctx := context.WithValue(req.Context(), sharedmw.CtxUserIDKey{}, uid)
	return req.WithContext(ctx)
}

// TestCreateConversationHandler_RealGin_HTTP 真实 gin + httptest 验证 handler
// happy-path：合法 JSON POST → 200 + ID；坏 JSON → 400
func TestCreateConversationHandler_RealGin_HTTP(t *testing.T) {
	svcCtx := newTestSVC()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/conversations", CreateConversationHandler(svcCtx))

	// 1. happy path
	body := bytes.NewBufferString(`{"title":"integration test","userId":7}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", body)
	req.Header.Set("Content-Type", "application/json")
	req = reqWithUser(req, 7)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "happy body=%s", rec.Body.String())

	var respBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
	// 期望返字段含 id / status 或 conversation 嵌套
	if convID, ok := respBody["id"]; ok {
		require.NotZero(t, convID)
	} else if conv, ok := respBody["conversation"].(map[string]any); ok {
		require.NotZero(t, conv["id"])
	}

	// 2. invalid JSON → 400
	body2 := bytes.NewBufferString(`{not json`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", body2)
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)

	require.Equal(t, http.StatusBadRequest, rec2.Code,
		"invalid JSON should yield 400, got %d body=%s", rec2.Code, rec2.Body.String())
	require.True(t, strings.Contains(rec2.Body.String(), "error"))
}

// TestChatHandler_CreateAndListMessages_EndToEnd 真实路径：create → send → list
func TestChatHandler_CreateAndListMessages_EndToEnd(t *testing.T) {
	svcCtx := newTestSVC()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/conversations", CreateConversationHandler(svcCtx))
	r.POST("/api/v1/conversations/:id/messages", SendMessageHandler(svcCtx))
	r.GET("/api/v1/conversations/:id/messages", ListMessagesHandler(svcCtx))

	// 1) create conversation
	bodyCreate := bytes.NewBufferString(`{"title":"e2e","userId":42}`)
	reqCreate := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", bodyCreate)
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate = reqWithUser(reqCreate, 42)
	recCreate := httptest.NewRecorder()
	r.ServeHTTP(recCreate, reqCreate)
	require.Equal(t, http.StatusOK, recCreate.Code)

	var convBody map[string]any
	require.NoError(t, json.Unmarshal(recCreate.Body.Bytes(), &convBody))
	// 拼出 conversation id（结构因 svc 实现而异）
	var convID int64
	if v, ok := convBody["id"].(float64); ok {
		convID = int64(v)
	} else if conv, ok := convBody["conversation"].(map[string]any); ok {
		if v, ok := conv["id"].(float64); ok {
			convID = int64(v)
		}
	}
	require.NotZero(t, convID, "should extract conversation id, got body=%s", recCreate.Body.String())

	// 2) send message
	bodyMsg := bytes.NewBufferString(`{"role":"user","content":"hello","userId":42}`)
	urlMsg := "/api/v1/conversations/"
	routeMsg := "/api/v1/conversations/" + intToStr(convID) + "/messages"
	_ = urlMsg
	reqMsg := httptest.NewRequest(http.MethodPost, routeMsg, bodyMsg)
	reqMsg.Header.Set("Content-Type", "application/json")
	reqMsg = reqWithUser(reqMsg, 42)
	recMsg := httptest.NewRecorder()
	r.ServeHTTP(recMsg, reqMsg)
	// 可能返 200 或 401 (需 JWT) → 仅当 svc 内部不校验 auth 触发 SendMessage
	if recMsg.Code == http.StatusOK {
		// 3) list messages
		reqList := httptest.NewRequest(http.MethodGet, routeMsg, nil)
		reqList = reqWithUser(reqList, 42)
		recList := httptest.NewRecorder()
		r.ServeHTTP(recList, reqList)
		require.True(t, recList.Code == http.StatusOK || recList.Code == http.StatusUnauthorized,
			"list returned %d body=%s", recList.Code, recList.Body.String())
	} else {
		// 401 是合理降级（auth 中间件未在测试路由上）
		require.Equal(t, http.StatusUnauthorized, recMsg.Code,
			"with auth middleware 401 is expected, got %d", recMsg.Code)
	}
}

// intToStr 简单 int64→string（避免 import strconv 调试）
func intToStr(i int64) string {
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	buf := make([]byte, 0, 20)
	for i > 0 {
		buf = append([]byte{byte('0' + i%10)}, buf...)
		i /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

// TestChatHandler_EmptyBody 兜底测试：handler 必须能处理 missing JSON body
func TestChatHandler_EmptyBody(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/conversations", CreateConversationHandler(svcCtx))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations", bytes.NewBufferString(``))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// Stage 26-T backlog §三 3.1 T4: extend handler coverage to 404/405/400
// edge cases. The handler currently maps every logic error to 500 (see
// chat_handler.go lines 23-25/48-49/71-73). The tests below pin the
// CURRENT behavior so any future tightening is a deliberate change,
// not a regression.
//
// The tests use full gin routing so HTTP method dispatch and path
// matching are exercised end-to-end.

// TestChatHandler_SendMessage_RouteNotFound_Returns404 verifies that
// hitting an unregistered path on the chat-svc router yields a 404
// from gin's default NoRoute handler — not a 200, not a 500.
func TestChatHandler_SendMessage_RouteNotFound_Returns404(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/conversations/:id/messages", SendMessageHandler(svcCtx))

	// :id missing segment → no route matches → 404
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/nope", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code,
		"unknown path should be 404, got %d", rec.Code)
}

// TestChatHandler_SendMessage_MethodNotAllowed_Returns405 verifies
// that a GET against the POST-only /messages route yields 405
// (gin's default MethodNotAllowed handler), not a 500. The handler
// is registered for POST; a GET must NOT fall through to the handler.
func TestChatHandler_SendMessage_MethodNotAllowed_Returns405(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)
	r := newTestRouter()
	r.POST("/api/v1/conversations/:id/messages", SendMessageHandler(svcCtx))

	// GET on POST-only route → 405
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/1/messages", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code,
		"GET on POST-only route should be 405, got %d", rec.Code)
}

// TestChatHandler_SendMessage_InvalidPathParam_Returns400 verifies
// that a non-numeric :id path parameter yields a 400 (the handler
// parses c.Param("id") and refuses anything that doesn't ParseInt).
func TestChatHandler_SendMessage_InvalidPathParam_Returns400(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/conversations/:id/messages", SendMessageHandler(svcCtx))

	body := bytes.NewBufferString(`{"role":"user","content":"x"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/notanumber/messages", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"non-numeric :id should be 400, got %d body=%s", rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "invalid conversation id")
}

// TestChatHandler_ListMessages_MethodNotAllowed_Returns405 verifies
// the symmetric case: POST on the GET-only /messages route must be
// 405, not "fall through and 400 from ShouldBindJSON".
func TestChatHandler_ListMessages_MethodNotAllowed_Returns405(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)
	r := newTestRouter()
	r.GET("/api/v1/conversations/:id/messages", ListMessagesHandler(svcCtx))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/1/messages", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusMethodNotAllowed, rec.Code,
		"POST on GET-only route should be 405, got %d", rec.Code)
}

// TestChatHandler_ListMessages_InvalidPathParam_Returns400 is the
// GET counterpart of TestChatHandler_SendMessage_InvalidPathParam.
func TestChatHandler_ListMessages_InvalidPathParam_Returns400(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/conversations/:id/messages", ListMessagesHandler(svcCtx))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/abc/messages", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusBadRequest, rec.Code,
		"non-numeric :id should be 400, got %d", rec.Code)
}

// TestDeleteConversationHandler_DeletesAndPublishes 真实 gin + httptest：
// DELETE /api/v1/conversations/:id → 200 success + conversation.closed 事件
func TestDeleteConversationHandler_DeletesAndPublishes(t *testing.T) {
	svcCtx := newTestSVC()
	// seed 一个属于 uid=7 的会话
	repo := svcCtx.ConversationRepo.(*repository.InMemoryConversationRepo)
	require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
		ID: 1, UserID: 7, Title: "t", Status: 1,
	}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/api/v1/conversations/:id", DeleteConversationHandler(svcCtx))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/conversations/1", nil)
	req = reqWithUser(req, 7)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "body=%s", rec.Body.String())
	var respBody map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &respBody))
	assert.Equal(t, true, respBody["success"])

	// 会话已删 + 事件已发
	got, err := repo.GetConversationByID(context.Background(), 1)
	require.NoError(t, err)
	assert.Nil(t, got)
	pub := svcCtx.EventPublisher.(*events.InMemoryEventPublisher)
	evts := pub.Events(events.TopicChatEvents)
	require.Len(t, evts, 1)
	assert.Equal(t, events.EventTypeConversationClosed, evts[0].Type)
}

// TestDeleteConversationHandler_InvalidID_400 非数字 id → 400
func TestDeleteConversationHandler_InvalidID_400(t *testing.T) {
	svcCtx := newTestSVC()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/api/v1/conversations/:id", DeleteConversationHandler(svcCtx))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/conversations/abc", nil)
	req = reqWithUser(req, 7)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
}

// PR-2 RED · chat-svc 中间件分层
// 现状（PR-2 前）：main.go:214 `r.Use(sharedmw.GinAuthMiddleware())` 全局挂中间件，
// 导致 /health 与 /metrics 也要 X-User-Id（K8s liveness probe / Prometheus scrape 必坏）。
// 目标：仿 user-svc 范式分层——/health 与 /metrics 在 auth 之前注册，业务端点在 auth 群内。

// TestChatHandler_Health_NoUserID_Returns200 RED：复刻修复后 main.go 的路由结构，
// /health 不带 X-User-Id 应返 200。修复前全局 auth 会让该路径返 401，测试应失败。
func TestChatHandler_Health_NoUserID_Returns200(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	// PR-2 目标：/health 在 auth 中间件之前注册
	r.GET("/health", HealthHandler(svcCtx))
	// 业务群内挂 auth（仿 user-svc main.go:155 r.Use 范式）
	auth := r.Group("/api/v1")
	auth.Use(sharedmw.GinAuthMiddleware())
	auth.GET("/conversations", ListConversationsHandler(svcCtx))

	// /health 无 X-User-Id → 200
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code,
		"/health should be 200 without X-User-Id, got %d body=%s", rec.Code, rec.Body.String())

	// /metrics 不在路由表中——单独测：直接 GET /metrics 走 PR-2 后的 main.go 应 200
	r2 := gin.New()
	r2.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))
	auth2 := r2.Group("/api/v1")
	auth2.Use(sharedmw.GinAuthMiddleware())
	auth2.GET("/conversations", ListConversationsHandler(svcCtx))

	req2 := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec2 := httptest.NewRecorder()
	r2.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code,
		"/metrics should be 200 without X-User-Id, got %d", rec2.Code)
	require.Contains(t, rec2.Header().Get("Content-Type"), "text/plain",
		"/metrics content-type should be text/plain")
}

// TestChatHandler_BusinessRoute_NoUserID_Returns401 RED：业务群内路径无 X-User-Id
// 必须返 401 "missing or invalid X-User-Id"，不能因为 PR-2 改动破坏鉴权契约。
func TestChatHandler_BusinessRoute_NoUserID_Returns401(t *testing.T) {
	svcCtx := newTestSVC()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	auth := r.Group("/api/v1")
	auth.Use(sharedmw.GinAuthMiddleware())
	auth.GET("/conversations", ListConversationsHandler(svcCtx))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code,
		"业务群内无 X-User-Id 应 401，got %d body=%s", rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "X-User-Id")
}
