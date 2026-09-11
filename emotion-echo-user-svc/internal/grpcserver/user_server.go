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
	grpcerr "github.com/emotion-echo/shared/pkg/grpcerr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// B4: 注册业务 sentinel errors（grpcerr.Map 优先匹配）
func init() {
	grpcerr.MapError(logic.ErrInvalidCredentials, codes.Unauthenticated)
	grpcerr.MapError(logic.ErrInvalidVerifyCode, codes.PermissionDenied)
	grpcerr.MapError(logic.ErrUsernameTaken, codes.AlreadyExists)
	grpcerr.MapError(logic.ErrValidation, codes.InvalidArgument)
}

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
		return nil, grpcerr.MapToError(err, "updateProfile")
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
		return nil, grpcerr.MapToError(err, "getUserById")
	}
	return toProtoUser(resp.User), nil
}

// Login 实现 Login RPC（Sprint E 2026-09-11）
//
// 行为契约：
//   - 复用 logic.NewAuthLogic.Login（与 HTTP handler 同源）
//   - 错误映射：
//     - ErrInvalidCredentials → codes.Unauthenticated（与 HTTP 401 对齐）
//     - ErrValidation → codes.InvalidArgument
//     - 其他 → codes.Internal
//   - accessToken 不在此 RPC 返回——由 BFF 收到 UserInfo 后用 jwt.Manager 签发
func (s *userServer) Login(ctx context.Context, req *emotionuser.LoginRequest) (*emotionuser.LoginResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	resp, err := logic.NewAuthLogic(ctx, s.svcCtx).Login(&types.LoginReq{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
	})
	if err != nil {
		return nil, mapAuthError(err)
	}
	return &emotionuser.LoginResponse{User: toProtoUser(resp.User)}, nil
}

// Register 实现 Register RPC（Sprint E 2026-09-11）
//
// 错误映射：
//   - ErrUsernameTaken → codes.AlreadyExists
//   - ErrValidation → codes.InvalidArgument
//   - 其他 → codes.Internal
func (s *userServer) Register(ctx context.Context, req *emotionuser.RegisterRequest) (*emotionuser.RegisterResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	resp, err := logic.NewAuthLogic(ctx, s.svcCtx).Register(&types.RegisterReq{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		// proto optional string → types string：空字符串 = 无验证码
		VerificationCode: req.GetVerificationCode(),
		Phone:            req.Phone,
		Nickname:         req.Nickname,
	})
	if err != nil {
		return nil, mapAuthError(err)
	}
	return &emotionuser.RegisterResponse{User: toProtoUser(resp.User)}, nil
}

// ResetPassword 实现 ResetPassword RPC（Sprint F 2026-09-11）
//
// 错误映射：
//   - ErrInvalidCredentials → codes.Unauthenticated（用户不存在）
//   - ErrInvalidVerifyCode → codes.PermissionDenied（验证码错）
//   - ErrValidation → codes.InvalidArgument
func (s *userServer) ResetPassword(ctx context.Context, req *emotionuser.ResetPasswordRequest) (*emotionuser.ResetPasswordResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	resp, err := logic.NewAuthLogic(ctx, s.svcCtx).ResetPassword(&types.ResetPasswordReq{
		Username:         req.GetUsername(),
		VerificationCode: req.GetVerificationCode(),
		NewPassword:      req.GetNewPassword(),
	})
	if err != nil {
		return nil, mapAuthError(err)
	}
	return &emotionuser.ResetPasswordResponse{User: toProtoUser(resp.User)}, nil
}

// Logout 实现 Logout RPC（Sprint F 2026-09-11）
//
// 服务端无状态（mock auth 模式），主要让客户端清 token。这里仅返 success=true。
// 鉴权：userid 拦截器**不跳过**此 RPC，调用方需带 metadata x-user-id。
func (s *userServer) Logout(ctx context.Context, req *emotionuser.LogoutRequest) (*emotionuser.LogoutResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	// 鉴权拦截器已从 ctx 注入 user_id，这里无需再读（mock 模式不做服务端黑名单）
	return &emotionuser.LogoutResponse{Success: true}, nil
}

// mapAuthError 把 logic.AuthLogic 错误映射到 gRPC status code
//
// B4：迁移到 grpcerr.Wrap，业务 sentinel errors 由 init() 注册到 grpcerr。
// 保留函数名作为 BFF handler 已有调用点的稳定入口。
func mapAuthError(err error) error {
	if err == nil {
		return nil
	}
	return grpcerr.MapToError(err, "auth")
}