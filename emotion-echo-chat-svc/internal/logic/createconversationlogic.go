// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"encoding/json"
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
	"gorm.io/gorm"
)

// CreateConversationLogic 处理 POST /api/v1/conversations
//
// 流程（Stage 30-C A3）:
//  1. 中间件注入 user_id
//  2. 开 DB 事务（如果 svcCtx.DB 非 nil）
//  3. 持久化会话 + 写 outbox 行（同事务，原子）
//  4. commit
//  5. 由 relay goroutine 异步发送事件（commit 后立即可见）
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

	// 3. Stage 30-C A3: 事务化持久化 + outbox
	if err := l.persistWithOutbox(uid, conv, now); err != nil {
		l.Errorf("CreateConversation persist err: %v", err)
		return nil, err
	}

	// 4. 响应
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

// persistWithOutbox 在事务中写业务 + outbox 行
//
// 路径优先级：
//   1. svcCtx.DB 非 nil && OutboxRepo 非 nil：开事务，业务 + outbox 同事务（生产场景）
//   2. svcCtx.DB nil && OutboxRepo 非 nil：退化（无事务），业务 + outbox 各自独立（部分测试用）
//   3. OutboxRepo nil：原行为，直接 EventPublisher.Publish（保留向后兼容，让旧单测通过）
func (l *CreateConversationLogic) persistWithOutbox(uid int64, conv *model.Conversation, now time.Time) error {
	evt := &events.Event{
		ID:     uuid.NewString(),
		Type:   events.EventTypeConversationCreated,
		Source: "chat-svc",
		Time:   now,
		Data: events.ConversationCreatedData{
			ConversationID: 0, // 写入后回填
			UserID:         uid,
			Title:          conv.Title,
			CreatedAt:      now.UnixMilli(),
		},
	}

	// 路径 1：生产场景 — DB + OutboxRepo 都齐备
	if l.svcCtx.DB != nil && l.svcCtx.OutboxRepo != nil {
		return l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
			if err := l.svcCtx.ConversationRepo.CreateConversationTx(tx, l.ctx, conv); err != nil {
				return err
			}
			d := evt.Data.(events.ConversationCreatedData)
			d.ConversationID = conv.ID
			evt.Data = d
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

	// 业务表持久化（路径 2/3 共用）
	if err := l.svcCtx.ConversationRepo.CreateConversation(l.ctx, conv); err != nil {
		return err
	}
	d := evt.Data.(events.ConversationCreatedData)
	d.ConversationID = conv.ID
	evt.Data = d

	// 路径 2：OutboxRepo 非 nil — 写 outbox（无事务，由 relay 异步发）
	if l.svcCtx.OutboxRepo != nil {
		payload, _ := json.Marshal(evt)
		return l.svcCtx.OutboxRepo.CreateInTx(nil, &repository.OutboxEvent{
			EventID:   evt.ID,
			EventType: evt.Type,
			Topic:     events.TopicChatEvents,
			Payload:   payload,
		})
	}

	// 路径 3：原行为 — 直接 EventPublisher.Publish（best-effort，失败仅 log）
	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, evt); err != nil {
		l.Errorf("publish conversation.created err: %v", err)
	}
	return nil
}