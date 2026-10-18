// internal/memory/llm_extractor_test.go
//
// CL-08a · A2：LLM 抽取器的解析与失败语义（httptest，不打真 LLM）。

package memory

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

const okFactsJSON = `{"facts":[
 {"subject":"用户","predicate":"职业","object":"后端工程师","text":"用户是一名后端工程师","valid_from":"2026-09-01T08:00:00Z","confidence":0.92}
]}`

func newChatServer(t *testing.T, status int, content string, assertReq func(*http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if assertReq != nil {
			assertReq(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		// 包装成 chat completion 响应
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":` + jsonString(content) + `}}]}`))
	}))
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func decodeModel(t *testing.T, r *http.Request) string {
	t.Helper()
	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Errorf("decode request: %v", err)
		return ""
	}
	return req.Model
}

func TestLLMExtractor_ParsesFacts(t *testing.T) {
	t.Parallel()
	srv := newChatServer(t, 200, okFactsJSON, nil)
	defer srv.Close()

	ex := NewLLMExtractor(LLMConfig{BaseURL: srv.URL, APIKey: "k-test", Model: "m", Timeout: 5 * time.Second})
	facts, err := ex.Extract(context.Background(), ExtractInput{
		UserID: 1, ConversationID: 7, MessageID: 100,
		Content: "我是后端工程师",
		OccurredAt: time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC),
	})
	require.NoError(t, err)
	require.Len(t, facts, 1)
	assert.Equal(t, "用户", facts[0].Subject)
	assert.Equal(t, "职业", facts[0].Predicate)
	assert.Equal(t, "后端工程师", facts[0].Object)
	assert.Equal(t, "用户是一名后端工程师", facts[0].FactText)
	assert.Equal(t, int64(100), facts[0].SourceID)
	assert.Equal(t, "message", facts[0].SourceKind)
	assert.Equal(t, int64(1), facts[0].UserID)
	assert.Equal(t, int64(7), facts[0].ConversationID)
	assert.Equal(t, "active", facts[0].Status)
	require.NotNil(t, facts[0].Confidence)
	assert.InDelta(t, 0.92, *facts[0].Confidence, 1e-6)
	assert.True(t, facts[0].ValidFrom.Equal(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)),
		"LLM 给出 valid_from 时必须采用（双时间轴）")
}

func TestLLMExtractor_SendsOpenAICompatibleRequest(t *testing.T) {
	t.Parallel()
	var gotPath, gotAuth, gotModel string
	srv := newChatServer(t, 200, `{"facts":[]}`, func(r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotModel = decodeModel(t, r)
	})
	defer srv.Close()

	ex := NewLLMExtractor(LLMConfig{BaseURL: srv.URL, APIKey: "k-test", Model: "deepseek-chat"})
	_, err := ex.Extract(context.Background(), ExtractInput{UserID: 1, MessageID: 1, Content: "hi"})
	require.NoError(t, err)

	assert.Equal(t, "/v1/chat/completions", gotPath)
	assert.Equal(t, "Bearer k-test", gotAuth)
	assert.Equal(t, "deepseek-chat", gotModel)
}

func TestLLMExtractor_HandlesCodeFence(t *testing.T) {
	t.Parallel()
	srv := newChatServer(t, 200, "```json\n"+okFactsJSON+"\n```", nil)
	defer srv.Close()

	ex := NewLLMExtractor(LLMConfig{BaseURL: srv.URL})
	facts, err := ex.Extract(context.Background(), ExtractInput{UserID: 1, MessageID: 1, Content: "x"})
	require.NoError(t, err)
	assert.Len(t, facts, 1, "模型偶尔会包 markdown 代码块，解析必须容忍")
}

func TestLLMExtractor_HTTPError(t *testing.T) {
	t.Parallel()
	srv := newChatServer(t, 500, "boom", nil)
	defer srv.Close()

	ex := NewLLMExtractor(LLMConfig{BaseURL: srv.URL})
	_, err := ex.Extract(context.Background(), ExtractInput{UserID: 1, MessageID: 1, Content: "x"})
	require.Error(t, err, "上游 5xx 必须报错（交给消费重试/DLQ）")
}

func TestLLMExtractor_NotConfigured(t *testing.T) {
	t.Parallel()
	ex := NewLLMExtractor(LLMConfig{})
	facts, err := ex.Extract(context.Background(), ExtractInput{Content: "x"})
	require.ErrorIs(t, err, ErrNotConfigured)
	assert.Nil(t, facts)
}

func TestLLMExtractor_EmptyFacts(t *testing.T) {
	t.Parallel()
	srv := newChatServer(t, 200, `{"facts":[]}`, nil)
	defer srv.Close()

	ex := NewLLMExtractor(LLMConfig{BaseURL: srv.URL})
	facts, err := ex.Extract(context.Background(), ExtractInput{UserID: 1, MessageID: 1, Content: "今天天气不错"})
	require.NoError(t, err)
	assert.Empty(t, facts, "寒暄不应产出事实")
}

func TestLLMExtractor_SkipsEmptyTriple(t *testing.T) {
	t.Parallel()
	srv := newChatServer(t, 200, `{"facts":[{"subject":"用户","predicate":"","object":"x","text":"残缺"}]}`, nil)
	defer srv.Close()

	ex := NewLLMExtractor(LLMConfig{BaseURL: srv.URL})
	facts, err := ex.Extract(context.Background(), ExtractInput{UserID: 1, MessageID: 1, Content: "x"})
	require.NoError(t, err)
	assert.Empty(t, facts, "三元组残缺宁缺毋滥")
}

func TestChatCompletionsURL(t *testing.T) {
	t.Parallel()
	cases := []struct{ in, want string }{
		{"https://api.deepseek.com", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.deepseek.com/", "https://api.deepseek.com/v1/chat/completions"},
		{"https://api.openai.com/v1", "https://api.openai.com/v1/chat/completions"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, chatCompletionsURL(c.in), "base=%q", c.in)
	}
}
