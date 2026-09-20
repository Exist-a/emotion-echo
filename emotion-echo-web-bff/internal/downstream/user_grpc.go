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
	"encoding/json"

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
		return nil, wrapGRPCError(err, "user getMe")
	}
	return fromProtoUserInfo(resp), nil
}

// UpdateMe gRPC RPC
func (c *userGRPCClient) UpdateMe(ctx context.Context, req UpdateProfileReq) (*UserInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	profileReq := &emotionuser.UpdateProfileRequest{
		Nickname: req.Nickname,
		Gender:   genderPtrToInt32(req.Gender),
		// E2E-11：avatar_url 必须透传，否则头像上传接口返 200 但数据库仍为 NULL
		// （proto 已定义该字段，user-svc 侧也已落库，只是客户端漏映射）。
		AvatarUrl: req.AvatarURL,
	}
	if req.Config != nil {
		if data, err := json.Marshal(*req.Config); err == nil {
			s := string(data)
			profileReq.Config = &s
		}
	}
	resp, err := cli.UpdateProfile(withUserID(ctx), profileReq)
	if err != nil {
		return nil, wrapGRPCError(err, "user updateMe")
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
		return nil, wrapGRPCError(err, "user getByID")
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
		return nil, wrapGRPCError(err, "user reset password")
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
		return wrapGRPCError(err, "user logout")
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
		return nil, wrapGRPCError(err, "user login")
	}
	return fromProtoUserInfo(resp.GetUser()), nil
}

// Register gRPC RPC（Sprint E 2026-09-11）
//
// 同 Login：匿名调用，不传 x-user-id metadata。
// E2E-06: 新增 securityQuestions 参数
func (c *userGRPCClient) Register(ctx context.Context, username, password, verificationCode string, securityQuestions []SecurityQuestion) (*UserInfo, error) {
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
	// E2E-06: 转换密保问题
	if len(securityQuestions) > 0 {
		sqs := make([]*emotionuser.SecurityQuestion, len(securityQuestions))
		for i, sq := range securityQuestions {
			sqs[i] = &emotionuser.SecurityQuestion{
				Question: sq.Question,
				Answer:   sq.Answer,
			}
		}
		req.SecurityQuestions = sqs
	}
	resp, err := cli.Register(ctx, req)
	if err != nil {
		return nil, wrapGRPCError(err, "user register")
	}
	return fromProtoUserInfo(resp.GetUser()), nil
}

// VerifySecurityAnswer gRPC RPC（E2E-06，供 D-01=C 找回密码）
func (c *userGRPCClient) VerifySecurityAnswer(ctx context.Context, userID int64, questionOrder int, answer string) error {
	cli := emotionuser.NewUserServiceClient(c.conn)
	_, err := cli.VerifySecurityAnswer(ctx, &emotionuser.VerifySecurityAnswerRequest{
		UserId:        userID,
		QuestionOrder: int32(questionOrder),
		Answer:        answer,
	})
	if err != nil {
		return wrapGRPCError(err, "user verifySecurityAnswer")
	}
	return nil
}

// VerifySecurityAnswerByUsername 按用户名验证密保答案（R-01 #2 真修通）
//
// 原先这里是 `return fmt.Errorf("... not implemented for gRPC transport, use HTTP")`
// 的桩，而 BFF 默认 transport 就是 gRPC ⇒ 找回密码在默认配置下恒 401。
// 现改为真实 RPC 调用，与 HTTP transport 行为对齐。
func (c *userGRPCClient) VerifySecurityAnswerByUsername(ctx context.Context, username string, questionOrder int, answer string) error {
	cli := emotionuser.NewUserServiceClient(c.conn)
	_, err := cli.VerifySecurityAnswerByUsername(ctx, &emotionuser.VerifySecurityAnswerByUsernameRequest{
		Username:      username,
		QuestionOrder: int32(questionOrder),
		Answer:        answer,
	})
	if err != nil {
		return wrapGRPCError(err, "user verifySecurityAnswerByUsername")
	}
	return nil
}

// GetSecurityQuestionsByUsername 按用户名获取密保问题列表（E2E-07）
func (c *userGRPCClient) GetSecurityQuestionsByUsername(ctx context.Context, username string) ([]SecurityQuestionInfo, error) {
	cli := emotionuser.NewUserServiceClient(c.conn)
	resp, err := cli.GetSecurityQuestionsByUsername(ctx, &emotionuser.GetSecurityQuestionsByUsernameRequest{
		Username: username,
	})
	if err != nil {
		return nil, wrapGRPCError(err, "user getSecurityQuestionsByUsername")
	}
	questions := make([]SecurityQuestionInfo, len(resp.GetQuestions()))
	for i, q := range resp.GetQuestions() {
		questions[i] = SecurityQuestionInfo{
			QuestionOrder: int16(q.GetQuestionOrder()),
			Question:      q.GetQuestion(),
		}
	}
	return questions, nil
}

// ============ proto → types 转换 ============

func fromProtoUserInfo(u *emotionuser.UserInfo) *UserInfo {
	if u == nil {
		return nil
	}
	info := &UserInfo{
		UserID:    u.Id,
		Account:   u.Username,
		Nickname:  u.Nickname,
		AvatarURL: u.AvatarUrl,
		CreatedAt: u.CreatedAt,
	}
	if u.Config != nil {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(*u.Config), &cfg); err == nil {
			info.Config = cfg
		}
	}
	return info
}

// genderPtrToInt16 types *int16 → proto *int32（proto3 optional wrapper）
func genderPtrToInt32(g *int16) *int32 {
	if g == nil {
		return nil
	}
	v := int32(*g)
	return &v
}