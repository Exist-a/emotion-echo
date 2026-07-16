// Package repository 定义 assessment-svc 的数据访问层
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-assessment-svc/internal/model"

	"gorm.io/gorm"
)

// SurveyRepo 量表仓储接口
type SurveyRepo interface {
	GetByID(ctx context.Context, id uint64) (*model.Survey, error)
	GetByCode(ctx context.Context, code string) (*model.Survey, error)
	List(ctx context.Context, limit int) ([]model.Survey, error)
	// SaveResult 保存量表作答结果
	SaveResult(ctx context.Context, r *model.SurveyResult) error
	// GetResult 按 ID 查结果（带 userID 鉴权：必须同一用户）
	GetResult(ctx context.Context, resultID uint64, userID int64) (*model.SurveyResult, error)
	// ListResultsByUser 列出当前用户的所有结果
	ListResultsByUser(ctx context.Context, userID int64, limit int) ([]model.SurveyResult, error)
	Ping(ctx context.Context) error
}

// ErrNotFound 不存在错误
var ErrNotFound = errors.New("survey result not found")

// =====================================================
// InMemorySurveyRepo（测试替身）
// =====================================================

type InMemorySurveyRepo struct {
	mu      sync.RWMutex
	surveys map[uint64]*model.Survey
	results map[uint64]*model.SurveyResult
}

func NewInMemorySurveyRepo() *InMemorySurveyRepo {
	return &InMemorySurveyRepo{
		surveys: make(map[uint64]*model.Survey),
		results: make(map[uint64]*model.SurveyResult),
	}
}

func (r *InMemorySurveyRepo) GetByID(ctx context.Context, id uint64) (*model.Survey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.surveys[id]
	if !ok {
		return nil, nil
	}
	return s, nil
}

func (r *InMemorySurveyRepo) GetByCode(ctx context.Context, code string) (*model.Survey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, s := range r.surveys {
		if s.Code == code {
			return s, nil
		}
	}
	return nil, nil
}

func (r *InMemorySurveyRepo) List(ctx context.Context, limit int) ([]model.Survey, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Survey, 0)
	for _, s := range r.surveys {
		out = append(out, *s)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *InMemorySurveyRepo) Add(s *model.Survey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.surveys[s.ID] = s
}

func (r *InMemorySurveyRepo) SaveResult(ctx context.Context, result *model.SurveyResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if result.ID == 0 {
		result.ID = uint64(len(r.results) + 1)
	}
	r.results[result.ID] = result
	return nil
}

func (r *InMemorySurveyRepo) GetResult(ctx context.Context, resultID uint64, userID int64) (*model.SurveyResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res, ok := r.results[resultID]
	if !ok {
		return nil, nil
	}
	if res.UserID != userID {
		return nil, nil // 鉴权：不是自己的结果视为不存在
	}
	return res, nil
}

func (r *InMemorySurveyRepo) ListResultsByUser(ctx context.Context, userID int64, limit int) ([]model.SurveyResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.SurveyResult, 0)
	for _, res := range r.results {
		if res.UserID == userID {
			out = append(out, *res)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (r *InMemorySurveyRepo) Ping(ctx context.Context) error { return nil }

// =====================================================
// PostgresSurveyRepo（生产实现）
// =====================================================

type PostgresSurveyRepo struct {
	db *gorm.DB
}

func NewPostgresSurveyRepo(db *gorm.DB) *PostgresSurveyRepo {
	return &PostgresSurveyRepo{db: db}
}

func (r *PostgresSurveyRepo) GetByID(ctx context.Context, id uint64) (*model.Survey, error) {
	var s model.Survey
	err := r.db.WithContext(ctx).First(&s, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *PostgresSurveyRepo) GetByCode(ctx context.Context, code string) (*model.Survey, error) {
	var s model.Survey
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&s).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *PostgresSurveyRepo) List(ctx context.Context, limit int) ([]model.Survey, error) {
	var out []model.Survey
	err := r.db.WithContext(ctx).Limit(limit).Order("id DESC").Find(&out).Error
	return out, err
}

func (r *PostgresSurveyRepo) SaveResult(ctx context.Context, result *model.SurveyResult) error {
	return r.db.WithContext(ctx).Create(result).Error
}

func (r *PostgresSurveyRepo) GetResult(ctx context.Context, resultID uint64, userID int64) (*model.SurveyResult, error) {
	var res model.SurveyResult
	err := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", resultID, userID).
		First(&res).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &res, nil
}

func (r *PostgresSurveyRepo) ListResultsByUser(ctx context.Context, userID int64, limit int) ([]model.SurveyResult, error) {
	var out []model.SurveyResult
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Limit(limit).
		Order("submitted_at DESC").
		Find(&out).Error
	return out, err
}

func (r *PostgresSurveyRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}