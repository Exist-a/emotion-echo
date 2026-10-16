// Package handler — emotion_query_handler_userid_test.go
//
// CL-07 前端半 · 阻塞缺陷修复（RED）：BFF `/api/v1/emotion/*` 三个端点
// **没有把 APISIX 注入的 X-User-Id 透传给下游 gRPC** —— 它们直接传
// `c.Request.Context()`，而其余 10 个 handler 都用 `session.WithRequestAuth(c)`。
//
// 后果（端到端实测，2026-10-16）：同一枚 token 下
//   GET /api/v1/conversations/705/messages → 200
//   GET /api/v1/emotion/message/951/fused  → 502 `missing x-user-id metadata`
// （ai-svc 拦截器要求 x-user-id metadata）⇒ **整族 emotion 端点在网关后恒不可用**，
// 依赖它的"情绪回显"永远取不到数据。这是 E2E-F-125 同型缺陷（unit 全绿但端到端恒 Unauthenticated）。
//
// 本测试钉死契约：handler 必须把请求里的 X-User-Id 带进下游 ctx。

package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// capturingEmotionClient 记录下游调用收到的 ctx 里的 user id。
type capturingEmotionClient struct {
	byMessageUID      int64
	byMessageOK       bool
	byConversationUID int64
	byConversationOK  bool
	byFusedUID        int64
	byFusedOK         bool
}

func (c *capturingEmotionClient) ByMessage(ctx context.Context, _ int64) (*emotionquery.Emotion, error) {
	c.byMessageUID, c.byMessageOK = downstream.UserIDFromContext(ctx)
	return &emotionquery.Emotion{}, nil
}

func (c *capturingEmotionClient) ByConversation(ctx context.Context, _ int64, _ int) ([]*emotionquery.Emotion, int32, error) {
	c.byConversationUID, c.byConversationOK = downstream.UserIDFromContext(ctx)
	return nil, 0, nil
}

func (c *capturingEmotionClient) ByFusedMessage(ctx context.Context, _ int64) (*emotionquery.FusedEmotion, error) {
	c.byFusedUID, c.byFusedOK = downstream.UserIDFromContext(ctx)
	return &emotionquery.FusedEmotion{PrimaryEmotion: "sad"}, nil
}

// TestEmotionQueryHandler_PropagatesUserIDToDownstream 钉死三个端点都透传 X-User-Id。
func TestEmotionQueryHandler_PropagatesUserIDToDownstream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &capturingEmotionClient{}
	h := NewEmotionQueryHandler(c)
	r := gin.New()
	h.Register(r)

	do := func(path string) {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(session.XUserIDHeader, "42") // APISIX 注入的 user id
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}
	do("/api/v1/emotion/message/1")
	do("/api/v1/emotion/message/1/fused")
	do("/api/v1/emotion/conversation/1")

	assert.True(t, c.byMessageOK, "ByMessage 下游 ctx 必须带 user id（否则 ai-svc 返 Unauthenticated）")
	assert.Equal(t, int64(42), c.byMessageUID)
	assert.True(t, c.byFusedOK, "ByFusedMessage 下游 ctx 必须带 user id（CL-07 情绪回显取数依赖它）")
	assert.Equal(t, int64(42), c.byFusedUID)
	assert.True(t, c.byConversationOK, "ByConversation 下游 ctx 必须带 user id")
	assert.Equal(t, int64(42), c.byConversationUID)
}

// TestEmotionQueryHandler_NoHeader_DownstreamHasNoUserID 无 X-User-Id（如 /health 场景）不得伪造身份。
func TestEmotionQueryHandler_NoHeader_DownstreamHasNoUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := &capturingEmotionClient{}
	h := NewEmotionQueryHandler(c)
	r := gin.New()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/1/fused", nil)
	r.ServeHTTP(httptest.NewRecorder(), req)

	assert.False(t, c.byFusedOK, "无 X-User-Id 时不得注入身份（不污染下游）")
}
