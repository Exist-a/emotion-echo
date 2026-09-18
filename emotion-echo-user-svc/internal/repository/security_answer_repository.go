// Package repository — security_answer_repository.go
//
// E2E-06: 密保问题仓储（供 D-01=C 找回密码）
package repository

import (
	"context"
	"sync"

	"emotion-echo-user-svc/internal/model"

	"gorm.io/gorm"
)

// SecurityAnswerRepo 密保问题仓储接口
type SecurityAnswerRepo interface {
	// GetByUserID 查询用户的所有密保问题（1~2 个）
	GetByUserID(ctx context.Context, userID int64) ([]*model.SecurityAnswer, error)
	// Save 批量保存密保问题（覆盖写入，用于注册/修改）
	Save(ctx context.Context, answers []*model.SecurityAnswer) error
	// DeleteByUserID 删除用户的所有密保问题
	DeleteByUserID(ctx context.Context, userID int64) error
}

// =====================================================
// InMemorySecurityAnswerRepo（测试替身）
// =====================================================

type InMemorySecurityAnswerRepo struct {
	mu      sync.RWMutex
	answers map[int64][]*model.SecurityAnswer // userID → answers
}

func NewInMemorySecurityAnswerRepo() *InMemorySecurityAnswerRepo {
	return &InMemorySecurityAnswerRepo{answers: make(map[int64][]*model.SecurityAnswer)}
}

func (r *InMemorySecurityAnswerRepo) GetByUserID(ctx context.Context, userID int64) ([]*model.SecurityAnswer, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	answers, ok := r.answers[userID]
	if !ok {
		return nil, nil
	}
	// 返回副本
	result := make([]*model.SecurityAnswer, len(answers))
	copy(result, answers)
	return result, nil
}

func (r *InMemorySecurityAnswerRepo) Save(ctx context.Context, answers []*model.SecurityAnswer) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(answers) == 0 {
		return nil
	}
	userID := answers[0].UserID
	// 覆盖写入
	r.answers[userID] = make([]*model.SecurityAnswer, len(answers))
	copy(r.answers[userID], answers)
	return nil
}

func (r *InMemorySecurityAnswerRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.answers, userID)
	return nil
}

// =====================================================
// PostgresSecurityAnswerRepo（生产实现）
// =====================================================

type PostgresSecurityAnswerRepo struct {
	db *gorm.DB
}

func NewPostgresSecurityAnswerRepo(db *gorm.DB) *PostgresSecurityAnswerRepo {
	return &PostgresSecurityAnswerRepo{db: db}
}

func (r *PostgresSecurityAnswerRepo) GetByUserID(ctx context.Context, userID int64) ([]*model.SecurityAnswer, error) {
	var answers []*model.SecurityAnswer
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("question_order ASC").
		Find(&answers).Error
	if err != nil {
		return nil, err
	}
	return answers, nil
}

func (r *PostgresSecurityAnswerRepo) Save(ctx context.Context, answers []*model.SecurityAnswer) error {
	if len(answers) == 0 {
		return nil
	}
	// 先删除旧的，再插入新的（覆盖语义）
	userID := answers[0].UserID
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.SecurityAnswer{}).Error
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Create(&answers).Error
}

func (r *PostgresSecurityAnswerRepo) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Delete(&model.SecurityAnswer{}).Error
}