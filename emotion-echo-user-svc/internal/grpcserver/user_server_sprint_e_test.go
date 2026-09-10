// Package grpcserver — user_server_sprint_e_test.go
//
// Sprint E RED: user-svc gRPC Login/Register 真实实现测试（解 user-svc auth 半 gRPC）。
//
// 行为契约（user.proto Sprint E 扩 rpc 后必须成立）：
//   - Login 调 logic.AuthLogic.Login（复用现有 logic，不重复实现）
//     - 成功 → 返 LoginResponse{user=UserInfo}
//     - 失败（用户不存在/密码错） → codes.Unauthenticated（与 HTTP 端一致）
//     - 入参校验失败 → codes.InvalidArgument
//   - Register 调 logic.AuthLogic.Register
//     - 成功 → 返 RegisterResponse{user=UserInfo}
//     - username 已存在 → codes.AlreadyExists
//     - 入参校验失败 → codes.InvalidArgument
//
// 拦截器：userid 拦截器需跳过 Login/Register（匿名调用，不带 x-user-id）
//
// 测试策略：
//   - startUserTestServerWithCtx 注入 mock svcCtx（InMemoryUserRepo）
//   - 走真实 gRPC server
//   - metadata x-user-id 不传（验证拦截器跳过）
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startUserTestServerWithCtx 注入 mock svcCtx 启 server
func startUserTestServerWithCtx(t *testing.T, svcCtx *svc.ServiceContext) (*grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	srv := New(svcCtx, port)
	require.NotNil(t, srv)

	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Start(ctx) }()

	addr := ""
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		addr = srv.Addr()
		if addr != "" && addr != fmt.Sprintf(":%d", port) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		cancel()
		select {
		case <-serveErr:
		case <-time.After(2 * time.Second):
		}
	}
	return conn, cleanup
}

// newMockSvcCtx 构造带 InMemoryUserRepo 的 svcCtx
func newMockSvcCtx() *svc.ServiceContext {
	repo := repository.NewInMemoryUserRepo()
	return &svc.ServiceContext{
		UserRepo: repo,
	}
}

// ============ Sprint E RED 测试 ============

// TestUserServer_Register_Success 断言 Register 不再返 Unimplemented，并真实创建用户
func TestUserServer_Register_Success(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)
	// 不带 x-user-id metadata — Register 是匿名调用
	resp, err := client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "sprint_e_user",
		Password: "password123",
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"Register 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.User)
	assert.Equal(t, "sprint_e_user", resp.User.Username)
	assert.NotZero(t, resp.User.Id, "Register 后 User 应有 ID")
}

// TestUserServer_Login_AfterRegister 完整流程：先 Register 再 Login
func TestUserServer_Login_AfterRegister(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)

	// 1. Register
	_, err := client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "sprint_e_login",
		Password: "password123",
	})
	require.NoError(t, err)

	// 2. Login（不带 x-user-id metadata）
	resp, err := client.Login(context.Background(), &emotionuser.LoginRequest{
		Username: "sprint_e_login",
		Password: "password123",
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"Login 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.User)
	assert.Equal(t, "sprint_e_login", resp.User.Username)
}

// TestUserServer_Login_WrongPassword 断言错误密码返 Unauthenticated 而非 OK
func TestUserServer_Login_WrongPassword(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)

	_, _ = client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "sprint_e_wrong",
		Password: "correct_password",
	})

	_, err := client.Login(context.Background(), &emotionuser.LoginRequest{
		Username: "sprint_e_wrong",
		Password: "wrong_password",
	})
	require.Error(t, err, "错误密码必须返 error 而非 OK")
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unauthenticated, st.Code(),
		"错误密码应返 Unauthenticated，实际=%s msg=%s", st.Code(), st.Message())
}

// ============ Sprint F RED 测试（ResetPassword + Logout）============

// TestUserServer_ResetPassword_Success 断言 ResetPassword 不再返 Unimplemented
func TestUserServer_ResetPassword_Success(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)

	// 先注册
	_, err := client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "sprint_f_reset",
		Password: "old_password",
	})
	require.NoError(t, err)

	// ResetPassword（匿名调用）
	resp, err := client.ResetPassword(context.Background(), &emotionuser.ResetPasswordRequest{
		Username:         "sprint_f_reset",
		VerificationCode: "111111",
		NewPassword:      "new_password",
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"ResetPassword 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.User)
	assert.Equal(t, "sprint_f_reset", resp.User.Username)
}

// TestUserServer_ResetPassword_ThenLogin 完整流程：ResetPassword 后用新密码 Login 成功
func TestUserServer_ResetPassword_ThenLogin(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)

	_, _ = client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "sprint_f_full",
		Password: "old_password",
	})

	_, err := client.ResetPassword(context.Background(), &emotionuser.ResetPasswordRequest{
		Username:         "sprint_f_full",
		VerificationCode: "111111",
		NewPassword:      "new_password",
	})
	require.NoError(t, err)

	// 用新密码登录
	resp, err := client.Login(context.Background(), &emotionuser.LoginRequest{
		Username: "sprint_f_full",
		Password: "new_password",
	})
	require.NoError(t, err, "ResetPassword 后用新密码应登录成功")
	require.NotNil(t, resp.User)
}

// TestUserServer_Logout_Success 断言 Logout 返 OK（鉴权要求 x-user-id）
func TestUserServer_Logout_Success(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)

	// 带 x-user-id metadata（Logout 是需鉴权调用）
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "1"))

	resp, err := client.Logout(ctx, &emotionuser.LogoutRequest{})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"Logout 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
}
