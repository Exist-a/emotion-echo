// Package handler — voice_handler_test.go
//
// Sprint 1 PR-4c-1: voice_handler 单元测试
//
// 行为契约：
//   - POST /api/v1/voice/upload 接 multipart (conversationId + file)
//   - 调 ai-svc MultiModalAnalyze(kind=audio, file)
//   - 返回 {messageId, transcript, emotion, audioUrl}
//   - ai-svc 不可达时返 503 (而非 500)
//   - 缺 file 字段时返 400

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAIClient 模拟 AIClient
type fakeAIClient struct {
	resp *downstream.MultiModalAnalyzeResp
	err  error
	// 记录调用时的 kind 用于断言
	gotKind string
}

func (f *fakeAIClient) MultiModalAnalyze(ctx context.Context, req downstream.MultiModalAnalyzeReq) (*downstream.MultiModalAnalyzeResp, error) {
	f.gotKind = req.Kind
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeAIClient) SynthesizeSpeech(ctx context.Context, req downstream.SynthesizeSpeechReq) (*downstream.SynthesizeSpeechResp, error) {
	return nil, nil
}

func (f *fakeAIClient) AIHealth(ctx context.Context) (*downstream.AIHealthResp, error) {
	return &downstream.AIHealthResp{}, nil
}

// newVoiceRouter 最小路由（仅 voice_handler）
func newVoiceRouter(ai downstream.AIClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&VoiceHandler{ai: ai}).Register(r)
	return r
}

func TestVoiceHandler_Upload_Success(t *testing.T) {
	fake := &fakeAIClient{
		resp: &downstream.MultiModalAnalyzeResp{
			Kind:       "audio",
			Emotion:    "neutral",
			Confidence: 0.92,
			Transcript: "你好世界",
		},
	}
	r := newVoiceRouter(fake)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	require.NoError(t, mw.WriteField("conversationId", "42"))
	fw, err := mw.CreateFormFile("file", "recording.webm")
	require.NoError(t, err)
	_, _ = io.WriteString(fw, "fake webm bytes")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "audio", fake.gotKind, "应传 kind=audio")
	assert.Equal(t, "你好世界", got["transcript"])
	assert.Equal(t, "neutral", got["emotion"])
	assert.NotEmpty(t, got["messageId"], "messageId 必须生成")
}

func TestVoiceHandler_Upload_AIServiceError_Returns503(t *testing.T) {
	// 用真 connection error 触发 isConnectionErr 分支
	fake := &fakeAIClient{err: errors.New("dial tcp 127.0.0.1:8891: connect: connection refused")}
	r := newVoiceRouter(fake)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("conversationId", "1")
	fw, _ := mw.CreateFormFile("file", "x.webm")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// ai-svc 不可达 → 503 (而非 500)，让前端能区分
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestVoiceHandler_Upload_MissingFile_Returns400(t *testing.T) {
	fake := &fakeAIClient{}
	r := newVoiceRouter(fake)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("conversationId", "1")
	// 故意缺 file 字段
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	// ai 不应被调用
	assert.Empty(t, fake.gotKind)
}

func TestVoiceHandler_Register_PathContract(t *testing.T) {
	// 防御性：保证 register 注册的是 POST /api/v1/voice/upload
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&VoiceHandler{ai: &fakeAIClient{}}).Register(r)
	assert.Equal(t, 1, len(r.Routes()), "VoiceHandler 应注册 1 条路由")
	for _, ri := range r.Routes() {
		assert.True(t, strings.HasPrefix(ri.Path, "/api/v1/voice/"), "path 应在 /api/v1/voice/ 下")
		assert.Equal(t, http.MethodPost, ri.Method)
	}
}