// Package grpcserver — user_server_nildeps_test.go
//
// Stage 77 RED：dev 栈实测（stage-76 §二.3）openPostgres 失败时 main.go 带
// UserRepo=nil 的 ServiceContext 启动 → Login RPC 在 logic 层 panic。
// 契约：repo 未注入时 gRPC 端点必须返 codes.Unavailable，绝不 panic。
package grpcserver

import (
	"context"
	"testing"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"emotion-echo-user-svc/internal/svc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// nilRepoServer 构造"svcCtx 在但 repo 没注入"的 server（dev 降级启动的真实形态）
func nilRepoServer() *userServer {
	return &userServer{svcCtx: &svc.ServiceContext{}}
}

func TestUserServer_Login_NilUserRepo_ReturnsUnavailable(t *testing.T) {
	s := nilRepoServer()
	_, err := s.Login(context.Background(), &emotionuser.LoginRequest{
		Username: "echo",
		Password: "echo123",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code(), "repo 未注入应返 Unavailable 而非 panic")
}

func TestUserServer_GetMe_NilUserRepo_ReturnsUnavailable(t *testing.T) {
	s := nilRepoServer()
	_, err := s.GetMe(context.Background(), &emotionuser.GetMeRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}

func TestUserServer_Register_NilUserRepo_ReturnsUnavailable(t *testing.T) {
	s := nilRepoServer()
	_, err := s.Register(context.Background(), &emotionuser.RegisterRequest{
		Username: "u1",
		Password: "password1",
	})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.Equal(t, codes.Unavailable, st.Code())
}
