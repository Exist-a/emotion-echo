// Package downstream — user_grpc.go
//
// Stage 62 PR-3.3: BFF → user-svc gRPC client
//
// 设计（与 chat_grpc.go 同模式）：
//   - UserClient interface 不变（user.go 已定义 6 个方法）
//   - userGRPCClient 实现 UserClient；通过内嵌 userHTTPClient 仅覆盖 3 个 gRPC 方法，
//     其余 3 个（Login/Register/ResetPassword）走 HTTP fallback
//     （user-svc 暂未暴露对应 gRPC 端点）
//   - feature flag: UserClientOptions.Transport=grpc（默认）| http
//
// proto 类型：直接复用 shared/pkg/emotionuser 生成代码
// 鉴权：metadata x-user-id（与 emotion_query.proto / emotion_chat.proto 一致）
//
// Sprint E + F（2026-09-11）：扩 proto user.proto 加 Login / Register / ResetPassword / Logout RPC；
// user-svc gRPC server 实现；userid 拦截器跳过 Login/Register/ResetPassword 3 个匿名调用。
// accessToken 仍由 BFF jwt.Manager 签发。
// **所有 7 个 UserClient 方法（Login/Register/ResetPassword/Logout/GetMe/UpdateProfile/GetUserById）全走 gRPC**。
package downstream

import (
	"context"
	"fmt"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"

	"google.golang.org/grpc"
)

// userGRPCClient 是 UserClient 的 gRPC 实现（组合 userHTTPClient）
type userGRPCClient struct {
	conn        *grpc.ClientConn
	httpFallback *userHTTPClient
}

// NewUserGRPCClient 构造（需已建立的 gRPC 连接；conn=nil → 返 nil）
func NewUserGRPCClient(conn *grpc.ClientConn) UserClient {
	if conn == nil {
		return nil
	}
	return &userGRPCClient{conn: conn}
}

// GetMe gRPC RPC
func (c *userGRPCClient) GetMe(ctx context.Context) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.GetMe(withUserID(ctx), &emotionuser.GetMeRequest{})
	if err != nil {
		return nil, fmt.Errorf("downstream: user getMe: %w", err)
	}
	return fromProtoUserInfo(resp), nil
}

// UpdateMe gRPC RPC
func (c *userGRPCClient) UpdateMe(ctx context.Context, req UpdateProfileReq) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.UpdateProfile(withUserID(ctx), &emotionuser.UpdateProfileRequest{
		Nickname: req.Nickname,
		Gender:   genderPtrToInt32(req.Gender),
	})
	if err != nil {
		return nil, fmt.Errorf("downstream: user updateMe: %w", err)
	}
	return fromProtoUserInfo(resp), nil
}

// GetByID gRPC RPC
func (c *userGRPCClient) GetByID(ctx context.Context, id int64) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.GetUserById(withUserID(ctx), &emotionuser.GetUserByIdRequest{
		UserId: id,
	})
	if err != nil {
		return nil, fmt.Errorf("downstream: user getByID: %w", err)
	}
	return fromProtoUserInfo(resp), nil
}

// ResetPassword gRPC RPC（Sprint F 2026-09-11）
//
// 同 Login：匿名调用，不传 x-user-id metadata。
func (c *userGRPCClient) ResetPassword(ctx context.Context, req ResetPasswordReq) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.ResetPassword(ctx, &emotionuser.ResetPasswordRequest{
		Username:         req.Username,
		VerificationCode: req.VerificationCode,
		NewPassword:      req.NewPassword,
	})
	if err != nil {
		return nil, fmt.Errorf("downstream: user reset password: %w", err)
	}
	return fromProtoUserInfo(resp.GetUser()), nil
}

// Logout gRPC RPC（Sprint F 2026-09-11）
//
// 需要 x-user-id metadata（Logout 不在拦截器跳过清单），所以用 withUserID(ctx)。
func (c *userGRPCClient) Logout(ctx context.Context) error {
	cli := emotionuser.NewUserServiceClient(c.conn)
	_, err := cli.Logout(withUserID(ctx), &emotionuser.LogoutRequest{})
	if err != nil {
		return fmt.Errorf("downstream: user logout: %w", err)
	}
	return nil
}

// Login gRPC RPC（Sprint E 2026-09-11）
//
// 注意：Login 是匿名调用，**不传** metadata x-user-id（user-svc 拦截器跳过 Login）。
// 不能用 withUserID(ctx)——会强制注入 x-user-id metadata，虽然 server 跳过但语义错误。
func (c *userGRPCClient) Login(ctx context.Context, username, password string) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.Login(ctx, &emotionuser.LoginRequest{
		Username: username,
		Password: password,
	})
	if err != nil {
		return nil, fmt.Errorf("downstream: user login: %w", err)
	}
	return fromProtoUserInfo(resp.GetUser()), nil
}

// Register gRPC RPC（Sprint E 2026-09-11）
//
// 同 Login：匿名调用，不传 x-user-id metadata。
func (c *userGRPCClient) Register(ctx context.Context, username, password, verificationCode string) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	req := &emotionuser.RegisterRequest{
		Username: username,
		Password: password,
	}
	// verificationCode 可选：空字符串不设字段
	if verificationCode != "" {
		vc := verificationCode
		req.VerificationCode = &vc
	}
	resp, err := cli.Register(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("downstream: user register: %w", err)
	}
	return fromProtoUserInfo(resp.GetUser()), nil
}

// ============ proto → types 转换 ============

func fromProtoUserInfo(u *emotionuser.UserInfo) *UserInfo {
	if u == nil {
		return nil
	}
	return &UserInfo{
		UserID:   u.Id,
		Account:  u.Username,
		Nickname: u.Nickname,
		Phone:    u.Phone,
	}
}

// genderPtrToInt16 types *int16 → proto *int32（proto3 optional wrapper）
func genderPtrToInt32(g *int16) *int32 {
	if g == nil {
		return nil
	}
	v := int32(*g)
	return &v
}