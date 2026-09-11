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
	})
	if err != nil {
		return nil, wrapGRPCError(err, "chat send msg")
	}
	return fromProtoMessage(resp), nil
}

// ListMessages RPC
func (c *chatGRPCClient) ListMessages(ctx context.Context, conversationID int64, limit int) ([]MessageView, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	resp, err := cli.ListMessages(withUserID(ctx), &emotionchat.ListMessagesRequest{
		ConversationId: conversationID,
		Limit:          int32(limit),
	})
	if err != nil {
		return nil, wrapGRPCError(err, "chat list messages")
	}
	out := make([]MessageView, 0, len(resp.Messages))
	for _, m := range resp.Messages {
		out = append(out, *fromProtoMessage(m))
	}
	return out, nil
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

// PinConversation 占位（chat-svc HTTP 端点也未实现；保留接口）
func (c *chatGRPCClient) PinConversation(ctx context.Context, conversationID int64) error {
	cli := emotionchat.NewChatServiceClient(c.conn)
	_, err := cli.PinConversation(withUserID(ctx), &emotionchat.PinConversationRequest{
		ConversationId: conversationID,
	})
	if err != nil {
		return wrapGRPCError(err, "chat pin conversation")
	}
	return nil
}

// StreamMessages gRPC server stream 客户端（PR-GRPC-5 阶段：架构判断）
//
// 架构判断（2026-09-09）：
//   chat-svc 当前没有"订阅消息流"业务语义——前端聊天走 POST /api/v1/ai/stream
//   （BFF 直连 LLM），不经 chat-svc 中转。因此 StreamMessages 是预留接口，
//   chat-svc 端 chatServer.StreamMessages 返 Unimplemented。
//
// 当前实现：直接调 gRPC StreamMessages RPC，把 server stream 包装为
// ChatEvent 通道返回。client 端收到 Unimplemented 时，channel 立即关闭 +
// io.EOF 错误（gRPC 标准行为）。
//
// 触发场景：未来多客户端实时协作（多人共编会话）/ 消息撤回广播等。
func (c *chatGRPCClient) StreamMessages(ctx context.Context, conversationID int64, fromMessageID int64) (<-chan *emotionchat.ChatEvent, error) {
	cli := emotionchat.NewChatServiceClient(c.conn)
	stream, err := cli.StreamMessages(withUserID(ctx), &emotionchat.StreamMessagesRequest{
		ConversationId:  conversationID,
		UserId:         uidFromCtx(ctx),
		FromMessageId:  fromMessageID,
	})
	if err != nil {
		return nil, wrapGRPCError(err, "chat stream messages")
	}

	out := make(chan *emotionchat.ChatEvent, 16)
	go func() {
		defer close(out)
		for {
			evt, err := stream.Recv()
			if err != nil {
				// io.EOF 或 Unimplemented 时正常退出
				return
			}
			select {
			case out <- evt:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

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