// Package repository 提供 ai-svc 的数据访问层。
//
// Stage 34 · PR-2 GREEN: FaceEmotionRepo 持久化 FER 分析结果。
//
// 设计原则（沿用 emotion_repository.go）：
//   - FaceEmotionRepo interface（依赖反转，AGENTS.md §三.1）
//   - InMemoryFaceEmotionRepo：测试替身，按 uploadID + messageID 建索引
//   - PostgresFaceEmotionRepo：生产实现，ON CONFLICT (upload_id) DO NOTHING 幂等
//
// UploadID 是前端去重 nonce（用户可能多端上传同一帧），DB 上挂 UNIQUE。
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound 资源不存在（保留向后兼容，与 emotion_repository 一致）。
var _ = errors.New // 当前未用，保留避免 import 警告

// FaceEmotionRepo FER 结果仓储接口
type FaceEmotionRepo interface {
	// Create 保存一条 FER 结果。
	//   - UploadID 已存在 → 静默幂等（不分配新 id，不抛错）
	//   - UploadID 为空 → 非幂等路径，照常分配 id
	Create(ctx context.Context, f *model.FaceEmotionResult) error

	// GetByUploadID 按前端上传去重键查
	GetByUploadID(ctx context.Context, uploadID string) (*model.FaceEmotionResult, error)

	// GetLatestByMessageID 同一 message 多次上传时取最新一条（按 created_at desc）
	// 用途：Fusion Worker 拼装 modality snapshot 时调用
	GetLatestByMessageID(ctx context.Context, messageID int64) (*model.FaceEmotionResult, error)

	// Ping 健康检查
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryFaceEmotionRepo（测试替身）
// =====================================================

// InMemoryFaceEmotionRepo 内存实现，按 uploadID + messageID 建索引。
//
// 语义对齐 Postgres ON CONFLICT (upload_id) DO NOTHING：
//   - 同 UploadID 第二次 Create 直接返回 nil，且 *f 被回填为已存在记录
//   - 空 UploadID 走非去重路径，照常分配 id
type InMemoryFaceEmotionRepo struct {
	mu             sync.RWMutex
	byID           map[int64]*model.FaceEmotionResult
	byUploadID     map[string]int64 // uploadID → result ID
	byMessageIndex map[int64][]int64 // messageID → result IDs（用于 GetLatestByMessageID）
	nextID         int64
}

func NewInMemoryFaceEmotionRepo() *InMemoryFaceEmotionRepo {
	return &InMemoryFaceEmotionRepo{
		byID:           make(map[int64]*model.FaceEmotionResult),
		byUploadID:     make(map[string]int64),
		byMessageIndex: make(map[int64][]int64),
		nextID:         1,
	}
}

func (r *InMemoryFaceEmotionRepo) Create(ctx context.Context, f *model.FaceEmotionResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 幂等去重（Stage 34 PR-2）：同 UploadID 第二次 Create 直接返回 nil，
	// *f 被回填为已存在记录（语义对齐 PG ON CONFLICT DO NOTHING + RETURNING）。
	if f.UploadID != "" {
		if existingID, ok := r.byUploadID[f.UploadID]; ok {
			if existing, hit := r.byID[existingID]; hit {
				*f = *existing
			}
			return nil
		}
	}

	if f.ID == 0 {
		f.ID = r.nextID
		r.nextID++
	}
	r.byID[f.ID] = f
	if f.UploadID != "" {
		r.byUploadID[f.UploadID] = f.ID
	}
	if f.MessageID != 0 {
		r.byMessageIndex[f.MessageID] = append(r.byMessageIndex[f.MessageID], f.ID)
	}
	return nil
}

func (r *InMemoryFaceEmotionRepo) GetByUploadID(ctx context.Context, uploadID string) (*model.FaceEmotionResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if uploadID == "" {
		return nil, nil
	}
	id, ok := r.byUploadID[uploadID]
	if !ok {
		return nil, nil
	}
	if e, hit := r.byID[id]; hit {
		return e, nil
	}
	return nil, nil
}

// GetLatestByMessageID 取同一 message 下 created_at 最新的一条。
//
// 注意：内存版按插入顺序近似 created_at desc（InMemoryEmotionRepo 同模式）。
// 生产 Postgres 版用 ORDER BY created_at DESC LIMIT 1。
func (r *InMemoryFaceEmotionRepo) GetLatestByMessageID(ctx context.Context, messageID int64) (*model.FaceEmotionResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byMessageIndex[messageID]
	if len(ids) == 0 {
		return nil, nil
	}
	// 取最后插入的（最新）。生产 SQL 用 ORDER BY created_at DESC LIMIT 1。
	latestID := ids[len(ids)-1]
	if e, hit := r.byID[latestID]; hit {
		return e, nil
	}
	return nil, nil
}

func (r *InMemoryFaceEmotionRepo) Ping(ctx context.Context) error { return nil }

// =====================================================
// PostgresFaceEmotionRepo（生产实现 — 真 SQL 留 PR-3+）
// =====================================================

// PostgresFaceEmotionRepo 生产实现。当前为占位，SQL 在后续 PR 补齐。
//
// 集成测试将在 Stage 34 PR-3+ 用 testcontainers-go 验：
//   - ON CONFLICT (upload_id) DO NOTHING 幂等
//   - ORDER BY created_at DESC LIMIT 1 取最新
type PostgresFaceEmotionRepo struct{ db *gorm.DB }

func NewPostgresFaceEmotionRepo(db *gorm.DB) *PostgresFaceEmotionRepo {
	return &PostgresFaceEmotionRepo{db: db}
}

func (r *PostgresFaceEmotionRepo) Create(ctx context.Context, f *model.FaceEmotionResult) error {
	f.ID = 0
	// JSONB 空串归一化为 '{}'：Postgres 不接受空串作为 JSONB 字面量。
	normalizeJSONB(f)

	// 幂等去重：UploadID 已存在则 backfill *f 为已存在记录（语义对齐 InMemoryEmotionRepo）。
	//
	// 设计：先 SELECT 拿 ID + 早退出（无冲突），再 INSERT（异常路径 ON CONFLICT 兜底）。
	// 用 SELECT 而不是 ON CONFLICT DO UPDATE SET id=id，是因为 PG 的
	// "column id is ambiguous" 在 conflict target = upload_id 时无法区分。
	if f.UploadID != "" {
		var existing model.FaceEmotionResult
		err := r.db.WithContext(ctx).Where("upload_id = ?", f.UploadID).First(&existing).Error
		if err == nil {
			// 已存在：回填 ID（不写）
			f.ID = existing.ID
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// 不存在：继续走 INSERT 路径（理论上 OnConflict 兜底，但实际很少触发）
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "upload_id"}},
		DoNothing: true,
	}).Create(f).Error
}

func (r *PostgresFaceEmotionRepo) GetByUploadID(ctx context.Context, uploadID string) (*model.FaceEmotionResult, error) {
	if uploadID == "" {
		return nil, nil
	}
	var f model.FaceEmotionResult
	err := r.db.WithContext(ctx).Where("upload_id = ?", uploadID).First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *PostgresFaceEmotionRepo) GetLatestByMessageID(ctx context.Context, messageID int64) (*model.FaceEmotionResult, error) {
	var f model.FaceEmotionResult
	err := r.db.WithContext(ctx).
		Where("message_id = ?", messageID).
		Order("created_at DESC").
		First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *PostgresFaceEmotionRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
