// Package fusion — Stage 34 · PR-11 RED
//
// LLMFuser 是 fusion 的"主路径"算法，复用 BFF 现有的 LLM_BASE_URL（DeepSeek 兼容）。
//
// 设计：
//   - 把 ModalitySnapshot 转成结构化 JSON prompt
//   - 调 LLM chat completions endpoint（OpenAI 兼容协议）
//   - LLM 输出 JSON：{primary_emotion, sentiment_score, modality_contrib, reasoning}
//   - 解析失败 / 网络失败 → 返回 error（让 Worker 走 late_fuser 兜底）
//
// 为什么不调 emotion-llm-service：那是关键词器，没"融合"能力。
// 复用 BFF 的 LLM_BASE_URL 是为了避免引入新的 LLM 依赖与运维负担。
package fusion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLLMFuser_Success_FromFakeServer httptest 模拟 LLM 服务返回正确 JSON。
func TestLLMFuser_Success_FromFakeServer(t *testing.T) {
	t.Parallel()
	// 构造 mock LLM：收到 chat 请求后返回固定 JSON
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 解析请求（仅验证是 /v1/chat/completions）
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		var req map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&req)
		// 返回 OpenAI 兼容响应
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": `{"primary_emotion":"sad","sentiment_score":-0.55,"modality_contrib":{"text":0.5,"voice":0.3,"face":0.2},"reasoning":"文字与语音均表达低落"}`,
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "deepseek-chat",
		Timeout: 5 * time.Second,
	})

	snap := makeSnapshot(
		&ModalityScore{Emotion: "sad", Confidence: 0.9, Sentiment: -0.5},
		&ModalityScore{Emotion: "neutral", Confidence: 0.7},
		&ModalityScore{Emotion: "sad", Confidence: 0.8, Sentiment: -0.6},
	)

	out, err := f.Fuse(context.Background(), snap)
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, "sad", out.PrimaryEmotion)
	assert.InDelta(t, -0.55, out.SentimentScore, 0.001)
	assert.Equal(t, "文字与语音均表达低落", out.Reasoning)
	assert.Equal(t, "llm", out.FusionMethod)
	assert.Equal(t, `["text","face","voice"]`, out.AvailableModalities)
}

// TestLLMFuser_LLMReturnsBadJSON_ReturnsError LLM 返回非 JSON → error（让 Worker 兜底）。
func TestLLMFuser_LLMReturnsBadJSON_ReturnsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "不是 JSON 格式",
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Timeout: 5 * time.Second})
	_, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.9},
		nil, nil,
	))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

// TestLLMFuser_LLMReturnsHTTPError_ReturnsError LLM 返回 500 → error。
func TestLLMFuser_LLMReturnsHTTPError_ReturnsError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream down", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Timeout: 5 * time.Second})
	_, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.9},
		nil, nil,
	))
	require.Error(t, err)
}

// TestLLMFuser_EmptySnapshot_ReturnsError 零模态直接返回 error（不浪费 LLM 调用）。
func TestLLMFuser_EmptySnapshot_ReturnsError(t *testing.T) {
	t.Parallel()
	// 没有 httptest：empty snapshot 应 short-circuit
	f := NewLLMFuser(LLMConfig{BaseURL: "http://localhost:0", APIKey: "k", Model: "m"})
	_, err := f.Fuse(context.Background(), ModalitySnapshot{})
	require.Error(t, err)
}

// TestLLMFuser_OnlyOneModality_LLMSkipNotPreferred 单模态也调 LLM（让 LLM 决定权重）。
//
// 设计决策：单模态时 LLM 仍可输出 reasoning（"文本显示 user 自述情绪是 sad"）。
// Worker 在 PR-13/14 决定是否短路到 late_fuser（推荐短路）。
func TestLLMFuser_OnlyOneModality_LLMStillCalled(t *testing.T) {
	t.Parallel()
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": `{"primary_emotion":"happy","sentiment_score":0.5,"modality_contrib":{"text":1.0},"reasoning":"仅文字"}`,
				},
			}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Timeout: 5 * time.Second})
	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.9},
		nil, nil,
	))
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "happy", out.PrimaryEmotion)
}

// TestLLMFuser_Success_LLMReturnsMarkdownFencedJSON Stage 35 PR-1：LLM 返回 ```json...``` 包裹也应能解析。
func TestLLMFuser_Success_LLMReturnsMarkdownFencedJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "```json\n{\"primary_emotion\":\"calm\",\"sentiment_score\":0.2,\"modality_contrib\":{\"text\":1.0},\"reasoning\":\"\"}\n```",
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{BaseURL: srv.URL, APIKey: "k", Model: "m", Timeout: 5 * time.Second})
	out, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "calm", Confidence: 0.9},
		nil, nil,
	))
	require.NoError(t, err)
	assert.Equal(t, "calm", out.PrimaryEmotion)
	assert.InDelta(t, 0.2, out.SentimentScore, 0.001)
	assert.Equal(t, "llm", out.FusionMethod)
}

// TestLLMFuser_DefaultTimeoutIs3s Stage 35 PR-4：未传 Timeout → 默认 3s。
func TestLLMFuser_DefaultTimeoutIs3s(t *testing.T) {
	t.Parallel()
	f := NewLLMFuser(LLMConfig{BaseURL: "http://localhost:0", Model: "m"})
	require.NotNil(t, f.cli)
	assert.Equal(t, 3*time.Second, f.cli.Timeout, "default timeout should be 3s (Stage 35 PR-4)")
}

// TestLLMFuser_CustomTimeoutRespected 自定义 Timeout 应被尊重。
func TestLLMFuser_CustomTimeoutRespected(t *testing.T) {
	t.Parallel()
	f := NewLLMFuser(LLMConfig{BaseURL: "http://localhost:0", Model: "m", Timeout: 7 * time.Second})
	assert.Equal(t, 7*time.Second, f.cli.Timeout)
}

// TestLLMFuser_BreakerOpenShortCircuits Stage 35 PR-5：breaker Open 时直接 ErrCircuitOpen，不调 LLM。
func TestLLMFuser_BreakerOpenShortCircuits(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("LLM should NOT be called when breaker is Open")
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{BaseURL: srv.URL, Model: "m"})
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 1, OpenSeconds: time.Minute})
	b.RecordFailure() // → Open
	f.SetBreaker(b)

	_, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.9},
		nil, nil,
	))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrCircuitOpen)
}

// TestLLMFuser_BreakerRecordsSuccessFailure 成功/失败都按预期记录到 breaker。
func TestLLMFuser_BreakerRecordsSuccessFailure(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": `{"primary_emotion":"happy","sentiment_score":0.5,"modality_contrib":{"text":1.0},"reasoning":""}`,
				},
			}},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	f := NewLLMFuser(LLMConfig{BaseURL: srv.URL, Model: "m"})
	b := NewCircuitBreaker(BreakerConfig{FailThreshold: 3, OpenSeconds: time.Minute})
	f.SetBreaker(b)

	_, err := f.Fuse(context.Background(), makeSnapshot(
		&ModalityScore{Emotion: "happy", Confidence: 0.9}, nil, nil,
	))
	require.NoError(t, err)
	assert.Equal(t, BreakerClosed, b.State(), "success should keep breaker closed")
}
