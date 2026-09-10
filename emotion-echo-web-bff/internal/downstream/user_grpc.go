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
// 鉴权：metadata x-user-id（与 emotion_query.proto / ai-svc 拦截器一致）
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

// ResetPassword 走 HTTP fallback（user-svc 未暴露 gRPC 端点）
func (c *userGRPCClient) ResetPassword(ctx context.Context, req ResetPasswordReq) (*UserInfo, error) {
	return c.httpFallback.ResetPassword(ctx, req)
}

// Login 走 HTTP fallback
func (c *userGRPCClient) Login(ctx context.Context, username, password string) (*UserInfo, error) {
	return c.httpFallback.Login(ctx, username, password)
}

// Register 走 HTTP fallback
func (c *userGRPCClient) Register(ctx context.Context, username, password, verificationCode string) (*UserInfo, error) {
	return c.httpFallback.Register(ctx, username, password, verificationCode)
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