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
	"time"
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