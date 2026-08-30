// Package repository — report_repository.go
//
// Stage 30-A Round 1 (GREEN): ReportRepo is the analytics-svc read-only
// cross-schema repository. Per docs/stage-30-A-analytics-business.md
// §三.1 "Pragmatic Reporting Database":
//
//   - reads from cross-schema VIEWs (emotion_echo_chat.msg_summary_v,
//     emotion_echo_ai.daily_emotion_v, emotion_echo_assessment.assessment_v,
//     emotion_echo_analytics.user_behavior_events / mv_daily_emotion)
//   - NEVER writes to other schemas (interface has only Read methods)
//   - implementation is a thin GORM layer; tests use InMemoryReportRepo
//
// RED phase of Round 1: this file declares the interface; the
// implementation (PostgresReportRepo + InMemoryReportRepo) is added
// in the GREEN commit alongside reports_daily/trend_logic.go.
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

// ReportRepo 跨 schema 只读聚合仓储（Stage 30-A §三.1）
//
// 实现必须只读：禁止写入 chat / ai / assessment schema。
// 所有查询都走 search_path 解析的 VIEW。
type ReportRepo interface {
	// GetDailyReport 返回指定日期的日报聚合
	//
	// 数据源：
	//   - emotion_echo_ai.daily_emotion_v（按 user_id + 日期聚 emotion）
	//   - emotion_echo_chat.msg_summary_v（按 user_id + 日期聚 message 数）
	//   - emotion_echo_assessment.assessment_v（按 user_id + 日期聚 assessment）
	//   - emotion_echo_analytics.user_behavior_events（按 user_id + 日期聚 conversation）
	GetDailyReport(ctx context.Context, userID int64, date time.Time) (*DailyReport, error)

	// GetTrendReport 返回区间趋势
	//
	// type: "weekly" | "monthly" | "yearly"
	// start_date / end_date: 区间边界（含）
	GetTrendReport(ctx context.Context, userID int64, trendType string, startDate, endDate time.Time) (*TrendReport, error)

	// Ping 健康检查（DB 可达）
	Ping(ctx context.Context) error
}

// DailyReport 单日报告聚合
type DailyReport struct {
	UserID             int64            `json:"userId"`
	Date               string           `json:"date"` // YYYY-MM-DD
	EmotionCounts      map[string]int64 `json:"emotionCounts"`
	MessageCount       int64            `json:"messageCount"`
	ConversationCount  int64            `json:"conversationCount"`
	AssessmentCount    int64            `json:"assessmentCount"`
	AvgSentiment       float64          `json:"avgSentiment"`
	AvgConfidence      float64          `json:"avgConfidence"`
}

// TrendPoint 趋势上一个数据点
type TrendPoint struct {
	Date           string  `json:"date"`           // bucket start
	PrimaryEmotion string  `json:"primaryEmotion"` // 该 bucket 主导情绪
	AvgSentiment   float64 `json:"avgSentiment"`
	AvgConfidence  float64 `json:"avgConfidence"`
	Count          int64   `json:"count"`
}

// TrendReport 区间趋势
type TrendReport struct {
	UserID    int64        `json:"userId"`
	Type      string       `json:"type"` // weekly | monthly | yearly
	StartDate string       `json:"startDate"`
	EndDate   string       `json:"endDate"`
	Points    []TrendPoint `json:"points"`
}

// trendTypeBucket 趋势类型 → 桶大小
var trendTypeBucket = map[string]time.Duration{
	"weekly":  7 * 24 * time.Hour,
	"monthly": 30 * 24 * time.Hour,
	"yearly":  365 * 24 * time.Hour,
}

// TrendBucketSize 给定 trend type 返回桶大小
//
// 用于 service 层把区间切成连续桶。也导出供 handler / logic
// 层做边界校验时复用。
func TrendBucketSize(trendType string) (time.Duration, bool) {
	d, ok := trendTypeBucket[trendType]
	return d, ok
}

