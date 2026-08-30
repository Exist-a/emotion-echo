// Package repository — mentalhealth_repository.go
//
// Stage 30-A Round 3 RED: MentalHealthRepo 跨 schema 只读仓储。
//
// 数据源（per docs/stage-30-A §三.1）：
//   - emotion_echo_assessment.assessment_v
//   - emotion_echo_ai.daily_emotion_v（趋势）
//   - emotion_echo_analytics.mv_daily_emotion（materialized view，日报加速）
//
// 本文件仅声明接口；Round 3 GREEN 实现 InMemoryMentalHealthRepo +
// Round 5 落地 PostgresMentalHealthRepo + 真 SQL。
package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"gorm.io/gorm"
)

// AssessmentType assessment type 枚举
//
// daily         : 当日最近一次 assessment（最近 24h）
// weekly        : 最近 7 天汇总
// comprehensive : PHQ-9 / GAD-7 / PSQI 综合（per 用户的多次评估）
type AssessmentType string

const (
	AssessmentDaily         AssessmentType = "daily"
	AssessmentWeekly        AssessmentType = "weekly"
	AssessmentComprehensive AssessmentType = "comprehensive"
)

// IsValidAssessmentType 校验 type 字符串
func IsValidAssessmentType(t string) bool {
	switch AssessmentType(t) {
	case AssessmentDaily, AssessmentWeekly, AssessmentComprehensive:
		return true
	}
	return false
}

// MentalAssessment 单次评估聚合结果
type MentalAssessment struct {
	UserID        int64           `json:"userId"`
	Type          string          `json:"type"`
	WindowStart   string          `json:"windowStart"`   // YYYY-MM-DD
	WindowEnd     string          `json:"windowEnd"`     // YYYY-MM-DD
	OverallScore  float64         `json:"overallScore"`  // 综合评分 0-100
	RiskLevel     string          `json:"riskLevel"`     // low|moderate|high|severe
	Dimensions    []DimensionScore `json:"dimensions"`    // 维度明细
	GeneratedAt   time.Time       `json:"generatedAt"`
}

// DimensionScore 单维度评分（PHQ-9 depression / GAD-7 anxiety / PSQI sleep）
type DimensionScore struct {
	Name      string  `json:"name"`      // e.g. "depression"
	Score     float64 `json:"score"`     // 0-100 normalized
	RiskLevel string  `json:"riskLevel"`
	Count     int     `json:"count"`     // 该维度被评估次数
}

// AssessmentHistoryItem 历史记录项
type AssessmentHistoryItem struct {
	ID           uint64    `json:"id"`
	UserID       int64     `json:"userId"`
	AssessmentType string  `json:"assessmentType"`
	PeriodStart  string    `json:"periodStart"`
	PeriodEnd    string    `json:"periodEnd"`
	OverallScore float64   `json:"overallScore"`
	RiskLevel    string    `json:"riskLevel"`
	SubmittedAt  time.Time `json:"submittedAt"`
}

// MentalHealthRepo 跨 schema 只读仓储（mental-health 聚合 + 历史）
type MentalHealthRepo interface {
	// GetLatestAssessment 取指定用户在指定 type 下的最近评估
	GetLatestAssessment(ctx context.Context, userID int64, atype AssessmentType) (*MentalAssessment, error)

	// ListAssessmentHistory 按 cursor 分页列出历史
	//
	// cursor: 上次响应的 nextCursor（首次传 ""）
	// limit: 1-100（>100 clamp 到 100，<=0 clamp 到 20）
	ListAssessmentHistory(ctx context.Context, userID int64, atype string, cursor string, limit int) (items []AssessmentHistoryItem, nextCursor string, err error)

	// GetTrendData 返回 trend series（weekly / monthly）
	//
	// type: weekly | monthly
	// points: 按时间从早到晚排序
	GetTrendData(ctx context.Context, userID int64, trendType string, startDate, endDate time.Time) ([]TrendPoint, error)

	Ping(ctx context.Context) error
}
// =====================================================
// InMemoryMentalHealthRepo（Round 3 GREEN 测试替身）
// =====================================================

// InMemoryMentalHealthRepo Round 3 GREEN 用的内存版 MentalHealthRepo。
//
// 当前内存实现为占位（Round 5 migrations 落地后再用真实 SQL）；
// tests 主要通过 stub 注入返回值验证 Logic 层的契约。
type InMemoryMentalHealthRepo struct{}

// NewInMemoryMentalHealthRepo 构造空 repo
func NewInMemoryMentalHealthRepo() *InMemoryMentalHealthRepo {
	return &InMemoryMentalHealthRepo{}
}

