// Package grpcserver — chat_server.go
//
// Stage 58 PR-GRPC-2: 实现 ChatServiceServer interface
//
// 7 个 rpc 方法对应 chat-svc 已有的 logic 层（HTTP 与 gRPC 共享业务逻辑）：
//   - CreateConversation     → logic.NewCreateConversationLogic
//   - SendMessage            → logic.NewSendMessageLogic
//   - ListMessages           → logic.NewListMessagesLogic
//   - ListConversations      → logic.NewListConversationsLogic
//   - DeleteConversation     → logic.NewDeleteConversationLogic
//   - PinConversation        → logic.NewPinConversationLogic（chat-svc 尚未实现 HTTP）
//   - StreamMessages         → gRPC server stream（BFF SSE 流式聊天替代）
//
// PR-GRPC-2 阶段：
//   - 实现所有 7 个方法签名（编译过 ChatServiceServer interface）
//   - CreateConversation 完整实现（含 user id 校验 + proto 类型转换）
//   - 其余方法：占位（返回 Unimplemented）+ 注释 PR-GRPC-3 阶段补完
//   - 这避免一次 PR 工作面过大（PR-GRPC-2 范围聚焦"gRPC server 链路通"）

package grpcserver

import (
	"context"
	"errors"

	"emotion-echo-chat-svc/internal/logic"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"
	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	grpcerr "github.com/emotion-echo/shared/pkg/grpcerr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// chatServer 实现 emotionchat.ChatServiceServer
type chatServer struct {
	emotionchat.UnimplementedChatServiceServer
	svcCtx *svc.ServiceContext
}

// toProtoConversation 把 chat-svc types.ConversationView 转 proto
func toProtoConversation(c types.ConversationView) *emotionchat.Conversation {
	return &emotionchat.Conversation{
		Id:        c.Id,
		UserId:    c.UserId,
		Title:     c.Title,
		MsgCount:  int32(c.MsgCount),
		Status:    int32(c.Status),
		IsPinned:  c.IsPinned,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// toProtoMessage 把 chat-svc types.MessageView 转 proto
func toProtoMessage(m types.MessageView) *emotionchat.Message {
	return &emotionchat.Message{
		Id:             m.Id,
		ConversationId: m.ConversationId,
		UserId:         m.UserId,
		Role:           m.Role,
		Content:        m.Content,
		ContentType:    m.ContentType, // Stage 79：响应视图回带（proto content_type=8）
		TokensUsed:     int32(m.TokensUsed),
		CreatedAt:      m.CreatedAt,
	}
}

// CreateConversation 实现 CreateConversation RPC
//
// 行为契约：
//   - 从 metadata x-user-id 取 user id（grpcinterceptor 注入 ctx）
//   - 调 logic.NewCreateConversationLogic.CreateConversation
//   - 把 types.CreateConversationResp 转 proto.Conversation
//   - svcCtx 为 nil → 返 Unavailable（PR-GRPC-3 阶段 main.go 注入真实 svcCtx）
func (s *chatServer) CreateConversation(ctx context.Context, req *emotionchat.CreateConversationRequest) (*emotionchat.Conversation, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}
	uid, ok := grpcinterceptor.UserIDFromGRPCContext(ctx)
	if !ok || uid <= 0 {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}

	resp, err := logic.NewCreateConversationLogic(ctx, s.svcCtx).CreateConversation(&types.CreateConversationReq{
		Title: req.GetTitle(),
	})
	if err != nil {
		return nil, grpcerr.MapToError(err, "create conversation")
	}
	if resp == nil || resp.Conversation.Id == 0 {
		return nil, status.Error(codes.Internal, "empty conversation response")
	}
	return toProtoConversation(resp.Conversation), nil
}

// SendMessage 实现 SendMessage RPC（Sprint D）
//
// 行为契约：
//   - 从 metadata x-user-id 取 user id（grpcinterceptor 注入 ctx，Sprint C ctxkey 别名通）
//   - 调 logic.NewSendMessageLogic.SendMessage（与 HTTP handler 共享业务逻辑）
//   - proto SendMessageRequest.ConversationId → types.SendMessageReq.Id
//   - 错误映射：logic 业务错误 → codes.Internal（无法细分类别时）；not found → codes.NotFound
func (s *chatServer) SendMessage(ctx context.Context, req *emotionchat.SendMessageRequest) (*emotionchat.Message, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}

	// proto ClientMsgId 是 string（非指针）；types.ClientMsgID 是 *string。
	// 空字符串 → nil（避免把空字符串当作有效幂等 key）
	var clientMsgID *string
	if req.GetClientMsgId() != "" {
		cmid := req.GetClientMsgId()
		clientMsgID = &cmid
	}
	resp, err := logic.NewSendMessageLogic(ctx, s.svcCtx).SendMessage(&types.SendMessageReq{
		Id:          req.GetConversationId(),
		Role:        req.GetRole(),
		Content:     req.GetContent(),
		ClientMsgID: clientMsgID,
		ContentType: req.GetContentType(),
		EmotionTag:  req.GetEmotionTag(),
	})
	if err != nil {
		return nil, mapLogicError(err, "send message")
	}
	if resp == nil {
		return nil, status.Error(codes.Internal, "empty send message response")
	}
	return toProtoMessage(resp.Message), nil
}

// ListMessages 实现 ListMessages RPC（Sprint D）
func (s *chatServer) ListMessages(ctx context.Context, req *emotionchat.ListMessagesRequest) (*emotionchat.ListMessagesResponse, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}

	resp, err := logic.NewListMessagesLogic(ctx, s.svcCtx).ListMessages(&types.ListMessagesReq{
		Id:    req.GetConversationId(),
		Limit: int(req.GetLimit()),
	})
	if err != nil {
		return nil, mapLogicError(err, "list messages")
	}
	out := &emotionchat.ListMessagesResponse{Messages: make([]*emotionchat.Message, 0, len(resp.Messages))}
	for _, m := range resp.Messages {
		out.Messages = append(out.Messages, toProtoMessage(m))
	}
	return out, nil
}

