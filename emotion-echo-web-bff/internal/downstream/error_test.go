package downstream

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// B4: 验证 BFF 全局 gRPC error → HTTP code 映射覆盖 7 类 gRPC code
func TestMapGRPCError_AllCodes(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"nil → 200", nil, 200},
		{"Unavailable → 503", status.Error(codes.Unavailable, "down"), 503},
		{"Unauthenticated → 401", status.Error(codes.Unauthenticated, "auth"), 401},
		{"PermissionDenied → 403", status.Error(codes.PermissionDenied, "forbid"), 403},
		{"NotFound → 404", status.Error(codes.NotFound, "missing"), 404},
		{"InvalidArgument → 400", status.Error(codes.InvalidArgument, "bad"), 400},
		{"AlreadyExists → 409", status.Error(codes.AlreadyExists, "conflict"), 409},
		{"DeadlineExceeded → 504", status.Error(codes.DeadlineExceeded, "slow"), 504},
		{"Internal → 500", status.Error(codes.Internal, "boom"), 500},
		{"Unknown → 500", status.Error(codes.Unknown, "?"), 500},
		{"context.DeadlineExceeded → 504", context.DeadlineExceeded, 504},
		{"其他 error → 502", errors.New("random"), 502},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st, _, _ := MapGRPCError(tt.err)
			assert.Equal(t, tt.wantStatus, st, "err=%v", tt.err)
		})
	}
}

// 包装的 gRPC status error 透传原 code（用 fmt.Errorf %w 保留 status 链）
func TestMapGRPCError_WrappedStatus(t *testing.T) {
	orig := status.Error(codes.AlreadyExists, "username taken")
	wrapped := fmt.Errorf("register failed: %w", orig)
	st, _, _ := MapGRPCError(wrapped)
	assert.Equal(t, 409, st, "fmt.Errorf %%w 应让 status.FromError 命中原始 code")
}