// GetLatestAssessment Round 3 GREEN 占位
func (r *InMemoryMentalHealthRepo) GetLatestAssessment(_ context.Context, _ int64, _ AssessmentType) (*MentalAssessment, error) {
	return nil, errors.New("InMemoryMentalHealthRepo.GetLatestAssessment: 占位实现，测试应注入 stub")
}

// ListAssessmentHistory Round 3 GREEN 占位
func (r *InMemoryMentalHealthRepo) ListAssessmentHistory(_ context.Context, _ int64, _, _ string, _ int) ([]AssessmentHistoryItem, string, error) {
	return nil, "", errors.New("InMemoryMentalHealthRepo.ListAssessmentHistory: 占位实现，测试应注入 stub")
}

// GetTrendData Round 3 GREEN 占位
func (r *InMemoryMentalHealthRepo) GetTrendData(_ context.Context, _ int64, _ string, _, _ time.Time) ([]TrendPoint, error) {
	return nil, errors.New("InMemoryMentalHealthRepo.GetTrendData: 占位实现，测试应注入 stub")
}

// Ping 实现 MentalHealthRepo 接口
func (r *InMemoryMentalHealthRepo) Ping(_ context.Context) error { return nil }

// 编译期断言
var _ MentalHealthRepo = (*InMemoryMentalHealthRepo)(nil)

// =====================================================
// PostgresMentalHealthRepo（Round 5 落地真 SQL）
// =====================================================

// PostgresMentalHealthRepo 跨 schema 只读仓储：mental-health 聚合 + 历史。
//
// 数据源：emotion_echo_assessment.assessment_v（VIEW 暴露真实存在的列；
// risk_level 由 Go 侧 riskFromScore 从 overall_score 推导）。
type PostgresMentalHealthRepo struct {
	db *gorm.DB
}

// NewPostgresMentalHealthRepo 构造（DB 已通过 search_path 配好）
func NewPostgresMentalHealthRepo(db *gorm.DB) *PostgresMentalHealthRepo {
	return &PostgresMentalHealthRepo{db: db}
}

// riskFromScore 把 0-100 overall_score 推导为风险等级
//
// 阈值：<40 low，<60 moderate，<80 high，>=80 severe。
func riskFromScore(score float64) string {
	switch {
	case score >= 80:
		return "severe"
	case score >= 60:
		return "high"
	case score >= 40:
		return "moderate"
	default:
		return "low"
	}
}

// assessmentRow assessment_v 原始行
type assessmentRow struct {
	ID             uint64
	UserID         int64
	AssessmentType string
	PeriodStart    *time.Time
	PeriodEnd      *time.Time
	OverallScore   float64
	Dimensions     []byte
	CreatedAt      time.Time
}

// GetLatestAssessment 取指定用户在指定窗口下的最近评估。
//
//   - daily: created_at >= now-24h
//   - weekly: created_at >= now-7d
//   - comprehensive: 不限制窗口（最新一条）
//   - 无结果 → (nil, nil)（合法，不返 error）
func (r *PostgresMentalHealthRepo) GetLatestAssessment(ctx context.Context, userID int64, atype AssessmentType) (*MentalAssessment, error) {
	const q = `
SELECT id, user_id, assessment_type, period_start, period_end,
       overall_score, dimensions, created_at
FROM emotion_echo_assessment.assessment_v
WHERE user_id = $1
  AND ($2::timestamptz IS NULL OR created_at >= $2::timestamptz)
ORDER BY created_at DESC
LIMIT 1`

	var windowStart *time.Time
	switch atype {
	case AssessmentDaily:
		w := time.Now().Add(-24 * time.Hour)
		windowStart = &w
	case AssessmentWeekly:
		w := time.Now().Add(-7 * 24 * time.Hour)
		windowStart = &w
	}

	var row assessmentRow
	if err := r.db.WithContext(ctx).Raw(q, userID, windowStart).Scan(&row).Error; err != nil {
		return nil, err
	}
	if row.ID == 0 {
		return nil, nil // 无结果
	}

	return &MentalAssessment{
		UserID:        row.UserID,
		Type:          string(atype),
		WindowStart:   formatDatePtr(row.PeriodStart),
		WindowEnd:     formatDatePtr(row.PeriodEnd),
		OverallScore:  row.OverallScore,
		RiskLevel:     riskFromScore(row.OverallScore),
		Dimensions:    parseDimensions(row.Dimensions),
		GeneratedAt:   row.CreatedAt,
	}, nil
}

