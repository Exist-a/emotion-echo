// Package grpcserver — user_server_security_answer_test.go
//
// R-01 #2 真修通（user-svc 侧）：gRPC 密保校验必须①存在②可匿名调用。
//
// 两层根因（本文件各钉一条）：
//
//	① 方法缺失：proto 有 VerifySecurityAnswer（按 user_id），但找回密码发起时
//	   用户尚未登录、只有用户名 ⇒ 需要 VerifySecurityAnswerByUsername。
//	   原先 gRPC 侧没有该方法（BFF 侧是 not-implemented 桩）。
//	② 拦截器漏配：proto 与 HTTP 端都把 VerifySecurityAnswer 定义为**匿名**调用，
//	   但 server.go 的匿名跳过清单只有 Login/Register/ResetPassword
//	   ⇒ 不带 x-user-id 调用会被 userid 拦截器以 Unauthenticated 拒掉，
//	   即"契约说匿名、实现要求带身份"。
//
// 测试策略：起真 gRPC server + InMemory 仓储，**不传 x-user-id metadata**，
// 走真实拦截器链；用 gRPC Register 建一个带密保问题的用户作为前置数据。
package grpcserver

import (
	"context"
	"testing"

	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// newSecurityAnswerSvcCtx 构造同时具备 UserRepo 与 SecurityAnswerRepo 的 svcCtx。
// 注意：logic.VerifySecurityAnswer 会直接调 svcCtx.SecurityAnswerRepo，
// 只给 UserRepo 会 nil panic。
func newSecurityAnswerSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		UserRepo:           repository.NewInMemoryUserRepo(),
		SecurityAnswerRepo: repository.NewInMemorySecurityAnswerRepo(),
	}
}

// registerUserWithSecurityAnswer 用真实 gRPC Register 建带密保的用户，返回用户名。
func registerUserWithSecurityAnswer(t *testing.T, client emotionuser.UserServiceClient, username, answer string) {
	t.Helper()
	_, err := client.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: username,
		Password: "Password123",
		SecurityQuestions: []*emotionuser.SecurityQuestion{
			{Question: "first pet", Answer: answer},
		},
	})
	require.NoError(t, err, "前置：带密保问题的注册应成功")
}

// TestUserServer_VerifySecurityAnswerByUsername_CorrectAnswer_Anonymous_Success
// 一条用例同时钉住两层根因：方法存在 + 匿名可调用 + 正确答案通过。
func TestUserServer_VerifySecurityAnswerByUsername_CorrectAnswer_Anonymous_Success(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newSecurityAnswerSvcCtx())
	defer cleanup()
	client := emotionuser.NewUserServiceClient(conn)

	registerUserWithSecurityAnswer(t, client, "security_user_ok", "blue")

	// 不带 x-user-id metadata —— 找回密码发起时用户本就未登录
	resp, err := client.VerifySecurityAnswerByUsername(context.Background(),
		&emotionuser.VerifySecurityAnswerByUsernameRequest{
			Username:      "security_user_ok",
			QuestionOrder: 1,
			Answer:        "blue",
		})

	require.NoError(t, err,
		"匿名 + 正确答案应通过；若返 Unauthenticated 说明拦截器漏把本 RPC 列入匿名清单")
	require.NotNil(t, resp)
	assert.True(t, resp.Success)
}

func TestUserServer_VerifySecurityAnswerByUsername_WrongAnswer_Denied(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newSecurityAnswerSvcCtx())
	defer cleanup()
	client := emotionuser.NewUserServiceClient(conn)

	registerUserWithSecurityAnswer(t, client, "security_user_wrong", "blue")

	resp, err := client.VerifySecurityAnswerByUsername(context.Background(),
		&emotionuser.VerifySecurityAnswerByUsernameRequest{
			Username:      "security_user_wrong",
			QuestionOrder: 1,
			Answer:        "WRONG",
		})

	require.Error(t, err, "答案错误必须被拒")
	assert.Nil(t, resp)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code())
}

// TestUserServer_VerifySecurityAnswerByUsername_UnknownUser_NotDistinguishable
// 用户不存在必须与"答案错误"返回同一个 code（防用户名枚举）。
func TestUserServer_VerifySecurityAnswerByUsername_UnknownUser_NotDistinguishable(t *testing.T) {
	conn, cleanup := startUserTestServerWithCtx(t, newSecurityAnswerSvcCtx())
	defer cleanup()
	client := emotionuser.NewUserServiceClient(conn)

	_, err := client.VerifySecurityAnswerByUsername(context.Background(),
		&emotionuser.VerifySecurityAnswerByUsernameRequest{
			Username:      "nobody_at_all",
			QuestionOrder: 1,
			Answer:        "blue",
		})

	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.PermissionDenied, st.Code(),
		"用户不存在应与答案错误同码（PermissionDenied），不得用 NotFound 泄露用户名是否存在")
}

// CL-02 清除（2026-10-14，用户裁定 D4）：按 user_id 的 VerifySecurityAnswer RPC 已删除
// （族内冗余；找回密码流程用不上——发起时用户尚未登录），其匿名/拦截器测试随之移除。
// 「匿名调用不被 userid 拦截器拒绝」这一契约仍由本文件上方 3 个 ByUsername 用例覆盖
// （CorrectAnswer_Anonymous_Success / WrongAnswer_Denied / UnknownUser_NotDistinguishable）。