// =====================================================
// InMemoryReportRepo（测试替身 — Round 1 GREEN）
// =====================================================

// InMemoryReportRepo 内存版 ReportRepo，用于 logic 单测。
//
// 实现策略：调用方（tests）注入预构造的 *DailyReport / *TrendReport，
// 仓库直接返回。最简单的 fake — 因为 logic 层只做了"校验 +
// 委派"，单测不需要模拟 SQL 行为（那属于 Round 5 集成测试）。
type InMemoryReportRepo struct {
	dailyReport  *DailyReport
	trendReport *TrendReport
	dailyErr    error
	trendErr    error
}

// NewInMemoryReportRepo 构造空 repo
func NewInMemoryReportRepo() *InMemoryReportRepo {
	return &InMemoryReportRepo{}
}

// SetDaily 注入 daily report 返回值
func (r *InMemoryReportRepo) SetDaily(d *DailyReport, err error) {
	r.dailyReport = d
	r.dailyErr = err
}

// SetTrend 注入 trend report 返回值
func (r *InMemoryReportRepo) SetTrend(t *TrendReport, err error) {
	r.trendReport = t
	r.trendErr = err
}

// GetDailyReport 实现 ReportRepo
func (r *InMemoryReportRepo) GetDailyReport(_ context.Context, _ int64, _ time.Time) (*DailyReport, error) {
	return r.dailyReport, r.dailyErr
}

// GetTrendReport 实现 ReportRepo
func (r *InMemoryReportRepo) GetTrendReport(_ context.Context, _ int64, _ string, _, _ time.Time) (*TrendReport, error) {
	return r.trendReport, r.trendErr
}

// Ping 实现 ReportRepo
func (r *InMemoryReportRepo) Ping(_ context.Context) error { return nil }

// =====================================================
// PostgresReportRepo（生产实现 — Round 1 GREEN，本 commit 仅
// 列出 SQL 契约；migration + 真连接在 Round 5 落地）
// =====================================================

// PostgresReportRepo 通过 search_path 解析跨 schema VIEW 做只读聚合。
//
// 数据源（per stage-30-A §六.6.1-3）：
//   - emotion_echo_chat.msg_summary_v
//   - emotion_echo_ai.daily_emotion_v / mv_daily_emotion
//   - emotion_echo_assessment.assessment_v
//   - emotion_echo_analytics.user_behavior_events
//
// Round 1 GREEN：仅搭骨架（依赖 gorm.DB），真实 SQL 在 Round 5
// migrations 落地后补齐。
type PostgresReportRepo struct {
	db *gorm.DB
}

// NewPostgresReportRepo 构造（DB 已通过 search_path 配好）
func NewPostgresReportRepo(db *gorm.DB) *PostgresReportRepo {
	return &PostgresReportRepo{db: db}
}

// GetDailyReport Round 1 GREEN 占位：真实 SELECT 在 Round 5 落地
func (r *PostgresReportRepo) GetDailyReport(_ context.Context, userID int64, date time.Time) (*DailyReport, error) {
	// 占位 — Round 5 实现跨 schema JOIN：
	//   SELECT emotion, COUNT(*) FROM emotion_echo_ai.daily_emotion_v
	//   WHERE user_id = $1 AND created_at::date = $2 GROUP BY emotion
	// 合并 msg_summary_v / assessment_v / user_behavior_events 聚合。
	_ = userID
	_ = date
	return nil, errors.New("PostgresReportRepo.GetDailyReport: Round 5 实现未落地")
}

// GetTrendReport Round 1 GREEN 占位
func (r *PostgresReportRepo) GetTrendReport(_ context.Context, userID int64, trendType string, start, end time.Time) (*TrendReport, error) {
	_ = userID
	_ = trendType
	_ = start
	_ = end
	return nil, errors.New("PostgresReportRepo.GetTrendReport: Round 5 实现未落地")
}

// Ping 健康检查
func (r *PostgresReportRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}