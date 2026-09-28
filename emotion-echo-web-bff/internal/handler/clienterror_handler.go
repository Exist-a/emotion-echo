// E2E-F-148：前端错误上报端点。
//
// 为什么不引第三方 SDK（Sentry 之类）：那会新增外部数据出口、需要用户隐私
// 评审（与端侧化的隐私定位冲突，见 ADR D-26 隐私定位），而且本项目刚把
// 日志链路打通（E2E-21），浏览器错误直接进同一条 Loki 是最短路径 ——
// 还能顺带拿到 trace_id，把"浏览器报的错"和"后端同一时刻的日志"对上。
//
// 设计约束（每条都有对应用例，见 clienterror_handler_test.go）：
//  1. **总是 200**：上报端点自己失败会让前端陷入"错误上报也失败"的循环，
//     并污染网络面板，干扰真正的错误排查。
//  2. msg 截断：前端可能把整段 HTML/大对象塞进来，不截断会打爆日志。
//  3. 无 trace_id 时不写空字段：空的 trace_id 在 Loki 里查不到东西，比没有更误导。
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/emotion-echo/shared/pkg/logging"
)

// maxClientErrField 单个字段最大保留长度，超出部分截断并标记。
const maxClientErrField = 2048

type clientErrorPayload struct {
	Kind  string `json:"kind"`  // error | unhandledrejection
	Msg   string `json:"msg"`
	Stack string `json:"stack"`
	URL   string `json:"url"`
	Line  int    `json:"line"`
	Col   int    `json:"col"`
}

func trunc(s string) string {
	if len(s) <= maxClientErrField {
		return s
	}
	return s[:maxClientErrField] + "...[truncated]"
}

// ClientErrorHandler 接收浏览器侧未捕获异常，写入结构化日志。
func ClientErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 兜底：无论后续发生什么，都必须 200（约束 1）
		defer func() {
			if r := recover(); r != nil {
				logging.PrintfContext(c.Request.Context(),
					"[client-error] handler panic: %v", r)
				c.JSON(http.StatusOK, gin.H{"ok": true})
			}
		}()

		var p clientErrorPayload
		if err := c.ShouldBindJSON(&p); err != nil {
			// 解析失败也记一笔 —— 往往是前端版本不一致导致字段变了，
			// 这种"上报格式漂移"本身就是要查的问题。
			logging.PrintfContext(c.Request.Context(),
				"[client-error] payload unparsable, err=%v", err)
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}

		kind := p.Kind
		if kind == "" {
			kind = "error"
		}

		logging.PrintfContext(c.Request.Context(),
			"[client-error] kind=%s msg=%q url=%s line=%d col=%d stack=%s",
			kind, trunc(p.Msg), trunc(p.URL), p.Line, p.Col, trunc(p.Stack))

		// 回传 trace_id：前端可展示成"错误编号"，用户报障时直接报这个 ID
		tid := strings.TrimSpace(c.GetHeader("X-Trace-Id"))
		if tid == "" {
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "trace_id": tid})
	}
}
