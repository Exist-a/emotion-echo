package discovery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestServiceNames_FullLongForm：所有 svc 长名以 "emotion-echo-" 前缀开头。
// 这是 PR-0 约束：APISIX nacos-discovery 按 serviceName 拉实例，短/长不一致会 404。
func TestServiceNames_FullLongForm(t *testing.T) {
	assert.Equal(t, "emotion-echo-user-svc", ServiceUser)
	assert.Equal(t, "emotion-echo-chat-svc", ServiceChat)
	assert.Equal(t, "emotion-echo-analytics-svc", ServiceAnalytics)
	assert.Equal(t, "emotion-echo-assessment-svc", ServiceAssessment)
	assert.Equal(t, "emotion-echo-ai-svc", ServiceAI)
	assert.Equal(t, "emotion-echo-web-bff", ServiceWebBFF)
	assert.Equal(t, "emotion-llm-service", ServiceLLM)
}

// TestServiceNames_AllDistinct：注册名不能重（否则 Nacos 上不同 svc 合并）。
func TestServiceNames_AllDistinct(t *testing.T) {
	all := []string{
		ServiceUser, ServiceChat, ServiceAnalytics, ServiceAssessment,
		ServiceAI, ServiceWebBFF, ServiceLLM,
	}
	seen := map[string]bool{}
	for _, s := range all {
		assert.False(t, seen[s], "duplicate service name: %s", s)
		seen[s] = true
	}
}