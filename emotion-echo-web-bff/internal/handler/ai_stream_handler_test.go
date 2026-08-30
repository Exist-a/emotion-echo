// Package handler — ai_stream_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T3.29 RED: ai_stream handler SSE headers 断言
//
// 校验：
//   - Content-Type: text/event-stream
//   - X-Accel-Buffering: no
//   - Cache-Control: no-cache
//   - body 为 SSE 事件序列（analysis + done）
//   - 参数缺失 → 400
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeEmotionQuery 实现 downstream.EmotionQueryClient（返回固定结果）
type fakeEmotionQuery struct {
	err error
}

func (f *fakeEmotionQuery) ByMessage(_ context.Context, messageID int64) (*emotionquery.Emotion, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &emotionquery.Emotion{
		Id: 1, MessageId: messageID, ConversationId: 10,
		PrimaryEmotion: "happy", SentimentScore: 0.7, Confidence: 0.9, Model: "keyword-stub-v1",
	}, nil
}

func (f *fakeEmotionQuery) ByConversation(_ context.Context, conversationID int64, limit int) ([]*emotionquery.Emotion, int32, error) {
	return nil, 0, nil
}

func TestAIStreamHandler_SSEHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler(&fakeEmotionQuery{}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"messageId":42}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// SSE headers 断言（文档 §八风险表）
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))

	// body 是 SSE 事件序列
	body := w.Body.String()
	assert.True(t, strings.Contains(body, "event: analysis\n"), "应含 analysis 事件")
	assert.True(t, strings.Contains(body, "event: done\n"), "应含 done 事件")
	assert.True(t, strings.Contains(body, `"primaryEmotion":"happy"`), "analysis 数据应含情绪")
	assert.True(t, strings.Contains(body, `"messageId":42`), "analysis 数据应含 messageId")
}

func TestAIStreamHandler_MissingMessageID_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler(&fakeEmotionQuery{}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "messageId is required")
}

func TestAIStreamHandler_UpstreamError_Returns502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler(&fakeEmotionQuery{err: context.DeadlineExceeded}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"messageId":42}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "upstream emotion query failed")
}

var _ downstream.EmotionQueryClient = (*fakeEmotionQuery)(nil) // 编译期断言
