// Package downstream — chat_grpc.go
//
// Stage 58 PR-GRPC-4: BFF → chat-svc gRPC client（替换 chatHTTPClient）
//
// 设计：
//   - ChatClient interface 不变（chat.go 已定义 7 个方法）
//   - chatGRPCClient 实现 ChatClient；保留 chatHTTPClient 作为 feature flag fallback
//   - feature flag CHAT_TRANSPORT=grpc（默认）| http
//     通过 NewChatClient + CHAT_TRANSPORT env 切换（PR-GRPC-4 阶段：默认 grpc）
//
// proto 类型：直接复用 shared/pkg/emotionchat 生成代码
// 鉴权：metadata x-user-id（与 emotion_query.proto / ai-svc 拦截器一致）
package downstream

import (
	"context"
	"fmt"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ChatGRPCClient 是 ChatClient 的 gRPC 实现
type chatGRPCClient struct {
	conn *grpc.ClientConn
}

// NewChatGRPCClient 构造（需已建立的 gRPC 连接；conn=nil → 返 nil，让 fallback 到 HTTP）
func NewChatGRPCClient(conn *grpc.ClientConn) ChatClient {
	if conn == nil {
		return nil
	}
	return &chatGRPCClient{conn: conn}
}

// withUserID 注入 x-user-id metadata（chat-svc 拦截器读这个）
//
// userID 优先级：ctx.Value(userIDCtxKey)（handler → downstream.WithUserID 注入）> 0
// 0 → 不注入 metadata（依赖 chat-svc 拦截器判 Unauthenticated）
func withUserID(ctx context.Context) context.Context {
	if uid, ok := ctx.Value(userIDCtxKey{}).(int64); ok && uid > 0 {
		return metadata.AppendToOutgoingContext(ctx, "x-user-id", fmt.Sprintf("%d", uid))
	}
	return ctx
}

// CreateConversation RPC
func (c *chatGRPCClient) CreateConversation(ctx context.Context, req CreateConversationReq) (*ConversationView, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	resp, err := cli.CreateConversation(withUserID(ctx), &emotionchat.CreateConversationRequest{
		Title:  req.Title,
		UserId: uidFromCtx(ctx),
	})
	if err != nil {
		return nil, wrapGRPCError(err, "chat create conv")
	}
	return fromProtoConversation(resp), nil
}

// SendMessage RPC
func (c *chatGRPCClient) SendMessage(ctx context.Context, conversationID int64, req SendMessageReq) (*MessageView, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	clientMsgID := req.ClientMsgID
	resp, err := cli.SendMessage(withUserID(ctx), &emotionchat.SendMessageRequest{
		ConversationId: conversationID,
		UserId:        uidFromCtx(ctx),
		Role:          req.Role,
		Content:       req.Content,
		ClientMsgId:   stringPtr(clientMsgID),
		ContentType:   req.ContentType,
		EmotionTag:    req.EmotionTag,
		Intent:        req.Intent, // Stage 82 PR-3b
		FileName:      req.FileName, // Stage 89 PR-3
	})
	if err != nil {
		return nil, wrapGRPCError(err, "chat send msg")
	}
	return fromProtoMessage(resp), nil
}

// ListMessages RPC（2026-10-13：游标分页，返回最新 limit 条 + hasMore）
func (c *chatGRPCClient) ListMessages(ctx context.Context, conversationID int64, limit int, cursor int64) ([]MessageView, bool, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	resp, err := cli.ListMessages(withUserID(ctx), &emotionchat.ListMessagesRequest{
		ConversationId: conversationID,
		Limit:          int32(limit),
		Cursor:         cursor,
	})
	if err != nil {
		return nil, false, wrapGRPCError(err, "chat list messages")
	}
	out := make([]MessageView, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		out = append(out, *fromProtoMessage(m))
	}
	return out, resp.GetHasMore(), nil
}

// ListConversations RPC
func (c *chatGRPCClient) ListConversations(ctx context.Context, limit, offset int) ([]ConversationView, bool, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	resp, err := cli.ListConversations(withUserID(ctx), &emotionchat.ListConversationsRequest{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return nil, false, wrapGRPCError(err, "chat list conversations")
	}
	out := make([]ConversationView, 0, len(resp.List))
	for _, c := range resp.List {
		out = append(out, *fromProtoConversation(c))
	}
	return out, resp.HasMore, nil
}

// DeleteConversation RPC
func (c *chatGRPCClient) DeleteConversation(ctx context.Context, conversationID int64) error {
	cli := emotionchat.NewChatServiceClient(c.conn)
	_, err := cli.DeleteConversation(withUserID(ctx), &emotionchat.DeleteConversationRequest{
		ConversationId: conversationID,
	})
	if err != nil {
		return wrapGRPCError(err, "chat delete conversation")
	}
	return nil
}

// PinConversation RPC（Stage 72：chat-svc 已实现）
func (c *chatGRPCClient) PinConversation(ctx context.Context, conversationID int64, isPinned bool) error {
	cli := emotionchat.NewChatServiceClient(c.conn)
	_, err := cli.PinConversation(withUserID(ctx), &emotionchat.PinConversationRequest{
		ConversationId: conversationID,
		IsPinned:       isPinned,
	})
	if err != nil {
		return wrapGRPCError(err, "chat pin conversation")
	}
	return nil
}

// UpdateConversation RPC（Stage 72：chat-svc 已实现，当前仅支持 title）
func (c *chatGRPCClient) UpdateConversation(ctx context.Context, conversationID int64, title string) (*UpdateConversationResp, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	resp, err := cli.UpdateConversation(withUserID(ctx), &emotionchat.UpdateConversationRequest{
		ConversationId: conversationID,
		Title:          title,
	})
	if err != nil {
		return nil, wrapGRPCError(err, "chat update conversation")
	}
	return &UpdateConversationResp{
		Success: resp.GetSuccess(),
		Id:      resp.GetId(),
		Title:   resp.GetTitle(),
	}, nil
}

// ── CL-02 归档（2026-10-14，用户裁定 D2）：原 StreamMessages client 方法已删除
// （它**不在 `ChatClient` 接口内**、零调用；proto 侧 RPC 保留但标 deprecated）。
// 聊天流实际走 BFF SSE（POST /api/v1/ai/stream）。重启条件见 proto/chat.proto 注释。
// ─────────────────────────────────────────────────────────────────────

// ============ proto → types 转换 ============

func fromProtoConversation(c *emotionchat.Conversation) *ConversationView {
	if c == nil {
		return nil
	}
	return &ConversationView{
		ID:        c.Id,
		UserID:    c.UserId,
		Title:     c.Title,
		MsgCount:  int(c.MsgCount),
		Status:    int(c.Status),
		IsPinned:  c.IsPinned,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

func fromProtoMessage(m *emotionchat.Message) *MessageView {
	if m == nil {
		return nil
	}
	return &MessageView{
		ID:             m.Id,
		ConversationID: m.ConversationId,
		UserID:         m.UserId,
		Role:           m.Role,
		Content:        m.Content,
		ContentType:    m.ContentType, // Stage 79：响应透传（proto content_type=8）
		Intent:         m.Intent,      // Stage 82 PR-3b
		FileName:       m.FileName,    // Stage 89 PR-3
		TokensUsed:     int(m.TokensUsed),
		CreatedAt:      m.CreatedAt,
	}
}

// uidFromCtx 从 ctx 取 user id（chat_handler 中间件注入）
func uidFromCtx(ctx context.Context) int64 {
	if uid, ok := ctx.Value(userIDCtxKey{}).(int64); ok {
		return uid
	}
	return 0
}

// stringPtr 把 *string 转 string（proto 字段是 string 非 *string）
func stringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}