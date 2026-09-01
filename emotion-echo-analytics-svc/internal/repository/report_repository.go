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
	"fmt"
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
	UserID                       int64                    `json:"userId"`
	Date                         string                   `json:"date"` // YYYY-MM-DD
	EmotionCounts                map[string]int64         `json:"emotionCounts"`
	MessageCount                 int64                    `json:"messageCount"`
	ConversationCount            int64                    `json:"conversationCount"`
	AssessmentCount              int64                    `json:"assessmentCount"`
	AvgSentiment                 float64                  `json:"avgSentiment"`
	AvgConfidence                float64                  `json:"avgConfidence"`
	// Stage 34: 按模态细分（text/face/voice），前端 ECharts 可自动多 series。
	// 老字段 EmotionCounts 保留（向后兼容 = text 模态合并）。
	EmotionDistributionByModality *ModalityEmotionDistribution `json:"emotionDistributionByModality,omitempty"`
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
// PostgresReportRepo（生产实现 — 真 SQL 已落地）
// =====================================================

// PostgresReportRepo 通过 search_path 解析跨 schema VIEW 做只读聚合。
//
// 数据源（per stage-30-A §六.6.1-3）：
//   - emotion_echo_chat.msg_summary_v
//   - emotion_echo_ai.daily_emotion_v
//   - emotion_echo_assessment.assessment_v
//   - emotion_echo_analytics.user_behavior_events
type PostgresReportRepo struct {
	db *gorm.DB
}

// NewPostgresReportRepo 构造（DB 已通过 search_path 配好）
func NewPostgresReportRepo(db *gorm.DB) *PostgresReportRepo {
	return &PostgresReportRepo{db: db}
}

// GetDailyReport 跨 schema 只读聚合：单日报告。
//
// 数据源（per stage-30-A §六.6.1-3）：
//   - emotion_echo_ai.daily_emotion_v（emotion counts + avg sentiment/confidence）
//   - emotion_echo_chat.msg_summary_v（message count）
//   - emotion_echo_analytics.user_behavior_events（conversation count）
//   - emotion_echo_assessment.assessment_v（assessment count）
//
// 日期以 YYYY-MM-DD 字符串传入（`$n::date`），避免 timestamptz→date
// 依赖会话时区。空数据日返回全 0 + 空 map（不返 error）。
func (r *PostgresReportRepo) GetDailyReport(ctx context.Context, userID int64, date time.Time) (*DailyReport, error) {
	const q = `
SELECT
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_chat.msg_summary_v
              WHERE user_id = $1 AND send_time::date = $2::date), 0) AS message_count,
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_analytics.user_behavior_events
              WHERE user_id = $1 AND event_type = 'conversation' AND occurred_at::date = $2::date), 0) AS conversation_count,
    COALESCE((SELECT COUNT(*)::bigint FROM emotion_echo_assessment.assessment_v
              WHERE user_id = $1 AND created_at::date = $2::date), 0) AS assessment_count,
    COALESCE((SELECT AVG(sentiment_score)::float8 FROM emotion_echo_ai.daily_emotion_v
              WHERE user_id = $1 AND created_at::date = $2::date), 0) AS avg_sentiment,
    COALESCE((SELECT AVG(confidence)::float8 FROM emotion_echo_ai.daily_emotion_v
              WHERE user_id = $1 AND created_at::date = $2::date), 0) AS avg_confidence`

	var row struct {
		MessageCount      int64
		ConversationCount int64
		AssessmentCount   int64
		AvgSentiment      float64
		AvgConfidence     float64
	}
	if err := r.db.WithContext(ctx).Raw(q, userID, date.Format("2006-01-02")).Scan(&row).Error; err != nil {
		return nil, err
	}

	const qCounts = `
SELECT primary_emotion AS emotion, COUNT(*)::bigint AS cnt
FROM emotion_echo_ai.daily_emotion_v
WHERE user_id = $1 AND created_at::date = $2::date
GROUP BY 1`

	var counts []struct {
		Emotion string
		Cnt     int64
	}
	if err := r.db.WithContext(ctx).Raw(qCounts, userID, date.Format("2006-01-02")).Scan(&counts).Error; err != nil {
		return nil, err
	}
	emotionCounts := make(map[string]int64, len(counts))
	for _, c := range counts {
		emotionCounts[c.Emotion] = c.Cnt
	}

	return &DailyReport{
		UserID:            userID,
		Date:              date.Format("2006-01-02"),
		EmotionCounts:     emotionCounts,
		MessageCount:      row.MessageCount,
		ConversationCount: row.ConversationCount,
		AssessmentCount:   row.AssessmentCount,
		AvgSentiment:      row.AvgSentiment,
		AvgConfidence:     row.AvgConfidence,
	}, nil
}

