// Package logic — updateconversationlogic.go
//
// Stage 72 GREEN：UpdateConversation RPC（决策 4 ADR §八 backlog 收口）
//
// 流程：
//  1. 鉴权（ctx 注入 user_id）
//  2. 入参校验（title 必填，当前唯一可改字段）
//  3. 会话存在 + owner 校验
//  4. repo.UpdateTitle 持久化（同时刷新 updated_at）
//
// 不发 outbox 事件：改标题不产生用户行为事件。
package logic

import (
	"context"
	"errors"

	"emotion-echo-chat-svc/internal/repository"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"
)

// UpdateConversationLogic 处理更新会话
type UpdateConversationLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUpdateConversationLogic 构造
func NewUpdateConversationLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UpdateConversationLogic {
	return &UpdateConversationLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// UpdateConversation 更新指定会话
func (l *UpdateConversationLogic) UpdateConversation(req *types.UpdateConversationReq) (resp *types.UpdateConversationResp, err error) {
	uid, ok := l.ctx.Value(sharedmw.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		return nil, errors.New("unauthorized: missing user id")
	}

	if req.Title == "" {
		return nil, errors.New("validation: title is required")
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

	if err := l.svcCtx.ConversationRepo.UpdateTitle(l.ctx, req.Id, req.Title); err != nil {
		return nil, err
	}

	return &types.UpdateConversationResp{
		Success: true,
		Id:      req.Id,
		Title:   req.Title,
	}, nil
}
