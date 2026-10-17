// Package downstream — chat_grpc_test.go
//
// Stage 58 PR-GRPC-4: BFF → chat-svc gRPC client 单元测试
//
// 行为契约：
//   - ChatGRPCClient 实现 ChatClient interface
//   - CreateConversation 调 CreateConversation RPC，把 proto.Conversation 转
//     downstream.ConversationView（含时间字段 ms）
//   - SendMessage / ListMessages / ListConversations / DeleteConversation 同款
//   - PinConversation / StreamMessages PR-GRPC-5 阶段补完（占位）
//   - 鉴权：user_id 通过 metadata x-user-id 传（grpcinterceptor.UserIDFromGRPCContext
//     兼容 HTTP middleware 注入的 user id → outgoing metadata）
//
// 测试策略：
//   - 用 grpc-go test/bufconn 在内存中连（避免依赖真 TCP + 真 chat-svc）
//   - 启动 mock server（bufconn）+ 真实 grpc.ClientConn + 真实 ChatGRPCClient
//   - 验证 happy / invalid arg / 不存在 3 类路径
package downstream

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// mockChatServer 在 bufconn 上提供 ChatService 实现（仅最小 happy path）
type mockChatServer struct {
	emotionchat.UnimplementedChatServiceServer

	// 注入响应（测试可控）
	convResp  *emotionchat.Conversation
	msgResp   *emotionchat.Message
	listConv  *emotionchat.ListConversationsResponse
	listMsg   *emotionchat.ListMessagesResponse
	delOK     bool

	// 记录收到的 metadata + req
	gotUserID string
	// CL-06：UpdateConversation 收到的请求（断言 title 与 agent_id **两字段都透传**）
	gotUpdateReq *emotionchat.UpdateConversationRequest
	updateErr    error
}

func (m *mockChatServer) CreateConversation(ctx context.Context, req *emotionchat.CreateConversationRequest) (*emotionchat.Conversation, error) {
	// 模拟拦截器行为：读 metadata x-user-id
	md, _ := metadata.FromIncomingContext(ctx)
	if vals := md.Get("x-user-id"); len(vals) > 0 {
		m.gotUserID = vals[0]
	}
	return m.convResp, nil
}

func (m *mockChatServer) SendMessage(ctx context.Context, req *emotionchat.SendMessageRequest) (*emotionchat.Message, error) {
	return m.msgResp, nil
}

func (m *mockChatServer) ListConversations(ctx context.Context, req *emotionchat.ListConversationsRequest) (*emotionchat.ListConversationsResponse, error) {
	return m.listConv, nil
}

func (m *mockChatServer) ListMessages(ctx context.Context, req *emotionchat.ListMessagesRequest) (*emotionchat.ListMessagesResponse, error) {
	return m.listMsg, nil
}

func (m *mockChatServer) DeleteConversation(ctx context.Context, req *emotionchat.DeleteConversationRequest) (*emotionchat.DeleteConversationResponse, error) {
	return &emotionchat.DeleteConversationResponse{Success: m.delOK, Id: req.ConversationId}, nil
}

func (m *mockChatServer) PinConversation(ctx context.Context, req *emotionchat.PinConversationRequest) (*emotionchat.PinConversationResponse, error) {
	return &emotionchat.PinConversationResponse{Success: true, Id: req.ConversationId}, nil
}

// UpdateConversation CL-06：记录请求体，并回显 title / agent_id
func (m *mockChatServer) UpdateConversation(ctx context.Context, req *emotionchat.UpdateConversationRequest) (*emotionchat.UpdateConversationResponse, error) {
	m.gotUpdateReq = req
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return &emotionchat.UpdateConversationResponse{
		Success: true,
		Id:      req.GetConversationId(),
		Title:   req.GetTitle(),
		AgentId: req.GetAgentId(),
	}, nil
}

// startMockBufConn 启动 bufconn 上的 mock gRPC server，返回 client conn
func startMockBufConn(t *testing.T, mock *mockChatServer) (*grpc.ClientConn, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	emotionchat.RegisterChatServiceServer(srv, mock)
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("mock server stopped: %v", err)
		}
	}()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return conn, cleanup
}

// ============ 测试用例 ============

