// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package logic

import (
	"context"
	"errors"

	"emotion-echo-chat-svc/internal/middleware"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ListMessagesLogic 处理 GET /api/v1/conversations/:id/messages
//
// 仅返回当前用户所属会话的消息
type ListMessagesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListMessagesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListMessagesLogic {
	return &ListMessagesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ListMessages 列出会话下的消息
func (l *ListMessagesLogic) ListMessages(req *types.ListMessagesReq) (resp *types.ListMessagesResp, err error) {
	uid, ok := l.ctx.Value(middleware.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id")
	}

	// 鉴权：先查会话归属
	conv, err := l.svcCtx.ConversationRepo.GetConversationByID(l.ctx, req.Id)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New("not found: conversation")
	}
	if conv.UserID != uid {
		return nil, errors.New("forbidden: conversation does not belong to current user")
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}

	msgs, err := l.svcCtx.ConversationRepo.ListMessages(l.ctx, req.Id, limit)
	if err != nil {
		return nil, err
	}

	out := &types.ListMessagesResp{Messages: make([]types.MessageView, 0, len(msgs))}
	for _, m := range msgs {
		out.Messages = append(out.Messages, types.MessageView{
			Id:             m.ID,
			ConversationId: m.ConversationID,
			UserId:         m.UserID,
			Role:           m.Role,
			Content:        m.Content,
			TokensUsed:     m.TokensUsed,
			CreatedAt:      m.CreatedAt.UnixMilli(),
		})
	}
	return out, nil
}