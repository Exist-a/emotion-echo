package skywalking

import (
	"net/http"
	"strconv"
	"time"

	"github.com/SkyAPM/go2sky"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

// TracingRoundTripper 包装底层 http.RoundTripper：为每次出站 HTTP 调用建一个
// ExitSpan，并把 sw8 头注入请求，使 trace 跨 HTTP 边界传播到下游服务。
//
// CL-01 G3：历史上各 svc 的裸 `http.Client` 无任何埋点（无 exit span、无 sw8），
// 导致 ai-svc→FER/SenseVoice/XTTS、BFF→各 svc 的 HTTP 降级路径在链路上"断头"。
type TracingRoundTripper struct {
	next http.RoundTripper
}

// NewTracingRoundTripper 用给定底层 transport 构造；next 为 nil 时用 http.DefaultTransport。
func NewTracingRoundTripper(next http.RoundTripper) *TracingRoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &TracingRoundTripper{next: next}
}

// NewTracedClient 返回一个所有出站请求都带 ExitSpan + sw8 的 *http.Client。
//
// 用途：替换各 svc 里裸的 `&http.Client{Timeout: ...}`（见 CL-01 覆盖清单）。
func NewTracedClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: NewTracingRoundTripper(nil),
	}
}

// RoundTrip 实现 http.RoundTripper。
//
// 边界（CL-01 约束③ / T-08）：**仅当请求 ctx 已带父 span 时才建 ExitSpan** ——
// go2sky 默认采样率=1.0，无父 span 时会新建"根 trace"（硬造根 span），为无 ctx 的
// 调用刷出大量孤立 trace。无父 span / 无 tracer / 参数不全时直接透传，请求照常发出。
func (t *TracingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	tgr := Tracer()
	if tgr == nil || req == nil || req.URL == nil {
		return t.next.RoundTrip(req)
	}
	if go2sky.ActiveSpan(req.Context()) == nil {
		return t.next.RoundTrip(req)
	}

	peer := req.URL.Host
	op := "http." + req.Method
	span, ctx2, err := tgr.CreateExitSpanWithContext(req.Context(), op+" "+req.URL.Path, peer,
		func(key, value string) error {
			// sw8 注入：让下游能从 header 重建父 trace
			req.Header.Set(key, value)
			return nil
		})
	if err != nil || span == nil {
		return t.next.RoundTrip(req)
	}
	span.SetSpanLayer(agentv3.SpanLayer_Http)

	resp, rerr := t.next.RoundTrip(req.WithContext(ctx2))
	if rerr != nil {
		span.Error(time.Now(), rerr.Error())
	} else if resp != nil {
		span.Tag(go2sky.TagStatusCode, strconv.Itoa(resp.StatusCode))
	}
	span.End()
	return resp, rerr
}

var _ http.RoundTripper = (*TracingRoundTripper)(nil)
