// Package repository 定义 ai-svc 的数据访问层
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrNotFound 在资源不存在时返回
var ErrNotFound = errors.New("ai: emotion analysis not found")

// EmotionRepo 情绪分析仓储接口
type EmotionRepo interface {
	// GetByID 按主键查（保留向后兼容）
	GetByID(ctx context.Context, id int64) (*model.EmotionAnalysis, error)
	// GetByMessageID 按 message_id 查（一条消息最多一条分析）
	GetByMessageID(ctx context.Context, messageID int64) (*model.EmotionAnalysis, error)
	// ListByConversationID 列出某会话的所有分析
	ListByConversationID(ctx context.Context, conversationID int64) ([]model.EmotionAnalysis, error)
	// Create 保存一条情绪分析结果
	Create(ctx context.Context, e *model.EmotionAnalysis) error
	// Ping 健康检查
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryEmotionRepo（测试替身）
// =====================================================

// InMemoryEmotionRepo 内存实现，按 messageID 建索引加速查询
//
// byEventID 索引用于消费幂等去重（Stage 30-C A1）：同一 EventID 二次 Create
// 直接返回 nil，不分配新 ID、不入 byID（与 PG ON CONFLICT DO NOTHING 语义一致）。
// 空 EventID（gRPC 同步分析路径）不参与去重。
type InMemoryEmotionRepo struct {
	mu             sync.RWMutex
	byID           map[int64]*model.EmotionAnalysis
	byMessageID    map[int64]int64 // messageID → analysis ID
	byConversation map[int64][]int64
	byEventID      map[string]int64 // eventID → analysis ID（幂等键）
	nextID         int64
}

func NewInMemoryEmotionRepo() *InMemoryEmotionRepo {
	return &InMemoryEmotionRepo{
		byID:           make(map[int64]*model.EmotionAnalysis),
		byMessageID:    make(map[int64]int64),
		byConversation: make(map[int64][]int64),
		byEventID:      make(map[string]int64),
		nextID:         1,
	}
}

func (r *InMemoryEmotionRepo) GetByID(ctx context.Context, id int64) (*model.EmotionAnalysis, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.byID[id]; ok {
		return e, nil
	}
	return nil, nil
}

func (r *InMemoryEmotionRepo) GetByMessageID(ctx context.Context, messageID int64) (*model.EmotionAnalysis, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byMessageID[messageID]
	if !ok {
		return nil, nil
	}
	return r.byID[id], nil
}

func (r *InMemoryEmotionRepo) ListByConversationID(ctx context.Context, conversationID int64) ([]model.EmotionAnalysis, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := r.byConversation[conversationID]
	out := make([]model.EmotionAnalysis, 0, len(ids))
	for _, id := range ids {
		if e, ok := r.byID[id]; ok {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (r *InMemoryEmotionRepo) Create(ctx context.Context, e *model.EmotionAnalysis) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// 幂等去重（Stage 30-C A1）：同 EventID 第二次 Create 直接返回 nil，不分配 ID。
	// 语义与 Postgres ON CONFLICT (event_id) DO NOTHING 对齐。
	if e.EventID != "" {
		if existingID, ok := r.byEventID[e.EventID]; ok {
			if existing, hit := r.byID[existingID]; hit {
				*e = *existing
			}
			return nil
		}
	}
	if e.ID == 0 {
		e.ID = r.nextID
		r.nextID++
	}
	r.byID[e.ID] = e
	if e.MessageID != 0 {
		r.byMessageID[e.MessageID] = e.ID
	}
	if e.ConversationID != 0 {
		r.byConversation[e.ConversationID] = append(r.byConversation[e.ConversationID], e.ID)
	}
	if e.EventID != "" {
		r.byEventID[e.EventID] = e.ID
	}
	return nil
}

func (r *InMemoryEmotionRepo) Ping(ctx context.Context) error { return nil }

// =====================================================
// PostgresEmotionRepo（生产实现）
// =====================================================

type PostgresEmotionRepo struct{ db *gorm.DB }

func NewPostgresEmotionRepo(db *gorm.DB) *PostgresEmotionRepo {
	return &PostgresEmotionRepo{db: db}
}

func (r *PostgresEmotionRepo) GetByID(ctx context.Context, id int64) (*model.EmotionAnalysis, error) {
	var e model.EmotionAnalysis
	err := r.db.WithContext(ctx).First(&e, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (r *PostgresEmotionRepo) GetByMessageID(ctx context.Context, messageID int64) (*model.EmotionAnalysis, error) {
	var e model.EmotionAnalysis
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&e).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (r *PostgresEmotionRepo) ListByConversationID(ctx context.Context, conversationID int64) ([]model.EmotionAnalysis, error) {
	var out []model.EmotionAnalysis
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Create 持久化一条情绪分析。
//
// Stage 30-C A1：event_id 上挂 UNIQUE 约束，INSERT 走 ON CONFLICT DO NOTHING
// 实现消费幂等。at-least-once 投递下重复消息不重复落库。
// 空 EventID（gRPC 同步路径）走非去重分支（DB UNIQUE 允许多个 NULL）。
func (r *PostgresEmotionRepo) Create(ctx context.Context, e *model.EmotionAnalysis) error {
	e.ID = 0
	tx := r.db.WithContext(ctx)
	if e.EventID != "" {
		tx = tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "event_id"}},
			DoNothing: true,
		})
	}
	return tx.Create(e).Error
}

func (r *PostgresEmotionRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}