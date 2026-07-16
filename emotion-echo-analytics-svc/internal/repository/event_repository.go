// Package repository 定义 analytics-svc 的数据访问层
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-analytics-svc/internal/model"

	"gorm.io/gorm"
)

// EventRepo 行为事件仓储接口
type EventRepo interface {
	GetByID(ctx context.Context, id int64) (*model.UserBehaviorEvent, error)
	Create(ctx context.Context, e *model.UserBehaviorEvent) error
	Ping(ctx context.Context) error
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