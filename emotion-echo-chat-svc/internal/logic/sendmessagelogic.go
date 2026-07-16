// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"errors"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/middleware"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

// SendMessageLogic 处理 POST /api/v1/conversations/:id/messages
//
// 流程：
//  1. 鉴权 + 验证 content
//  2. 检查会话存在
//  3. 落 message
//  4. 增 message_count（原子操作）
//  5. 发布 message.created 事件（ai-svc 异步消费做情绪分析）
type SendMessageLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendMessageLogic {
	return &SendMessageLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// 合法角色集合
var allowedRoles = map[string]bool{"user": true, "assistant": true, "system": true}

// SendMessage 追加一条消息到指定会话
func (l *SendMessageLogic) SendMessage(req *types.SendMessageReq) (resp *types.SendMessageResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id")
	}
	if req.Content == "" {
		return nil, errors.New("validation: content is required")
	}
	role := req.Role
	if role == "" {
		role = "user"
	}
	if !allowedRoles[role] {
		return nil, errors.New("validation: role must be one of user/assistant/system")
	}

	// 检查会话存在
	conv, err := l.svcCtx.ConversationRepo.GetConversationByID(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, repository.ErrNotFound
	}
	// 鉴权：只能发到自己的会话
	if conv.UserID != uid {
		return nil, errors.New("forbidden: conversation does not belong to current user")
	}

	// 落 message
	now := time.Now()
	msg := &model.Message{
		ConversationID: req.Id,
		UserID:         uid,
		Role:           role,
		Content:        req.Content,
		ContentType:    "text",
		TokensUsed:     0,
		CreatedAt:      now,
	}
	if err := l.svcCtx.ConversationRepo.AppendMessage(l.ctx, msg); err != nil {
		l.Errorf("AppendMessage err: %v", err)
		return nil, err
	}

	// 增计数
	if err := l.svcCtx.ConversationRepo.IncrementMessageCount(l.ctx, req.Id); err != nil {
		// 非致命：日志后继续
		l.Errorf("IncrementMessageCount err: %v", err)
	}

	// 发布 message.created 事件
	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, &events.Event{
		ID:     uuid.NewString(),
		Type:   events.EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   now,
		Data: events.MessageCreatedData{
			MessageID:      msg.ID,
			ConversationID: req.Id,
			UserID:         uid,
			Role:           role,
			Content:        req.Content,
			CreatedAt:      now.UnixMilli(),
		},
	}); err != nil {
		l.Errorf("publish message.created err: %v", err)
	}

	return &types.SendMessageResp{
		Message: types.MessageView{
			Id:             msg.ID,
			ConversationId: msg.ConversationID,
			UserId:         msg.UserID,
			Role:           msg.Role,
			Content:        msg.Content,
			TokensUsed:     msg.TokensUsed,
			CreatedAt:      msg.CreatedAt.UnixMilli(),
		},
	}, nil
}