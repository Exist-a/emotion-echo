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
	"errors"
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

	// ── 下面两个方法是**原子**版本，服务层应当用它们而不是 "Count 之后 Create" ──
	//
	// 为什么：`CountActive()` + `Create()` 是**两次独立往返**，两个并发请求可以都读到
	// `active=4` 然后双双插入 ⇒ 第 6 个角色被写进库（TOCTOU）。本阶段第二方独立核对
	// 读码确认了该竞态（无事务、无行锁、DDL 无 CHECK/UNIQUE）。
	// 下面的实现把"复核 + 插入"放进**同一事务**，并先取**每用户咨询锁**串行化。

	// CreateWithActiveLimit 在活跃角色数 < maxActive 时插入；否则返回 ErrActiveLimitReached。
	CreateWithActiveLimit(ctx context.Context, p *model.UserAgentProfile, maxActive int) error
	// CreateIfNoHistory 仅当该用户**从未有过任何角色**（含已软删除）时插入默认角色；
	// 返回是否真的创建（false = 已有历史，无需物化）。
	CreateIfNoHistory(ctx context.Context, p *model.UserAgentProfile) (bool, error)
}

// ErrActiveLimitReached 活跃角色数已达上限（仓储层原子复核的结果，由服务层翻译为业务错误）。
var ErrActiveLimitReached = errors.New("agent profile active limit reached")

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

// CreateWithActiveLimit InMemory：单锁内"复核 + 插入"（与 Postgres 的咨询锁语义等价）。
func (r *InMemoryUserAgentProfileRepo) CreateWithActiveLimit(ctx context.Context, p *model.UserAgentProfile, maxActive int) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	active := 0
	for _, it := range r.items {
		if it.UserID == p.UserID && it.DeletedAt == nil {
			active++
		}
	}
	if active >= maxActive {
		return ErrActiveLimitReached
	}
	r.nextID++
	created := clone(p)
	created.ID = r.nextID
	now := time.Now()
	created.CreatedAt, created.UpdatedAt, created.DeletedAt = now, now, nil
	r.items[created.ID] = created
	*p = *clone(created) // 回填（含自增 ID），与 Create 的契约一致
	return nil
}

// CreateIfNoHistory InMemory：单锁内"复核（含软删除）+ 插入"。
func (r *InMemoryUserAgentProfileRepo) CreateIfNoHistory(ctx context.Context, p *model.UserAgentProfile) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, it := range r.items {
		if it.UserID == p.UserID {
			return false, nil // 有过任何角色（哪怕已删）⇒ 不再物化
		}
	}
	r.nextID++
	created := clone(p)
	created.ID = r.nextID
	now := time.Now()
	created.CreatedAt, created.UpdatedAt, created.DeletedAt = now, now, nil
	r.items[created.ID] = created
	*p = *clone(created)
	return true, nil
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

// =====================================================
// Postgres：原子路径（事务 + 每用户咨询锁）
// =====================================================

// agentProfileLockClass 咨询锁的"类"标识（与 userID 组成 64 位锁键）。
//
// 为什么用咨询锁而不是 `SELECT ... FOR UPDATE`：要防的是**插入**竞态，
// 而"还没有行"时行锁无从谈起（首次并发会双双读到 0 行）。咨询锁与行是否存在无关，
// 且 `pg_advisory_xact_lock` 随事务结束自动释放（不会因 panic 泄漏）。
//
// ⚠️ 用**单参 bigint** 形式而不是双参 `(int4, int4)`：双参版本的两个参数都必须落在
// int4 范围内，而"类 + 用户 id"拼进一个参数更简单且不受此限。
// （实测教训：第一版用 `pg_advisory_xact_lock(7751467009, 1)` 直接报
//
//	`unable to encode ... into binary format for int4` —— 该错误**只在真 Postgres 上出现**，
//	单测用的 InMemory 替身根本没有锁，永远测不到 ⇒ 这类"替身与实现的语义差"必须靠运行态验证。）
const agentProfileLockClass int64 = 0x1CE0_6001

func lockUserAgentProfiles(tx *gorm.DB, userID int64) error {
	key := agentProfileLockClass<<32 | (userID & 0xFFFF_FFFF)
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", key).Error
}

// CreateWithActiveLimit Postgres：事务 + 咨询锁 ⇒ 复核与插入之间不会被并发插入插队。
func (r *PostgresUserAgentProfileRepo) CreateWithActiveLimit(ctx context.Context, p *model.UserAgentProfile, maxActive int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUserAgentProfiles(tx, p.UserID); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.UserAgentProfile{}).
			Where("user_id = ? AND deleted_at IS NULL", p.UserID).Count(&n).Error; err != nil {
			return err
		}
		if int(n) >= maxActive {
			return ErrActiveLimitReached
		}
		return tx.Create(p).Error
	})
}

// CreateIfNoHistory Postgres：同一把锁下复核"含软删除的历史数"，为 0 才物化。
func (r *PostgresUserAgentProfileRepo) CreateIfNoHistory(ctx context.Context, p *model.UserAgentProfile) (bool, error) {
	created := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := lockUserAgentProfiles(tx, p.UserID); err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&model.UserAgentProfile{}).
			Where("user_id = ?", p.UserID).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	return created, err
}
