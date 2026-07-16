// Package repository 定义 ai-svc 的数据访问层
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
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
type InMemoryEmotionRepo struct {
	mu             sync.RWMutex
	byID           map[int64]*model.EmotionAnalysis
	byMessageID    map[int64]int64 // messageID → analysis ID
	byConversation map[int64][]int64
	nextID         int64
}

func NewInMemoryEmotionRepo() *InMemoryEmotionRepo {
	return &InMemoryEmotionRepo{
		byID:           make(map[int64]*model.EmotionAnalysis),
		byMessageID:    make(map[int64]int64),
		byConversation: make(map[int64][]int64),
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

func (r *PostgresEmotionRepo) Create(ctx context.Context, e *model.EmotionAnalysis) error {
	e.ID = 0
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *PostgresEmotionRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}