// Package logic — reports_trend_logic.go
//
// Stage 30-A Round 1 GREEN: implements the GET /api/v1/reports/trend
// endpoint contract per docs/stage-30-A-analytics-business.md §二.
//
// Trend type validation (weekly|monthly|yearly) lives in
// repository.TrendBucketSize — we delegate the whitelist check
// there and only error if the type is unknown.
//
// Date range validation: start_date <= end_date.
package logic

import (
	"context"
	"errors"
	"fmt"
	"time"

	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

// ReportsTrendLogic 处理 GET /api/v1/reports/trend
type ReportsTrendLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReportsTrendLogic 构造 trend report logic
func NewReportsTrendLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReportsTrendLogic {
	return &ReportsTrendLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// GetTrendReport 校验 + 委派给 ReportRepo.GetTrendReport
//
//   - req.Type 必须在 {weekly, monthly, yearly} 内（per TrendBucketSize）
//   - start_date 必须 <= end_date
//   - 日期格式 YYYY-MM-DD
func (l *ReportsTrendLogic) GetTrendReport(req *types.GetTrendReportReq) (*types.GetTrendReportResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}
	if _, ok := repository.TrendBucketSize(req.Type); !ok {
		return nil, fmt.Errorf("validation: invalid type %q (must be weekly|monthly|yearly)", req.Type)
	}

	startDate, err := time.ParseInLocation(dateLayout, req.StartDate, time.Local)
	if err != nil {
		return nil, fmt.Errorf("validation: invalid start_date %q: %w", req.StartDate, err)
	}
	endDate, err := time.ParseInLocation(dateLayout, req.EndDate, time.Local)
	if err != nil {
		return nil, fmt.Errorf("validation: invalid end_date %q: %w", req.EndDate, err)
	}
	if startDate.After(endDate) {
		return nil, fmt.Errorf("validation: start_date %s must be <= end_date %s",
			req.StartDate, req.EndDate)
	}

	if l.svcCtx.ReportRepo == nil {
		return nil, errors.New("internal: ReportRepo not configured")
	}

	report, err := l.svcCtx.ReportRepo.GetTrendReport(l.ctx, req.UserID, req.Type, startDate, endDate)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, errors.New("internal: ReportRepo returned nil without error")
	}

	// 保证 Points 序列化为 [] 而不是 null
	if report.Points == nil {
		report.Points = []repository.TrendPoint{}
	}

	return &types.GetTrendReportResp{Report: report}, nil
}

var _ = logx.WithContext