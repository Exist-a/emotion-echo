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
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	emotionchat "github.com/emotion-echo/shared/pkg/emotionchat"

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
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
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
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	uid, ok := grpcinterceptor.UserIDFromGRPCContext(ctx)
	if !ok || uid <= 0 {
		return nil, status.Error(codes.Unauthenticated, "missing x-user-id metadata")
	}

	resp, err := logic.NewCreateConversationLogic(ctx, s.svcCtx).CreateConversation(&types.CreateConversationReq{
		Title: req.GetTitle(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create conversation: %v", err)
	}
	if resp == nil || resp.Conversation.Id == 0 {
		return nil, status.Error(codes.Internal, "empty conversation response")
	}
	return toProtoConversation(resp.Conversation), nil
}

// SendMessage 占位实现（PR-GRPC-3 阶段补完）
func (s *chatServer) SendMessage(ctx context.Context, req *emotionchat.SendMessageRequest) (*emotionchat.Message, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "SendMessage: PR-GRPC-3 阶段补完")
}

// ListMessages 占位实现
func (s *chatServer) ListMessages(ctx context.Context, req *emotionchat.ListMessagesRequest) (*emotionchat.ListMessagesResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "ListMessages: PR-GRPC-3 阶段补完")
}

// ListConversations 占位实现
func (s *chatServer) ListConversations(ctx context.Context, req *emotionchat.ListConversationsRequest) (*emotionchat.ListConversationsResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "ListConversations: PR-GRPC-3 阶段补完")
}

// DeleteConversation 占位实现
func (s *chatServer) DeleteConversation(ctx context.Context, req *emotionchat.DeleteConversationRequest) (*emotionchat.DeleteConversationResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "DeleteConversation: PR-GRPC-3 阶段补完")
}

// PinConversation 占位实现
func (s *chatServer) PinConversation(ctx context.Context, req *emotionchat.PinConversationRequest) (*emotionchat.PinConversationResponse, error) {
	if s.svcCtx == nil {
		return nil, status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	return nil, status.Error(codes.Unimplemented, "PinConversation: PR-GRPC-3 阶段补完")
}

// StreamMessages gRPC server stream（PR-GRPC-5 阶段补完流式逻辑）
func (s *chatServer) StreamMessages(req *emotionchat.StreamMessagesRequest, stream emotionchat.ChatService_StreamMessagesServer) error {
	if s.svcCtx == nil {
		return status.Error(codes.Unavailable, "chat-svc service context not initialized")
	}
	return status.Error(codes.Unimplemented, "StreamMessages: PR-GRPC-5 阶段补完流式逻辑")
}

// Suppress unused warning for errors
var _ = errors.New