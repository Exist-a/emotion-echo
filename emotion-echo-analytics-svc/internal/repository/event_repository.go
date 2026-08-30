// Package repository 定义 analytics-svc 的数据访问层
package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"emotion-echo-analytics-svc/internal/model"

	"gorm.io/gorm"
)

// EventRepo 行为事件仓储接口
//
// Stage 30-A Round 2 扩展：增加 user-behavior 查询方法（day-night 桶、
// interaction depth、frequency trend）。这些是 read-only 聚合，仍由
// analytics_reader role 走 search_path。
type EventRepo interface {
	GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error)
	Create(ctx context.Context, e *model.UserBehaviorEvent) error

	// GetDayNightPattern 按 24 小时桶聚合指定用户在 [start, end] 内
	// 的事件。hour=0..23 → count。返回桶必须 len=24（缺失的 hour 计为 0）。
	GetDayNightPattern(ctx context.Context, userID int64, start, end time.Time) (map[int]int64, error)

	// GetInteractionDepth 返回 [start, end] 窗口内用户活跃度指标：
	//   totalMessages  消息总数
	//   totalConversations  会话总数
	//   avgMessagesPerConv  平均会话消息数数（保留 2 位小数）
	//   longestConversationMs  最长单次会话跨度（毫秒）
	GetInteractionDepth(ctx context.Context, userID int64, start, end time.Time) (*InteractionDepth, error)

	// GetFrequencyTrend 返回 [start, end] 内按天聚合的事件总数
	// （daily granularity，窗口最大 90 天）。
	GetFrequencyTrend(ctx context.Context, userID int64, start, end time.Time) ([]DailyCount, error)

	Ping(ctx context.Context) error
}

// InteractionDepth 用户交互深度指标
type InteractionDepth struct {
	TotalMessages           int64   `json:"totalMessages"`
	TotalConversations      int64   `json:"totalConversations"`
	AvgMessagesPerConv      float64 `json:"avgMessagesPerConv"`
	LongestConversationMs  int64   `json:"longestConversationMs"`
}

// DailyCount 单日事件计数
type DailyCount struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	Count int64  `json:"count"`
}

var ErrNotFound = errors.New("analytics: event not found")

type InMemoryEventRepo struct {
	mu     sync.RWMutex
	data   map[int64]*model.UserBehaviorEvent
	nextID int64
}

func NewInMemoryEventRepo() *InMemoryEventRepo {
	return &InMemoryEventRepo{data: make(map[int64]*model.UserBehaviorEvent), nextID: 1}
}

func (r *InMemoryEventRepo) GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if e, ok := r.data[id]; ok {
		return e, nil
	}
	return nil, nil
}

func (r *InMemoryEventRepo) Create(ctx context.Context, e *model.UserBehaviorEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == 0 {
		e.ID = r.nextID
		r.nextID++
	}
	r.data[e.ID] = e
	return nil
}

func (r *InMemoryEventRepo) Ping(ctx context.Context) error { return nil }

type PostgresEventRepo struct{ db *gorm.DB }

func NewPostgresEventRepo(db *gorm.DB) *PostgresEventRepo {
	return &PostgresEventRepo{db: db}
}

func (r *PostgresEventRepo) GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error) {
	var e model.UserBehaviorEvent
	err := r.db.WithContext(ctx).First(&e, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (r *PostgresEventRepo) Create(ctx context.Context, e *model.UserBehaviorEvent) error {
	e.ID = 0
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *PostgresEventRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}