// Package repository — user_agent_profile_repository.go
//
// CL-06 Phase 2：角色档案仓储（接口 + InMemory 测试替身 + Postgres 生产实现）。
//
// **每条方法都带 userID**：角色是用户私有数据，按 id 直取/直改必须同时限定属主
// （防 IDOR —— 与仓库既有的 x-user-id 注入纪律一致）。
// 软删除语义：`deleted_at IS NULL` 才可见；删除即置时间戳（幂等）。
package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"emotion-echo-user-svc/internal/model"

	"gorm.io/gorm"
)

// UserAgentProfileRepo 角色档案仓储接口
type UserAgentProfileRepo interface {
	// ListByUserID 列出该用户**未删除**的角色（按 id 升序，稳定顺序）
	ListByUserID(ctx context.Context, userID int64) ([]*model.UserAgentProfile, error)
	// GetByID 按 (属主, id) 取单个；不存在 / 他人 / 已删除 ⇒ (nil, nil)
	GetByID(ctx context.Context, userID, id int64) (*model.UserAgentProfile, error)
	// Create 新建并回填自增 ID
	Create(ctx context.Context, p *model.UserAgentProfile) (*model.UserAgentProfile, error)
	// Update 按 (属主, id) 更新允许修改的字段；跨用户 / 已删除 ⇒ 不生效（不报错）
	Update(ctx context.Context, p *model.UserAgentProfile) error
	// SoftDelete 按 (属主, id) 软删除；幂等
	SoftDelete(ctx context.Context, userID, id int64) error
	// CountActive 统计未删除角色数（服务层据此强制「每户 ≤５」）
	CountActive(ctx context.Context, userID int64) (int, error)
	// CountAll 统计**含软删除**的角色数 —— 用于区分"从未建过角色"（首次读要物化默认「小E」）
	// 与"用户把角色删光了"（**不得**再物化，否则删除形同虚设）。
	CountAll(ctx context.Context, userID int64) (int, error)
}

// =====================================================
// InMemoryUserAgentProfileRepo（测试替身）
// =====================================================

type InMemoryUserAgentProfileRepo struct {
	mu     sync.RWMutex
	nextID int64
	items  map[int64]*model.UserAgentProfile
}

func NewInMemoryUserAgentProfileRepo() *InMemoryUserAgentProfileRepo {
	return &InMemoryUserAgentProfileRepo{items: make(map[int64]*model.UserAgentProfile)}
}

// clone 复制一份并深拷贝 Boundaries，避免调用方与仓储共享底层切片。
func clone(p *model.UserAgentProfile) *model.UserAgentProfile {
	if p == nil {
		return nil
	}
	cp := *p
	if len(p.Boundaries) > 0 {
		cp.Boundaries = make([]string, len(p.Boundaries))
		copy(cp.Boundaries, p.Boundaries)
	}
	return &cp
}