func TestChatGRPCClient_CreateConversation_Success(t *testing.T) {
	mock := &mockChatServer{
		convResp: &emotionchat.Conversation{
			Id:        42,
			UserId:    7,
			Title:     "test",
			MsgCount:  0,
			Status:    0,
			CreatedAt: time.Now().UnixMilli(),
			UpdatedAt: time.Now().UnixMilli(),
		},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn)
	require.NotNil(t, client)

	conv, err := client.CreateConversation(context.Background(), CreateConversationReq{
		Title: "test",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(42), conv.ID)
	assert.Equal(t, int64(7), conv.UserID)
	assert.Equal(t, "test", conv.Title)
	assert.Equal(t, 0, conv.MsgCount)
}

func TestChatGRPCClient_SendMessage_Success(t *testing.T) {
	mock := &mockChatServer{
		msgResp: &emotionchat.Message{
			Id:             100,
			ConversationId: 42,
			UserId:         7,
			Role:           "user",
			Content:        "hello",
			TokensUsed:     5,
			CreatedAt:      time.Now().UnixMilli(),
		},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn)
	msg, err := client.SendMessage(context.Background(), 42, SendMessageReq{
		Role:    "user",
		Content: "hello",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(100), msg.ID)
	assert.Equal(t, int64(42), msg.ConversationID)
	assert.Equal(t, "hello", msg.Content)
}

func TestChatGRPCClient_ListMessages_ReturnsViews(t *testing.T) {
	mock := &mockChatServer{
		listMsg: &emotionchat.ListMessagesResponse{
			Messages: []*emotionchat.Message{
				{Id: 1, ConversationId: 5, UserId: 7, Role: "user", Content: "hi"},
				{Id: 2, ConversationId: 5, UserId: 7, Role: "assistant", Content: "hello!"},
			},
		},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn)
	msgs, _, err := client.ListMessages(context.Background(), 5, 50, 0)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "hi", msgs[0].Content)
	assert.Equal(t, "hello!", msgs[1].Content)
}

func TestChatGRPCClient_ListConversations_ReturnsViewsAndHasMore(t *testing.T) {
	mock := &mockChatServer{
		listConv: &emotionchat.ListConversationsResponse{
			List: []*emotionchat.Conversation{
				{Id: 1, UserId: 7, Title: "a", MsgCount: 3},
				{Id: 2, UserId: 7, Title: "b", MsgCount: 0},
			},
			HasMore:    true,
			NextCursor: "1760594400123456:2", // CL-05 Phase 5
		},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn)
	page, err := client.ListConversations(context.Background(), 20, 0, "")
	require.NoError(t, err)
	assert.Len(t, page.List, 2)
	assert.True(t, page.HasMore)
	assert.Equal(t, "a", page.List[0].Title)
	assert.Equal(t, "1760594400123456:2", page.NextCursor,
		"chat-svc 给出的不透明游标必须被 BFF **原样回传**（BFF 不解析游标内容）")
}

func TestChatGRPCClient_DeleteConversation_Success(t *testing.T) {
	mock := &mockChatServer{delOK: true}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn)
	err := client.DeleteConversation(context.Background(), 99)
	assert.NoError(t, err)
}

func TestChatGRPCClient_NilConn_ReturnsNilClient(t *testing.T) {
	client := NewChatGRPCClient(nil)
	assert.Nil(t, client, "nil conn 应返 nil client（与 HTTP 客户端对称）")
}

// CL-02 归档（2026-10-14，用户裁定 D2）：StreamMessages client 方法已删除
// （不在 ChatClient 接口内、零调用；proto 侧 RPC 保留但标 deprecated）⇒ 其测试移除。

// ============ ctx user_id 桥接（Stage 63 收口）============
//
// 行为契约：
//   - handler 通过 session.WithRequestAuth → downstream.WithUserID(ctx, uid) 注入 user_id
//   - gRPC client 调任何 RPC 时必须把 ctx 里的 user_id 写到 outgoing metadata x-user-id
//   - 否则 chat-svc userid 拦截器返 Unauthenticated
//
// bug 背景：chat_grpc.go 私有类型 userIDKey{} 与 downstream.WithUserID 用的 userIDCtxKey{}
// 不兼容，导致 ctx.Value 永远 miss → metadata 不注入 → 401。本测试 RED 阶段。
func TestChatGRPCClient_WithUserIDFromDownstream_InjectsMetadata(t *testing.T) {
	mock := &mockChatServer{
		convResp: &emotionchat.Conversation{Id: 1, Title: "x", UserId: 42},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn).(*chatGRPCClient)
	require.NotNil(t, client)

	ctx := WithUserID(context.Background(), 42)
	_, err := client.CreateConversation(ctx, CreateConversationReq{Title: "t"})
	require.NoError(t, err)
	assert.Equal(t, "42", mock.gotUserID, "metadata x-user-id 必须从下游 ctx 取出并注入")
}

