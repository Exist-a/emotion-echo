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
	"encoding/json"
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

// ensureRepo 统一守卫：svcCtx 未注入，或 dev 降级启动（Stage 77，stage-76 §二.3）
// 下 UserRepo 为 nil 时，端点返 Unavailable 而非在 logic 层 panic。
func (s *userServer) ensureRepo() error {
	if s.svcCtx == nil {
		return status.Error(codes.Unavailable, "user-svc service context not initialized")
	}
	if s.svcCtx.UserRepo == nil {
		return status.Error(codes.Unavailable, "user-svc repository not initialized (degraded start)")
	}
	return nil
}

// toProtoUser 把 types.UserInfo 转 proto UserInfo
func toProtoUser(u types.UserInfo) *emotionuser.UserInfo {
	pb := &emotionuser.UserInfo{
		Id:        u.UserId,
		Username:  u.Account,
		Nickname:  u.Nickname,
		AvatarUrl: u.AvatarURL,
		CreatedAt: u.CreatedAt,
	}
	if u.Config != nil {
		if data, err := json.Marshal(u.Config); err == nil {
			s := string(data)
			pb.Config = &s
		}
	}
	return pb
}

// GetMe 实现 GetMe RPC（复用 logic.NewGetMeLogic）
func (s *userServer) GetMe(ctx context.Context, req *emotionuser.GetMeRequest) (*emotionuser.UserInfo, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	resp, err := logic.NewGetMeLogic(ctx, s.svcCtx).GetMe(&types.GetMeReq{})
	if err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "getMe: %v", err)
	}
	return toProtoUser(resp.User), nil
}

