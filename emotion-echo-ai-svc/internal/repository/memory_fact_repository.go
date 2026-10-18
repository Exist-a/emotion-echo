// Package repository — memory_fact_repository.go
//
// CL-08a · A1：记忆事实仓储（接口 + 内存实现 + Postgres 实现）。
//
// 两条硬语义（与 planner 一起构成规格）：
//   - 幂等：同一 message_id 只登记一次抽取；同一消息重复 ApplyFacts 不重复入库。
//   - 用户隔离：**所有**查询强制带 user_id（记忆绝不能跨用户）。
package repository

import (
	"context"
	"sort"
	"sync"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MemoryFactRepo 记忆事实仓储。
type MemoryFactRepo interface {
	// TryMarkExtractRun 幂等登记一条消息的抽取。
	// 返回 first=true 表示首次（调用方可以抽取）；false 表示已处理过（跳过，避免重复调 LLM）。
	TryMarkExtractRun(ctx context.Context, run *model.MemoryExtractRun) (first bool, err error)
	// FinishExtractRun 回填抽取结果（done/failed + 事实数 + 错误）。
	FinishExtractRun(ctx context.Context, messageID int64, status string, factsCount int, errMsg string) error
	// GetExtractRun 查询抽取运行记录（不存在返回 nil, nil）。
	GetExtractRun(ctx context.Context, messageID int64) (*model.MemoryExtractRun, error)
	// ApplyFacts 在一个事务里执行落库计划（先失效旧行、再插新行）。返回新插入条数。
	ApplyFacts(ctx context.Context, plan PlanResult) (written int, err error)
	// ListActiveByUser 当前有效事实（valid_from DESC；limit<=0 = 不限）。
	ListActiveByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryFact, error)
	// ListAllByUser 该用户全部未软删事实（含 superseded，供追溯/测试；limit<=0 = 不限）。
	ListAllByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryFact, error)
	// ListActiveByConversation 某会话当前有效事实（会话收尾合并用）。
	ListActiveByConversation(ctx context.Context, conversationID int64) ([]model.MemoryFact, error)
	// Ping 健康检查。
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryMemoryFactRepo（测试替身）
// =====================================================

// InMemoryMemoryFactRepo 内存实现：与 Postgres 实现共享同一组语义，
// 供抽取 worker 与摘要构建的单测使用（不连 DB）。
type InMemoryMemoryFactRepo struct {
	mu     sync.RWMutex
	facts  map[int64]*model.MemoryFact
	runs   map[int64]*model.MemoryExtractRun
	nextID int64
}

func NewInMemoryMemoryFactRepo() *InMemoryMemoryFactRepo {
	return &InMemoryMemoryFactRepo{
		facts:  make(map[int64]*model.MemoryFact),
		runs:   make(map[int64]*model.MemoryExtractRun),
		nextID: 1,
	}
}

func (r *InMemoryMemoryFactRepo) TryMarkExtractRun(ctx context.Context, run *model.MemoryExtractRun) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.runs[run.MessageID]; ok {
		return false, nil
	}
	c := *run
	if c.Status == "" {
		c.Status = model.MemoryExtractStatusPending
	}
	now := time.Now().UTC()
	c.CreatedAt, c.UpdatedAt = now, now
	r.runs[c.MessageID] = &c
	return true, nil
}

func (r *InMemoryMemoryFactRepo) FinishExtractRun(ctx context.Context, messageID int64, status string, factsCount int, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, ok := r.runs[messageID]
	if !ok {
		return nil
	}
	run.Status = status
	run.FactsCount = factsCount
	run.Error = errMsg
	run.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *InMemoryMemoryFactRepo) GetExtractRun(ctx context.Context, messageID int64) (*model.MemoryExtractRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	run, ok := r.runs[messageID]
	if !ok {
		return nil, nil
	}
	c := *run
	return &c, nil
}

func (r *InMemoryMemoryFactRepo) ApplyFacts(ctx context.Context, plan PlanResult) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()

	for _, s := range plan.Supersede {
		f, ok := r.facts[s.ID]
		if !ok || !isActive(*f) {
			continue
		}
		validTo := s.ValidTo
		f.ValidTo = &validTo
		f.Status = model.MemoryFactStatusSuperseded
		f.UpdatedAt = now
	}

	written := 0
	for _, in := range plan.Insert {
		if in.Subject == "" || in.Predicate == "" || in.Object == "" {
			continue
		}
		if r.existsMessageTripleLocked(in) {
			continue
		}
		if r.hasActiveObjectLocked(in.UserID, in.Subject, in.Predicate, in.Object) {
			continue
		}
		c := in
		if c.Status == "" {
			c.Status = model.MemoryFactStatusActive
		}
		if c.ID == 0 {
			c.ID = r.nextID
			r.nextID++
		}
		c.CreatedAt, c.UpdatedAt = now, now
		r.facts[c.ID] = &c
		written++
	}
	return written, nil
}

func (r *InMemoryMemoryFactRepo) existsMessageTripleLocked(in model.MemoryFact) bool {
	for _, f := range r.facts {
		if f.SourceMessageID == in.SourceMessageID &&
			f.Subject == in.Subject && f.Predicate == in.Predicate && f.Object == in.Object {
			return true
		}
	}
	return false
}

func (r *InMemoryMemoryFactRepo) hasActiveObjectLocked(userID int64, subject, predicate, object string) bool {
	for _, f := range r.facts {
		if f.UserID == userID && f.Subject == subject && f.Predicate == predicate &&
			f.Object == object && isActive(*f) {
			return true
		}
	}
	return false
}

