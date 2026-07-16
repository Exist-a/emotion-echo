package analyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPAnalyzer_CallLLMService_HappyPath(t *testing.T) {
	t.Parallel()

	// 构造一个 mock HTTP 服务，模拟 emotion-llm-service
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "/analyze", r.URL.Path)

		var req struct {
			Text string `json:"text"`
		}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotEmpty(t, req.Text)

		// 返回模拟响应
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"primaryEmotion": "happy",
			"sentimentScore": 0.75,
			"confidence":     0.85,
			"model":          "llm-v1",
		})
	}))
	defer server.Close()

	a := NewHTTPAnalyzer(server.URL)
	got, err := a.Analyze(context.Background(), "今天很开心")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	assert.Equal(t, 0.75, got.SentimentScore)
	assert.Equal(t, "llm-v1", got.Model)
}

func TestHTTPAnalyzer_LLMServiceDown_ReturnsError(t *testing.T) {
	t.Parallel()

	// 故意指向不存在的地址
	a := NewHTTPAnalyzer("http://localhost:1") // port 1 没人监听
	_, err := a.Analyze(context.Background(), "test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "llm")
}

func TestHTTPAnalyzer_LLMReturns500_ReturnsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"detail":"model crashed"}`))
	}))
	defer server.Close()

	a := NewHTTPAnalyzer(server.URL)
	_, err := a.Analyze(context.Background(), "test")
	require.Error(t, err)
}

// TestChainedAnalyzer_LLMFirst_FallbackKeyword 验证 fallback 链
func TestChainedAnalyzer_LLMFirst_FallbackKeyword(t *testing.T) {
	t.Parallel()

	// LLM 不可达 → 应回退到 keyword analyzer
	a := NewChainedAnalyzer(
		NewHTTPAnalyzer("http://localhost:1"), // unreachable
		NewKeywordAnalyzer(),
	)

	got, err := a.Analyze(context.Background(), "今天很开心")
	require.NoError(t, err, "fallback should succeed")
	require.NotNil(t, got)
	assert.Equal(t, "happy", got.PrimaryEmotion)
	// 模型应是 keyword（fallback 用了）
	assert.Contains(t, got.Model, "keyword")
}

// TestChainedAnalyzer_LLMFirst_Success 用 LLM 路径
func TestChainedAnalyzer_LLMFirst_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"primaryEmotion": "anxious",
			"sentimentScore": -0.4,
			"confidence":     0.7,
			"model":          "llm-v1",
		})
	}))
	defer server.Close()

	a := NewChainedAnalyzer(
		NewHTTPAnalyzer(server.URL),
		NewKeywordAnalyzer(),
	)

	got, err := a.Analyze(context.Background(), "我最近压力很大")
	require.NoError(t, err)
	assert.Equal(t, "anxious", got.PrimaryEmotion)
	assert.Equal(t, "llm-v1", got.Model, "should use LLM when primary works")
}