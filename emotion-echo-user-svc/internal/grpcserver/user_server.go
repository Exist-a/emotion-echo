// Package grpcserver — user_server.go
//
// Stage 62 PR-3.2: 实现 UserServiceServer interface
//
// 3 个 rpc 方法对应 user-svc 已有的 logic 层（HTTP 与 gRPC 共享业务逻辑）：
//   - GetMe            → logic.GetMeLogic（PR-3.3 阶段补完）
//   - UpdateProfile    → logic.UpdateProfileLogic（PR-3.3 阶段补完）
//   - GetUserById      → logic.GetUserByIdLogic（PR-3.3 阶段补完）
//
// PR-3.2 阶段：所有方法占位返 Unimplemented（与 chat-svc PR-GRPC-2 同步节奏）；
// PR-3.3 阶段补完真实实现。

package grpcserver

import (
	"context"

	"emotion-echo-user-svc/internal/svc"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// userServer 实现 emotionuser.UserServiceServer
type userServer struct {
	emotionuser.UnimplementedUserServiceServer
	svcCtx *svc.ServiceContext
}

// GetMe 占位实现（PR-3.3 阶段补完）
func (s *userServer) GetMe(ctx context.Context, req *emotionuser.GetMeRequest) (*emotionuser.UserInfo, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "GetMe: PR-3.3 阶段补完")
}

// UpdateProfile 占位实现
func (s *userServer) UpdateProfile(ctx context.Context, req *emotionuser.UpdateProfileRequest) (*emotionuser.UserInfo, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "UpdateProfile: PR-3.3 阶段补完")
}

// GetUserById 占位实现
func (s *userServer) GetUserById(ctx context.Context, req *emotionuser.GetUserByIdRequest) (*emotionuser.UserInfo, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "GetUserById: PR-3.3 阶段补完")
}