// ListConversations 实现 ListConversations RPC（Sprint D）
func (s *chatServer) ListConversations(ctx context.Context, req *emotionchat.ListConversationsRequest) (*emotionchat.ListConversationsResponse, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}

	resp, err := logic.NewListConversationsLogic(ctx, s.svcCtx).ListConversations(&types.ListConversationsReq{
		Limit:  int(req.GetLimit()),
		Offset: int(req.GetOffset()),
	})
	if err != nil {
		return nil, mapLogicError(err, "list conversations")
	}
	out := &emotionchat.ListConversationsResponse{
		List:    make([]*emotionchat.Conversation, 0, len(resp.List)),
		HasMore: resp.HasMore,
	}
	for _, c := range resp.List {
		out.List = append(out.List, toProtoConversation(c))
	}
	return out, nil
}

// DeleteConversation 实现 DeleteConversation RPC（Sprint D）
func (s *chatServer) DeleteConversation(ctx context.Context, req *emotionchat.DeleteConversationRequest) (*emotionchat.DeleteConversationResponse, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}

	resp, err := logic.NewDeleteConversationLogic(ctx, s.svcCtx).DeleteConversation(&types.DeleteConversationReq{
		Id: req.GetConversationId(),
	})
	if err != nil {
		return nil, mapLogicError(err, "delete conversation")
	}
	return &emotionchat.DeleteConversationResponse{
		Success: resp.Success,
		Id:      resp.Id,
	}, nil
}

// mapLogicError 把 logic 层业务错误映射到 gRPC status code。
//
// B4：迁移到 grpcerr.Wrap + MapError 注册。chat-svc 业务 sentinel
// （repository.ErrNotFound 等）由 init() 注册到 grpcerr 全局表。
func init() {
	grpcerr.MapError(repository.ErrNotFound, codes.NotFound)
}

