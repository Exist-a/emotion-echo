// Package downstream — user_grpc_updateprofile_test.go
//
// E2E-11：gRPC transport 下 UpdateProfile 必须透传 avatar_url。
//
// 背景（与 E2E-F-72「端点已补 ≠ 功能可用」同源）：
//   - proto user.proto 的 UpdateProfileRequest 定义 `optional string avatar_url = 2`；
//   - user-svc 侧 logic/repository 都正确落库 avatar_url 列；
//   - 但 BFF 的 userGRPCClient.UpdateMe 只映射了 Nickname + Gender，
//     **AvatarURL 被静默丢弃** ⇒ 头像上传接口返回 200，数据库 avatar_url 仍为 NULL。
//
// dev 实测（2026-09-19）：Playwright 头像上传用例断言 200 通过，
// 但 `SELECT avatar_url FROM users WHERE id=1` 仍为 NULL（has_avatar=f）。
package downstream

import (
	"context"
	"net"
	"testing"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// capturingUpdateProfileSrv 记录收到的 UpdateProfileRequest
type capturingUpdateProfileSrv struct {
	emotionuser.UnimplementedUserServiceServer
	gotAvatar   string
	gotNickname string
	calls       int
}

func (c *capturingUpdateProfileSrv) UpdateProfile(_ context.Context, req *emotionuser.UpdateProfileRequest) (*emotionuser.UserInfo, error) {
	c.calls++
	c.gotAvatar = req.GetAvatarUrl()
	c.gotNickname = req.GetNickname()
	return &emotionuser.UserInfo{
		Id:        1,
		Username:  "echo",
		Nickname:  req.GetNickname(),
		AvatarUrl: req.GetAvatarUrl(),
	}, nil
}

// newCaptureUserClient 起一个包装了 capturingUpdateProfileSrv 的 gRPC UserClient
func newCaptureUserClient(t *testing.T, srv *capturingUpdateProfileSrv) UserClient {
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

func TestUserGRPCClient_UpdateMe_PassesAvatarURL(t *testing.T) {
	srv := &capturingUpdateProfileSrv{}
	client := newCaptureUserClient(t, srv)

	avatar := "http://localhost:9000/emotion-echo/avatars/1-abc123.png"
	nick := "新昵称"
	_, err := client.UpdateMe(WithUserID(context.Background(), 1), UpdateProfileReq{
		Nickname:  &nick,
		AvatarURL: &avatar,
	})
	require.NoError(t, err)
	require.Equal(t, 1, srv.calls)

	assert.Equal(t, avatar, srv.gotAvatar,
		"E2E-11: UpdateMe 必须把 AvatarURL 映射到 proto 的 avatar_url 字段。"+
			"原实现只映射 Nickname+Gender ⇒ 头像上传接口返 200 但数据库 avatar_url 恒为 NULL。")
	assert.Equal(t, nick, srv.gotNickname, "nickname 仍应正常透传")
}

// 回归保护：AvatarURL 为 nil 时不得 panic，也不得写出空串以外的副作用
func TestUserGRPCClient_UpdateMe_NilAvatarURL_NoPanic(t *testing.T) {
	srv := &capturingUpdateProfileSrv{}
	client := newCaptureUserClient(t, srv)

	nick := "only-nick"
	_, err := client.UpdateMe(WithUserID(context.Background(), 1), UpdateProfileReq{Nickname: &nick})
	require.NoError(t, err)
	assert.Equal(t, "", srv.gotAvatar, "未传 AvatarURL 时不应写入值")
}
