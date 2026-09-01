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

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/google/uuid"
	"github.com/zeromicro/go-zero/core/logx"
	"gorm.io/gorm"
)

// SendMessageLogic 处理 POST /api/v1/conversations/:id/messages
//
// 流程（Stage 30-C A3）：
//  1. 鉴权 + 验证 content
//  2. 检查会话存在
//  3. 开 DB 事务（如 svcCtx.DB 非 nil）
//  4. 落 message + 增 message_count + 写 outbox 行（同事务，原子）
//  5. commit；relay goroutine 异步发事件
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

	// Stage 33 PR-18：幂等查重。若 client_msg_id 已存在，直接返回原 message
	// 而非新建（网络重试 / 浏览器刷新重发场景）。
	if req.ClientMsgID != nil && *req.ClientMsgID != "" {
		existing, err := l.svcCtx.ConversationRepo.GetMessageByClientMsgID(l.ctx, uid, req.Id, *req.ClientMsgID)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return &types.SendMessageResp{
				Message: types.MessageView{
					Id:             existing.ID,
					ConversationId: existing.ConversationID,
					UserId:         existing.UserID,
					Role:           existing.Role,
					Content:        existing.Content,
					TokensUsed:     existing.TokensUsed,
					CreatedAt:      existing.CreatedAt.UnixMilli(),
				},
			}, nil
		}
	}

	now := time.Now()
	contentType := req.ContentType
	if contentType == "" {
		contentType = "text"
	}
	msg := &model.Message{
		ConversationID: req.Id,
		UserID:         uid,
		Role:           role,
		Content:        req.Content,
		ContentType:    contentType,
		TokensUsed:     0,
		ClientMsgID:    req.ClientMsgID,
		CreatedAt:      now,
	}

	if err := l.persistWithOutbox(uid, req.Id, role, req.Content, msg, now); err != nil {
		l.Errorf("SendMessage persist err: %v", err)
		return nil, err
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

// persistWithOutbox Stage 30-C A3 事务化（与 CreateConversation 同模式）
func (l *SendMessageLogic) persistWithOutbox(
	uid, convID int64, role, content string,
	msg *model.Message, now time.Time,
) error {
	evt := &events.Event{
		ID:     uuid.NewString(),
		Type:   events.EventTypeMessageCreated,
		Source: "chat-svc",
		Time:   now,
		Data: events.MessageCreatedData{
			MessageID:      0, // 写入后回填
			ConversationID: convID,
			UserID:         uid,
			Role:           role,
			Content:        content,
			CreatedAt:      now.UnixMilli(),
		},
	}

	// 路径 1：生产场景
	if l.svcCtx.DB != nil && l.svcCtx.OutboxRepo != nil {
		if err := l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
			if err := l.svcCtx.ConversationRepo.AppendMessageTx(tx, l.ctx, msg); err != nil {
				return err
			}
			if err := l.svcCtx.ConversationRepo.IncrementMessageCountTx(tx, l.ctx, convID); err != nil {
				return err
			}
			d := evt.Data.(events.MessageCreatedData)
			d.MessageID = msg.ID
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
		}); err != nil {
			return err
		}
		// Stage 36-A3.2：commit 后再做 dev fallback（不在事务内，避免 ai-svc 调用阻塞事务）
		l.maybeUpsertNeutralEmotion(uid, convID, msg.ID, evt.ID)
		return nil
	}

	// 退化路径
	if err := l.svcCtx.ConversationRepo.AppendMessage(l.ctx, msg); err != nil {
		return err
	}
	_ = l.svcCtx.ConversationRepo.IncrementMessageCount(l.ctx, convID) // best-effort

	if l.svcCtx.OutboxRepo != nil {
		d := evt.Data.(events.MessageCreatedData)
		d.MessageID = msg.ID
		evt.Data = d
		payload, _ := json.Marshal(evt)
		if err := l.svcCtx.OutboxRepo.CreateInTx(nil, &repository.OutboxEvent{
			EventID:   evt.ID,
			EventType: evt.Type,
			Topic:     events.TopicChatEvents,
			Payload:   payload,
		}); err != nil {
			return err
		}
		// Stage 36-A3.2：同上，commit 后再调 ai-svc
		l.maybeUpsertNeutralEmotion(uid, convID, msg.ID, evt.ID)
		return nil
	}

	// 原行为（向后兼容）— 用 AppendMessage 已回填的 msg.ID
	d := evt.Data.(events.MessageCreatedData)
	d.MessageID = msg.ID
	evt.Data = d
	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, evt); err != nil {
		l.Errorf("publish message.created err: %v", err)
	}

	// Stage 36-A3.2：InMemory / 退化路径（无 Kafka 无 Outbox）也走 dev fallback，
	// 因为本场景下消息不会经过 Kafka → ai-svc 必须同步写。
	l.maybeUpsertNeutralEmotion(uid, convID, msg.ID, evt.ID)
	return nil
}

// maybeUpsertNeutralEmotion Stage 36-A3.2：dev fallback 同步写中性占位情绪。
//
// 调用条件：
//   - KAFKA_ENABLED=false（dev 模式 / 离线模式）：Kafka 没起来，必须同步写
//   - KAFKA_ENABLED=true（生产模式）：Kafka 异步管道已能写入 ai-svc，不重复写
//
// 错误处理：best-effort，失败只 log 不阻塞消息返回（dev fallback 语义）。
func (l *SendMessageLogic) maybeUpsertNeutralEmotion(uid, convID, messageID int64, eventID string) {
	if l.svcCtx.Config.Kafka.Enabled {
		// 生产模式：Kafka consumer 会负责写 emotion_analysis，不重复
		return
	}
	if l.svcCtx.AIClient == nil {
		// 不应发生（NewServiceContext 默认注入 NoopAIClient）但做防御
		return
	}
	req := &emotionquery.UpsertNeutralEmotionRequest{
		MessageId:      messageID,
		UserId:         uid,
		ConversationId: convID,
		EventId:        eventID,
	}
	if _, err := l.svcCtx.AIClient.UpsertNeutralEmotion(l.ctx, req); err != nil {
		l.Errorf("dev fallback UpsertNeutralEmotion failed (msgID=%d eventID=%s): %v",
			messageID, eventID, err)
	}
}