func TestChatGRPCClient_NoUserID_NoMetadata(t *testing.T) {
	mock := &mockChatServer{
		convResp: &emotionchat.Conversation{Id: 1, Title: "x"},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	client := NewChatGRPCClient(conn).(*chatGRPCClient)
	_, err := client.CreateConversation(context.Background(), CreateConversationReq{Title: "t"})
	require.NoError(t, err)
	assert.Empty(t, mock.gotUserID, "ctx 无 user_id 时不应注入 metadata（让拦截器判 Unauthenticated）")
}

// ============ CL-06：角色字段透传 ============

// 会话视图必须带 agent_id —— 前端切换器要靠它显示"本会话当前角色"。
func TestChatGRPCClient_Conversation_CarriesAgentID(t *testing.T) {
	mock := &mockChatServer{
		convResp: &emotionchat.Conversation{Id: 42, UserId: 7, Title: "t", AgentId: 9},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	conv, err := NewChatGRPCClient(conn).CreateConversation(context.Background(), CreateConversationReq{Title: "t"})
	require.NoError(t, err)
	assert.Equal(t, int64(9), conv.AgentID, "proto Conversation.agent_id=9 必须映到 ConversationView.AgentID")
}

// 消息视图必须带 agent_id —— 前端历史消息"这条是谁回的"靠它。
func TestChatGRPCClient_Message_CarriesAgentID(t *testing.T) {
	mock := &mockChatServer{
		msgResp: &emotionchat.Message{Id: 100, ConversationId: 42, Role: "assistant", Content: "hi", AgentId: 9},
	}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	msg, err := NewChatGRPCClient(conn).SendMessage(context.Background(), 42, SendMessageReq{Role: "assistant", Content: "hi"})
	require.NoError(t, err)
	assert.Equal(t, int64(9), msg.AgentID, "proto Message.agent_id=11 必须映到 MessageView.AgentID")
}

// 更新会话：title 与 agent_id 是**同一个 RPC 的两个可选动作**，两字段都必须透传。
func TestChatGRPCClient_UpdateConversation_PassesTitleAndAgentID(t *testing.T) {
	mock := &mockChatServer{}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	resp, err := NewChatGRPCClient(conn).UpdateConversation(context.Background(), 42, "新标题", 9)
	require.NoError(t, err)
	require.NotNil(t, mock.gotUpdateReq)
	assert.Equal(t, int64(42), mock.gotUpdateReq.GetConversationId())
	assert.Equal(t, "新标题", mock.gotUpdateReq.GetTitle())
	assert.Equal(t, int64(9), mock.gotUpdateReq.GetAgentId())
	assert.True(t, resp.Success)
	assert.Equal(t, "新标题", resp.Title)
	assert.Equal(t, int64(9), resp.AgentID, "响应要回显生效后的角色（前端据此直接更新本地态）")
}

// 只切角色（不改标题）：title 必须留空 —— 否则会把标题清成 ""。
func TestChatGRPCClient_UpdateConversation_AgentOnly_LeavesTitleEmpty(t *testing.T) {
	mock := &mockChatServer{}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	_, err := NewChatGRPCClient(conn).UpdateConversation(context.Background(), 42, "", 9)
	require.NoError(t, err)
	require.NotNil(t, mock.gotUpdateReq)
	assert.Empty(t, mock.gotUpdateReq.GetTitle(), "空 title = 不改标题（chat-svc 侧按空串跳过）")
	assert.Equal(t, int64(9), mock.gotUpdateReq.GetAgentId())
}

func TestChatGRPCClient_UpdateConversation_UpstreamError_Wrapped(t *testing.T) {
	mock := &mockChatServer{updateErr: status.Error(codes.NotFound, "no such conversation")}
	conn, cleanup := startMockBufConn(t, mock)
	defer cleanup()

	_, err := NewChatGRPCClient(conn).UpdateConversation(context.Background(), 42, "t", 9)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update conversation")
	var apiErr *APIError
	assert.True(t, errors.As(err, &apiErr))
}

// ============ 类型断言 ============

func TestChatGRPCClient_ImplementsChatClient(t *testing.T) {
	// 编译期校验：ChatGRPCClient 满足 ChatClient interface
	var _ ChatClient = (*chatGRPCClient)(nil)
}