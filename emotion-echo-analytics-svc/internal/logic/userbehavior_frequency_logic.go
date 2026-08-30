// Package logic — userbehavior_frequency_logic.go
//
// Stage 30-A Round 2 GREEN: implements GET /api/v1/user-behavior/frequency.
//
// 30d 日计数（per backlog §二 / stage-30-A §三.3）。
// 当前实现无窗口上限；backlog 没强制 90 天 hard limit，留作
// 未来扩展点（响应大小限制）。
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

// UserBehaviorFrequencyLogic 处理频率趋势
type UserBehaviorFrequencyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUserBehaviorFrequencyLogic 构造
func NewUserBehaviorFrequencyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserBehaviorFrequencyLogic {
	return &UserBehaviorFrequencyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetFrequencyTrend 校验 + 委派
func (l *UserBehaviorFrequencyLogic) GetFrequencyTrend(req *types.GetFrequencyTrendReq) (*types.GetFrequencyTrendResp, error) {
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

	counts, err := l.svcCtx.EventRepo.GetFrequencyTrend(l.ctx, req.UserID, start, end)
	if err != nil {
		return nil, fmt.Errorf("frequency trend query: %w", err)
	}
	if counts == nil {
		counts = []repository.DailyCount{}
	}

	return &types.GetFrequencyTrendResp{Counts: counts}, nil
}