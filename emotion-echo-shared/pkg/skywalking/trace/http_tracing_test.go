package trace

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/SkyAPM/go2sky"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nopReporter 是最小 go2sky.Reporter 实现（不真上报）。
//
// 必须提供 reporter：无 reporter 时 go2sky 的 tracer initFlag=0 ⇒
// CreateExitSpanWithContext 直接返回 NoopSpan 并**跳过 sw8 注入**
// （go2sky trace.go："Ignored, there is no need to inject SW8"）——测试将永远看不到头。
type nopReporter struct{}

func (nopReporter) Boot(string, string, []go2sky.AgentConfigChangeWatcher) {}
func (nopReporter) Send([]go2sky.ReportedSpan)                             {}
func (nopReporter) Close()                                                 {}

// newTestTracer 造一个"会真建 span"的 tracer（带 reporter）并注册为全局，测试结束清空。
func newTestTracer(t *testing.T) *go2sky.Tracer {
	t.Helper()
	tr, err := go2sky.NewTracer("http-tracing-test", go2sky.WithReporter(nopReporter{}))
	require.NoError(t, err)
	require.NotNil(t, tr)
	SetTracer(tr)
	t.Cleanup(func() { SetTracer(nil) })
	return tr
}

// entryCtx 造一个"带父 span"的 ctx（模拟经过入站中间件的请求上下文）。
func entryCtx(t *testing.T, tr *go2sky.Tracer) context.Context {
	t.Helper()
	span, ctx, err := tr.CreateEntrySpan(context.Background(), "test-entry",
		func(string) (string, error) { return "", nil })
	require.NoError(t, err)
	require.NotNil(t, span)
	t.Cleanup(span.End)
	return ctx
}

func TestTracingRoundTripper_InjectsSw8_WhenParented(t *testing.T) {
	tr := newTestTracer(t)

	var seenSw8 string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenSw8 = r.Header.Get("sw8")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(entryCtx(t, tr), http.MethodGet, srv.URL+"/probe", nil)
	require.NoError(t, err)

	client := &http.Client{Transport: NewTracingRoundTripper(nil)}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.NotEmpty(t, seenSw8, "有父 span 时出站请求必须携带 sw8 头（trace 跨 HTTP 传播）")
}

// T-08 / 约束③：无父 span 时不得硬造根 span，故也不注入 sw8。
func TestTracingRoundTripper_NoSw8_WhenNoParentSpan(t *testing.T) {
	newTestTracer(t)

	var seenSw8 = "unset"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenSw8 = r.Header.Get("sw8")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/probe", nil)
	require.NoError(t, err)

	client := &http.Client{Transport: NewTracingRoundTripper(nil)}
	resp, err := client.Do(req)
	require.NoError(t, err, "无父 span 不应影响请求发出（仅不埋点）")
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, seenSw8, "无父 span 时不得建 span / 注入 sw8（约束③ T-08）")
}

func TestTracingRoundTripper_NilTracer_Passthrough(t *testing.T) {
	SetTracer(nil)

	var hit bool
	var seenSw8 string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		seenSw8 = r.Header.Get("sw8")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewTracingRoundTripper(nil)}
	resp, err := client.Do(mustReq(t, srv.URL))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, hit, "无 tracer 时请求仍须照常发出")
	assert.Empty(t, seenSw8)
}

func TestTracingRoundTripper_ServerError_StillReturnsResponse(t *testing.T) {
	newTestTracer(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewTracingRoundTripper(nil)}
	resp, err := client.Do(mustReq(t, srv.URL))
	require.NoError(t, err, "HTTP 5xx 不是 transport error")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestTracingRoundTripper_NilNext_DefaultsToDefaultTransport(t *testing.T) {
	rt := NewTracingRoundTripper(nil)
	assert.Equal(t, http.DefaultTransport, rt.next)
}

func TestNewTracedClient_WiresTracingTransport(t *testing.T) {
	c := NewTracedClient(3 * time.Second)
	assert.Equal(t, 3*time.Second, c.Timeout)
	_, ok := c.Transport.(*TracingRoundTripper)
	assert.True(t, ok, "NewTracedClient 的 Transport 必须是 *TracingRoundTripper")
}

func mustReq(t *testing.T, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	return req
}
