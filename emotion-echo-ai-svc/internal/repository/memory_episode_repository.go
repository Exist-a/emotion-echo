// Package repository — memory_episode_repository.go
//
// CL-08c · Phase 1：情节与实体仓储（接口 + 内存实现 + Postgres 实现）。
//
// 两条硬语义（与 i012 迁移一起构成规格）：
//   - 幂等：同一 (source_kind, source_id) 至多一个情节，重复 upsert 不覆盖；
//   - 用户隔离：**所有**查询强制带 user_id（记忆绝不能跨用户）。
//
// 删除入口不在本仓储：删除闭环（PG 为准 + 图随删 + Redis 摘要失效）归
// Phase 5 / CL-08d，届时与图侧联动一起加，避免半套删除先落地。
package repository

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrInvalidMemoryGraph 非法的情节 / 实体输入（空内容、空来源、空实体名等）。
var ErrInvalidMemoryGraph = errors.New("invalid memory episode/entity input")

// MemoryGraphRepo 情节与实体的仓储。
type MemoryGraphRepo interface {
	// UpsertEpisode 幂等写一条情节。
	// created=true 表示首次落行；false 表示该来源已有情节（**不覆盖**既有内容）。
	UpsertEpisode(ctx context.Context, ep *model.MemoryEpisode) (created bool, err error)
	// GetEpisode 取单条情节（不存在返回 nil, nil）。
	GetEpisode(ctx context.Context, sourceKind string, sourceID int64) (*model.MemoryEpisode, error)
	// ListEpisodesByUser 按用户取未删除情节（valid_at DESC；limit<=0 = 不限）。
	ListEpisodesByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryEpisode, error)

	// UpsertEntity 幂等写一个实体。
	// created=true 表示新落行（含软删行被复活）；false 表示已有行 ——
	// 此时刷新 summary 为最新值，不新增行。
	UpsertEntity(ctx context.Context, ent *model.MemoryEntity) (created bool, err error)
	// GetEntityByName 取用户的一个实体（不存在或已软删返回 nil, nil）。
	GetEntityByName(ctx context.Context, userID int64, name string) (*model.MemoryEntity, error)
	// ListEntitiesByUser 按用户取未删除实体（name ASC；limit<=0 = 不限）。
	ListEntitiesByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryEntity, error)

	// Ping 健康检查。
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryMemoryGraphRepo（测试替身）
// =====================================================

// InMemoryMemoryGraphRepo 内存实现：与 Postgres 实现共享同一组语义，
// 供单测使用（不连 DB）。
type InMemoryMemoryGraphRepo struct {
	mu        sync.RWMutex
	episodes  map[int64]*model.MemoryEpisode
	entities  map[int64]*model.MemoryEntity
	nextEpID  int64
	nextEntID int64
}

func NewInMemoryMemoryGraphRepo() *InMemoryMemoryGraphRepo {
	return &InMemoryMemoryGraphRepo{
		episodes:  make(map[int64]*model.MemoryEpisode),
		entities:  make(map[int64]*model.MemoryEntity),
		nextEpID:  1,
		nextEntID: 1,
	}
}

func (r *InMemoryMemoryGraphRepo) UpsertEpisode(ctx context.Context, ep *model.MemoryEpisode) (bool, error) {
	if err := validateEpisode(ep); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.episodes {
		if e.SourceKind == ep.SourceKind && e.SourceID == ep.SourceID {
			ep.ID = e.ID
			return false, nil
		}
	}
	c := *ep
	c.ID = r.nextEpID
	r.nextEpID++
	if c.Status == "" {
		c.Status = model.MemoryNodeStatusActive
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	r.episodes[c.ID] = &c
	ep.ID = c.ID
	return true, nil
}

func (r *InMemoryMemoryGraphRepo) GetEpisode(ctx context.Context, sourceKind string, sourceID int64) (*model.MemoryEpisode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.episodes {
		if e.SourceKind == sourceKind && e.SourceID == sourceID && e.DeletedAt == nil {
			c := *e
			return &c, nil
		}
	}
	return nil, nil
}

func (r *InMemoryMemoryGraphRepo) ListEpisodesByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryEpisode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.MemoryEpisode, 0, len(r.episodes))
	for _, e := range r.episodes {
		if e.UserID != userID || e.DeletedAt != nil {
			continue
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].ValidAt.Equal(out[j].ValidAt) {
			return out[i].ValidAt.After(out[j].ValidAt)
		}
		return out[i].ID > out[j].ID
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryMemoryGraphRepo) UpsertEntity(ctx context.Context, ent *model.MemoryEntity) (bool, error) {
	if err := validateEntity(ent); err != nil {
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for _, e := range r.entities {
		if e.UserID == ent.UserID && e.Name == ent.Name {
			wasDeleted := e.DeletedAt != nil
			e.Summary = ent.Summary
			e.Status = model.MemoryNodeStatusActive
			e.DeletedAt = nil
			e.UpdatedAt = now
			ent.ID = e.ID
			// 复活视为一次新落行（active 集合里重新出现）
			return wasDeleted, nil
		}
	}
	c := *ent
	c.ID = r.nextEntID
	r.nextEntID++
	if c.Status == "" {
		c.Status = model.MemoryNodeStatusActive
	}
	c.CreatedAt, c.UpdatedAt = now, now
	r.entities[c.ID] = &c
	ent.ID = c.ID
	return true, nil
}

func (r *InMemoryMemoryGraphRepo) GetEntityByName(ctx context.Context, userID int64, name string) (*model.MemoryEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entities {
		if e.UserID == userID && e.Name == name && e.DeletedAt == nil {
			c := *e
			return &c, nil
		}
	}
	return nil, nil
}

func (r *InMemoryMemoryGraphRepo) ListEntitiesByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryEntity, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.MemoryEntity, 0, len(r.entities))
	for _, e := range r.entities {
		if e.UserID != userID || e.DeletedAt != nil {
			continue
		}
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *InMemoryMemoryGraphRepo) Ping(ctx context.Context) error { return nil }

