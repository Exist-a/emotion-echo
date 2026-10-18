// Package logic — updateconversationlogic.go
//
// Stage 72 GREEN：UpdateConversation RPC（决策 4 ADR §八 backlog 收口）
//
// 流程：
//  1. 鉴权（ctx 注入 user_id）
//  2. 入参校验（`title` 与 `agent_id` **可单传可同传；两者都空即拒** —— CL-06 前是"title 必填"，
//     若沿用会让"只切角色"的请求被标题校验挡住）
//  3. 会话存在 + owner 校验
//  4. repo 持久化（title → UpdateTitle；agent_id>0 → UpdateAgentID，同时刷新 updated_at）
//
// CL-06：`agent_id` 语义 = 本会话**当前角色**；0 表示"本次不改"（"用默认角色"由 NULL/0 表达）。
// 本层**不校验角色的属主**（角色归 user-svc，跨服务不耦合）——属主校验在 BFF 调用前完成。
//
// 不发 outbox 事件：改标题不产生用户行为事件。
package logic

import (
	"context"
	"errors"

	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
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

	if req.Title == "" && req.AgentId == 0 {
		return nil, errors.New("validation: title or agent_id is required")
	}
	if req.AgentId < 0 {
		return nil, errors.New("validation: agent_id must be non-negative")
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

	if req.Title != "" {
		if err := l.svcCtx.ConversationRepo.UpdateTitle(l.ctx, req.Id, req.Title); err != nil {
			return nil, err
		}
	}
	if req.AgentId > 0 {
		if err := l.svcCtx.ConversationRepo.UpdateAgentID(l.ctx, req.Id, req.AgentId); err != nil {
			return nil, err
		}
	}

	// 回显：标题以请求值为准（未传则回原值）；角色以生效值为准（未改则回原绑定，便于前端直接更新本地态）
	title := req.Title
	if title == "" {
		title = conv.Title
	}
	agentID := req.AgentId
	if agentID == 0 && conv.AgentID != nil {
		agentID = *conv.AgentID
	}

	return &types.UpdateConversationResp{
		Success: true,
		Id:      req.Id,
		Title:   title,
		AgentId: agentID,
	}, nil
}
