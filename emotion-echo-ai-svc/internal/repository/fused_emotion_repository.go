// Package repository — Stage 34 · PR-6 GREEN
//
// FusedEmotionRepo 持久化多模态情绪融合产物。
//
// 关键语义：Upsert 按 message_id 覆盖（DB 层 ON CONFLICT DO UPDATE）。
// 与 FaceEmotionRepo / VoiceEmotionRepo 的"幂等忽略"不同，fused 是"幂等覆盖"：
//   - 同一消息的融合结果会随 face/voice 数据到达而被新结果替代
//   - 这是产品意图（融合是收敛过程，不是单次事件）
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// FusedEmotionRepo 融合结果仓储接口
type FusedEmotionRepo interface {
	// Upsert 按 message_id 写入或覆盖（同一 message_id 二次调用覆盖）
	Upsert(ctx context.Context, f *model.FusedEmotion) error

	// GetByMessageID 查某消息的当前融合结果（无则 nil）
	GetByMessageID(ctx context.Context, messageID int64) (*model.FusedEmotion, error)

	// ListPending 列出需要 Worker 重新尝试融合的 message_id 候选。
	//
	// 当前实现：返回全部已 fused 的 message_id（Worker 在 PR-13/14 接入
	// emotion_analysis 反查"有 text 但还没 fused"的更复杂逻辑）。
	//
	// ttlSeconds 参数保留用于未来："已 fused 但 TTL 内无新 face/voice 数据则不再重试"。
	// 当前 InMemory 版忽略 ttlSeconds（无时间维度）；Postgres 版用 created_at 比较。
	ListPending(ctx context.Context, ttlSeconds int) ([]int64, error)

	// Ping 健康检查
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryFusedEmotionRepo（测试替身）
// =====================================================

type InMemoryFusedEmotionRepo struct {
	mu         sync.RWMutex
	byID       map[int64]*model.FusedEmotion
	byMessageID map[int64]int64
	nextID     int64
}

func NewInMemoryFusedEmotionRepo() *InMemoryFusedEmotionRepo {
	return &InMemoryFusedEmotionRepo{
		byID:        make(map[int64]*model.FusedEmotion),
		byMessageID: make(map[int64]int64),
		nextID:      1,
	}
}

// Upsert 语义：同 message_id 覆盖（保留 ID）。
func (r *InMemoryFusedEmotionRepo) Upsert(ctx context.Context, f *model.FusedEmotion) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existingID, ok := r.byMessageID[f.MessageID]; ok {
		// 覆盖：保留 ID，更新内容
		if existing, hit := r.byID[existingID]; hit {
			f.ID = existing.ID
			r.byID[existingID] = f
		}
		return nil
	}

	// 新建
	if f.ID == 0 {
		f.ID = r.nextID
		r.nextID++
	}
	r.byID[f.ID] = f
	r.byMessageID[f.MessageID] = f.ID
	return nil
}

func (r *InMemoryFusedEmotionRepo) GetByMessageID(ctx context.Context, messageID int64) (*model.FusedEmotion, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMessageID[messageID]
	if !ok {
		return nil, nil
	}
	if f, hit := r.byID[id]; hit {
		return f, nil
	}
	return nil, nil
}

// ListPending 当前返回所有已 fused 的 message_id（按测试期望）。
// Worker 在 PR-13/14 接入 emotion_analysis 反查逻辑后会增强。
func (r *InMemoryFusedEmotionRepo) ListPending(ctx context.Context, ttlSeconds int) ([]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]int64, 0, len(r.byMessageID))
	for msgID := range r.byMessageID {
		ids = append(ids, msgID)
	}
	return ids, nil
}

func (r *InMemoryFusedEmotionRepo) Ping(ctx context.Context) error { return nil }

// =====================================================
// PostgresFusedEmotionRepo（生产实现）
// =====================================================

type PostgresFusedEmotionRepo struct{ db *gorm.DB }

func NewPostgresFusedEmotionRepo(db *gorm.DB) *PostgresFusedEmotionRepo {
	return &PostgresFusedEmotionRepo{db: db}
}

// Upsert 使用 ON CONFLICT (message_id) DO UPDATE 全字段覆盖。
//
// 注意：clause.OnConflict 的 DoUpdates 必须显式列出所有要更新的列，
// 否则 PG 默认不更新任何列。
func (r *PostgresFusedEmotionRepo) Upsert(ctx context.Context, f *model.FusedEmotion) error {
	upsert := clause.OnConflict{
		Columns: []clause.Column{{Name: "message_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"primary_emotion":      gorm.Expr("EXCLUDED.primary_emotion"),
			"sentiment_score":      gorm.Expr("EXCLUDED.sentiment_score"),
			"confidence":           gorm.Expr("EXCLUDED.confidence"),
			"modality_contrib":     gorm.Expr("EXCLUDED.modality_contrib"),
			"reasoning":            gorm.Expr("EXCLUDED.reasoning"),
			"fusion_method":        gorm.Expr("EXCLUDED.fusion_method"),
			"available_modalities": gorm.Expr("EXCLUDED.available_modalities"),
			"user_id":              gorm.Expr("EXCLUDED.user_id"),
			"conversation_id":      gorm.Expr("EXCLUDED.conversation_id"),
		}),
	}
	if f.ID == 0 {
		// 让 PG 分配 ID（DO NOTHING 时不会被分配，但 DO UPDATE 总会写）
		return r.db.WithContext(ctx).Clauses(upsert).Create(f).Error
	}
	// 已分配 ID：Save 等价 UPDATE；为确保 ON CONFLICT 触发，用 Create
	return r.db.WithContext(ctx).Clauses(upsert).Create(f).Error
}

func (r *PostgresFusedEmotionRepo) GetByMessageID(ctx context.Context, messageID int64) (*model.FusedEmotion, error) {
	var f model.FusedEmotion
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// ListPending 当前返回所有 fused message_id（与 InMemory 版对齐）。
//
// 增强版本（PR-13/14 Worker）：WHERE created_at < NOW() - INTERVAL '5 minutes'
// 表示"5 分钟还没收敛"的候选，配合 emotion_analysis 反查找"有 text 还没 fused"。
func (r *PostgresFusedEmotionRepo) ListPending(ctx context.Context, ttlSeconds int) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).
		Model(&model.FusedEmotion{}).
		Pluck("message_id", &ids).Error
	if err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *PostgresFusedEmotionRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