// ListAssessmentHistory 按 id DESC keyset 分页列出历史。
//
// cursor: 上次响应的 nextCursor（首次 ""，十进制 id）；limit: 1-100。
// nextCursor 语义：还有更多 → 本页最后一条的 id；没有 → ""。
func (r *PostgresMentalHealthRepo) ListAssessmentHistory(ctx context.Context, userID int64, atype, cursor string, limit int) ([]AssessmentHistoryItem, string, error) {
	var cursorID int64
	if cursor != "" {
		c, err := strconv.ParseInt(cursor, 10, 64)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor %q: %w", cursor, err)
		}
		cursorID = c
	}

	const q = `
SELECT id, user_id, assessment_type, period_start, period_end, overall_score, created_at
FROM emotion_echo_assessment.assessment_v
WHERE user_id = $1
  AND ($2 = '' OR assessment_type = $2)
  AND ($3 = 0 OR id < $3)
ORDER BY id DESC
LIMIT $4`

	var rows []struct {
		ID             uint64
		UserID         int64
		AssessmentType string
		PeriodStart    *time.Time
		PeriodEnd      *time.Time
		OverallScore   float64
		CreatedAt      time.Time
	}
	// 多取 1 行判断是否还有更多
	if err := r.db.WithContext(ctx).Raw(q, userID, atype, cursorID, limit+1).Scan(&rows).Error; err != nil {
		return nil, "", err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	items := make([]AssessmentHistoryItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, AssessmentHistoryItem{
			ID:             row.ID,
			UserID:         row.UserID,
			AssessmentType: row.AssessmentType,
			PeriodStart:    formatDatePtr(row.PeriodStart),
			PeriodEnd:      formatDatePtr(row.PeriodEnd),
			OverallScore:   row.OverallScore,
			RiskLevel:      riskFromScore(row.OverallScore),
			SubmittedAt:    row.CreatedAt,
		})
	}
	next := ""
	if hasMore {
		next = strconv.FormatUint(items[len(items)-1].ID, 10)
	}
	return items, next, nil
}

// GetTrendData 返回 trend series（weekly=7 天桶 / monthly=30 天桶）。
//
// TrendPoint 复用：AvgSentiment = 桶内平均 overall_score；
// PrimaryEmotion = 桶平均分的风险等级；AvgConfidence = 0（无 confidence 数据）。
// 空桶补 Count=0（与 buildTrendPoints 一致）。
func (r *PostgresMentalHealthRepo) GetTrendData(ctx context.Context, userID int64, trendType string, startDate, endDate time.Time) ([]TrendPoint, error) {
	bucketDays := int64(7)
	if trendType == "monthly" {
		bucketDays = 30
	}

	const q = `
SELECT DATE_TRUNC('day', created_at)::date AS day,
       COUNT(*)::bigint AS cnt,
       AVG(overall_score)::float8 AS avg_score
FROM emotion_echo_assessment.assessment_v
WHERE user_id = $1 AND created_at::date BETWEEN $2::date AND $3::date
GROUP BY 1
ORDER BY 1`

	var rows []struct {
		Day      time.Time
		Cnt      int64
		AvgScore float64
	}
	if err := r.db.WithContext(ctx).Raw(q, userID,
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02"),
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	trendRows := make([]trendRow, 0, len(rows))
	for _, row := range rows {
		trendRows = append(trendRows, trendRow{
			Day:          row.Day,
			Emotion:      riskFromScore(row.AvgScore),
			Cnt:          row.Cnt,
			AvgSentiment: row.AvgScore,
		})
	}
	return buildTrendPoints(trendRows, bucketDays,
		startDate.Format("2006-01-02"), endDate.Format("2006-01-02")), nil
}

// formatDatePtr DATE 指针 → YYYY-MM-DD（nil → ""）
func formatDatePtr(d *time.Time) string {
	if d == nil {
		return ""
	}
	return d.Format("2006-01-02")
}

// parseDimensions 解析 assessment_v.dimensions JSONB → DimensionScore 列表。
//
// 支持两种形状（宽松解析，失败返 nil 不报错）：
//   - {"depression":{"score":45,"riskLevel":"moderate","count":2}}
//   - {"depression":45}
// 按 Name 排序保证响应确定性。
func parseDimensions(raw []byte) []DimensionScore {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	out := make([]DimensionScore, 0, len(m))
	for name, v := range m {
		d := DimensionScore{Name: name}
		var obj struct {
			Score     float64 `json:"score"`
			RiskLevel string  `json:"riskLevel"`
			Count     int     `json:"count"`
		}
		if err := json.Unmarshal(v, &obj); err == nil {
			d.Score = obj.Score
			d.RiskLevel = obj.RiskLevel
			d.Count = obj.Count
		} else if err := json.Unmarshal(v, &d.Score); err != nil {
			continue // 无法解析的维度跳过
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Ping 健康检查
func (r *PostgresMentalHealthRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

var _ MentalHealthRepo = (*PostgresMentalHealthRepo)(nil)