func (r *InMemoryUserAgentProfileRepo) ListByUserID(ctx context.Context, userID int64) ([]*model.UserAgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []*model.UserAgentProfile
	for _, it := range r.items {
		if it.UserID == userID && it.DeletedAt == nil {
			out = append(out, clone(it))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *InMemoryUserAgentProfileRepo) GetByID(ctx context.Context, userID, id int64) (*model.UserAgentProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	it, ok := r.items[id]
	if !ok || it.UserID != userID || it.DeletedAt != nil {
		return nil, nil
	}
	return clone(it), nil
}

func (r *InMemoryUserAgentProfileRepo) Create(ctx context.Context, p *model.UserAgentProfile) (*model.UserAgentProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.nextID++
	created := clone(p)
	created.ID = r.nextID
	now := time.Now()
	created.CreatedAt = now
	created.UpdatedAt = now
	created.DeletedAt = nil
	r.items[created.ID] = created
	return clone(created), nil
}

func (r *InMemoryUserAgentProfileRepo) Update(ctx context.Context, p *model.UserAgentProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	it, ok := r.items[p.ID]
	if !ok || it.UserID != p.UserID || it.DeletedAt != nil {
		return nil // 跨用户 / 不存在 / 已删除：静默不生效（与 Postgres 侧语义一致）
	}
	it.Name = p.Name
	it.Gender = p.Gender
	it.Persona = p.Persona
	it.Tone = p.Tone
	it.Boundaries = append([]string(nil), p.Boundaries...)
	it.Voice = p.Voice
	it.IsDefault = p.IsDefault
	it.TemplateID = p.TemplateID
	it.UpdatedAt = time.Now()
	return nil
}

func (r *InMemoryUserAgentProfileRepo) SoftDelete(ctx context.Context, userID, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	it, ok := r.items[id]
	if !ok || it.UserID != userID || it.DeletedAt != nil {
		return nil // 幂等 / 跨用户不生效
	}
	now := time.Now()
	it.DeletedAt = &now
	it.UpdatedAt = now
	return nil
}

func (r *InMemoryUserAgentProfileRepo) CountActive(ctx context.Context, userID int64) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := 0
	for _, it := range r.items {
		if it.UserID == userID && it.DeletedAt == nil {
			n++
		}
	}
	return n, nil
}

func (r *InMemoryUserAgentProfileRepo) CountAll(ctx context.Context, userID int64) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n := 0
	for _, it := range r.items {
		if it.UserID == userID {
			n++
		}
	}
	return n, nil
}

// =====================================================
// PostgresUserAgentProfileRepo（生产实现）
// =====================================================

type PostgresUserAgentProfileRepo struct {
	db *gorm.DB
}

func NewPostgresUserAgentProfileRepo(db *gorm.DB) *PostgresUserAgentProfileRepo {
	return &PostgresUserAgentProfileRepo{db: db}
}

func (r *PostgresUserAgentProfileRepo) ListByUserID(ctx context.Context, userID int64) ([]*model.UserAgentProfile, error) {
	var out []*model.UserAgentProfile
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("id ASC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresUserAgentProfileRepo) GetByID(ctx context.Context, userID, id int64) (*model.UserAgentProfile, error) {
	var p model.UserAgentProfile
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
		First(&p).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *PostgresUserAgentProfileRepo) Create(ctx context.Context, p *model.UserAgentProfile) (*model.UserAgentProfile, error) {
	if err := r.db.WithContext(ctx).Create(p).Error; err != nil {
		return nil, err
	}
	return p, nil
}

// Update 只更新白名单字段（不含 user_id/id，防越权改属主）。
func (r *PostgresUserAgentProfileRepo) Update(ctx context.Context, p *model.UserAgentProfile) error {
	return r.db.WithContext(ctx).
		Model(&model.UserAgentProfile{}).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", p.ID, p.UserID).
		Updates(map[string]any{
			"name":        p.Name,
			"gender":      p.Gender,
			"persona":     p.Persona,
			"tone":        p.Tone,
			"boundaries":  p.Boundaries,
			"voice":       p.Voice,
			"is_default":  p.IsDefault,
			"template_id": p.TemplateID,
			"updated_at":  time.Now(),
		}).Error
}

func (r *PostgresUserAgentProfileRepo) SoftDelete(ctx context.Context, userID, id int64) error {
	return r.db.WithContext(ctx).
		Model(&model.UserAgentProfile{}).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", id, userID).
		Updates(map[string]any{"deleted_at": time.Now(), "updated_at": time.Now()}).Error
}

func (r *PostgresUserAgentProfileRepo) CountActive(ctx context.Context, userID int64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.UserAgentProfile{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Count(&n).Error
	return int(n), err
}

// CountAll 含软删除（不加 deleted_at 过滤）。
func (r *PostgresUserAgentProfileRepo) CountAll(ctx context.Context, userID int64) (int, error) {
	var n int64
	err := r.db.WithContext(ctx).
		Model(&model.UserAgentProfile{}).
		Where("user_id = ?", userID).
		Count(&n).Error
	return int(n), err
}
