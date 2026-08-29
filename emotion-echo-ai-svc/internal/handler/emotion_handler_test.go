// Package handler — emotion_handler_test.go
//
// Sibling test for emotion_handler.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover emotion_handler.go (61 LOC) test
// surface. Coverage targets:
//
//   - happy path: GET /api/v1/emotion/message/:messageId with valid id → 200
//   - invalid id format → 400 (handler-level validation, not the logic layer)
//   - id <= 0 → 400 (the handler explicitly rejects zero / negative)
//   - logic-layer not-found → 500 (current behavior, pinned; tightening
//     to 404 would be a deliberate change)
//   - happy path: GET /api/v1/emotion/conversation/:conversationId → 200
//   - invalid conversationId → 400
//   - GET /health → 200 (HealthHandler)
//
// Uses real gin + InMemoryEmotionRepo; no HTTP mocks needed.
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func init() { gin.SetMode(gin.TestMode) }

// newEmotionHandlerTestSvc returns a minimal *svc.ServiceContext with
// only the EmotionRepo wired (logic + handler tests don't need
// AI client init).
func newEmotionHandlerTestSvc(t *testing.T) *svc.ServiceContext {
	t.Helper()
	return &svc.ServiceContext{
		EmotionRepo: repository.NewInMemoryEmotionRepo(),
	}
}

// newEmotionHandlerRouter builds a gin engine with the emotion + health
// routes registered. Uses gin.New() WITHOUT HandleMethodNotAllowed
// because emotion_handler_test predates the 405-dispatch convention
// introduced in chat-svc; if you add method-not-allowed coverage
// here, switch to chat-svc's newTestRouter helper.
func newEmotionHandlerRouter(svcCtx *svc.ServiceContext) *gin.Engine {
	r := gin.New()
	r.GET("/api/v1/emotion/message/:messageId", GetEmotionByMessageHandler(svcCtx))
	r.GET("/api/v1/emotion/conversation/:conversationId", ListEmotionByConversationHandler(svcCtx))
	r.GET("/health", HealthHandler(svcCtx))
	return r
}

// TestEmotionHandler_GetByMessage_HappyPath covers the canonical
// read path: existing analysis → 200 + JSON body with the analysis.
func TestEmotionHandler_GetByMessage_HappyPath(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	require.NoError(t, svcCtx.EmotionRepo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID:      100,
		UserID:         7,
		ConversationID: 50,
		PrimaryEmotion: "happy",
		SentimentScore: 0.6,
		Confidence:     0.85,
		Model:          "keyword-v1",
	}))

	r := newEmotionHandlerRouter(svcCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/100", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "happy path should be 200, got %d body=%s",
		rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "happy")
}

// TestEmotionHandler_GetByMessage_InvalidID_Returns400 covers the
// path-param validation: non-numeric id → 400.
func TestEmotionHandler_GetByMessage_InvalidID_Returns400(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	r := newEmotionHandlerRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/notanumber", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"non-numeric id should yield 400, got %d", rec.Code)
	require.Contains(t, rec.Body.String(), "invalid messageId")
}

// TestEmotionHandler_GetByMessage_ZeroID_Returns400 covers the
// explicit zero-id rejection: handler's strconv.ParseInt accepts "0"
// but the implementation rejects id <= 0. This pins the validation
// as part of the HTTP contract, not just the logic layer.
func TestEmotionHandler_GetByMessage_ZeroID_Returns400(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	r := newEmotionHandlerRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/0", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code,
		"id=0 should yield 400, got %d", rec.Code)
}

// TestEmotionHandler_GetByMessage_NotFound_Returns500 pins the
// current behavior: logic returns "not found" error → handler maps
// to 500. Tightening this to 404 would be a deliberate change to
// emotion_handler.go (the other handlers do the same; cross-service
// consistency).
func TestEmotionHandler_GetByMessage_NotFound_Returns500(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	r := newEmotionHandlerRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/999", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusInternalServerError, rec.Code,
		"not-found currently returns 500; got %d", rec.Code)
}

// TestEmotionHandler_ListByConversation_HappyPath covers the list
// endpoint with existing analyses.
func TestEmotionHandler_ListByConversation_HappyPath(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	require.NoError(t, svcCtx.EmotionRepo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 1, ConversationID: 50, PrimaryEmotion: "happy",
	}))
	require.NoError(t, svcCtx.EmotionRepo.Create(context.Background(), &model.EmotionAnalysis{
		MessageID: 2, ConversationID: 50, PrimaryEmotion: "anxious",
	}))

	r := newEmotionHandlerRouter(svcCtx)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/conversation/50", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "happy list should be 200, got %d body=%s",
		rec.Code, rec.Body.String())
	require.Contains(t, rec.Body.String(), "happy")
	require.Contains(t, rec.Body.String(), "anxious")
}

// TestEmotionHandler_ListByConversation_InvalidID_Returns400 covers
// path-param validation on the list endpoint.
func TestEmotionHandler_ListByConversation_InvalidID_Returns400(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	r := newEmotionHandlerRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/conversation/abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Contains(t, rec.Body.String(), "invalid conversationId")
}

// TestEmotionHandler_Health_OK covers the HealthHandler exposed by
// emotion_handler.go (the same path as chat-svc /health).
func TestEmotionHandler_Health_OK(t *testing.T) {
	t.Parallel()

	svcCtx := newEmotionHandlerTestSvc(t)
	r := newEmotionHandlerRouter(svcCtx)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "/health should be 200, got %d body=%s",
		rec.Code, rec.Body.String())
	// HealthResp.Status field is "ok" per HealthLogic convention.
	require.True(t, strings.Contains(rec.Body.String(), `"status"`))
}