func (r *InMemoryMemoryFactRepo) ListActiveByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryFact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.MemoryFact, 0, len(r.facts))
	for _, f := range r.facts {
		if f.UserID != userID || !isActive(*f) {
			continue
		}
		out = append(out, *f)
	}
	return sortFactsDescLimit(out, limit), nil
}

func (r *InMemoryMemoryFactRepo) ListAllByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryFact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.MemoryFact, 0, len(r.facts))
	for _, f := range r.facts {
		if f.UserID != userID || f.DeletedAt != nil {
			continue
		}
		out = append(out, *f)
	}
	return sortFactsDescLimit(out, limit), nil
}

func (r *InMemoryMemoryFactRepo) ListActiveByConversation(ctx context.Context, conversationID int64) ([]model.MemoryFact, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.MemoryFact, 0, len(r.facts))
	for _, f := range r.facts {
		if f.ConversationID != conversationID || !isActive(*f) {
			continue
		}
		out = append(out, *f)
	}
	return sortFactsDescLimit(out, 0), nil
}

func (r *InMemoryMemoryFactRepo) Ping(ctx context.Context) error { return nil }

// sortFactsDescLimit 按 valid_from DESC、id DESC 排序并裁剪（与 Postgres 的 ORDER BY 对齐）。
func sortFactsDescLimit(in []model.MemoryFact, limit int) []model.MemoryFact {
	sort.Slice(in, func(i, j int) bool {
		if !in[i].ValidFrom.Equal(in[j].ValidFrom) {
			return in[i].ValidFrom.After(in[j].ValidFrom)
		}
		return in[i].ID > in[j].ID
	})
	if limit > 0 && len(in) > limit {
		return in[:limit]
	}
	return in
}

// =====================================================
// PostgresMemoryFactRepo（生产实现）
// =====================================================

type PostgresMemoryFactRepo struct{ db *gorm.DB }

func NewPostgresMemoryFactRepo(db *gorm.DB) *PostgresMemoryFactRepo {
	return &PostgresMemoryFactRepo{db: db}
}

// TryMarkExtractRun 依赖 message_id 主键做 ON CONFLICT DO NOTHING：
// RowsAffected==1 ⇒ 首次登记；0 ⇒ 已存在（幂等跳过）。
func (r *PostgresMemoryFactRepo) TryMarkExtractRun(ctx context.Context, run *model.MemoryExtractRun) (bool, error) {
	if run.Status == "" {
		run.Status = model.MemoryExtractStatusPending
	}
	res := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(run)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

func (r *PostgresMemoryFactRepo) FinishExtractRun(ctx context.Context, messageID int64, status string, factsCount int, errMsg string) error {
	return r.db.WithContext(ctx).
		Model(&model.MemoryExtractRun{}).
		Where("message_id = ?", messageID).
		Updates(map[string]any{
			"status":      status,
			"facts_count": factsCount,
			"error":       errMsg,
			"updated_at":  time.Now().UTC(),
		}).Error
}

func (r *PostgresMemoryFactRepo) GetExtractRun(ctx context.Context, messageID int64) (*model.MemoryExtractRun, error) {
	var run model.MemoryExtractRun
	err := r.db.WithContext(ctx).Where("message_id = ?", messageID).First(&run).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &run, nil
}

// ApplyFacts 在一个事务里先关旧区间、再插新行。
//
// 先失效后插入是硬顺序：i011 的 uq_memory_fact_active_key（partial unique on
// user_id+subject+predicate WHERE status='active'）会拦"同一键两条 active"。
func (r *PostgresMemoryFactRepo) ApplyFacts(ctx context.Context, plan PlanResult) (int, error) {
	written := 0
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		for _, s := range plan.Supersede {
			if err := tx.Model(&model.MemoryFact{}).
				Where("id = ? AND status = ?", s.ID, model.MemoryFactStatusActive).
				Updates(map[string]any{
					"valid_to":   s.ValidTo,
					"status":     model.MemoryFactStatusSuperseded,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
		for i := range plan.Insert {
			f := plan.Insert[i]
			if f.Subject == "" || f.Predicate == "" || f.Object == "" {
				continue
			}
			if f.Status == "" {
				f.Status = model.MemoryFactStatusActive
			}
			f.ID = 0
			res := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "source_message_id"}, {Name: "subject"},
					{Name: "predicate"}, {Name: "object"},
				},
				DoNothing: true,
			}).Create(&f)
			if res.Error != nil {
				return res.Error
			}
			written += int(res.RowsAffected)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return written, nil
}

func (r *PostgresMemoryFactRepo) ListActiveByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryFact, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, model.MemoryFactStatusActive).
		Order("valid_from DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []model.MemoryFact
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresMemoryFactRepo) ListAllByUser(ctx context.Context, userID int64, limit int) ([]model.MemoryFact, error) {
	q := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("valid_from DESC, id DESC")
	if limit > 0 {
		q = q.Limit(limit)
	}
	var out []model.MemoryFact
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresMemoryFactRepo) ListActiveByConversation(ctx context.Context, conversationID int64) ([]model.MemoryFact, error) {
	var out []model.MemoryFact
	err := r.db.WithContext(ctx).
		Where("conversation_id = ? AND status = ? AND deleted_at IS NULL", conversationID, model.MemoryFactStatusActive).
		Order("valid_from DESC, id DESC").
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *PostgresMemoryFactRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}
