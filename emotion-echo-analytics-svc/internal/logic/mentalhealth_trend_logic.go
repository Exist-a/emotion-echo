// Package logic — mentalhealth_trend_logic.go
//
// Stage 30-A Round 3 part 2 GREEN: GET /api/v1/mental-health/trend.
//
// 复用 reports_trend_logic 的日期窗口解析 + repo 委派模式。
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

// trendTypeWhitelist mentalhealth trend 的合法 type
var trendTypeWhitelist = map[string]bool{
	"weekly":  true,
	"monthly": true,
}

// MentalHealthTrendLogic 处理心境趋势
type MentalHealthTrendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMentalHealthTrendLogic 构造
func NewMentalHealthTrendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MentalHealthTrendLogic {
	return &MentalHealthTrendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetTrend 校验 + 委派 MentalHealthRepo.GetTrendData
func (l *MentalHealthTrendLogic) GetTrend(req *types.GetMentalHealthTrendReq) (*types.GetMentalHealthTrendResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}
	if !trendTypeWhitelist[req.Type] {
		return nil, fmt.Errorf("validation: invalid type %q (must be weekly|monthly)", req.Type)
	}

	start, end, err := parseDateWindow(req.StartDate, req.EndDate)
	if err != nil {
		return nil, err
	}

	if l.svcCtx.MentalHealthRepo == nil {
		return nil, errors.New("internal: MentalHealthRepo not configured")
	}

	points, err := l.svcCtx.MentalHealthRepo.GetTrendData(l.ctx, req.UserID, req.Type, start, end)
	if err != nil {
		return nil, fmt.Errorf("mental-health trend query: %w", err)
	}
	if points == nil {
		points = []repository.TrendPoint{}
	}

	return &types.GetMentalHealthTrendResp{
		Report: &repository.TrendReport{
			UserID:    req.UserID,
			Type:      req.Type,
			StartDate: req.StartDate,
			EndDate:   req.EndDate,
			Points:    points,
		},
	}, nil
}