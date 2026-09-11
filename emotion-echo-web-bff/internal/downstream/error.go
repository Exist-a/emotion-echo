// Package downstream — error.go
//
// APIError 携带 HTTP 状态码，让 handler 层能精确映射错误码（400/404/502 等）。
//
// Sprint G（2026-09-11）：加 MapGRPCError helper 把 gRPC error 映射到 HTTP code。
// 解决 BFF 全局 gRPC error 映射 gap（`docs/plans/bff-grpc-error-mapping-backlog.md`）。
//
// gRPC → HTTP code 映射语义（与 HTTP handler 503/401/403/404/400/504/500 对齐）：
//   - gRPC codes.Unavailable       → HTTP 503 Service Unavailable（XTTS 不可用等）
//   - gRPC codes.Unauthenticated   → HTTP 401 Unauthorized
//   - gRPC codes.PermissionDenied  → HTTP 403 Forbidden
//   - gRPC codes.NotFound          → HTTP 404 Not Found
//   - gRPC codes.InvalidArgument   → HTTP 400 Bad Request
//   - gRPC codes.AlreadyExists     → HTTP 409 Conflict（B4 补：Sprint G 漏）
//   - gRPC codes.DeadlineExceeded  → HTTP 504 Gateway Timeout
//   - gRPC codes.Unauthenticated   → HTTP 401 Unauthorized
//   - 其他 gRPC（Internal/Unknown/Canceled）→ HTTP 500 Internal Server Error
//   - 非 gRPC error（context 超时 / net 错误）：
//     - context.DeadlineExceeded → HTTP 504
//     - 其他 → HTTP 502 Bad Gateway（向后兼容）
//   - nil → HTTP 200（handler 不应调，约定用）
package downstream

import (
	"context"
	"errors"
	"net/http"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// MapGRPCError 把 gRPC error 或普通 error 映射为 (httpStatus, code, message)。
//
// 返回三元组供 handler 用：
//
//	Fail(c, status, code, msg)
//
// 7 类 gRPC code 映射见包注释。
//
// Sprint G（2026-09-11）：解决 BFF 全局 gRPC error 映射 backlog。
// 原问题：Sprint F2 V3 tts/synthesize 走 gRPC Unavailable 返 HTTP 502，
// 应为 HTTP 503（与 HTTP handler 503 行为对齐）。
func MapGRPCError(err error) (httpStatus int, code int, message string) {
	if err == nil {
		return http.StatusOK, 0, ""
	}
	// gRPC error：用 status.FromError 提取 code
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.Unavailable:
			return http.StatusServiceUnavailable, 1, "upstream unavailable: " + st.Message()
		case codes.Unauthenticated:
			return http.StatusUnauthorized, 1, "unauthenticated: " + st.Message()
		case codes.PermissionDenied:
			return http.StatusForbidden, 1, "permission denied: " + st.Message()
		case codes.NotFound:
			return http.StatusNotFound, 1, "not found: " + st.Message()
		case codes.InvalidArgument:
			return http.StatusBadRequest, 1, "invalid argument: " + st.Message()
		case codes.AlreadyExists:
			return http.StatusConflict, 1, "conflict: " + st.Message()
		case codes.DeadlineExceeded:
			return http.StatusGatewayTimeout, 1, "timeout: " + st.Message()
		default:
			// codes.Internal / codes.Unknown / codes.Canceled / codes.Aborted
			// / codes.Unavailable 的 grpc 包装版本 / codes.DataLoss 等
			return http.StatusInternalServerError, 1, "internal: " + st.Message()
		}
	}
	// 非 gRPC error：尝试识别 context 超时
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, 1, err.Error()
	}
	// 其他普通 error（net 错误、JSON 解析失败等）
	return http.StatusBadGateway, 1, err.Error()
}
