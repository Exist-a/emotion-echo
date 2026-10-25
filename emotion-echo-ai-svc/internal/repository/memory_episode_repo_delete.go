// Package repository — memory_episode_repo_delete.go
//
// CL-08d：删除闭环的仓储面（PG 软删 + graph_uuid 回填 + 列举）。
//
// 语义（ADR 决策 3 / 用户裁定 D3）：
//   - PG 永远软删（status='deleted' + deleted_at），审计可追溯；
//   - 删单条记忆 = 删该 fact + 其来源 episode 行（情节是"这件事"的原始记录，
//     删记忆即忘掉这件事 ⇒ 图侧 remove_episode / 重建都不会复活它）；
//     同源的其他 fact **保留**（用户没让删的不动；其图上内容由 remove_episode
//     一并摘除是已知边界 —— 图是可重建索引、注入路径不查图，见 CL-08d 报告）。
//   - 所有操作强制 user_id（记忆绝不跨用户）。
package repository

import (
	"context"
	"errors"
	"sort"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
)

// ErrMemoryNotFound 目标不存在或不属于该用户（幂等删除：不区分）。
var ErrMemoryNotFound = errors.New("memory item not found")

// SetEpisodeGraphUUID 回填 graphiti episode uuid（CL-08d 删除定位键）。
// 不存在或已软删的行静默跳过（best-effort，由 handler 记日志）。
func (r *PostgresMemoryFactRepo) SetEpisodeGraphUUID(ctx context.Context, sourceKind string, sourceID int64, uuid string) error {
	return r.db.WithContext(ctx).
		Model(&model.MemoryEpisode{}).
		Where("source_kind = ? AND source_id = ? AND deleted_at IS NULL", sourceKind, sourceID).
		Update("graph_uuid", uuid).Error
}

// GetEpisodeGraphUUID 取情节的 graph_uuid（**含已软删行**——删除流程在软删前取）。
func (r *PostgresMemoryFactRepo) GetEpisodeGraphUUID(ctx context.Context, sourceKind string, sourceID int64) (string, error) {
	var ep model.MemoryEpisode
	err := r.db.WithContext(ctx).
		Select("graph_uuid").
		Where("source_kind = ? AND source_id = ?", sourceKind, sourceID).
		First(&ep).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil
		}
		return "", err
	}
	return ep.GraphUUID, nil
}

// SoftDeleteFact 软删一条事实（user 隔离）。返回是否真的删了（幂等：重复删=false）。
func (r *PostgresMemoryFactRepo) SoftDeleteFact(ctx context.Context, userID, factID int64) (bool, error) {
	res := r.db.WithContext(ctx).
		Model(&model.MemoryFact{}).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", factID, userID).
		Updates(map[string]any{
			"status":     model.MemoryFactStatusDeleted,
			"deleted_at": time.Now().UTC(),
		})
	return res.RowsAffected > 0, res.Error
}

// SoftDeleteFactsBySource 软删某来源的全部 facts（note 删除时用）。
func (r *PostgresMemoryFactRepo) SoftDeleteFactsBySource(ctx context.Context, userID int64, sourceKind string, sourceID int64) (int64, error) {
	res := r.db.WithContext(ctx).
		Model(&model.MemoryFact{}).
		Where("user_id = ? AND source_kind = ? AND source_id = ? AND deleted_at IS NULL", userID, sourceKind, sourceID).
		Updates(map[string]any{
			"status":     model.MemoryFactStatusDeleted,
			"deleted_at": time.Now().UTC(),
		})
	return res.RowsAffected, res.Error
}

// SoftDeleteEpisode 软删一条来源情节（任何用户——调用方已通过 fact/note 校验归属）。
func (r *PostgresMemoryFactRepo) SoftDeleteEpisode(ctx context.Context, sourceKind string, sourceID int64) error {
	return r.db.WithContext(ctx).
		Model(&model.MemoryEpisode{}).
		Where("source_kind = ? AND source_id = ? AND deleted_at IS NULL", sourceKind, sourceID).
		Updates(map[string]any{
			"status":     model.MemoryNodeStatusDeleted,
			"deleted_at": time.Now().UTC(),
		}).Error
}

