package downstream

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emotion-echo/shared/pkg/skywalking/trace"
)

// TestNewChatHTTPClient_UsesTracedTransport 钉住 CL-02 收口补齐的 **HTTP 出站埋点**：
// chat 的 HTTP client 必须带 `TracingRoundTripper`（exit span + sw8 注入），
// 否则该下游调用的 trace 链在此断掉，且与同目录的 ai.go / analytics.go 等不一致。
//
// 背景（为什么这条钉子由 CL-02 立）：`downstream/chat.go` 属 **C 轨独占列**，
// CL-01（可观测闭环）收口时明确豁免并移交本阶段（见 cl-01 report §收口自检 第 5 项）。
// 本钉子把"移交项已补齐"变成机器可校验的事实，防止后续被无声退回。
//
// 断言口径：只看**构造出的 client 的 Transport 类型**，不发真实请求
// （BaseURL 指向不可达端口，构造期不会拨号）。
func TestNewChatHTTPClient_UsesTracedTransport(t *testing.T) {
	c := NewChatClient(ChatClientOptions{
		Transport: "http",
		BaseURL:   "http://127.0.0.1:9",
		TimeoutMs: 1000,
	})
	require.NotNil(t, c, "Transport=http 且给了 BaseURL 时应返回 client")

	hc, ok := c.(*chatHTTPClient)
	require.True(t, ok, "Transport=http 应返回 *chatHTTPClient，实际 %T", c)
	require.NotNil(t, hc.http, "http client 不应为 nil")

	_, isTraced := hc.http.Transport.(*trace.TracingRoundTripper)
	assert.True(t, isTraced,
		"chat HTTP client 必须用 trace.NewTracedClient（Transport 应为 *trace.TracingRoundTripper），实际 %T",
		hc.http.Transport)
}
