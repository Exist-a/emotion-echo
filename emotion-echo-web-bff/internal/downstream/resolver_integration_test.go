package downstream

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
)

// stubResolver 是 web-bff discovery.Resolver 的轻量替身（避免循环依赖）。
type stubResolver struct {
	host string
	port int
	err  error
}

func (s *stubResolver) Resolve(context.Context, string) (string, int, error) {
	return s.host, s.port, s.err
}

// TestNewAIClient_UsesResolverWhenBaseURLEmpty：BaseURL 为空时走 Resolver。
func TestNewAIClient_UsesResolverWhenBaseURLEmpty(t *testing.T) {
	c := NewAIClient(AIClientOptions{
		BaseURL: "",
		Resolver: &stubResolver{host: "emotion-echo-ai-svc", port: 8891},
	})
	require.NotNil(t, c)
	ai, ok := c.(*aiHTTPClient)
	require.True(t, ok)
	assert.Equal(t, "http://emotion-echo-ai-svc:8891", ai.baseURL)
}

// TestNewAIClient_BaseURLWinsOverResolver：env 注入优先。
func TestNewAIClient_BaseURLWinsOverResolver(t *testing.T) {
	c := NewAIClient(AIClientOptions{
		BaseURL:  "http://override:1234",
		Resolver: &stubResolver{host: "should-not-use", port: 9999},
	})
	require.NotNil(t, c)
	ai := c.(*aiHTTPClient)
	assert.Equal(t, "http://override:1234", ai.baseURL)
}

// TestNewAIClient_NilWhenNoBaseURLAndResolverErrors：兜底失败 → nil。
func TestNewAIClient_NilWhenNoBaseURLAndResolverErrors(t *testing.T) {
	c := NewAIClient(AIClientOptions{
		BaseURL:  "",
		Resolver: &stubResolver{err: errors.New("nacos down")},
	})
	assert.Nil(t, c)
}

// TestNewChatClient_UsesResolverWhenBaseURLEmpty。
func TestNewChatClient_UsesResolverWhenBaseURLEmpty(t *testing.T) {
	c := NewChatClient(ChatClientOptions{
		BaseURL:  "",
		Resolver: &stubResolver{host: "emotion-echo-chat-svc", port: 8890},
	})
	require.NotNil(t, c)
	ch := c.(*chatHTTPClient)
	assert.Equal(t, "http://emotion-echo-chat-svc:8890", ch.baseURL)
}

// TestNewAnalyticsClient_UsesResolverWhenBaseURLEmpty。
func TestNewAnalyticsClient_UsesResolverWhenBaseURLEmpty(t *testing.T) {
	c := NewAnalyticsClient(AnalyticsClientOptions{
		BaseURL:  "",
		Resolver: &stubResolver{host: "emotion-echo-analytics-svc", port: 8893},
	})
	require.NotNil(t, c)
	a := c.(*analyticsHTTPClient)
	assert.Equal(t, "http://emotion-echo-analytics-svc:8893", a.baseURL)
}

// 编译期断言：shareddiscovery 引用未漂移。
var _ = shareddiscovery.ServiceAI
var _ sync.Mutex
var _ = time.Second