// Package logic — deleteconversationlogic.go
//
// Stage 30-B GREEN: DELETE /api/v1/conversations/:id
//
// 流程：
//  1. 鉴权（ctx 注入 user_id）
//  2. 会话存在 + owner 校验（只能删自己的会话）
//  3. repo 删除（会话 + 级联消息）
//  4. 发布 conversation.closed（best-effort，与 create/send 同模式）
//
// conversation.closed 是 analytics-svc 用户行为统计的输入之一；
// 此前 chat-svc 定义了该事件类型但从未发布（本端点使其成为唯一生产者）。
package logic

import (
	"context"
	"errors"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/middleware"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
)

// DeleteConversationLogic 处理删除会话
type DeleteConversationLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewDeleteConversationLogic 构造
func NewDeleteConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *DeleteConversationLogic {
	return &DeleteConversationLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// DeleteConversation 删除指定会话
func (l *DeleteConversationLogic) DeleteConversation(req *types.DeleteConversationReq) (resp *types.DeleteConversationResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id")
	}

	conv, err := l.svcCtx.ConversationRepo.GetConversationByID(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, repository.ErrNotFound
	}
	if conv.UserID != uid {
		return nil, errors.New("forbidden: conversation does not belong to current user")
	}

	if err := l.svcCtx.ConversationRepo.DeleteConversation(l.ctx, req.Id); err != nil {
		l.Errorf("DeleteConversation DB err: %v", err)
		return nil, err
	}

	// 发布 conversation.closed（best-effort：失败不回滚删除，写日志）
	now := time.Now()
	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, &events.Event{
		ID:     uuid.NewString(),
		Type:   events.EventTypeConversationClosed,
		Source: "chat-svc",
		Time:   now,
		Data: events.ConversationClosedData{
			ConversationID: req.Id,
			UserID:         uid,
			ClosedAt:       now.UnixMilli(),
		},
	}); err != nil {
		l.Errorf("publish conversation.closed err: %v", err)
	}

	return &types.DeleteConversationResp{
		Success: true,
		Id:      req.Id,
	}, nil
}