// SoftDeleteNote 软删一条自述（user 隔离）。
func (r *PostgresMemoryFactRepo) SoftDeleteNote(ctx context.Context, userID, noteID int64) (bool, error) {
	res := r.db.WithContext(ctx).
		Model(&model.MemoryNote{}).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", noteID, userID).
		Update("deleted_at", time.Now().UTC())
	return res.RowsAffected > 0, res.Error
}

// SoftDeleteAllForUser 软删用户全部记忆（facts + notes + episodes + entities）。
// 返回各类删除行数（审计用）。
func (r *PostgresMemoryFactRepo) SoftDeleteAllForUser(ctx context.Context, userID int64) (facts, notes, episodes, entities int64, err error) {
	now := time.Now().UTC()
	// facts（status 值域不同，单独删）
	res := r.db.WithContext(ctx).Model(&model.MemoryFact{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Updates(map[string]any{"status": model.MemoryFactStatusDeleted, "deleted_at": now})
	facts, err = res.RowsAffected, res.Error
	if err != nil {
		return
	}
	// notes
	res = r.db.WithContext(ctx).Model(&model.MemoryNote{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Update("deleted_at", now)
	notes, err = res.RowsAffected, res.Error
	if err != nil {
		return
	}
	// episodes
	res = r.db.WithContext(ctx).Model(&model.MemoryEpisode{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Updates(map[string]any{"status": model.MemoryNodeStatusDeleted, "deleted_at": now})
	episodes, err = res.RowsAffected, res.Error
	if err != nil {
		return
	}
	// entities
	res = r.db.WithContext(ctx).Model(&model.MemoryEntity{}).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Updates(map[string]any{"status": model.MemoryNodeStatusDeleted, "deleted_at": now})
	entities, err = res.RowsAffected, res.Error
	return
}

// GetFactByID 取一条事实（user 隔离；含 superseded——删除入口由页面给 active 项，但以服务端校验为准）。
func (r *PostgresMemoryFactRepo) GetFactByID(ctx context.Context, userID, factID int64) (*model.MemoryFact, error) {
	var f model.MemoryFact
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", factID, userID).
		First(&f).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

// GetNoteByID 取一条自述（user 隔离；含已软删——删除流程要幂等读）。
func (r *PostgresMemoryFactRepo) GetNoteByID(ctx context.Context, userID, noteID int64) (*model.MemoryNote, error) {
	var n model.MemoryNote
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", noteID, userID).
		First(&n).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &n, nil
}

// ListEpisodesByUserAll 列出用户全部未软删情节（重建工具用；含无 uuid 行）。
func (r *PostgresMemoryFactRepo) ListEpisodesByUserAll(ctx context.Context, userID int64) ([]model.MemoryEpisode, error) {
	var out []model.MemoryEpisode
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("valid_at ASC, id ASC").
		Find(&out).Error
	return out, err
}

// ListDistinctEpisodeUsers 有未软删情节的用户（重建工具 -all 用）。
func (r *PostgresMemoryFactRepo) ListDistinctEpisodeUsers(ctx context.Context) ([]int64, error) {
	var ids []int64
	err := r.db.WithContext(ctx).Model(&model.MemoryEpisode{}).
		Distinct("user_id").
		Where("deleted_at IS NULL").
		Order("user_id ASC").
		Pluck("user_id", &ids).Error
	return ids, err
}

// ---- InMemoryMemoryFactRepo 的 CL-08d 实现 ----
//
// InMemory 实现：事实/自述/情节全部**真实实现**（与 Postgres 共享语义）；
// episode 存储在 InMemoryMemoryFactRepo.episodes（UpsertEpisodeForTest 注入）。

func (r *InMemoryMemoryFactRepo) SetEpisodeGraphUUID(ctx context.Context, sourceKind string, sourceID int64, uuid string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.episodes {
		if e.SourceKind == sourceKind && e.SourceID == sourceID && e.DeletedAt == nil {
			e.GraphUUID = uuid
		}
	}
	return nil
}

func (r *InMemoryMemoryFactRepo) GetEpisodeGraphUUID(ctx context.Context, sourceKind string, sourceID int64) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.episodes {
		if e.SourceKind == sourceKind && e.SourceID == sourceID {
			return e.GraphUUID, nil
		}
	}
	return "", nil
}

func (r *InMemoryMemoryFactRepo) SoftDeleteEpisode(ctx context.Context, sourceKind string, sourceID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	for _, e := range r.episodes {
		if e.SourceKind == sourceKind && e.SourceID == sourceID && e.DeletedAt == nil {
			e.Status = model.MemoryNodeStatusDeleted
			e.DeletedAt = &now
		}
	}
	return nil
}

func (r *InMemoryMemoryFactRepo) ListEpisodesByUserAll(ctx context.Context, userID int64) ([]model.MemoryEpisode, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.MemoryEpisode, 0)
	for _, e := range r.episodes {
		if e.UserID == userID && e.DeletedAt == nil {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (r *InMemoryMemoryFactRepo) ListDistinctEpisodeUsers(ctx context.Context) ([]int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[int64]bool{}
	for _, e := range r.episodes {
		if e.DeletedAt == nil {
			seen[e.UserID] = true
		}
	}
	out := make([]int64, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (r *InMemoryMemoryFactRepo) SoftDeleteFact(ctx context.Context, userID, factID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.facts[factID]
	if !ok || f.UserID != userID || f.DeletedAt != nil {
		return false, nil
	}
	now := time.Now().UTC()
	f.Status = model.MemoryFactStatusDeleted
	f.DeletedAt = &now
	return true, nil
}

func (r *InMemoryMemoryFactRepo) SoftDeleteFactsBySource(ctx context.Context, userID int64, sourceKind string, sourceID int64) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var n int64
	now := time.Now().UTC()
	for _, f := range r.facts {
		if f.UserID == userID && f.SourceKind == sourceKind && f.SourceID == sourceID && f.DeletedAt == nil {
			f.Status = model.MemoryFactStatusDeleted
			f.DeletedAt = &now
			n++
		}
	}
	return n, nil
}

func (r *InMemoryMemoryFactRepo) SoftDeleteNote(ctx context.Context, userID, noteID int64) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, ok := r.notes[noteID]
	if !ok || n.UserID != userID || n.DeletedAt != nil {
		return false, nil
	}
	now := time.Now().UTC()
	n.DeletedAt = &now
	return true, nil
}

func (r *InMemoryMemoryFactRepo) SoftDeleteAllForUser(ctx context.Context, userID int64) (int64, int64, int64, int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	var facts, notes int64
	for _, f := range r.facts {
		if f.UserID == userID && f.DeletedAt == nil {
			f.Status = model.MemoryFactStatusDeleted
			f.DeletedAt = &now
			facts++
		}
	}
	for _, n := range r.notes {
		if n.UserID == userID && n.DeletedAt == nil {
			n.DeletedAt = &now
			notes++
		}
	}
	// episodes 此处真实计数；entities 存于 InMemoryMemoryGraphRepo ⇒ 记 0
	var episodes int64
	for _, e := range r.episodes {
		if e.UserID == userID && e.DeletedAt == nil {
			e.Status = model.MemoryNodeStatusDeleted
			e.DeletedAt = &now
			episodes++
		}
	}
	return facts, notes, episodes, 0, nil
}

func (r *InMemoryMemoryFactRepo) GetFactByID(ctx context.Context, userID, factID int64) (*model.MemoryFact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	f, ok := r.facts[factID]
	if !ok || f.UserID != userID || f.DeletedAt != nil {
		return nil, nil
	}
	c := *f
	return &c, nil
}

func (r *InMemoryMemoryFactRepo) GetNoteByID(ctx context.Context, userID, noteID int64) (*model.MemoryNote, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n, ok := r.notes[noteID]
	if !ok || n.UserID != userID {
		return nil, nil
	}
	c := *n
	return &c, nil
}
