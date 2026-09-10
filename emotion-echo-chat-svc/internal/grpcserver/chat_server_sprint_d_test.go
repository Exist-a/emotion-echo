// Package grpcserver — chat_server_sprint_d_test.go
//
// Sprint D RED: chat-svc gRPC 5 RPC 真实实现测试（解 #27）。
//
// 行为契约（chat-svc gRPC PR-GRPC-3 应已落地，Stage 58 失真后 Sprint D 收口）：
//   - 5 RPC（SendMessage / ListMessages / ListConversations / DeleteConversation / PinConversation）
//     不再返 codes.Unimplemented，而是真正调到 logic 层
//   - ListConversations 真实返回 list（即使空）
//   - DeleteConversation 真实返 success=true（空仓 mock repo）
//
// StreamMessages 不在本 sprint 范围（架构判断见 chat_server.go:124）。
//
// 测试策略：
//   - startChatTestServerWithCtx 注入 mock svcCtx（ConversationRepo + EventPublisher）
//   - 用 InMemory repo（同 logic 测试）保证无 DB 依赖
//   - metadata x-user-id 模拟 APISIX 注入
package grpcserver

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// startChatTestServerWithCtx 注入 mock svcCtx 启 server（与 startChatTestServer 不同）
func startChatTestServerWithCtx(t *testing.T, svcCtx *svc.ServiceContext) (*grpc.ClientConn, func()) {
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

// newMockSvcCtx 构造带 InMemory repo 的 svcCtx（与 logic 测试同源）
func newMockSvcCtx() *svc.ServiceContext {
	repo := repository.NewInMemoryConversationRepo()
	pub := events.NewInMemoryEventPublisher()
	return &svc.ServiceContext{
		ConversationRepo: repo,
		EventPublisher:   pub,
	}
}

// ============ Sprint D RED 测试 ============

// TestChatServer_ListConversations_NotUnimplemented 断言调用 ListConversations 不再返 Unimplemented。
//
// 当前（Sprint D 之前）：返 codes.Unimplemented "ListConversations: PR-GRPC-3 阶段补完"
// 期望（Sprint D 之后）：正常返回（即使 list 空，code=OK）
func TestChatServer_ListConversations_NotUnimplemented(t *testing.T) {
	conn, cleanup := startChatTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionchat.NewChatServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "1"))

	resp, err := client.ListConversations(ctx, &emotionchat.ListConversationsRequest{
		Limit:  20,
		Offset: 0,
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"ListConversations 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
	require.NotNil(t, resp, "ListConversations 应返非 nil response（即使 list 空）")
	assert.Equal(t, 0, len(resp.List), "空用户应返空 list")
}

// TestChatServer_SendMessage_NotUnimplemented
func TestChatServer_SendMessage_NotUnimplemented(t *testing.T) {
	conn, cleanup := startChatTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionchat.NewChatServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "1"))

	_, err := client.SendMessage(ctx, &emotionchat.SendMessageRequest{
		ConversationId: 1,
		Role:           "user",
		Content:        "hello",
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"SendMessage 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
}

// TestChatServer_ListMessages_NotUnimplemented
func TestChatServer_ListMessages_NotUnimplemented(t *testing.T) {
	conn, cleanup := startChatTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionchat.NewChatServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "1"))

	_, err := client.ListMessages(ctx, &emotionchat.ListMessagesRequest{
		ConversationId: 1,
		Limit:          50,
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"ListMessages 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
}

// TestChatServer_DeleteConversation_NotUnimplemented
func TestChatServer_DeleteConversation_NotUnimplemented(t *testing.T) {
	conn, cleanup := startChatTestServerWithCtx(t, newMockSvcCtx())
	defer cleanup()

	client := emotionchat.NewChatServiceClient(conn)
	ctx := metadata.NewOutgoingContext(context.Background(),
		metadata.Pairs("x-user-id", "1"))

	_, err := client.DeleteConversation(ctx, &emotionchat.DeleteConversationRequest{
		ConversationId: 999,
	})
	if err != nil {
		st, _ := status.FromError(err)
		assert.NotEqual(t, codes.Unimplemented, st.Code(),
			"DeleteConversation 不应返 Unimplemented，实际 code=%s msg=%s", st.Code(), st.Message())
	}
}

// PinConversation 不在本 sprint 范围（chat-svc 当前无 pin 功能，无 model 字段、无 repo 方法、无 HTTP 端点）。
// 保留 Unimplemented 是正确设计（proto 留接口，业务未触发），不是 bug。
// BFF 端 PinConversation 当前也未调用（chat.go:11 注释"下游尚未实现"）。
//
// 如未来需要置顶功能，应作为独立 PR 加 ConversationRepo.Pin 方法 + PinConversationLogic + DB schema
// migration，而不是在 Sprint D 里"为了去掉 Unimplemented 而去掉 Unimplemented"。
