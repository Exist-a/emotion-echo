// Package logic — mentalhealth_assessment_logic.go
//
// Stage 30-A Round 3 part 1 GREEN: GET /api/v1/mental-health/assessment.
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

// MentalHealthAssessmentLogic 处理最新评估查询
type MentalHealthAssessmentLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewMentalHealthAssessmentLogic 构造
func NewMentalHealthAssessmentLogic(ctx context.Context, svcCtx *svc.ServiceContext) *MentalHealthAssessmentLogic {
	return &MentalHealthAssessmentLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetLatestAssessment 校验 type + 委派 repo
//
// nil handling 契约：用户无评估记录 → 返回 nil resp（Logic 不返
// error）；caller（handler）根据 resp.Assessment == nil 决定 200 empty
// 或 404。
func (l *MentalHealthAssessmentLogic) GetLatestAssessment(req *types.GetMentalAssessmentReq) (*types.GetMentalAssessmentResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}
	if !repository.IsValidAssessmentType(req.Type) {
		return nil, fmt.Errorf("validation: invalid type %q (must be daily|weekly|comprehensive)", req.Type)
	}

	if l.svcCtx.MentalHealthRepo == nil {
		return nil, errors.New("internal: MentalHealthRepo not configured")
	}

	assessment, err := l.svcCtx.MentalHealthRepo.GetLatestAssessment(
		l.ctx, req.UserID, repository.AssessmentType(req.Type),
	)
	if err != nil {
		return nil, fmt.Errorf("mental assessment query: %w", err)
	}
	// nil 是合法返回值（用户无评估），不返 error

	return &types.GetMentalAssessmentResp{Assessment: assessment}, nil
}