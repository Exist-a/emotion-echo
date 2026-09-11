// Package logic — pinconversationlogic.go
//
// Stage 72 GREEN：PinConversation RPC（决策 4 ADR §八 backlog 收口）
//
// 流程：
//  1. 鉴权（ctx 注入 user_id）
//  2. 会话存在 + owner 校验（只能置顶自己的会话）
//  3. repo.SetPinned 持久化（同时刷新 updated_at）
//
// 不发 outbox 事件：置顶是纯 UI 偏好，不属于 analytics-svc 统计的
// 用户行为事件（message.created / conversation.created / conversation.closed）。
package logic

import (
	"context"
	"errors"

	"emotion-echo-chat-svc/internal/repository"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"
)

// PinConversationLogic 处理置顶/取消置顶会话
type PinConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewPinConversationLogic 构造
func NewPinConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *PinConversationLogic {
	return &PinConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// PinConversation 置顶/取消置顶指定会话
func (l *PinConversationLogic) PinConversation(req *types.PinConversationReq) (resp *types.PinConversationResp, err error) {
	uid, ok := l.ctx.Value(sharedmw.CtxUserIDKey{}).(int64)
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

	if err := l.svcCtx.ConversationRepo.SetPinned(l.ctx, req.Id, req.IsPinned); err != nil {
		return nil, err
	}

	return &types.PinConversationResp{
		Success:  true,
		Id:       req.Id,
		IsPinned: req.IsPinned,
	}, nil
}