// UpdateProfile 实现 UpdateProfile RPC（复用 logic.NewUpdateProfileLogic）
func (s *userServer) UpdateProfile(ctx context.Context, req *emotionuser.UpdateProfileRequest) (*emotionuser.UserInfo, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	// proto optional → types optional
	//
	// E2E-11：avatar_url 必须一并透传。原先只组装 Nickname + Gender，
	// avatar_url 在 proto→types 转换里被丢掉 ⇒ 头像上传接口返 200、
	// MinIO 对象已写入，但数据库 avatar_url 恒为 NULL。
	profileReq := &types.UpdateProfileReq{
		Nickname:  req.Nickname,
		AvatarURL: req.AvatarUrl,
	}
	if req.Gender != nil {
		g := int16(*req.Gender)
		profileReq.Gender = &g
	}
	if req.Config != nil {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(*req.Config), &cfg); err == nil {
			profileReq.Config = &cfg
		}
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
	if err := s.ensureRepo(); err != nil {
		return nil, err
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
//   - ErrInvalidCredentials → codes.Unauthenticated（与 HTTP 401 对齐）
//   - ErrValidation → codes.InvalidArgument
//   - 其他 → codes.Internal
//   - accessToken 不在此 RPC 返回——由 BFF 收到 UserInfo 后用 jwt.Manager 签发
func (s *userServer) Login(ctx context.Context, req *emotionuser.LoginRequest) (*emotionuser.LoginResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
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
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	// E2E-06: 转换密保问题
	securityQuestions := make([]types.SecurityQuestion, len(req.GetSecurityQuestions()))
	for i, sq := range req.GetSecurityQuestions() {
		securityQuestions[i] = types.SecurityQuestion{
			Question: sq.GetQuestion(),
			Answer:   sq.GetAnswer(),
		}
	}
	resp, err := logic.NewAuthLogic(ctx, s.svcCtx).Register(&types.RegisterReq{
		Username: req.GetUsername(),
		Password: req.GetPassword(),
		// proto optional string → types string：空字符串 = 无验证码
		VerificationCode:  req.GetVerificationCode(),
		Nickname:          req.Nickname,
		SecurityQuestions: securityQuestions,
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
	if err := s.ensureRepo(); err != nil {
		return nil, err
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
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	// 鉴权拦截器已从 ctx 注入 user_id，这里无需再读（mock 模式不做服务端黑名单）
	return &emotionuser.LogoutResponse{Success: true}, nil
}

// VerifySecurityAnswer 实现 VerifySecurityAnswer RPC（E2E-06，供 D-01=C 找回密码）
//
// 错误映射：
//   - ErrNotFound → codes.NotFound（用户无密保问题）
//   - ErrValidation → codes.InvalidArgument（questionOrder 不合法）
//   - ErrSecurityAnswerMismatch → codes.PermissionDenied（答案错误）
func (s *userServer) VerifySecurityAnswer(ctx context.Context, req *emotionuser.VerifySecurityAnswerRequest) (*emotionuser.VerifySecurityAnswerResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	err := logic.NewAuthLogic(ctx, s.svcCtx).VerifySecurityAnswer(
		req.GetUserId(),
		int(req.GetQuestionOrder()),
		req.GetAnswer(),
	)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "security answers not found")
		}
		if errors.Is(err, logic.ErrSecurityAnswerMismatch) {
			return nil, status.Error(codes.PermissionDenied, "security answer mismatch")
		}
		return nil, grpcerr.MapToError(err, "verifySecurityAnswer")
	}
	return &emotionuser.VerifySecurityAnswerResponse{Success: true}, nil
}

// VerifySecurityAnswerByUsername 实现 VerifySecurityAnswerByUsername RPC（R-01 #2）
//
// 为什么需要：找回密码发起时用户尚未登录，前端只有用户名、没有 user_id，
// 故走不了上面的 VerifySecurityAnswer。HTTP transport 侧的
// POST /api/v1/users/verify-security-answer 早已支持按用户名校验，
// 此 RPC 让 gRPC 侧行为对齐（BFF 默认走 gRPC，否则找回密码恒 401）。
//
// 错误映射：
//   - ErrValidation → codes.InvalidArgument（questionOrder 不合法 / username 为空）
//   - ErrNotFound → codes.PermissionDenied（用户不存在或无密保）
//   - ErrSecurityAnswerMismatch → codes.PermissionDenied（答案错误）
//
// 注意「用户不存在」刻意**不**映射为 NotFound：否则调用方可据返回码判断
// 用户名是否存在（用户名枚举）。与 HTTP 端「统一 401」的防枚举策略一致。
func (s *userServer) VerifySecurityAnswerByUsername(ctx context.Context, req *emotionuser.VerifySecurityAnswerByUsernameRequest) (*emotionuser.VerifySecurityAnswerByUsernameResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	if s.svcCtx.SecurityAnswerRepo == nil {
		return nil, status.Error(codes.Unavailable, "user-svc security answer repository not initialized (degraded start)")
	}
	err := logic.NewAuthLogic(ctx, s.svcCtx).VerifySecurityAnswerByUsername(
		req.GetUsername(),
		int(req.GetQuestionOrder()),
		req.GetAnswer(),
	)
	if err != nil {
		if errors.Is(err, logic.ErrValidation) {
			return nil, status.Error(codes.InvalidArgument, "invalid question order or username")
		}
		if errors.Is(err, repository.ErrNotFound) || errors.Is(err, logic.ErrSecurityAnswerMismatch) {
			return nil, status.Error(codes.PermissionDenied, "security answer verification failed")
		}
		return nil, grpcerr.MapToError(err, "verifySecurityAnswerByUsername")
	}
	return &emotionuser.VerifySecurityAnswerByUsernameResponse{Success: true}, nil
}

// GetSecurityQuestionsByUsername 按用户名获取密保问题列表（E2E-07）
//
// 防枚举：用户不存在时返回空列表（而非错误）。
func (s *userServer) GetSecurityQuestionsByUsername(ctx context.Context, req *emotionuser.GetSecurityQuestionsByUsernameRequest) (*emotionuser.GetSecurityQuestionsByUsernameResponse, error) {
	if err := s.ensureRepo(); err != nil {
		return nil, err
	}
	if s.svcCtx.SecurityAnswerRepo == nil {
		return nil, status.Error(codes.Unavailable, "user-svc security answer repository not initialized (degraded start)")
	}
	questions, err := logic.NewAuthLogic(ctx, s.svcCtx).GetSecurityQuestionsByUsername(req.GetUsername())
	if err != nil {
		if errors.Is(err, logic.ErrValidation) {
			return nil, status.Error(codes.InvalidArgument, "username is required")
		}
		return nil, grpcerr.MapToError(err, "getSecurityQuestionsByUsername")
	}
	pbQuestions := make([]*emotionuser.SecurityQuestionInfo, len(questions))
	for i, q := range questions {
		pbQuestions[i] = &emotionuser.SecurityQuestionInfo{
			QuestionOrder: int32(q.QuestionOrder),
			Question:      q.Question,
		}
	}
	return &emotionuser.GetSecurityQuestionsByUsernameResponse{Questions: pbQuestions}, nil
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
