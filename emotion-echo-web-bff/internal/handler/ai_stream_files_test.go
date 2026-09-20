// Package handler — ai_stream_files_test.go
//
// Stage 89 PR-3 RED：ai/stream 必须从会话最近消息收集 file 类型消息（最新 ≤2 条），
// 把 MinIO 公开 URL 重写为 llm-service 可达的内部端点后放进 LLMStreamRequest.Files。
// 这是"文件+提问一起发 + 会话内持续引用"的 BFF 侧核心逻辑。
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFileLister 实现 fileMessageLister
type fakeFileLister struct {
	msgs []downstream.MessageView
	err  error
}

func (f *fakeFileLister) ListMessages(_ context.Context, conversationID int64, limit int) ([]downstream.MessageView, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.msgs, nil
}

func minioTestConfig() config.Config {
	cfg := config.Config{}
	cfg.MinIO.PublicBaseURL = "http://localhost:9000"
	cfg.MinIO.Endpoint = "emotion-echo-minio:9000"
	cfg.MinIO.Bucket = "avatars"
	return cfg
}

func TestAIStreamHandler_CollectsFileMessagesIntoFiles(t *testing.T) {
	lister := &fakeFileLister{msgs: []downstream.MessageView{
		{ID: 3, Role: "user", Content: "http://localhost:9000/avatars/uploads/u3-33333333.txt", ContentType: "file", FileName: "notes.txt"},
		{ID: 2, Role: "user", Content: "刚才那个文件说了什么", ContentType: "text"},
		{ID: 1, Role: "user", Content: "http://localhost:9000/avatars/uploads/u1-11111111.pdf", ContentType: "file", FileName: "report.pdf"},
	}}
	streamer := &fakeLLMStreamer{deltas: []string{"好的"}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(minioTestConfig(), AIStreamDeps{LLM: streamer, Files: lister}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		strings.NewReader(`{"message":"刚才那个文件说了什么","conversationId":"7"}`))
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Len(t, streamer.gotFiles, 2, "最新 ≤2 条 file 消息必须注入")
	assert.Equal(t, "notes.txt", streamer.gotFiles[0].Name)
	assert.Equal(t, "http://emotion-echo-minio:9000/avatars/uploads/u3-33333333.txt", streamer.gotFiles[0].URL,
		"公开 URL 必须重写为内部 MinIO 端点")
	assert.Equal(t, "report.pdf", streamer.gotFiles[1].Name)
}

func TestAIStreamHandler_NoFileMessages_EmptyFiles(t *testing.T) {
	lister := &fakeFileLister{msgs: []downstream.MessageView{
		{ID: 1, Role: "user", Content: "纯文字", ContentType: "text"},
	}}
	streamer := &fakeLLMStreamer{deltas: []string{"好的"}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(minioTestConfig(), AIStreamDeps{LLM: streamer, Files: lister}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		strings.NewReader(`{"message":"hi","conversationId":"7"}`))
	router.ServeHTTP(w, req)

	assert.Empty(t, streamer.gotFiles)
}

// lister 失败不阻断对话（文件上下文是增强，不是依赖）
func TestAIStreamHandler_ListerError_StillStreams(t *testing.T) {
	lister := &fakeFileLister{err: assert.AnError}
	streamer := &fakeLLMStreamer{deltas: []string{"好的"}}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(minioTestConfig(), AIStreamDeps{LLM: streamer, Files: lister}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		strings.NewReader(`{"message":"hi","conversationId":"7"}`))
	router.ServeHTTP(w, req)

	assert.Contains(t, w.Body.String(), "好的")
}

// URL 重写纯函数：公开前缀 → 内部端点；不匹配时原样返回（白名单兜底拒绝）
func TestFileSourceURL_Rewrite(t *testing.T) {
	cfg := minioTestConfig()
	got := fileSourceURL("http://localhost:9000/avatars/uploads/u1-11111111.pdf", cfg)
	assert.Equal(t, "http://emotion-echo-minio:9000/avatars/uploads/u1-11111111.pdf", got)
	assert.Equal(t, "https://example.com/other", fileSourceURL("https://example.com/other", cfg))
}
