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
	"errors"
	"time"
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
// PostgresMentalHealthRepo（Round 5 落地真 SQL；当前 stub）
// =====================================================

type PostgresMentalHealthRepo struct{}

// NewPostgresMentalHealthRepo 构造
func NewPostgresMentalHealthRepo() *PostgresMentalHealthRepo {
	return &PostgresMentalHealthRepo{}
}

func (r *PostgresMentalHealthRepo) GetLatestAssessment(_ context.Context, _ int64, _ AssessmentType) (*MentalAssessment, error) {
	return nil, errors.New("PostgresMentalHealthRepo.GetLatestAssessment: Round 5 落地")
}

func (r *PostgresMentalHealthRepo) ListAssessmentHistory(_ context.Context, _ int64, _, _ string, _ int) ([]AssessmentHistoryItem, string, error) {
	return nil, "", errors.New("PostgresMentalHealthRepo.ListAssessmentHistory: Round 5 落地")
}

func (r *PostgresMentalHealthRepo) GetTrendData(_ context.Context, _ int64, _ string, _, _ time.Time) ([]TrendPoint, error) {
	return nil, errors.New("PostgresMentalHealthRepo.GetTrendData: Round 5 落地")
}

func (r *PostgresMentalHealthRepo) Ping(_ context.Context) error { return nil }

var _ MentalHealthRepo = (*PostgresMentalHealthRepo)(nil)
