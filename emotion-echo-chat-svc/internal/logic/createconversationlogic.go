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
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

// CreateConversationLogic 处理 POST /api/v1/conversations
//
// 流程：
//  1. 中间件注入 user_id
//  2. 持久化会话
//  3. 异步发 conversation.created 事件
type CreateConversationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateConversationLogic {
	return &CreateConversationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateConversation 新建一个会话
func (l *CreateConversationLogic) CreateConversation(req *types.CreateConversationReq) (resp *types.CreateConversationResp, err error) {
	// 1. 鉴权（由 AuthMiddleware 注入 user_id）
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id")
	}

	// 2. 构造实体
	now := time.Now()
	conv := &model.Conversation{
		UserID:    uid,
		Title:     req.Title,
		Status:    1, // 1 = open
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 3. 持久化
	if err := l.svcCtx.ConversationRepo.CreateConversation(l.ctx, conv); err != nil {
		l.Errorf("CreateConversation DB err: %v", err)
		return nil, err
	}

	// 4. 发布事件（失败不回滚 DB，写日志；事件发布是 best-effort）
	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, &events.Event{
		ID:     uuid.NewString(),
		Type:   events.EventTypeConversationCreated,
		Source: "chat-svc",
		Time:   now,
		Data: events.ConversationCreatedData{
			ConversationID: conv.ID,
			UserID:         uid,
			Title:          conv.Title,
			CreatedAt:      now.UnixMilli(),
		},
	}); err != nil {
		l.Errorf("publish conversation.created err: %v", err)
		// 不返回 error：业务上会话已建，事件失败可后续用 outbox 重试
	}

	// 5. 响应
	return &types.CreateConversationResp{
		Conversation: types.ConversationView{
			Id:        conv.ID,
			UserId:    conv.UserID,
			Title:     conv.Title,
			MsgCount:  conv.MessageCount,
			Status:    int(conv.Status),
			CreatedAt: conv.CreatedAt.UnixMilli(),
			UpdatedAt: conv.UpdatedAt.UnixMilli(),
		},
	}, nil
}