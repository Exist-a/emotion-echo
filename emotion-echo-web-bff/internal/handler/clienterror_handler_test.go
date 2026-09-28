// E2E-F-148：前端错误上报端点的契约测试。
//
// 背景：本项目 58 处 console.* 无任何上报手段，浏览器端异常在服务端日志里
// **完全不可见**。E2E-21 打通日志链路后，最省事且不引第三方 SDK 的做法是：
// 浏览器把错误 POST 到 BFF，BFF 写进与业务日志同一条结构化日志 → promtail → Loki。
//
// 三条硬约束（每条都有对应用例）：
//  1. 必须**总是 200**：上报端点自己报错会让前端陷入"错误上报也失败"的循环，
//     且会污染网络面板、干扰真正的业务错误排查。
//  2. **绝不外传用户输入原文**以外的敏感信息；msg 需截断，防止超大 payload 打爆日志。
//  3. 必须带 trace_id —— 前端请求经 APISIX 时会拿到 X-Trace-Id，
//     有了它才能把"浏览器报的错"和"后端同一时刻的日志"对上。
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/emotion-echo/shared/pkg/logging"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

func init() { gin.SetMode(gin.TestMode) }

func newClientErrorRouter() *gin.Engine {
	r := gin.New()
	// 必须挂上生产同一条中间件：X-Trace-Id → ctx 的注入是在
	// GinSkywalkingMiddleware 里做的（tracer 传 nil 时它只做 ctx 注入，
	// 不打 span）。不挂它就测不到真实链路。
	r.Use(sharedmw.GinSkywalkingMiddleware(nil))
	r.POST("/api/v1/client-error", ClientErrorHandler())
	return r
}

func postErr(t *testing.T, body string, traceID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client-error",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if traceID != "" {
		req.Header.Set("X-Trace-Id", traceID)
	}
	w := httptest.NewRecorder()
	newClientErrorRouter().ServeHTTP(w, req)
	return w
}

func TestClientErrorHandler_AlwaysReturns200(t *testing.T) {
	ginCtxInvoked := false
	_ = ginCtxInvoked
	for _, body := range []string{
		`{"kind":"error","msg":"boom","stack":"a\nb","url":"/chat"}`,
		`{}`,
		`not-json-at-all`,
		``,
	} {
		w := postErr(t, body, "")
		if w.Code != http.StatusOK {
			t.Fatalf("body=%q 期望 200，实得 %d", body, w.Code)
		}
	}
}

func TestClientErrorHandler_LogsWithTraceID(t *testing.T) {
	var buf bytes.Buffer
	logging.InitTo(&buf)
	t.Cleanup(func() { logging.Init() })

	postErr(t, `{"kind":"error","msg":"render failed","url":"/chat"}`, "trace-fe-1")

	got := buf.String()
	if !strings.Contains(got, "render failed") {
		t.Fatalf("日志未含前端错误消息: %s", got)
	}
	if !strings.Contains(got, `"trace_id":"trace-fe-1"`) {
		t.Fatalf("日志缺 trace_id，无法与后端日志对时间线: %s", got)
	}
}

func TestClientErrorHandler_TruncatesOversizedMessage(t *testing.T) {
	var buf bytes.Buffer
	logging.InitTo(&buf)
	t.Cleanup(func() { logging.Init() })

	huge := strings.Repeat("x", 64*1024)
	body, _ := json.Marshal(map[string]any{"kind": "error", "msg": huge})
	postErr(t, string(body), "")

	got := buf.String()
	if len(got) > 8*1024 {
		t.Fatalf("超大 msg 未被截断，日志行 %d 字节", len(got))
	}
	if !strings.Contains(got, "truncated") && !strings.Contains(got, "...") {
		t.Fatalf("截断后应有可识别标记: %s", got[:min(len(got), 400)])
	}
}

func TestClientErrorHandler_NoTraceIDStillLogs(t *testing.T) {
	var buf bytes.Buffer
	logging.InitTo(&buf)
	t.Cleanup(func() { logging.Init() })

	// 无 X-Trace-Id（直连 BFF、未过网关）也必须记录，只是没有 trace_id 字段
	postErr(t, `{"kind":"error","msg":"no trace header"}`, "")

	got := buf.String()
	if !strings.Contains(got, "no trace header") {
		t.Fatalf("无 trace_id 时也必须记录: %s", got)
	}
	if strings.Contains(got, `"trace_id":""`) {
		t.Fatalf("不应写入空 trace_id 字段（查不到东西，比没有更误导）: %s", got)
	}
}

func TestClientErrorHandler_ExposesTraceIDToBrowser(t *testing.T) {
	// 后端把 trace_id 回给前端，前端才能在 UI 上显示"可复制的错误编号"
	w := postErr(t, `{"kind":"error","msg":"x"}`, "trace-fe-2")
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("响应不是 JSON: %s", w.Body.String())
	}
	if resp["trace_id"] != "trace-fe-2" {
		t.Fatalf("响应未回传 trace_id: %s", w.Body.String())
	}
}

var _ = context.Background
