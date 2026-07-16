// Package aiclient 提供 3 个 AI 模型服务（FER/SenseVoice/XTTS）的 HTTP 客户端。
//
// 设计原则：
//   - 每个 client 都是一个独立的结构体，便于单独 mock
//   - 失败 / 不可用时返回明确错误，不 panic
//   - 调用方（analyzer / consumer）应自己处理降级
//
// 所有 client 都是可选的：URL 为空时 New* 返回 nil，调用前必须检查
package aiclient

import (
	"errors"
	"time"
)

// ErrNotConfigured 当 BaseURL 为空（AI 服务未启用）时返回
var ErrNotConfigured = errors.New("ai client not configured: BaseURL is empty")

// common 默认 HTTP timeout
const defaultHTTPTimeout = 10 * time.Second

// ErrUpstream 模型服务返回非 2xx 时返回，body 包含响应内容用于诊断
type ErrUpstream struct {
	StatusCode int
	Body       string
}

func (e *ErrUpstream) Error() string {
	return "upstream error: status=" + itoa(e.StatusCode)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