func mapLogicError(err error, op string) error {
	if err == nil {
		return nil
	}
	return grpcerr.MapToError(err, op)
}

// PinConversation 实现 PinConversation RPC（Stage 72，决策 4 ADR §八 收口）
//
// 行为契约：
//   - 从 metadata x-user-id 取 user id
//   - 调 logic.NewPinConversationLogic.PinConversation（owner 校验 + repo.SetPinned）
//   - 错误映射：not found → NotFound；forbidden → PermissionDenied（grpcerr 关键词表）
func (s *chatServer) PinConversation(ctx context.Context, req *emotionchat.PinConversationRequest) (*emotionchat.PinConversationResponse, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}
	uid, ok := grpcinterceptor.UserIDFromGRPCContext(ctx)
	if !ok || uid <= 0 {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}

	resp, err := logic.NewPinConversationLogic(ctx, s.svcCtx).PinConversation(&types.PinConversationReq{
		Id:       req.GetConversationId(),
		IsPinned: req.GetIsPinned(),
	})
	if err != nil {
		return nil, mapLogicError(err, "pin conversation")
	}
	return &emotionchat.PinConversationResponse{
		Success:  resp.Success,
		Id:       resp.Id,
		IsPinned: resp.IsPinned,
	}, nil
}

// UpdateConversation 实现 UpdateConversation RPC（Stage 72，决策 4 ADR §八 收口）
//
// 行为契约：
//   - 从 metadata x-user-id 取 user id
//   - 调 logic.NewUpdateConversationLogic.UpdateConversation（空标题校验 + owner 校验 + repo.UpdateTitle）
//   - 错误映射：not found → NotFound；forbidden → PermissionDenied；validation → InvalidArgument
func (s *chatServer) UpdateConversation(ctx context.Context, req *emotionchat.UpdateConversationRequest) (*emotionchat.UpdateConversationResponse, error) {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}
	uid, ok := grpcinterceptor.UserIDFromGRPCContext(ctx)
	if !ok || uid <= 0 {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}

	resp, err := logic.NewUpdateConversationLogic(ctx, s.svcCtx).UpdateConversation(&types.UpdateConversationReq{
		Id:    req.GetConversationId(),
		Title: req.GetTitle(),
	})
	if err != nil {
		return nil, mapLogicError(err, "update conversation")
	}
	return &emotionchat.UpdateConversationResponse{
		Success: resp.Success,
		Id:      resp.Id,
		Title:   resp.Title,
	}, nil
}

// StreamMessages gRPC server stream（PR-GRPC-5 阶段架构判断：暂不实现）
//
// 架构判断（2026-09-09）：
//   chat-svc 当前没有"订阅消息流"业务语义——SendMessage 是同步 RPC，
//   写库 + outbox 即返回。前端聊天走 POST /api/v1/ai/stream（直连 LLM），
//   不经 chat-svc 中转。
//
// 因此 StreamMessages 是 proto 预留接口（proto/chat.proto §StreamMessagesRequest），
// 当前业务无触发场景。保留 chat.go grpc interface 定义供未来扩展（多客户端
// 实时协作、消息撤回广播等场景），实现留待业务明确时再补。
//
// 当前行为：返 Unimplemented。client 端 chatGRPCClient.StreamMessages
// 收到 codes.Unimplemented 时 fallback 到现有路径（暂无 fallback，因为
// BFF 没有用 StreamMessages 的调用点）。
func (s *chatServer) StreamMessages(req *emotionchat.StreamMessagesRequest, stream emotionchat.ChatService_StreamMessagesServer) error {
	if s.svcCtx == nil || s.svcCtx.ConversationRepo == nil {
		return status.Error(codes.Unavailable, "chat-svc repository not initialized (degraded start)")
	}
	return status.Error(codes.Unimplemented, "StreamMessages: chat-svc 当前无流式订阅业务；留待未来多客户端实时协作场景")
}

// Suppress unused warning for errors
var _ = errors.New