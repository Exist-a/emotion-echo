// Package downstream — ai_test.go
//
// Stage 30 / stage-30-web-bff.md T2.6-8 RED: AIClient 契约测试
//
// 用 httptest.NewServer mock ai-svc 端点，断言：
//   - 请求路径 / 方法 / Content-Type / Authorization 透传
//   - 响应 JSON 反序列化正确
//   - 下游 4xx/5xx → 返回 error（readError）
//
// 跑：go test ./internal/downstream/...
package downstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAIHandler 构造 mock ai-svc HTTP server，返回 baseURL + 断言 helper
func mockAIHandler(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestAIClient_MultiModalAnalyze_Success(t *testing.T) {
	baseURL := mockAIHandler(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/multimodal/analyze", r.URL.Path)
		// multipart content-type
		assert.True(t, strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data"),
			"Content-Type 应为 multipart/form-data")
		// JWT 透传
		assert.Equal(t, "Bearer test-jwt", r.Header.Get("Authorization"))
		// 解析 multipart 验证字段
		require.NoError(t, r.ParseMultipartForm(1<<20))
		assert.Equal(t, "image", r.FormValue("kind"))
		assert.Equal(t, "demo text", r.FormValue("text"))
		_, fh, err := r.FormFile("file")
		require.NoError(t, err)
		assert.Equal(t, "photo.jpg", fh.Filename)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MultiModalAnalyzeResp{
			Kind:           "image",
			Emotion:        "happy",
			Confidence:     0.92,
			SentimentScore: 0.7,
			Model:          "keyword-stub-v1",
		})
	})

	c := NewAIClient(AIClientOptions{BaseURL: baseURL, TimeoutMs: 1000})
	ctx := WithJWT(context.Background(), "test-jwt")
	resp, err := c.MultiModalAnalyze(ctx, MultiModalAnalyzeReq{
		Kind:     "image",
		File:     strings.NewReader("fake-image-bytes"),
		FileName: "photo.jpg",
		Text:     "demo text",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "happy", resp.Emotion)
	assert.Equal(t, 0.92, resp.Confidence)
	assert.Equal(t, "keyword-stub-v1", resp.Model)
}

func TestAIClient_SynthesizeSpeech_Success(t *testing.T) {
	baseURL := mockAIHandler(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/tts/synthesize", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer t1", r.Header.Get("Authorization"))

		var req SynthesizeSpeechReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "你好", req.Text)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(SynthesizeSpeechResp{
			Audio:      "base64wav",
			SampleRate: 24000,
			Mime:       "audio/wav",
			Bytes:      1024,
			Text:       "你好",
			Language:   "zh-cn",
		})
	})

	c := NewAIClient(AIClientOptions{BaseURL: baseURL, TimeoutMs: 1000})
	resp, err := c.SynthesizeSpeech(WithJWT(context.Background(), "t1"), SynthesizeSpeechReq{Text: "你好"})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "base64wav", resp.Audio)
	assert.Equal(t, 24000, resp.SampleRate)
	assert.Equal(t, "audio/wav", resp.Mime)
}

func TestAIClient_AIHealth_Success(t *testing.T) {
	baseURL := mockAIHandler(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/ai/health", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(AIHealthResp{
			Time:       123456,
			AllHealthy: false,
			XTTS: &AIHealthEntry{Enabled: true, Healthy: false, Error: "model not loaded"},
		})
	})

	c := NewAIClient(AIClientOptions{BaseURL: baseURL, TimeoutMs: 1000})
	resp, err := c.AIHealth(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.False(t, resp.AllHealthy)
	require.NotNil(t, resp.XTTS)
	assert.False(t, resp.XTTS.Healthy)
	assert.Equal(t, "model not loaded", resp.XTTS.Error)
}

func TestAIClient_Upstream500_ReturnsError(t *testing.T) {
	baseURL := mockAIHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "analyzer exploded"})
	})

	c := NewAIClient(AIClientOptions{BaseURL: baseURL, TimeoutMs: 1000})
	_, err := c.AIHealth(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "analyzer exploded", "readError 应提取下游 error 消息")
}

func TestAIClient_Timeout_ReturnsError(t *testing.T) {
	baseURL := mockAIHandler(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	c := NewAIClient(AIClientOptions{BaseURL: baseURL, TimeoutMs: 20})
	_, err := c.AIHealth(context.Background())
	require.Error(t, err, "超时下游应返回 error")
}
