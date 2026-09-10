// Package grpcserver — user_server.go
//
// Stage 62 PR-3.3: 实现 UserServiceServer interface（HTTP 与 gRPC 共享 logic 层）
//
// 3 个 rpc 方法对应 user-svc 已有的 logic 层：
//   - GetMe            → logic.NewGetMeLogic
//   - UpdateProfile    → logic.NewUpdateProfileLogic
//   - GetUserById      → logic.NewGetUserByIdLogic

package grpcserver

import (
	"context"
	"errors"

	"emotion-echo-user-svc/internal/logic"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// userServer 实现 emotionuser.UserServiceServer
type userServer struct {
	emotionuser.UnimplementedUserServiceServer
	svcCtx *svc.ServiceContext
}

// toProtoUser 把 types.UserInfo 转 proto UserInfo
func toProtoUser(u types.UserInfo) *emotionuser.UserInfo {
	return &emotionuser.UserInfo{
		Id:       u.UserId,
		Username: u.Account,
		Nickname: u.Nickname,
		Phone:    u.Phone,
	}
}

// GetMe 实现 GetMe RPC（复用 logic.NewGetMeLogic）
func (s *userServer) GetMe(ctx context.Context, req *emotionuser.GetMeRequest) (*emotionuser.UserInfo, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	resp, err := logic.NewGetMeLogic(ctx, s.svcCtx).GetMe(&types.GetMeReq{})
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "getMe: %v", err)
	}
	return toProtoUser(resp.User), nil
}

// UpdateProfile 实现 UpdateProfile RPC（复用 logic.NewUpdateProfileLogic）
func (s *userServer) UpdateProfile(ctx context.Context, req *emotionuser.UpdateProfileRequest) (*emotionuser.UserInfo, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	// proto optional → types optional
	profileReq := &types.UpdateProfileReq{
		Nickname: req.Nickname,
	}
	if req.Gender != nil {
		g := int16(*req.Gender)
		profileReq.Gender = &g
	}
	resp, err := logic.NewUpdateProfileLogic(ctx, s.svcCtx).UpdateProfile(profileReq)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "updateProfile: %v", err)
	}
	return toProtoUser(resp.User), nil
}

// GetUserById 实现 GetUserById RPC（复用 logic.NewGetUserByIdLogic）
func (s *userServer) GetUserById(ctx context.Context, req *emotionuser.GetUserByIdRequest) (*emotionuser.UserInfo, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	resp, err := logic.NewGetUserByIdLogic(ctx, s.svcCtx).GetUserById(&types.GetUserByIdReq{
		Id: req.UserId,
	})
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "getUserById: %v", err)
	}
	return toProtoUser(resp.User), nil
}