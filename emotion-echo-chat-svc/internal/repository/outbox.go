// Package repository — outbox.go
//
// Stage 30-C A3: 事务性 Outbox — chat-svc 侧实现。
//
// 背景：
//   chat-svc 三个 logic（create / send / delete）都是「DB 落库 → Publish」，
//   Publish 失败仅写日志 → 事件静默丢失。
//
// A3 方案（Transactional Outbox 模式）：
//   1. 业务表写入 + outbox_events 写入在**同一 DB 事务**
//   2. relay goroutine 周期性读 outbox 未发送行 → Publish → 标记已发
//   3. relay 重发天然会重复，靠 A1（event_id UNIQUE）兜底幂等
//
// 设计：
//   - CreateInTx 必须接受 *gorm.DB（让调用方开事务，业务表与 outbox 表同事务）
//   - ListPending 仅查 status='pending' 的行（按 created_at ASC）
//   - MarkSent / MarkFailed 是状态机迁移
//   - OutboxEvent 是 model struct（不是 model 包里的，因为只 chat-svc 用）
package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// OutboxStatus 是 outbox_events.status 字段的状态机
const (
	OutboxStatusPending = "pending"
	OutboxStatusSent    = "sent"
	OutboxStatusFailed  = "failed"
	// OutboxStatusDead ADR-19 PR-A6.1: 超过 MaxAttempts 的行,不再被 ListPending
	// 扫描,保留供人工排查/回放。dead 行不进 relay 队列。
	OutboxStatusDead = "dead"
)

// OutboxEvent 是 outbox_events 表对应 model
type OutboxEvent struct {
	ID           int64
	EventID      string // 业务事件 ID（UUID）— 与事件 payload 内 ID 字段一致，用于 A1 幂等去重
	EventType    string
	Topic        string
	Payload      []byte // 已序列化的事件 JSON
	Status       string
	Attempts     int
	LastError    string
	CreatedAt    time.Time
	SentAt       *time.Time
}

// TableName 显式 schema 名（与 chat-svc 其他表一致）
func (OutboxEvent) TableName() string { return "emotion_echo_chat.outbox_events" }

// OutboxRepo 接口
type OutboxRepo interface {
	// CreateInTx 在调用方提供的事务中写一条 outbox（关键：同事务保证原子性）
	// 调用方负责开/提交/回滚事务；本方法只负责 insert
	CreateInTx(tx *gorm.DB, e *OutboxEvent) error

	// ListPending 拉一批未发送行（status='pending'），按 created_at ASC
	ListPending(ctx context.Context, limit int) ([]OutboxEvent, error)

	// MarkSent 标记已发（status='sent' + sent_at=now）
	MarkSent(ctx context.Context, id int64) error

	// MarkFailed 增加 attempts + 记录错误（status 保留 'pending'，等下次重试）
	MarkFailed(ctx context.Context, id int64, errMsg string) error

	// MarkDead ADR-19 PR-A6.1: 把状态置为 'dead'（attempts 超阈值后由 relay 调用）
	// dead 行不再被 ListPending 扫描，保留供人工排查/回放。
	MarkDead(ctx context.Context, id int64, errMsg string) error

	// DeleteOlderThan Round 2.1 §D2: 删 status=X 且时间戳 < cutoff 的行。
	//   - status="sent"  → 用 sent_at 判定
	//   - status="dead"  → 用 created_at 判定（last_error 是 msg string 不能当时间）
	//   - limit > 0 时单轮最多删 limit 行（防长事务）
	// 返回删除行数。
	DeleteOlderThan(ctx context.Context, status string, cutoff time.Time, limit int) (int64, error)
}

// =====================================================
// InMemoryOutboxRepo（测试替身）
// =====================================================

// InMemoryOutboxRepo 内存实现，线程安全
type InMemoryOutboxRepo struct {
	mu       sync.RWMutex
	events   map[int64]*OutboxEvent
	nextID   int64
}

// NewInMemoryOutboxRepo 构造空仓库
func NewInMemoryOutboxRepo() *InMemoryOutboxRepo {
	return &InMemoryOutboxRepo{events: make(map[int64]*OutboxEvent), nextID: 1}
}

// CreateInTx InMemory 实现：tx 参数忽略（直接写）
func (r *InMemoryOutboxRepo) CreateInTx(_ *gorm.DB, e *OutboxEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e.ID == 0 {
		e.ID = r.nextID
		r.nextID++
	}
	if e.Status == "" {
		e.Status = OutboxStatusPending
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	// 深拷贝 payload（避免外部修改影响内部状态）
	payload := make([]byte, len(e.Payload))
	copy(payload, e.Payload)
	cp := *e
	cp.Payload = payload
	r.events[cp.ID] = &cp
	return nil
}

func (r *InMemoryOutboxRepo) ListPending(_ context.Context, limit int) ([]OutboxEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]OutboxEvent, 0)
	for _, e := range r.events {
		if e.Status == OutboxStatusPending {
			out = append(out, *e)
		}
	}
	// 简单排序：按 ID ASC（保证稳定顺序）
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ID < out[i].ID {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryOutboxRepo) MarkSent(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.events[id]
	if !ok {
		return errors.New("outbox: id not found")
	}
	now := time.Now()
	e.Status = OutboxStatusSent
	e.SentAt = &now
	return nil
}

