// Package repository — Stage 34 · PR-16 GREEN
//
// ModalityReportRepo 提供按模态（text/face/voice）维度的情绪分布，
// 数据源是 ai-svc 的 emotion_echo_ai.daily_emotion_by_modality_v VIEW（Stage 34 migration 005）。
//
// 设计：
//   - InMemoryModalityReportRepo：测试替身，按 userID × date × modality × emotion 嵌套 map
//   - PostgresModalityReportRepo：生产实现，Raw SQL 调跨 schema VIEW
//
// 与现有 DailyReport.EmotionCounts 字段**并存**（向后兼容）：
//   - 老字段：emotionAnalysis 总计数（文本）
//   - 新字段：EmotionDistributionByModality 按模态细分（Stage 34 加）
package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ModalityEmotionDistribution 单日按模态分组的情绪计数。
//
// Text / Face / Voice 三路分别对应 emotion_echo_ai 上的三张表（emotion_analysis /
// face_emotion_results / voice_emotion_results）。Fused 留给未来 Stage 35+（融合结果
// 不与单模态并列聚合，避免双计）。
//
// 三 map 一律为非 nil 空 map（无数据也是空 map），便于前端无判断直接遍历。
type ModalityEmotionDistribution struct {
	Text  map[string]int64
	Face  map[string]int64
	Voice map[string]int64
}

// ModalityReportRepo 跨模态情绪分布仓储接口
type ModalityReportRepo interface {
	// GetDailyEmotionByModality 单日按模态聚合情绪计数
	GetDailyEmotionByModality(ctx context.Context, userID int64, date time.Time) (*ModalityEmotionDistribution, error)
	Ping(ctx context.Context) error
}

// InMemoryModalityReportRepo 测试替身。
//
// data 结构：userID → date → modality → emotion → count
//   - 第一层 map[int64]：按用户
//   - 第二层 map[time.Time]：按日期（精确到 day）
//   - 第三层 map[string]：按 modality（"text"/"face"/"voice"）
//   - 第四层 map[string]int64：按 emotion
type InMemoryModalityReportRepo struct {
	data map[int64]map[time.Time]map[string]map[string]int64
}

func NewInMemoryModalityReportRepo() *InMemoryModalityReportRepo {
	return &InMemoryModalityReportRepo{
		data: make(map[int64]map[time.Time]map[string]map[string]int64),
	}
}

// GetDailyEmotionByModality 是 ModalityReportRepo 接口实现。
func (r *InMemoryModalityReportRepo) GetDailyEmotionByModality(ctx context.Context, userID int64, date time.Time) (*ModalityEmotionDistribution, error) {
	out := &ModalityEmotionDistribution{
		Text:  map[string]int64{},
		Face:  map[string]int64{},
		Voice: map[string]int64{},
	}
	userData, ok := r.data[userID]
	if !ok {
		return out, nil
	}
	dayData, ok := userData[date]
	if !ok {
		return out, nil
	}
	for k, v := range dayData["text"] {
		out.Text[k] = v
	}
	for k, v := range dayData["face"] {
		out.Face[k] = v
	}
	for k, v := range dayData["voice"] {
		out.Voice[k] = v
	}
	return out, nil
}

func (r *InMemoryModalityReportRepo) Ping(ctx context.Context) error { return nil }

// copyEmotionMap 已删除：改为直接返回非 nil map（与 Postgres 实现行为对齐）

// =====================================================
// PostgresModalityReportRepo（生产实现 — 真 SQL 留集成测试）
// =====================================================

// PostgresModalityReportRepo 生产实现。
//
// SQL：调 emotion_echo_ai.daily_emotion_by_modality_v VIEW（Stage 34 migration 005），
// 按 user × date × modality × emotion 聚合，GROUP BY 返回行。
//
// 当前为占位，SQL 在 Stage 34 PR-16 内补齐（需 analytics-svc 集成测试验证）。
type PostgresModalityReportRepo struct{ db *gorm.DB }

func NewPostgresModalityReportRepo(db *gorm.DB) *PostgresModalityReportRepo {
	return &PostgresModalityReportRepo{db: db}
}

// GetDailyEmotionByModality 单日按模态聚合情绪计数。
//
// SQL：调 daily_emotion_by_modality_v VIEW，按 modality 拆出三个 map。
func (r *PostgresModalityReportRepo) GetDailyEmotionByModality(ctx context.Context, userID int64, date time.Time) (*ModalityEmotionDistribution, error) {
	const q = `
SELECT modality, primary_emotion, cnt
FROM emotion_echo_ai.daily_emotion_by_modality_v
WHERE user_id = $1 AND day = $2::date`

	type row struct {
		Modality  string
		Emotion   string
		Cnt       int64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Raw(q, userID, date.Format("2006-01-02")).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := &ModalityEmotionDistribution{
		Text:  map[string]int64{},
		Face:  map[string]int64{},
		Voice: map[string]int64{},
	}
	for _, r := range rows {
		switch r.Modality {
		case "text":
			out.Text[r.Emotion] = r.Cnt
		case "face":
			out.Face[r.Emotion] = r.Cnt
		case "voice":
			out.Voice[r.Emotion] = r.Cnt
		}
	}
	return out, nil
}

func (r *PostgresModalityReportRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}