// Package grpcserver — chat_server_stage72_test.go
//
// Stage 72 RED: PinConversation / UpdateConversation 真实实现测试
// （决策 4 ADR §八 backlog 收口，docs/plans/backlog-order-2026-09-12.md 项 1）。
//
// 行为契约：
//   - PinConversation：置顶/取消置顶自己的会话；owner 校验；NotFound；pinned 持久化
//   - UpdateConversation：改自己的会话标题；owner 校验；NotFound；空标题 InvalidArgument
//
// 之前状态：PinConversation 占位返 Unimplemented（chat_server.go:212-217）；
// UpdateConversation 不存在（proto 未定义，走 UnimplementedChatServiceServer 兜底）。
//
// 测试策略：与 Sprint D 同源 —— InMemory repo + InMemory publisher，无 DB 依赖。
package grpcserver

import (
	"context"
	"testing"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// newStage72SvcCtx 构造 repo 与 svcCtx 同源引用的测试环境
func newStage72SvcCtx() (*repository.InMemoryConversationRepo, *svc.ServiceContext) {
	repo := repository.NewInMemoryConversationRepo()
	svcCtx := &svc.ServiceContext{
		ConversationRepo: repo,
		EventPublisher:   events.NewInMemoryEventPublisher(),
	}
	return repo, svcCtx
}

// seedConversation 直接往 repo 放一条会话，返回会话 ID
func seedConversation(t *testing.T, repo *repository.InMemoryConversationRepo, userID int64, title string) int64 {
	t.Helper()
	c := &model.Conversation{UserID: userID, Title: title}
	require.NoError(t, repo.CreateConversation(context.Background(), c))
	return c.ID
}

// TestChatServer_PinConversation 表驱动：置顶/取消置顶/越权/不存在/缺鉴权
func TestChatServer_PinConversation(t *testing.T) {
	tests := []struct {
		name       string
		seedUser   int64 // 被操作会话的 owner
		callUser   string
		isPinned   bool
		wantCode   codes.Code
		wantPinned bool
	}{
		{name: "pin owned conversation", seedUser: 1, callUser: "1", isPinned: true, wantCode: codes.OK, wantPinned: true},
		{name: "unpin owned conversation", seedUser: 1, callUser: "1", isPinned: false, wantCode: codes.OK, wantPinned: false},
		{name: "not owner", seedUser: 1, callUser: "2", isPinned: true, wantCode: codes.PermissionDenied},
		{name: "conversation not found", seedUser: 1, callUser: "1", isPinned: true, wantCode: codes.NotFound},
		{name: "missing x-user-id", seedUser: 1, callUser: "", isPinned: true, wantCode: codes.Unauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, svcCtx := newStage72SvcCtx()
			var convID int64 = 999
			if tt.name != "conversation not found" {
				convID = seedConversation(t, repo, tt.seedUser, "conv")
			}

			conn, cleanup := startChatTestServerWithCtx(t, svcCtx)
			defer cleanup()
			client := emotionchat.NewChatServiceClient(conn)

			ctx := context.Background()
			if tt.callUser != "" {
				ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-user-id", tt.callUser))
			}
			resp, err := client.PinConversation(ctx, &emotionchat.PinConversationRequest{
				ConversationId: convID,
				IsPinned:       tt.isPinned,
			})
			st, _ := status.FromError(err)
			require.Equal(t, tt.wantCode, st.Code(), "code=%s msg=%s", st.Code(), st.Message())
			if tt.wantCode != codes.OK {
				return
			}
			require.NotNil(t, resp)
			assert.True(t, resp.Success)
			assert.Equal(t, convID, resp.Id)
			assert.Equal(t, tt.wantPinned, resp.IsPinned)

			got, err := repo.GetConversationByID(context.Background(), convID)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantPinned, got.Pinned, "repo 中 pinned 状态应持久化")
		})
	}
}

// TestChatServer_UpdateConversation 表驱动：改名/空标题/越权/不存在/缺鉴权
func TestChatServer_UpdateConversation(t *testing.T) {
	tests := []struct {
		name      string
		seedUser  int64
		callUser  string
		newTitle  string
		wantCode  codes.Code
		wantTitle string
	}{
		{name: "rename owned conversation", seedUser: 1, callUser: "1", newTitle: "renamed", wantCode: codes.OK, wantTitle: "renamed"},
		{name: "empty title rejected", seedUser: 1, callUser: "1", newTitle: "", wantCode: codes.InvalidArgument},
		{name: "not owner", seedUser: 1, callUser: "2", newTitle: "hacked", wantCode: codes.PermissionDenied},
		{name: "conversation not found", seedUser: 1, callUser: "1", newTitle: "x", wantCode: codes.NotFound},
		{name: "missing x-user-id", seedUser: 1, callUser: "", newTitle: "x", wantCode: codes.Unauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, svcCtx := newStage72SvcCtx()
			var convID int64 = 999
			if tt.name != "conversation not found" {
				convID = seedConversation(t, repo, tt.seedUser, "original")
			}

			conn, cleanup := startChatTestServerWithCtx(t, svcCtx)
			defer cleanup()
			client := emotionchat.NewChatServiceClient(conn)

			ctx := context.Background()
			if tt.callUser != "" {
				ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("x-user-id", tt.callUser))
			}
			resp, err := client.UpdateConversation(ctx, &emotionchat.UpdateConversationRequest{
				ConversationId: convID,
				Title:          tt.newTitle,
			})
			st, _ := status.FromError(err)
			require.Equal(t, tt.wantCode, st.Code(), "code=%s msg=%s", st.Code(), st.Message())
			if tt.wantCode != codes.OK {
				return
			}
			require.NotNil(t, resp)
			assert.True(t, resp.Success)
			assert.Equal(t, convID, resp.Id)
			assert.Equal(t, tt.wantTitle, resp.Title)

			got, err := repo.GetConversationByID(context.Background(), convID)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.wantTitle, got.Title, "repo 中标题应已更新")
		})
	}
}