// trendRow SQL 原始聚合行（day 级别）
type trendRow struct {
	Day           time.Time
	Emotion       string
	Cnt           int64
	AvgSentiment  float64
	AvgConfidence float64
}

// GetTrendReport 跨 schema 只读聚合：区间趋势。
//
// type: weekly|monthly|yearly（桶大小见 TrendBucketSize：7/30/365 天）。
// SQL 负责按桶日期 GROUP BY（DATE 域整数运算，不依赖会话时区），
// Go 侧把稀疏桶补成 [start, end] 的连续桶（空桶 Count=0）。
func (r *PostgresReportRepo) GetTrendReport(ctx context.Context, userID int64, trendType string, start, end time.Time) (*TrendReport, error) {
	bucket, ok := TrendBucketSize(trendType)
	if !ok {
		return nil, fmt.Errorf("invalid trend type %q", trendType)
	}
	bucketDays := int64(bucket / (24 * time.Hour))

	const q = `
SELECT bucket_date AS day, primary_emotion AS emotion,
       COUNT(*)::bigint AS cnt,
       AVG(sentiment_score)::float8 AS avg_sentiment,
       AVG(confidence)::float8 AS avg_confidence
FROM (
    SELECT *, ($2::date + ((created_at::date - $2::date) / $4) * $4) AS bucket_date
    FROM emotion_echo_ai.daily_emotion_v
    WHERE user_id = $1
      AND created_at::date BETWEEN $2::date AND $3::date
) t
GROUP BY 1, 2
ORDER BY 1`

	var rows []trendRow
	if err := r.db.WithContext(ctx).Raw(q, userID,
		start.Format("2006-01-02"), end.Format("2006-01-02"), bucketDays,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}

	return &TrendReport{
		UserID:    userID,
		Type:      trendType,
		StartDate: start.Format("2006-01-02"),
		EndDate:   end.Format("2006-01-02"),
		Points:    buildTrendPoints(rows, bucketDays, start.Format("2006-01-02"), end.Format("2006-01-02")),
	}, nil
}

// buildTrendPoints 把 day 级稀疏聚合补成 [startLabel, endLabel] 的连续桶。
//
// 桶边界 = startLabel + k*bucketDays（DATE 域整数运算，与时区无关）。
// 空桶：Count=0、主导情绪为空串、avg 为 0。有数据桶：主导情绪 = 桶内
// count 最大的 emotion；avg = 按 count 加权平均。
func buildTrendPoints(rows []trendRow, bucketDays int64, startLabel, endLabel string) []TrendPoint {
	startDay := parseDayLabel(startLabel)
	endDay := parseDayLabel(endLabel)

	// 稀疏行 → 桶聚合（按桶日期）
	type bucketAgg struct {
		primary  string
		maxCnt   int64
		totalCnt int64
		sumSent  float64
		sumConf  float64
	}
	byBucket := map[int64]*bucketAgg{}
	for _, row := range rows {
		day := parseDayLabel(row.Day.Format("2006-01-02"))
		if day < startDay {
			continue // SQL 已过滤，防御性跳过
		}
		k := (day - startDay) / bucketDays
		bucketDay := startDay + k*bucketDays
		agg, ok := byBucket[bucketDay]
		if !ok {
			agg = &bucketAgg{}
			byBucket[bucketDay] = agg
		}
		agg.totalCnt += row.Cnt
		agg.sumSent += row.AvgSentiment * float64(row.Cnt)
		agg.sumConf += row.AvgConfidence * float64(row.Cnt)
		if row.Cnt > agg.maxCnt {
			agg.maxCnt = row.Cnt
			agg.primary = row.Emotion
		}
	}

	points := make([]TrendPoint, 0)
	for b := startDay; b <= endDay; b += bucketDays {
		agg := byBucket[b]
		if agg == nil {
			points = append(points, TrendPoint{Date: dayLabel(b)})
			continue
		}
		p := TrendPoint{
			Date:           dayLabel(b),
			PrimaryEmotion: agg.primary,
			Count:          agg.totalCnt,
		}
		if agg.totalCnt > 0 {
			p.AvgSentiment = agg.sumSent / float64(agg.totalCnt)
			p.AvgConfidence = agg.sumConf / float64(agg.totalCnt)
		}
		points = append(points, p)
	}
	return points
}

// parseDayLabel 解析 YYYY-MM-DD → 距 epoch 的天数（UTC，与时区无关）
func parseDayLabel(label string) int64 {
	t, err := time.Parse("2006-01-02", label)
	if err != nil {
		return 0
	}
	return t.Unix() / 86400
}

// dayLabel 天数 → YYYY-MM-DD（UTC）
func dayLabel(day int64) string {
	return time.Unix(day*86400, 0).UTC().Format("2006-01-02")
}

// Ping 健康检查
func (r *PostgresReportRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}