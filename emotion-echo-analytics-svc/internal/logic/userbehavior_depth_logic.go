// Package logic — userbehavior_depth_logic.go
//
// Stage 30-A Round 2 GREEN: implements GET /api/v1/user-behavior/depth.
package logic

import (
	"context"
	"errors"
	"fmt"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UserBehaviorDepthLogic 处理交互深度指标
type UserBehaviorDepthLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUserBehaviorDepthLogic 构造
func NewUserBehaviorDepthLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserBehaviorDepthLogic {
	return &UserBehaviorDepthLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetInteractionDepth 校验 + 委派
//
// AvgMessagesPerConv 的除零保护：
//   - 若 repo 返回 AvgMessagesPerConv = 0 且 TotalConversations = 0
//     （用户当日无会话），logic 不重写 — repo 自己负责返回 0
//   - 这避免了 "x / 0 = NaN" 的 panic
func (l *UserBehaviorDepthLogic) GetInteractionDepth(req *types.GetInteractionDepthReq) (*types.GetInteractionDepthResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}

	start, end, err := parseDateWindow(req.StartDate, req.EndDate)
	if err != nil {
		return nil, err
	}

	if l.svcCtx.EventRepo == nil {
		return nil, errors.New("internal: EventRepo not configured")
	}

	depth, err := l.svcCtx.EventRepo.GetInteractionDepth(l.ctx, req.UserID, start, end)
	if err != nil {
		return nil, fmt.Errorf("interaction depth query: %w", err)
	}
	if depth == nil {
		return nil, errors.New("internal: EventRepo returned nil without error")
	}

	return &types.GetInteractionDepthResp{Depth: depth}, nil
}

var _ = repository.InteractionDepth{}