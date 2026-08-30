// Package downstream — error.go
//
// APIError 携带 HTTP 状态码，让 handler 层能精确映射错误码（400/404/502 等）。
package downstream

import (
	"errors"
	"net/http"
)

// APIError 是下游调用失败的错误（含状态码 + 消息）
type APIError struct {
	StatusCode int
	Msg        string
}

func (e *APIError) Error() string { return e.Msg }

// StatusCodeOf 从 error 链中提取 APIError 状态码；无则返回 502（上游失败默认）
func StatusCodeOf(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode
	}
	return http.StatusBadGateway
}
