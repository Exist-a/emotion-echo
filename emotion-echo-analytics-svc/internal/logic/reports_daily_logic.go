// Package logic — reports_daily_logic.go
//
// Stage 30-A Round 1 GREEN: implements the GET /api/v1/reports/daily
// endpoint contract per docs/stage-30-A-analytics-business.md §二.
//
// The logic is thin: validation + delegation to ReportRepo. The
// heavy lifting (cross-schema aggregation) lives in
// repository/report_repository.go.
//
// Empty-date defaults to today (per test TestReportsDailyLogic_EmptyDate_DefaultsToToday).
// Bad date format rejected before repo call.
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

// ReportsDailyLogic 处理 GET /api/v1/reports/daily
type ReportsDailyLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

// NewReportsDailyLogic 构造 daily report logic
func NewReportsDailyLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ReportsDailyLogic {
	return &ReportsDailyLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// dateLayout YYYY-MM-DD
const dateLayout = "2006-01-02"

// GetDailyReport 校验 + 委派给 ReportRepo.GetDailyReport
//
//   - req.Date 为空 → today (UTC)
//   - req.Date 非空但格式错 → validation error（不调 repo）
//   - req.UserID <= 0 → validation error
func (l *ReportsDailyLogic) GetDailyReport(req *types.GetDailyReportReq) (*types.GetDailyReportResp, error) {
	if req == nil {
		return nil, errors.New("validation: request is nil")
	}
	if req.UserID <= 0 {
		return nil, errors.New("validation: user id must be positive")
	}

	dateStr := req.Date
	if dateStr == "" {
		// "today" — 用本地时区的午夜，避免跨时区偏移。
		// 业务边界：UTC vs Local 不影响 SQL（Postgres 也会按
		// Date 类型归到用户实际所在日）。本地化让测试更可重现。
		now := time.Now()
		dateStr = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format(dateLayout)
	}

	parsedDate, err := time.ParseInLocation(dateLayout, dateStr, time.Local)
	if err != nil {
		return nil, fmt.Errorf("validation: invalid date %q (expected YYYY-MM-DD): %w", req.Date, err)
	}

	if l.svcCtx.ReportRepo == nil {
		return nil, errors.New("internal: ReportRepo not configured")
	}

	report, err := l.svcCtx.ReportRepo.GetDailyReport(l.ctx, req.UserID, parsedDate)
	if err != nil {
		return nil, err
	}
	if report == nil {
		return nil, errors.New("internal: ReportRepo returned nil without error")
	}

	// 保证 EmotionCounts 序列化时是 {} 而不是 null（前端能正常渲染）
	if report.EmotionCounts == nil {
		report.EmotionCounts = map[string]int64{}
	}

	// Stage 34: 按模态细分（text/face/voice）— 复用同一 date 参数。
	// ModalityReportRepo 可选（nil 时跳过，前端通过 omitempty 隐藏字段）。
	if l.svcCtx.ModalityReportRepo != nil {
		modality, err := l.svcCtx.ModalityReportRepo.GetDailyEmotionByModality(l.ctx, req.UserID, parsedDate)
		if err != nil {
			return nil, err
		}
		if modality != nil {
			report.EmotionDistributionByModality = modality
		}
	}

	return &types.GetDailyReportResp{Report: report}, nil
}

// 编译期断言：保证 repository.DailyReport 与 types.DailyReport 同构
// （types/reports.go 里通过 type alias 引用了 repository 类型）。
var _ = repository.DailyReport{}