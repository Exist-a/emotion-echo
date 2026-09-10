// Package grpcserver — user_server_test.go
//
// Stage 62 PR-3.2: user-svc gRPC server 实现单元测试
//
// 仿 chat-svc/internal/grpcserver/chat_server_test.go 范式：
//   - 真实 TCP loopback（不 bufconn）
//   - metadata x-user-id 传 user id
//   - 验证 type 编译 + health check + 拦截器链
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startTestServer 在临时端口启 gRPC server
func startUserTestServer(t *testing.T) (*Server, *grpc.ClientConn, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().(*net.TCPAddr).Port
	lis.Close()

	srv := New(nil, port) // PR-3.2 阶段暂不注入真实 svcCtx
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
	return srv, conn, cleanup
}

// ============ Unimplemented 阶段行为契约 ============

func TestUserServer_GetMe_MissingUserID_ReturnsNonOK(t *testing.T) {
	_, conn, cleanup := startUserTestServer(t)
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)
	_, err := client.GetMe(context.Background(), &emotionuser.GetMeRequest{})
	require.Error(t, err)
	st, _ := status.FromError(err)
	assert.NotEqual(t, "OK", st.Code().String(), "期望被拦截（非 OK）")
}

func TestUserServer_UpdateProfile_RequiresUserID(t *testing.T) {
	_, conn, cleanup := startUserTestServer(t)
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	// 带 user_id metadata 但 PR-3.2 阶段仍 Unimplemented
	_, err := client.UpdateProfile(ctx, &emotionuser.UpdateProfileRequest{
		Nickname: stringPtr("test"),
	})
	if err == nil {
		t.Fatal("PR-3.2 阶段 UpdateProfile 应返 Unimplemented（PR-3.3 补完）")
	}
	st, _ := status.FromError(err)
	t.Logf("UpdateProfile code=%s msg=%s", st.Code(), st.Message())
}

func TestUserServer_GetUserById_NoUserIDRequired(t *testing.T) {
	// GetUserById 是按 user_id 查公开资料，端点本身不强制 x-user-id metadata。
	// 但当前 user id 拦截器对所有 RPC 强制要求 metadata（chat-svc PR-GRPC-2 同模式）。
	// PR-3.2 阶段：带 metadata 时应走到 logic 层（Unimplemented）；
	// 不带 metadata 时被拦截器拒绝（Unauthenticated）。
	_, conn, cleanup := startUserTestServer(t)
	defer cleanup()

	client := emotionuser.NewUserServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "7"))

	_, err := client.GetUserById(ctx, &emotionuser.GetUserByIdRequest{
		UserId: 100,
	})
	if err == nil {
		t.Fatal("PR-3.2 阶段 GetUserById 应返 Unavailable（svcCtx 未注入，PR-3.3 阶段补完）")
	}
	st, _ := status.FromError(err)
	// chat-svc PR-GRPC-2 同模式：拦截器通过 + svcCtx=nil → Unavailable
	assert.Equal(t, "Unavailable", st.Code().String(), "PR-3.2 阶段期望 Unavailable（svcCtx 未注入）")
}

// ============ 单元方法：type / New 存在性 ============

func TestUserServer_TypeImplement(t *testing.T) {
	var _ emotionuser.UserServiceServer = (*userServer)(nil)
}

func TestUserServer_New_ReturnsNonNil(t *testing.T) {
	srv := New(nil, 8887)
	require.NotNil(t, srv)
	assert.Equal(t, 8887, srv.port)
}

func TestUserServer_PortPropagated(t *testing.T) {
	srv := New(nil, 9091)
	assert.Equal(t, 9091, srv.port)
}

// ============ Message types ============

func TestUserServer_MessageTypesExist(t *testing.T) {
	assert.NotNil(t, &emotionuser.UserInfo{})
	assert.NotNil(t, &emotionuser.UpdateProfileRequest{})
}

// helper
func stringPtr(s string) *string { return &s }