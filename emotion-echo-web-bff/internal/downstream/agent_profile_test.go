// Package downstream — agent_profile_test.go
//
// CL-06 · Phase 4（BFF 段）：BFF → user-svc **角色档案** gRPC 客户端契约。
//
// 为什么单独一个窄接口（而非塞进 UserClient）：UserClient 的 3 个测试替身
// （user_handler_test / auth_handler_test / avatar_handler_test）会被迫实现
// 与自身无关的 3 个角色方法；窄接口只让真正需要它的装配点感知（本仓既有先例：
// titleConvUpdater / fileMessageLister / personalitySource）。
//
// 测试策略与 chat_grpc_test.go 一致：bufconn 内存连接 + mock UserService +
// 真实 grpc.ClientConn ⇒ happy / 错误 / nil-conn 三类路径，全离线可跑。
package downstream

import (
	"context"
	"errors"
	"net"
	"testing"

	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// mockAgentServer 只实现 CL-06 的 3 个角色 RPC（其余走 Unimplemented 兜底）
type mockAgentServer struct {
	emotionuser.UnimplementedUserServiceServer

	listResp   *emotionuser.ListAgentProfilesResponse
	listErr    error
	upsertResp *emotionuser.AgentProfile
	upsertErr  error
	deleteErr  error

	gotUserID  string
	gotUpsert  *emotionuser.UpsertAgentProfileRequest
	gotDeleted int64
}

func (m *mockAgentServer) ListAgentProfiles(ctx context.Context, _ *emotionuser.ListAgentProfilesRequest) (*emotionuser.ListAgentProfilesResponse, error) {
	m.gotUserID = userIDFromMD(ctx)
	return m.listResp, m.listErr
}

func (m *mockAgentServer) UpsertAgentProfile(ctx context.Context, req *emotionuser.UpsertAgentProfileRequest) (*emotionuser.AgentProfile, error) {
	m.gotUserID = userIDFromMD(ctx)
	m.gotUpsert = req
	return m.upsertResp, m.upsertErr
}

func (m *mockAgentServer) DeleteAgentProfile(ctx context.Context, req *emotionuser.DeleteAgentProfileRequest) (*emotionuser.DeleteAgentProfileResponse, error) {
	m.gotUserID = userIDFromMD(ctx)
	m.gotDeleted = req.GetId()
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return &emotionuser.DeleteAgentProfileResponse{Deleted: true}, nil
}

func userIDFromMD(ctx context.Context) string {
	md, _ := metadata.FromIncomingContext(ctx)
	if vals := md.Get("x-user-id"); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// newAgentProfileClient 起 bufconn mock server 并返回连上它的真实客户端
func newAgentProfileClient(t *testing.T, srv *mockAgentServer) AgentProfileClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	emotionuser.RegisterUserServiceServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	cli := NewAgentProfileGRPCClient(conn)
	require.NotNil(t, cli)
	return cli
}

func TestAgentProfileGRPCClient_NilConn_ReturnsNil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, NewAgentProfileGRPCClient(nil), "nil conn 必须返 nil（与 UserGRPCClient 同纪律）")
}

func TestListAgentProfiles_MapsItemsAndSendsUserID(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{listResp: &emotionuser.ListAgentProfilesResponse{
		Total: 2,
		Items: []*emotionuser.AgentProfile{
			{
				Id: 7, UserId: 42, Name: "小E", Gender: "female",
				Persona: "温柔、愿意倾听的陪伴者。", Tone: "温柔、口语化",
				Boundaries: []string{"不要说教"}, Voice: "FunAudioLLM/CosyVoice2-0.5B:anna",
				IsDefault: true, CreatedAt: 1700000000, UpdatedAt: 1700000001,
			},
			{Id: 9, UserId: 42, Name: "阿哲", Persona: "直率的朋友", Voice: "FunAudioLLM/CosyVoice2-0.5B:alex"},
		},
	}}
	cli := newAgentProfileClient(t, srv)

	got, err := cli.ListAgentProfiles(WithUserID(context.Background(), 42))
	require.NoError(t, err)
	require.Len(t, got, 2)

	assert.Equal(t, int64(7), got[0].ID)
	assert.Equal(t, int64(42), got[0].UserID)
	assert.Equal(t, "小E", got[0].Name)
	assert.Equal(t, "温柔、愿意倾听的陪伴者。", got[0].Persona)
	assert.Equal(t, "温柔、口语化", got[0].Tone)
	assert.Equal(t, []string{"不要说教"}, got[0].Boundaries)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:anna", got[0].Voice)
	assert.True(t, got[0].IsDefault)
	assert.Equal(t, int64(1700000000), got[0].CreatedAt)
	assert.Equal(t, int64(1700000001), got[0].UpdatedAt)

	assert.Equal(t, int64(9), got[1].ID)
	assert.False(t, got[1].IsDefault)
	assert.Empty(t, got[1].Boundaries, "无 boundaries 时必须是空切片语义（nil 可接受）")

	assert.Equal(t, "42", srv.gotUserID, "鉴权必须走 metadata x-user-id")
}

