// Package repository — CL-07（后端切片）Phase 2 · GREEN
//
// FusedReportRepo 提供"融合产物（fused_emotions）"的日聚合，
// 数据源是 ai-svc 的 emotion_echo_ai.daily_emotion_fused_v VIEW（CL-07 migration i010）。
//
// 与 ModalityReportRepo 的分工：
//   - ModalityReportRepo：按**单模态**（text/face/voice）拆分布
//   - FusedReportRepo：**收敛后**的综合情绪分布 + 融合方法占比（不与单模态并列，避免双计）
//
// 全部 map 一律非 nil（无数据也是空 map），前端可直接遍历/判空。

package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// FusedEmotionDistribution 单日"融合产物"聚合。
//
//   - ByEmotion：primary_emotion（收敛后综合情绪）→ 计数
//   - ByMethod：fusion_method（"llm" | "late_fusion_weighted"）→ 计数
//     ⇒ 可算"多少比例真 LLM 融合 / 多少降级加权"（面试硬指标）
//   - FaceRatio / VoiceRatio：当日融合消息中**实际带上** face / voice 模态的占比
//     ⇒ 回答"多模态到底用了多少"
//   - Total：当日融合消息总数（= Σ ByMethod = Σ ByEmotion）
type FusedEmotionDistribution struct {
	ByEmotion  map[string]int64 `json:"byEmotion"`
	ByMethod   map[string]int64 `json:"byMethod"`
	FaceRatio  float64          `json:"faceRatio"`
	VoiceRatio float64          `json:"voiceRatio"`
	Total      int64            `json:"total"`
}

// FusedReportRepo 融合情绪日聚合仓储接口
type FusedReportRepo interface {
	// GetDailyFusedEmotion 单日融合情绪聚合（无数据返回非 nil 空结构）
	GetDailyFusedEmotion(ctx context.Context, userID int64, date time.Time) (*FusedEmotionDistribution, error)
	Ping(ctx context.Context) error
}

func emptyFusedDistribution() *FusedEmotionDistribution {
	return &FusedEmotionDistribution{
		ByEmotion: map[string]int64{},
		ByMethod:  map[string]int64{},
	}
}

// fusedDayKey 把时间归一到"日历日"（UTC 零点）。
//
// 必要性：time.Time 作 map key 时**含时区** —— logic 层用 time.Local 解析日期，
// 测试/调用方可能用 time.UTC 存，二者日历日相同但 Time 不相等 ⇒ 查不到。
// 统一归一到 UTC 零点，使查找对时区不敏感（只认日历日，与 SQL `day` 语义一致）。
func fusedDayKey(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// =====================================================
// InMemoryFusedReportRepo（测试替身）
// =====================================================

// InMemoryFusedReportRepo 测试替身。data：userID → date → 分布。
type InMemoryFusedReportRepo struct {
	data map[int64]map[time.Time]*FusedEmotionDistribution
	err  error
}

func NewInMemoryFusedReportRepo() *InMemoryFusedReportRepo {
	return &InMemoryFusedReportRepo{
		data: make(map[int64]map[time.Time]*FusedEmotionDistribution),
	}
}

// SetErr 注入错误（测试错误传播路径）。
func (r *InMemoryFusedReportRepo) SetErr(err error) { r.err = err }

// SetDaily 注入某用户某日的融合分布（测试用；仿 InMemoryReportRepo.SetDaily）。
func (r *InMemoryFusedReportRepo) SetDaily(userID int64, date time.Time, d *FusedEmotionDistribution) {
	if r.data[userID] == nil {
		r.data[userID] = make(map[time.Time]*FusedEmotionDistribution)
	}
	r.data[userID][fusedDayKey(date)] = d
}

func (r *InMemoryFusedReportRepo) GetDailyFusedEmotion(_ context.Context, userID int64, date time.Time) (*FusedEmotionDistribution, error) {
	if r.err != nil {
		return nil, r.err
	}
	ud, ok := r.data[userID]
	if !ok {
		return emptyFusedDistribution(), nil
	}
	d, ok := ud[fusedDayKey(date)]
	if !ok || d == nil {
		return emptyFusedDistribution(), nil
	}
	// 深拷贝非 nil map，避免调用方改动污染测试替身内部状态
	out := emptyFusedDistribution()
	out.FaceRatio = d.FaceRatio
	out.VoiceRatio = d.VoiceRatio
	out.Total = d.Total
	for k, v := range d.ByEmotion {
		out.ByEmotion[k] = v
	}
	for k, v := range d.ByMethod {
		out.ByMethod[k] = v
	}
	return out, nil
}

func (r *InMemoryFusedReportRepo) Ping(_ context.Context) error { return nil }

// =====================================================
// PostgresFusedReportRepo（生产实现）
// =====================================================

// PostgresFusedReportRepo 生产实现：读跨 schema VIEW，Go 侧汇总为分布。
type PostgresFusedReportRepo struct{ db *gorm.DB }

func NewPostgresFusedReportRepo(db *gorm.DB) *PostgresFusedReportRepo {
	return &PostgresFusedReportRepo{db: db}
}

// GetDailyFusedEmotion 单日融合情绪聚合。
//
// SQL：调 daily_emotion_fused_v（user × day × emotion × fusion_method）。
// face_ratio / voice_ratio 是**分组内**占比 ⇒ 必须按 cnt 加权汇总到当日。
func (r *PostgresFusedReportRepo) GetDailyFusedEmotion(ctx context.Context, userID int64, date time.Time) (*FusedEmotionDistribution, error) {
	const q = `
SELECT primary_emotion, fusion_method, cnt, face_ratio, voice_ratio
FROM emotion_echo_ai.daily_emotion_fused_v
WHERE user_id = $1 AND day = $2::date`

	type row struct {
		PrimaryEmotion string
		FusionMethod   string
		Cnt            int64
		FaceRatio      float64
		VoiceRatio     float64
	}
	var rows []row
	if err := r.db.WithContext(ctx).Raw(q, userID, date.Format("2006-01-02")).Scan(&rows).Error; err != nil {
		return nil, err
	}

	out := emptyFusedDistribution()
	var weightedFace, weightedVoice float64
	for _, x := range rows {
		out.ByEmotion[x.PrimaryEmotion] += x.Cnt
		out.ByMethod[x.FusionMethod] += x.Cnt
		out.Total += x.Cnt
		weightedFace += x.FaceRatio * float64(x.Cnt)
		weightedVoice += x.VoiceRatio * float64(x.Cnt)
	}
	if out.Total > 0 {
		out.FaceRatio = weightedFace / float64(out.Total)
		out.VoiceRatio = weightedVoice / float64(out.Total)
	}
	return out, nil
}

func (r *PostgresFusedReportRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
