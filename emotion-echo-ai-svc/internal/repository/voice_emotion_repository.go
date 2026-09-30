// Package repository — Stage 34 · PR-4 GREEN
//
// VoiceEmotionRepo 持久化 SenseVoice 分析结果。
//
// 设计：与 FaceEmotionRepo 完全同模式（PR-2 GREEN 已落地）。
//   - VoiceEmotionRepo interface（依赖反转）
//   - InMemoryVoiceEmotionRepo 测试替身
//   - PostgresVoiceEmotionRepo 生产实现，ON CONFLICT (upload_id) DO NOTHING
package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// VoiceEmotionRepo SenseVoice 结果仓储接口
type VoiceEmotionRepo interface {
	Create(ctx context.Context, v *model.VoiceEmotionResult) error
	GetByUploadID(ctx context.Context, uploadID string) (*model.VoiceEmotionResult, error)
	GetLatestByMessageID(ctx context.Context, messageID int64) (*model.VoiceEmotionResult, error)
	// Delete 软删除（Round 1 follow-up）：与 FaceEmotionRepo.Delete 同语义
	Delete(ctx context.Context, id int64) error
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryVoiceEmotionRepo（测试替身）
// =====================================================

type InMemoryVoiceEmotionRepo struct {
	mu             sync.RWMutex
	byID           map[int64]*model.VoiceEmotionResult
	byUploadID     map[string]int64
	byMessageIndex map[int64][]int64
	nextID         int64
}

func NewInMemoryVoiceEmotionRepo() *InMemoryVoiceEmotionRepo {
	return &InMemoryVoiceEmotionRepo{
		byID:           make(map[int64]*model.VoiceEmotionResult),
		byUploadID:     make(map[string]int64),
		byMessageIndex: make(map[int64][]int64),
		nextID:         1,
	}
}

func (r *InMemoryVoiceEmotionRepo) Create(ctx context.Context, v *model.VoiceEmotionResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if v.UploadID != "" {
		if existingID, ok := r.byUploadID[v.UploadID]; ok {
			if existing, hit := r.byID[existingID]; hit {
				*v = *existing
			}
			return nil
		}
	}

	if v.ID == 0 {
		v.ID = r.nextID
		r.nextID++
	}
	r.byID[v.ID] = v
	if v.UploadID != "" {
		r.byUploadID[v.UploadID] = v.ID
	}
	if v.MessageID != 0 {
		r.byMessageIndex[v.MessageID] = append(r.byMessageIndex[v.MessageID], v.ID)
	}
	return nil
}

func (r *InMemoryVoiceEmotionRepo) GetByUploadID(ctx context.Context, uploadID string) (*model.VoiceEmotionResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if uploadID == "" {
		return nil, nil
	}
	id, ok := r.byUploadID[uploadID]
	if !ok {
		return nil, nil
	}
	if v, hit := r.byID[id]; hit && !v.DeletedAt.Valid {
		return v, nil
	}
	return nil, nil
}

func (r *InMemoryVoiceEmotionRepo) GetLatestByMessageID(ctx context.Context, messageID int64) (*model.VoiceEmotionResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byMessageIndex[messageID]
	// 反向遍历找第一个未软删的（Round 1 follow-up）
	for i := len(ids) - 1; i >= 0; i-- {
		v, hit := r.byID[ids[i]]
		if hit && !v.DeletedAt.Valid {
			return v, nil
		}
	}
	return nil, nil
}

func (r *InMemoryVoiceEmotionRepo) Ping(ctx context.Context) error { return nil }

// Delete 软删除：设置 DeletedAt.Valid=true；与 FaceEmotionRepo.Delete 同模式
// （emotion_repository.go:142 EmotionRepo.Delete 软删语义对齐）。
func (r *InMemoryVoiceEmotionRepo) Delete(ctx context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.byID[id]
	if !ok {
		return nil
	}
	if v.DeletedAt.Valid {
		return nil
	}
	v.DeletedAt = gorm.DeletedAt{Time: time.Now(), Valid: true}
	return nil
}

// =====================================================
// PostgresVoiceEmotionRepo（生产实现）
// =====================================================

type PostgresVoiceEmotionRepo struct{ db *gorm.DB }

func NewPostgresVoiceEmotionRepo(db *gorm.DB) *PostgresVoiceEmotionRepo {
	return &PostgresVoiceEmotionRepo{db: db}
}

func (r *PostgresVoiceEmotionRepo) Create(ctx context.Context, v *model.VoiceEmotionResult) error {
	v.ID = 0
	normalizeJSONB(v)

	// 幂等去重：与 FaceEmotionRepo 同模式（先 SELECT 再 INSERT）
	if v.UploadID != "" {
		var existing model.VoiceEmotionResult
		err := r.db.WithContext(ctx).Where("upload_id = ?", v.UploadID).First(&existing).Error
		if err == nil {
			v.ID = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	// ⚠️ 同 face：`Where` 谓词必须与 i008 建的 partial 唯一索引逐字一致，
	// 否则 42P10。已在运行中的 dev 库上复现（voice 侧同样每次插入都失败）。
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "upload_id"}},
		// 用 TargetWhere 而不是 Where：GORM 把 TargetWhere 拼在冲突目标列**之后、
		// 动作之前**（`ON CONFLICT (col) WHERE <pred> DO NOTHING`），
		// 而 Where 被拼在动作**之后**（那是给 DO UPDATE 用的），位置错了会 42601。
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Neq{Column: clause.Column{Name: "upload_id"}, Value: "__legacy__"}}},
		DoNothing:   true,
	}).Create(v).Error
}

func (r *PostgresVoiceEmotionRepo) GetByUploadID(ctx context.Context, uploadID string) (*model.VoiceEmotionResult, error) {
	if uploadID == "" {
		return nil, nil
	}
	var v model.VoiceEmotionResult
	err := r.db.WithContext(ctx).Where("upload_id = ?", uploadID).First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *PostgresVoiceEmotionRepo) GetLatestByMessageID(ctx context.Context, messageID int64) (*model.VoiceEmotionResult, error) {
	var v model.VoiceEmotionResult
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Order("created_at DESC").
		First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (r *PostgresVoiceEmotionRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Delete 软删除：model 含 gorm.DeletedAt → UPDATE SET deleted_at = NOW()
func (r *PostgresVoiceEmotionRepo) Delete(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Delete(&model.VoiceEmotionResult{}, id).Error
}
