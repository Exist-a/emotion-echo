package grpcerr_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/emotion-echo/shared/pkg/grpcerr"
)

// B4 RED · 错误码统一映射
//
// 目标：抽 shared/pkg/grpcerr.Map(err) (codes.Code, string) helper，
// 让 4 svc 的 mapLogicError / mapAuthError / mapAIError 都能复用，
// 删除 analytics-svc / assessment-svc 的 status.Errorf 散写（19 处）。
//
// 设计原则：
//  1. sentinel error errors.Is 优先（业务自定义）
//  2. 通用 sentinel error errors.Is 兜底（grpcerr.ErrInvalidArgument 等）
//  3. 字符串前缀兜底（兼容 chat-svc "unauthorized: missing user id" 类型）
//  4. 默认 → codes.Internal + 原 err.Error()

func TestMap_Nil_ReturnsOK(t *testing.T) {
	c, msg := grpcerr.Map(nil)
	assert.Equal(t, codes.OK, c)
	assert.Empty(t, msg)
}

// 通用 sentinel → 标准 gRPC code
func TestMap_GenericSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"ErrInvalidArgument", grpcerr.ErrInvalidArgument, codes.InvalidArgument},
		{"ErrUnauthenticated", grpcerr.ErrUnauthenticated, codes.Unauthenticated},
		{"ErrPermissionDenied", grpcerr.ErrPermissionDenied, codes.PermissionDenied},
		{"ErrNotFound", grpcerr.ErrNotFound, codes.NotFound},
		{"ErrUnavailable", grpcerr.ErrUnavailable, codes.Unavailable},
		{"ErrAlreadyExists", grpcerr.ErrAlreadyExists, codes.AlreadyExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := grpcerr.Map(tt.err)
			assert.Equal(t, tt.want, c)
		})
	}
}

// 字符串前缀兜底（chat-svc "unauthorized: missing user id" 模式）
func TestMap_StringPrefixFallback(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"unauthorized prefix", errors.New("unauthorized: missing user id"), codes.Unauthenticated},
		{"forbidden prefix", errors.New("forbidden: not owner"), codes.PermissionDenied},
		{"validation prefix", errors.New("validation: bad input"), codes.InvalidArgument},
		{"not found prefix", errors.New("not found: id missing"), codes.NotFound},
		{"conflict prefix", errors.New("conflict: duplicate"), codes.AlreadyExists},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := grpcerr.Map(tt.err)
			assert.Equal(t, tt.want, c)
		})
	}
}

// 用户自定义 sentinel 优先匹配（user-svc 业务错误）
func TestMap_BusinessSentinel_HighPriority(t *testing.T) {
	// 业务 sentinel errors 通过 MapError 注册后优先级最高
	myInvalidCred := errors.New("invalid credentials - custom")
	grpcerr.MapError(myInvalidCred, codes.Unauthenticated)
	c, _ := grpcerr.Map(myInvalidCred)
	assert.Equal(t, codes.Unauthenticated, c)

	// 不注册的 sentinel 走通用规则
	unknown := errors.New("some random error")
	c2, _ := grpcerr.Map(unknown)
	assert.Equal(t, codes.Internal, c2)
}

// 字符串前缀匹配在 errors.Is 失败后生效
func TestMap_ErrorsIsBeatsStringPrefix(t *testing.T) {
	// 即使 err.Error() 含 "forbidden"，errors.Is 命中 ErrInvalidArgument
	// 应优先返回 InvalidArgument（不是 PermissionDenied）
	err := fmt.Errorf("forbidden access but it's actually: %w", grpcerr.ErrInvalidArgument)
	c, _ := grpcerr.Map(err)
	assert.Equal(t, codes.InvalidArgument, c, "errors.Is 应优先于字符串前缀")
}

// 包装的 gRPC status error 直接返回原 code（防止双重 wrap）
func TestMap_WrappedGRPCStatus(t *testing.T) {
	orig := status.Error(codes.NotFound, "user not found")
	wrapped := fmt.Errorf("getUser: %w", orig)
	c, msg := grpcerr.Map(wrapped)
	assert.Equal(t, codes.NotFound, c)
	assert.Contains(t, msg, "user not found")
}

// context deadline error → codes.DeadlineExceeded（保持向后兼容 BFF handler 503/504）
func TestMap_ContextDeadline(t *testing.T) {
	err := context.DeadlineExceeded
	c, _ := grpcerr.Map(err)
	assert.Equal(t, codes.DeadlineExceeded, c)
}

// 默认 fallback
func TestMap_UnknownError_ReturnsInternal(t *testing.T) {
	err := errors.New("totally unknown")
	c, msg := grpcerr.Map(err)
	assert.Equal(t, codes.Internal, c)
	assert.Equal(t, "totally unknown", msg)
}

// Wrap helper 直接生成 status.Status
func TestWrap_ReturnsStatusErr(t *testing.T) {
	err := grpcerr.Wrap(grpcerr.ErrNotFound, "user lookup")
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "Wrap result must be gRPC status")
	assert.Equal(t, codes.NotFound, st.Code())
	assert.Contains(t, st.Message(), "user lookup")
	assert.Contains(t, st.Message(), "not found")
}

// Wrap 业务 sentinel
func TestWrap_BusinessSentinel(t *testing.T) {
	authErr := errors.New("my custom auth")
	grpcerr.MapError(authErr, codes.Unauthenticated)

	wrapped := grpcerr.Wrap(authErr, "login failed")
	st, ok := status.FromError(wrapped)
	require.True(t, ok)
	assert.Equal(t, codes.Unauthenticated, st.Code())
	assert.Contains(t, st.Message(), "login failed")
	assert.Contains(t, st.Message(), "my custom auth")
}