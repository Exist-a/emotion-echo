// Package logic — mentalhealth_history_logic.go
//
// Stage 30-A Round 3 part 1 GREEN: GET /api/v1/mental-health/history.
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

// historyDefaultLimit 默认 page size
const historyDefaultLimit = 20

// historyMaxLimit 上限
const historyMaxLimit = 100

// MentalHealthHistoryLogic 处理评估历史分页查询
type MentalHealthHistoryLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMentalHealthHistoryLogic 构造
func NewMentalHealthHistoryLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MentalHealthHistoryLogic {
	return &MentalHealthHistoryLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// clampHistoryLimit 把 user-provided limit 落到 [1, 100]，<=0 用 default 20。
func clampHistoryLimit(limit int) int {
	if limit <= 0 {
		return historyDefaultLimit
	}
	if limit > historyMaxLimit {
		return historyMaxLimit
	}
	return limit
}

// ListHistory 校验 + clamp limit + 委派 repo
func (l *MentalHealthHistoryLogic) ListHistory(req *types.GetMentalHealthHistoryReq) (*types.GetMentalHealthHistoryResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}

	limit := clampHistoryLimit(req.Limit)

	if l.svcCtx.MentalHealthRepo == nil {
		return nil, errors.New("internal: MentalHealthRepo not configured")
	}

	items, nextCursor, err := l.svcCtx.MentalHealthRepo.ListAssessmentHistory(
		l.ctx, req.UserID, req.Type, req.Cursor, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("assessment history query: %w", err)
	}
	if items == nil {
		items = []repository.AssessmentHistoryItem{}
	}

	return &types.GetMentalHealthHistoryResp{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}