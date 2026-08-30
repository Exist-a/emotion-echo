// Package logic — deleteconversationlogic.go
//
// Stage 30-B GREEN: DELETE /api/v1/conversations/:id
// Stage 30-C A3: 事务化（业务删除 + outbox 行同事务）
//
// 流程：
//  1. 鉴权（ctx 注入 user_id）
//  2. 会话存在 + owner 校验（只能删自己的会话）
//  3. 开事务：repo 删除（会话 + 级联消息）+ 写 outbox
//  4. commit；relay 异步发 conversation.closed
//
// conversation.closed 是 analytics-svc 用户行为统计的输入之一；
// 此前 chat-svc 定义了该事件类型但从未发布（本端点使其成为唯一生产者）。
package logic

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/middleware"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
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

	now := time.Now()
	if err := l.persistWithOutbox(uid, req.Id, now); err != nil {
		l.Errorf("DeleteConversation persist err: %v", err)
		return nil, err
	}

	return &types.DeleteConversationResp{
		Success: true,
		Id:      req.Id,
	}, nil
}

// persistWithOutbox Stage 30-C A3 事务化（与 CreateConversation / SendMessage 同模式）
func (l *DeleteConversationLogic) persistWithOutbox(uid, convID int64, now time.Time) error {
	evt := &events.Event{
		ID:     uuid.NewString(),
		Type:   events.EventTypeConversationClosed,
		Source: "chat-svc",
		Time:   now,
		Data: events.ConversationClosedData{
			ConversationID: convID,
			UserID:         uid,
			ClosedAt:       now.UnixMilli(),
		},
	}

	if l.svcCtx.DB != nil && l.svcCtx.OutboxRepo != nil {
		return l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
			if err := l.svcCtx.ConversationRepo.DeleteConversationTx(tx, l.ctx, convID); err != nil {
				return err
			}
			payload, err := json.Marshal(evt)
			if err != nil {
				return err
			}
			return l.svcCtx.OutboxRepo.CreateInTx(tx, &repository.OutboxEvent{
				EventID:   evt.ID,
				EventType: evt.Type,
				Topic:     events.TopicChatEvents,
				Payload:   payload,
			})
		})
	}

	if err := l.svcCtx.ConversationRepo.DeleteConversation(l.ctx, convID); err != nil {
		return err
	}

	if l.svcCtx.OutboxRepo != nil {
		payload, _ := json.Marshal(evt)
		return l.svcCtx.OutboxRepo.CreateInTx(nil, &repository.OutboxEvent{
			EventID:   evt.ID,
			EventType: evt.Type,
			Topic:     events.TopicChatEvents,
			Payload:   payload,
		})
	}

	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, evt); err != nil {
		l.Errorf("publish conversation.closed err: %v", err)
	}
	return nil
}
