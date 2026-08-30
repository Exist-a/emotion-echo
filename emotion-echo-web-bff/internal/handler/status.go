// Package handler — status.go
//
// statusFor 把下游错误映射为 HTTP 状态码（downstream.APIError 精确映射）。
package handler

import (
	"emotion-echo-web-bff/internal/downstream"
)

// statusFor 返回下游错误对应的 HTTP 状态码；无 APIError 时默认 502。
func statusFor(err error) int {
	return downstream.StatusCodeOf(err)
}
