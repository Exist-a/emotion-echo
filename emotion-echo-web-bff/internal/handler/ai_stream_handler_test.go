// Package handler — ai_stream_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T3.29 RED: ai_stream handler 测试
//
// 校验：
//   - SSE headers（Content-Type: text/event-stream + X-Accel-Buffering: no）
//   - OpenAI 兼容请求 {messages, stream} → SSE delta 流 + [DONE]
//   - 缺 messages → 400
package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestAIStreamHandler_SSEHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"我今天很开心"}]}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
}

func TestAIStreamHandler_OpenAIFormat_EmitsDeltas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"我今天心情很好"}]}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	body := w.Body.String()
	// OpenAI SSE：data: {choices:[{delta:{content}}]} + data: [DONE]
	assert.True(t, strings.Contains(body, "data: "), "应输出 SSE data 行")
	assert.True(t, strings.Contains(body, `"delta"`), "delta 字段应存在")
	assert.True(t, strings.Contains(body, "data: [DONE]"), "应以 [DONE] 结束")
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAIStreamHandler_MissingMessages_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "messages is required")
}

func TestAIStreamHandler_SadReply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"我今天很难过"}]}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	// 分块会切断多字词，具体话术由 mockEmpathyReply 单测覆盖；这里验证有 delta 输出
	assert.True(t, strings.Contains(w.Body.String(), `"delta"`), "负面情绪应得到 delta 回复")
}

func TestAIStreamHandler_DefaultReply(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandler())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"model":"m","messages":[{"role":"user","content":"随便聊聊"}]}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), `"delta"`), "默认回复应输出 delta")
}