func validateEpisode(ep *model.MemoryEpisode) error {
	switch {
	case ep == nil:
		return ErrInvalidMemoryGraph
	case ep.SourceKind == "":
		return fmt.Errorf("%w: empty source kind", ErrInvalidMemoryGraph)
	case ep.SourceID == 0:
		return fmt.Errorf("%w: zero source id", ErrInvalidMemoryGraph)
	case ep.Name == "":
		return fmt.Errorf("%w: empty episode name", ErrInvalidMemoryGraph)
	case ep.Content == "":
		return fmt.Errorf("%w: empty episode content", ErrInvalidMemoryGraph)
	}
	return nil
}

func validateEntity(ent *model.MemoryEntity) error {
	switch {
	case ent == nil:
		return ErrInvalidMemoryGraph
	case ent.UserID == 0:
		return fmt.Errorf("%w: zero user id", ErrInvalidMemoryGraph)
	case ent.Name == "":
		return fmt.Errorf("%w: empty entity name", ErrInvalidMemoryGraph)
	}
	return nil
}

// =====================================================
// PostgresMemoryGraphRepo（生产实现）
// =====================================================

type PostgresMemoryGraphRepo struct{ db *gorm.DB }

func NewPostgresMemoryGraphRepo(db *gorm.DB) *PostgresMemoryGraphRepo {
	return &PostgresMemoryGraphRepo{db: db}
}

// UpsertEpisode 依赖 uq_memory_episode_source (source_kind, source_id) 做
// ON CONFLICT DO NOTHING：RowsAffected==1 ⇒ 首次落行；0 ⇒ 已有情节，幂等跳过
// （不覆盖 —— 重复投递不得改写首次存档）。
func (r *PostgresMemoryGraphRepo) UpsertEpisode(ctx context.Context, ep *model.MemoryEpisode) (bool, error) {
	if err := validateEpisode(ep); err != nil {
		return false, err
	}
	if ep.Status == "" {
		ep.Status = model.MemoryNodeStatusActive
	}
	ep.ID = 0
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "source_kind"}, {Name: "source_id"}},
			DoNothing: true,
		}).
		Create(ep)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 1 {
		return true, nil
	}
	// 已有情节：把既有行取回填 id（调用方可能依赖 ep.ID）
	existing, err := r.GetEpisode(ctx, ep.SourceKind, ep.SourceID)
	if err != nil {
		return false, err
	}
	if existing != nil {
		ep.ID = existing.ID
	}
	return false, nil
}

func (r *PostgresMemoryGraphRepo) GetEpisode(ctx context.Context, sourceKind string, sourceID int64) (*model.MemoryEpisode, error) {
	var ep model.MemoryEpisode
	err := r.db.WithContext(ctx).
		Where("source_kind = ? AND source_id = ? AND deleted_at IS NULL", sourceKind, sourceID).
		First(&ep).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ep, nil
}

func (r *PostgresMemoryGraphRepo) ListEpisodesByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryEpisode, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("valid_at DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []model.MemoryEpisode
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// UpsertEntity 依赖 uq_memory_entity_user_name (user_id, name)：
// RowsAffected==1 ⇒ 新落行；0 ⇒ 已有行 —— 刷新 summary 并复活（若曾软删）。
// 复活语义：用户删掉实体后又重新说起新证据 ⇒ 记忆重新生效（不新建行，
// 保持 (user_id, name) 唯一锚点）。
func (r *PostgresMemoryGraphRepo) UpsertEntity(ctx context.Context, ent *model.MemoryEntity) (bool, error) {
	if err := validateEntity(ent); err != nil {
		return false, err
	}
	if ent.Status == "" {
		ent.Status = model.MemoryNodeStatusActive
	}
	ent.ID = 0
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "name"}},
			DoNothing: true,
		}).
		Create(ent)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected == 1 {
		return true, nil
	}
	// 已有行：先查原状态（含软删行），再刷新 summary 并复活（若曾软删）。
	var prior model.MemoryEntity
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND name = ?", ent.UserID, ent.Name).
		First(&prior).Error
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	upd := r.db.WithContext(ctx).
		Model(&model.MemoryEntity{}).
		Where("user_id = ? AND name = ?", ent.UserID, ent.Name).
		Updates(map[string]any{
			"summary":    ent.Summary,
			"status":     model.MemoryNodeStatusActive,
			"deleted_at": nil,
			"updated_at": now,
		})
	if upd.Error != nil {
		return false, upd.Error
	}
	existing, err := r.GetEntityByName(ctx, ent.UserID, ent.Name)
	if err != nil {
		return false, err
	}
	if existing != nil {
		ent.ID = existing.ID
	}
	// 复活视为一次新落行（active 集合里重新出现）
	return prior.DeletedAt != nil, nil
}

func (r *PostgresMemoryGraphRepo) GetEntityByName(ctx context.Context, userID int64, name string) (*model.MemoryEntity, error) {
	var ent model.MemoryEntity
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND name = ? AND deleted_at IS NULL", userID, name).
		First(&ent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ent, nil
}

func (r *PostgresMemoryGraphRepo) ListEntitiesByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryEntity, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("name ASC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []model.MemoryEntity
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresMemoryGraphRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