func (r *InMemoryOutboxRepo) MarkFailed(_ context.Context, id int64, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.events[id]
	if !ok {
		return errors.New("outbox: id not found")
	}
	e.Attempts++
	e.LastError = errMsg
	// status 保留 pending（下次 relay 再试）；attempts 是失败次数
	return nil
}

func (r *InMemoryOutboxRepo) MarkDead(_ context.Context, id int64, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.events[id]
	if !ok {
		return errors.New("outbox: id not found")
	}
	e.Status = OutboxStatusDead
	e.LastError = errMsg
	return nil
}

// DeleteOlderThan Round 2.1 §D2 InMemory 实现：遍历 events map 删满足条件的行
//
// 判定规则（与 Postgres 实现保持一致）：
//   - status="sent" → 用 SentAt 判定（SentAt == nil 的行视为"刚 MarkSent 未刷时间"，保留）
//   - status="dead" → 用 CreatedAt 判定（LastError 是 msg string，不能当时间戳）
//
// 收集待删 id 后一次性删（不在锁内做 IO；InMemory 无 IO）。
func (r *InMemoryOutboxRepo) DeleteOlderThan(_ context.Context, status string, cutoff time.Time, limit int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var toDelete []int64
	for id, e := range r.events {
		if e.Status != status {
			continue
		}
		var age time.Time
		switch status {
		case OutboxStatusSent:
			if e.SentAt == nil {
				continue
			}
			age = *e.SentAt
		case OutboxStatusDead:
			age = e.CreatedAt
		default:
			return 0, fmt.Errorf("outbox: DeleteOlderThan unsupported status %q", status)
		}
		if age.Before(cutoff) {
			toDelete = append(toDelete, id)
		}
		if limit > 0 && len(toDelete) >= limit {
			break
		}
	}
	for _, id := range toDelete {
		delete(r.events, id)
	}
	return int64(len(toDelete)), nil
}

// Get 直接根据 ID 查（测试断言用）
func (r *InMemoryOutboxRepo) Get(id int64) (*OutboxEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.events[id]
	if !ok {
		return nil, nil
	}
	cp := *e
	return &cp, nil
}

// =====================================================
// PostgresOutboxRepo（生产实现）
// =====================================================

// PostgresOutboxRepo GORM 实现
type PostgresOutboxRepo struct{ db *gorm.DB }

// NewPostgresOutboxRepo 构造生产仓库
func NewPostgresOutboxRepo(db *gorm.DB) *PostgresOutboxRepo {
	return &PostgresOutboxRepo{db: db}
}

// CreateInTx 在调用方提供的事务中 insert 一条 outbox
func (r *PostgresOutboxRepo) CreateInTx(tx *gorm.DB, e *OutboxEvent) error {
	if e.Status == "" {
		e.Status = OutboxStatusPending
	}
	return tx.Create(e).Error
}

func (r *PostgresOutboxRepo) ListPending(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []OutboxEvent
	err := r.db.WithContext(ctx).
		Where("status = ?", OutboxStatusPending).
		Order("created_at ASC, id ASC").
		Limit(limit).
		Find(&out).Error
	return out, err
}

func (r *PostgresOutboxRepo) MarkSent(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":  OutboxStatusSent,
			"sent_at": time.Now(),
		}).Error
}

func (r *PostgresOutboxRepo) MarkFailed(ctx context.Context, id int64, errMsg string) error {
	return r.db.WithContext(ctx).
		Exec(`UPDATE emotion_echo_chat.outbox_events
		      SET attempts = attempts + 1, last_error = ?
		      WHERE id = ?`, errMsg, id).Error
}

func (r *PostgresOutboxRepo) MarkDead(ctx context.Context, id int64, errMsg string) error {
	return r.db.WithContext(ctx).
		Model(&OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":     OutboxStatusDead,
			"last_error": errMsg,
		}).Error
}

// DeleteOlderThan Round 2.1 §D2 Postgres 实现
//
// status="sent" → WHERE status='sent' AND sent_at < $1
// status="dead" → WHERE status='dead' AND created_at < $1
//
// LIMIT 在 GORM 用 .Limit()（无 limit 参数时不限）。
func (r *PostgresOutboxRepo) DeleteOlderThan(ctx context.Context, status string, cutoff time.Time, limit int) (int64, error) {
	tx := r.db.WithContext(ctx).
		Where("status = ?", status)

	switch status {
	case OutboxStatusSent:
		tx = tx.Where("sent_at IS NOT NULL AND sent_at < ?", cutoff)
	case OutboxStatusDead:
		tx = tx.Where("created_at < ?", cutoff)
	default:
		return 0, fmt.Errorf("outbox: DeleteOlderThan unsupported status %q", status)
	}

	if limit > 0 {
		tx = tx.Limit(limit)
	}

	res := tx.Delete(&OutboxEvent{})
	return res.RowsAffected, res.Error
}
