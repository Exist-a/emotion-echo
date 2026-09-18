// Package downstream — user_grpc_security_answer_test.go
//
// R-01 #2 真修通：验证 BFF 走 **gRPC transport** 时也能按用户名校验密保答案。
//
// 背景（E2E-F-60 的第二层根因）：
//   - BFF 默认 transport = gRPC（user_grpc.go），dev compose 亦然；
//   - 但 userGRPCClient.VerifySecurityAnswerByUsername 原先只是
//     `return fmt.Errorf("... not implemented for gRPC transport, use HTTP")` 的桩；
//   - 于是"端点已补、APISIX 已白名单"之后，找回密码**仍然恒 401** ——
//     HTTP transport 的客户端与服务端都通了，但默认走的 gRPC 是空实现。
//   本组用例用 bufconn 真 gRPC server 把该 transport 钉住。
package downstream

import (
	"context"
	"net"
	"testing"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// fakeUserSecurityAnswerSrv 记录收到的请求并按其「已知用户 + 已知答案」作答。
type fakeUserSecurityAnswerSrv struct {
	emotionuser.UnimplementedUserServiceServer

	knownUser   string
	knownAnswer string

	// 记录最后一次收到的请求，用于断言参数透传
	gotUsername      string
	gotQuestionOrder int32
	gotAnswer        string
	calls            int
}

func (f *fakeUserSecurityAnswerSrv) VerifySecurityAnswerByUsername(_ context.Context, req *emotionuser.VerifySecurityAnswerByUsernameRequest) (*emotionuser.VerifySecurityAnswerByUsernameResponse, error) {
	f.calls++
	f.gotUsername = req.GetUsername()
	f.gotQuestionOrder = req.GetQuestionOrder()
	f.gotAnswer = req.GetAnswer()

	// 用户不存在与答案错误返回同一种码（防用户名枚举，与 user-svc 语义一致）
	if req.GetUsername() != f.knownUser || req.GetAnswer() != f.knownAnswer {
		return nil, status.Error(codes.PermissionDenied, "security answer mismatch")
	}
	return &emotionuser.VerifySecurityAnswerByUsernameResponse{Success: true}, nil
}

func startFakeUserGRPCServer(t *testing.T, srv *fakeUserSecurityAnswerSrv) UserClient {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	gs := grpc.NewServer()
	emotionuser.RegisterUserServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.DialContext(context.Background(), lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return NewUserGRPCClient(conn)
}

// TestUserGRPCClient_VerifySecurityAnswerByUsername_Success 是本组最核心的一条：
// 答案正确必须返回 nil。若 gRPC 侧退回"未实现"桩，本用例立即变红。
func TestUserGRPCClient_VerifySecurityAnswerByUsername_Success(t *testing.T) {
	srv := &fakeUserSecurityAnswerSrv{knownUser: "alice", knownAnswer: "blue"}
	c := startFakeUserGRPCServer(t, srv)

	err := c.VerifySecurityAnswerByUsername(context.Background(), "alice", 1, "blue")

	require.NoError(t, err, "gRPC transport 必须真的实现该 RPC，不能又是 not-implemented 桩")
	assert.Equal(t, 1, srv.calls, "应真的发起了一次 RPC")
	assert.Equal(t, "alice", srv.gotUsername)
	assert.Equal(t, int32(1), srv.gotQuestionOrder)
	assert.Equal(t, "blue", srv.gotAnswer)
}

func TestUserGRPCClient_VerifySecurityAnswerByUsername_WrongAnswer_ReturnsError(t *testing.T) {
	srv := &fakeUserSecurityAnswerSrv{knownUser: "alice", knownAnswer: "blue"}
	c := startFakeUserGRPCServer(t, srv)

	err := c.VerifySecurityAnswerByUsername(context.Background(), "alice", 1, "WRONG")

	require.Error(t, err, "答案错误必须返回 error（BFF handler 才会转 401）")
	assert.Contains(t, err.Error(), "security answer mismatch", "服务端错误应透传")
}

func TestUserGRPCClient_VerifySecurityAnswerByUsername_UnknownUser_ReturnsError(t *testing.T) {
	srv := &fakeUserSecurityAnswerSrv{knownUser: "alice", knownAnswer: "blue"}
	c := startFakeUserGRPCServer(t, srv)

	err := c.VerifySecurityAnswerByUsername(context.Background(), "nobody", 1, "blue")

	require.Error(t, err, "用户不存在必须返回 error（防枚举，绝不返回成功）")
	// 关键：断言"确实发起了 RPC"。否则该用例在 not-implemented 桩下也会
	// 因为"拿到的就是 error"而假通过 —— 那就抓不到 transport 未实现。
	assert.Equal(t, 1, srv.calls, "必须真的把请求发到服务端，而不是本地桩直接报错")
}