func TestListAgentProfiles_UpstreamError_Wrapped(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{listErr: status.Error(codes.Internal, "boom")}
	cli := newAgentProfileClient(t, srv)

	_, err := cli.ListAgentProfiles(WithUserID(context.Background(), 42))
	require.Error(t, err)
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr), "必须包成 *APIError（handler 用 statusFor 判码）")
	assert.Contains(t, err.Error(), "list agent profiles")
}

func TestUpsertAgentProfile_Create_PassesZeroIDAndMapsResponse(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{upsertResp: &emotionuser.AgentProfile{
		Id: 11, UserId: 42, Name: "阿哲", Persona: "直率的朋友", Tone: "直接",
		Boundaries: []string{"不评价外貌", "不催他"}, Voice: "FunAudioLLM/CosyVoice2-0.5B:alex",
	}}
	cli := newAgentProfileClient(t, srv)

	got, err := cli.UpsertAgentProfile(WithUserID(context.Background(), 42), AgentProfileUpsertReq{
		Name: "阿哲", Persona: "直率的朋友", Tone: "直接",
		Boundaries: []string{"不评价外貌", "不催他"}, Voice: "FunAudioLLM/CosyVoice2-0.5B:alex",
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(11), got.ID)
	assert.Equal(t, "阿哲", got.Name)

	require.NotNil(t, srv.gotUpsert)
	assert.Equal(t, int64(0), srv.gotUpsert.GetId(), "id=0 ⇒ 新建（不是更新）")
	assert.Equal(t, []string{"不评价外貌", "不催他"}, srv.gotUpsert.GetBoundaries())
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B:alex", srv.gotUpsert.GetVoice())
	assert.Equal(t, "42", srv.gotUserID)
}

func TestUpsertAgentProfile_Update_PassesID(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{upsertResp: &emotionuser.AgentProfile{Id: 11, Name: "阿哲2"}}
	cli := newAgentProfileClient(t, srv)

	_, err := cli.UpsertAgentProfile(WithUserID(context.Background(), 42), AgentProfileUpsertReq{ID: 11, Name: "阿哲2"})
	require.NoError(t, err)
	require.NotNil(t, srv.gotUpsert)
	assert.Equal(t, int64(11), srv.gotUpsert.GetId())
}

func TestUpsertAgentProfile_UpstreamError_Wrapped(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{upsertErr: status.Error(codes.InvalidArgument, "name too long")}
	cli := newAgentProfileClient(t, srv)

	_, err := cli.UpsertAgentProfile(WithUserID(context.Background(), 42), AgentProfileUpsertReq{Name: "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert agent profile")
	var apiErr *APIError
	assert.True(t, errors.As(err, &apiErr))
}

func TestDeleteAgentProfile_PassesIDAndSucceeds(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{}
	cli := newAgentProfileClient(t, srv)

	require.NoError(t, cli.DeleteAgentProfile(WithUserID(context.Background(), 42), 11))
	assert.Equal(t, int64(11), srv.gotDeleted)
	assert.Equal(t, "42", srv.gotUserID)
}

func TestDeleteAgentProfile_UpstreamError_Wrapped(t *testing.T) {
	t.Parallel()
	srv := &mockAgentServer{deleteErr: status.Error(codes.NotFound, "no such agent")}
	cli := newAgentProfileClient(t, srv)

	err := cli.DeleteAgentProfile(WithUserID(context.Background(), 42), 999)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete agent profile")
	var apiErr *APIError
	assert.True(t, errors.As(err, &apiErr))
}
