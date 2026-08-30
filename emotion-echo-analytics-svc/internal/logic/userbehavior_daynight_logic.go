// Package logic — userbehavior_daynight_logic.go
//
// Stage 30-A Round 2 GREEN: implements GET /api/v1/user-behavior/day-night.
//
// Logic 责任：
//   - 校验 userID / 日期
//   - 委派给 EventRepo.GetDayNightPattern
//   - 补齐缺失 hour（24 桶全部存在）
package logic

import (
	"context"
	"errors"
	"fmt"

	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// UserBehaviorDayNightLogic 处理昼夜模式聚合
type UserBehaviorDayNightLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewUserBehaviorDayNightLogic 构造
func NewUserBehaviorDayNightLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserBehaviorDayNightLogic {
	return &UserBehaviorDayNightLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetDayNightPattern 校验 + 委派 + 24 桶补齐
func (l *UserBehaviorDayNightLogic) GetDayNightPattern(req *types.GetDayNightPatternReq) (*types.GetDayNightPatternResp, error) {
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

	raw, err := l.svcCtx.EventRepo.GetDayNightPattern(l.ctx, req.UserID, start, end)
	if err != nil {
		return nil, fmt.Errorf("day-night pattern query: %w", err)
	}

	// 补齐 24 个 hour（缺失的填 0）— 让 JSON 响应始终是 24-keyed map
	pattern := make(map[int]int64, 24)
	for h := 0; h < 24; h++ {
		pattern[h] = raw[h]
	}

	return &types.GetDayNightPatternResp{Pattern: pattern}, nil
}