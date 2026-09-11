// Package downstream — error_sprint_g_test.go
//
// Sprint G RED：BFF 全局 gRPC error → HTTP code 映射 helper 单测
//
// 行为契约：
//   - MapGRPCError(err) → (httpStatus, code, message)
//   - 7 类 gRPC code 各自映射到对应 HTTP code（与 HTTP handler 503/401/403/404/400/504/500 对齐）
//   - 非 gRPC error（context 超时 / net 错误）→ 504/502
//   - nil → 200（OK），handler 不应调
//
// 解决决策 18 #35 之外的 BFF 全局错误映射 backlog
// （`docs/plans/bff-grpc-error-mapping-backlog.md`）
//
// Sprint G（2026-09-11）：tts/synthesize 走 gRPC Unavailable 应返 503（不是 502）。
package downstream

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestMapGRPCError_Unavailable 断言 gRPC Unavailable → 503（不是 502）。
//
// 这是 Sprint F2 V3 暴露的 BFF bug 修复目标：
// ai-svc gRPC server 调 XTTS 失败时返 codes.Unavailable，BFF handler
// 应映射为 HTTP 503（语义对齐 HTTP 503 "Service Unavailable"）。
func TestMapGRPCError_Unavailable(t *testing.T) {
	err := status.Error(codes.Unavailable, "call XTTS: dial tcp: lookup emotion-echo-xtts")
	status, code, msg := MapGRPCError(err)
	assert.Equal(t, http.StatusServiceUnavailable, status, "gRPC Unavailable 应映射 HTTP 503")
	assert.Equal(t, 1, code)
	assert.Contains(t, msg, "upstream unavailable")  // helper 前缀
	assert.Contains(t, msg, "call XTTS")  // 原始 gRPC message
}

// TestMapGRPCError_Unauthenticated → 401
func TestMapGRPCError_Unauthenticated(t *testing.T) {
	err := status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	status, code, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusUnauthorized, status, "gRPC Unauthenticated 应映射 HTTP 401")
	assert.Equal(t, 1, code)
}

// TestMapGRPCError_PermissionDenied → 403
func TestMapGRPCError_PermissionDenied(t *testing.T) {
	err := status.Error(codes.PermissionDenied, "forbidden: conversation does not belong to user")
	status, code, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusForbidden, status, "gRPC PermissionDenied 应映射 HTTP 403")
	assert.Equal(t, 1, code)
}

// TestMapGRPCError_NotFound → 404
func TestMapGRPCError_NotFound(t *testing.T) {
	err := status.Error(codes.NotFound, "chat: resource not found")
	status, code, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusNotFound, status, "gRPC NotFound 应映射 HTTP 404")
	assert.Equal(t, 1, code)
}

// TestMapGRPCError_InvalidArgument → 400
func TestMapGRPCError_InvalidArgument(t *testing.T) {
	err := status.Error(codes.InvalidArgument, "message_id is required and must be > 0")
	status, code, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusBadRequest, status, "gRPC InvalidArgument 应映射 HTTP 400")
	assert.Equal(t, 1, code)
}

// TestMapGRPCError_DeadlineExceeded → 504
func TestMapGRPCError_DeadlineExceeded(t *testing.T) {
	err := status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	status, code, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusGatewayTimeout, status, "gRPC DeadlineExceeded 应映射 HTTP 504")
	assert.Equal(t, 1, code)
}

// TestMapGRPCError_Internal → 500
func TestMapGRPCError_Internal(t *testing.T) {
	err := status.Error(codes.Internal, "create neutral placeholder: db error")
	status, code, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusInternalServerError, status, "gRPC Internal 应映射 HTTP 500")
	assert.Equal(t, 1, code)
}

// TestMapGRPCError_ContextDeadlineExceeded 非 gRPC error：context 超时 → 504
func TestMapGRPCError_ContextDeadlineExceeded(t *testing.T) {
	err := context.DeadlineExceeded
	status, _, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusGatewayTimeout, status, "context.DeadlineExceeded 应映射 HTTP 504")
}

// TestMapGRPCError_OtherNonGRPC 非 gRPC error：其他 → 502（向后兼容）
func TestMapGRPCError_OtherNonGRPC(t *testing.T) {
	err := errors.New("connection refused")
	status, _, _ := MapGRPCError(err)
	assert.Equal(t, http.StatusBadGateway, status, "其他非 gRPC error 应映射 HTTP 502（向后兼容）")
}

// TestMapGRPCError_Nil → 200（handler 不应调；约定 nil 应 OK）
func TestMapGRPCError_Nil(t *testing.T) {
	status, _, _ := MapGRPCError(nil)
	assert.Equal(t, http.StatusOK, status, "nil error 应返 HTTP 200")
}
