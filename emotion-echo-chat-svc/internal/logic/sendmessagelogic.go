
package logic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"emotion-echo-chat-svc/internal/events"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
	
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
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendMessageLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendMessageLogic {
	return &SendMessageLogic{

		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// 合法角色集合
var allowedRoles = map[string]bool{"user": true, "assistant": true, "system": true}

// SendMessage 追加一条消息到指定会话
// allowedIntents 意图白名单（§契约 5：与 deploy/db/02-create-tables-in-schemas.sql
// messages.intent 注释、emotion-llm-service intent.INTENTS 三方一致）
var allowedIntents = map[string]bool{
	"emotional_support": true,
	"study_help":        true,
	"tech_help":         true,
	"career_help":       true,
	"lifestyle":         true,
	"other":             true,
}

func (l *SendMessageLogic) SendMessage(req *types.SendMessageReq) (resp *types.SendMessageResp, err error) {
	uid, ok := l.ctx.Value(sharedmw.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id")
	}
	if req.Content == "" {
		return nil, errors.New("validation: content is required")
	}
	// P1-13 (Round 1): 消息体大小无限制 → outbox 100 次后死信。
	// 4 KiB 与单条消息平均 50-200 字符对齐，留 20x 缓冲。
	const maxMessageLen = 4 * 1024
	if len(req.Content) > maxMessageLen {
		return nil, fmt.Errorf("validation: content too long (%d > %d)", len(req.Content), maxMessageLen)
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
	// Stage 82 PR-3b：意图白名单校验（§契约 5）——非法值/空值落 ''（未分类）
	intent := req.Intent
	if !allowedIntents[intent] {
		intent = ""
	}
	// Stage 89 PR-2：文件原始名（§契约 5：超 255 截断，与 DDL VARCHAR(255) 一致）
	fileName := req.FileName
	if len(fileName) > 255 {
		fileName = fileName[:255]
	}
	msg := &model.Message{
		ConversationID: req.Id,
		UserID:         uid,
		Role:           role,
		Content:        req.Content,
		ContentType:    contentType,
		Intent:         intent,
		FileName:       fileName,
		TokensUsed:     0,
		ClientMsgID:    req.ClientMsgID,
		CreatedAt:      now,
	}

	if err := l.persistWithOutbox(uid, req.Id, role, req.Content, msg, now); err != nil {
		slog.ErrorContext(l.ctx, "SendMessage persist failed", "err", err)
		return nil, err
	}

	return &types.SendMessageResp{
		Message: types.MessageView{
			Id:             msg.ID,
			ConversationId: msg.ConversationID,
			UserId:         msg.UserID,
			Role:           msg.Role,
			Content:        msg.Content,
			ContentType:    msg.ContentType, // Stage 79：响应回带（第 4 处缺口）
			Intent:         msg.Intent,      // Stage 82 PR-3b：意图回带
			FileName:       msg.FileName,    // Stage 89 PR-2：文件名回带
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

	// 退化路径(DB nil，dev / 测试场景)
// §P0-8 修复:删"业务 AppendMessage + 独立 CreateInTx(nil, ...)"无事务拆分模式
// —— 业务写完但 outbox 写失败 = 事件静默丢失。现在直接 best-effort Publish。
	if err := l.svcCtx.ConversationRepo.AppendMessage(l.ctx, msg); err != nil {
		return err
	}
	_ = l.svcCtx.ConversationRepo.IncrementMessageCount(l.ctx, convID) // best-effort

	// 用 AppendMessage 已回填的 msg.ID
	d := evt.Data.(events.MessageCreatedData)
	d.MessageID = msg.ID
	evt.Data = d

	// §P0-8:OutboxRepo 在退化路径(DB nil)下也不调用——避免拆开写黑洞。
	// 直接 best-effort Publish(原行为);dev fallback 同路径。
	if l.svcCtx.EventPublisher != nil {
		if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, evt); err != nil {
			slog.ErrorContext(l.ctx, "publish message.created failed (no DB, dev only)", "err", err)
		}
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
//
// Stage 69 修复：调 UpsertNeutralEmotion 前必须把 uid 注入 outgoing metadata
// 的 x-user-id 头，否则 ai-svc gRPC server NewServerUserIDInterceptor
// （shared/pkg/grpcinterceptor/auth.go）从 incoming metadata 找不到
// x-user-id → 返 codes.Unauthenticated。修复前 docker logs 出现
// "missing x-user-id metadata"。
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
	// Stage 69：注入 x-user-id metadata 让 ai-svc 拦截器能读到 user_id
	outCtx := metadata.AppendToOutgoingContext(l.ctx, "x-user-id", strconv.FormatInt(uid, 10))
	if _, err := l.svcCtx.AIClient.UpsertNeutralEmotion(outCtx, req); err != nil {
		slog.ErrorContext(l.ctx, "dev fallback UpsertNeutralEmotion failed",
			"msg_id", messageID, "event_id", eventID, "err", err)
	}
}