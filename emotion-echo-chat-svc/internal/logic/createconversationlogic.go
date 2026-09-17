
package logic

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"log/slog"

	"emotion-echo-chat-svc/internal/events"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/google/uuid"
	
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
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCreateConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CreateConversationLogic {
	return &CreateConversationLogic{

		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// CreateConversation 新建一个会话
func (l *CreateConversationLogic) CreateConversation(req *types.CreateConversationReq) (resp *types.CreateConversationResp, err error) {
	// 1. 鉴权（由 AuthMiddleware 注入 user_id）
	uid, ok := l.ctx.Value(sharedmw.CtxUserIDKey{}).(int64)
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
		slog.ErrorContext(l.ctx, "CreateConversation persist failed", "err", err)
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
			IsPinned:  conv.Pinned,
			CreatedAt: conv.CreatedAt.UnixMilli(),
			UpdatedAt: conv.UpdatedAt.UnixMilli(),
		},
	}, nil
}

// persistWithOutbox 在事务中写业务 + outbox 行
//
// 路径优先级（Stage 94 PR-7 §P0-8 修复后）：
//   1. svcCtx.DB 非 nil && OutboxRepo 非 nil：开事务，业务 + outbox 同事务（生产场景）
//   2. svcCtx.DB 非 nil && OutboxRepo nil：开事务，只写业务（直 Publish 退化）
//   3. svcCtx.DB nil：业务直接 CreateConversation（非事务），事件走 EventPublisher.Publish
//      （best-effort；dev / 测试场景。注：原"路径 2 = DB nil + OutboxRepo 非 nil"的
//      拆分无事务写模式已被 §P0-8 修复移除——这种配置本就不安全:
//      业务写完但 outbox 写失败 → 事件静默丢失）
//
// §P0-8 修复要点:不允许"业务写成功后再独立调 CreateInTx(nil, ...)"模式。
// 退化路径(DB nil)直接走 best-effort Publish,允许事件丢失的可见降级
// (log 触发告警)而非"看似成功但实际黑洞"。
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

	// 路径 1 + 2：DB 齐备 → 事务化
	if l.svcCtx.DB != nil {
		if err := l.svcCtx.DB.Transaction(func(tx *gorm.DB) error {
			// 业务写（事务内）
			if err := l.svcCtx.ConversationRepo.CreateConversationTx(tx, l.ctx, conv); err != nil {
				return err
			}
			// 回填 evt.Data
			d := evt.Data.(events.ConversationCreatedData)
			d.ConversationID = conv.ID
			evt.Data = d
			// OutboxRepo 存在 → 同事务写 outbox
			if l.svcCtx.OutboxRepo != nil {
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
			}
			// OutboxRepo nil（prod 不该出现）→ 事务提交后 best-effort Publish
			// 事务内调 Publish 会阻塞事务,不当
			return nil
		}); err != nil {
			return err
		}
		// 事务外 best-effort Publish（仅 OutboxRepo nil 时）
		if l.svcCtx.OutboxRepo == nil {
			if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, evt); err != nil {
				slog.ErrorContext(l.ctx, "publish conversation.created failed (no outbox, dev only)", "err", err)
			}
		}
		return nil
	}

	// 路径 3：DB nil — dev / 测试场景。直接非事务业务写 + best-effort Publish
	if err := l.svcCtx.ConversationRepo.CreateConversation(l.ctx, conv); err != nil {
		return err
	}
	d := evt.Data.(events.ConversationCreatedData)
	d.ConversationID = conv.ID
	evt.Data = d

	if err := l.svcCtx.EventPublisher.Publish(l.ctx, events.TopicChatEvents, evt); err != nil {
		slog.ErrorContext(l.ctx, "publish conversation.created failed (no DB, dev only)", "err", err)
	}
	return nil
}