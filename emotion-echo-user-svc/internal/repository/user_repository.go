// Package repository 定义 user-svc 的数据访问层。
//
// 原则：
//   - 接口（UserRepo）是契约，由测试驱动设计
//   - InMemoryUserRepo 仅测试用，不进入生产
//   - PostgresUserRepo 是真正的实现（生产）
//   - 两个实现都满足同一接口 → 上层 logic 不感知实现细节
package repository

import (
	"context"
	"errors"
	"sync"
	"time"

	"emotion-echo-user-svc/internal/model"

	"gorm.io/gorm"
)

// UserRepo 用户仓储接口
type UserRepo interface {
	GetByID(ctx context.Context, id int64) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetByPhone(ctx context.Context, phone string) (*model.User, error)
	Create(ctx context.Context, u *model.User) error
	// UpdateProfile 修改用户可编辑字段（昵称/性别/生日/头像）
	// 传 nil 表示该字段不动，传 *string 设置/覆盖值
	UpdateProfile(ctx context.Context, id int64, nickname *string, gender *int16, birthday *time.Time, avatarURL *string) error
	Ping(ctx context.Context) error
}

// ErrNotFound 不存在错误
var ErrNotFound = errors.New("user not found")

// =====================================================
// InMemoryUserRepo（测试替身）
// =====================================================

type InMemoryUserRepo struct {
	mu    sync.RWMutex
	users map[int64]*model.User
}

func NewInMemoryUserRepo() *InMemoryUserRepo {
	return &InMemoryUserRepo{users: make(map[int64]*model.User)}
}

func (r *InMemoryUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	u, ok := r.users[id]
	if !ok {
		return nil, nil // 按测试约定：不存在返回 nil，无 error
	}
	return u, nil
}

func (r *InMemoryUserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Username == username {
			return u, nil
		}
	}
	return nil, nil
}

func (r *InMemoryUserRepo) GetByPhone(ctx context.Context, phone string) (*model.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, u := range r.users {
		if u.Phone != nil && *u.Phone == phone {
			return u, nil
		}
	}
	return nil, nil
}

func (r *InMemoryUserRepo) Create(ctx context.Context, u *model.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.users[u.ID] = u
	return nil
}

func (r *InMemoryUserRepo) UpdateProfile(ctx context.Context, id int64, nickname *string, gender *int16, birthday *time.Time, avatarURL *string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return ErrNotFound
	}
	if nickname != nil {
		u.Nickname = nickname
	}
	if gender != nil {
		u.Gender = *gender
	}
	if birthday != nil {
		u.Birthday = birthday
	}
	if avatarURL != nil {
		u.AvatarURL = avatarURL
	}
	u.UpdatedAt = time.Now()
	return nil
}

func (r *InMemoryUserRepo) Ping(ctx context.Context) error { return nil }

// =====================================================
// PostgresUserRepo（生产实现）
// =====================================================

type PostgresUserRepo struct {
	db *gorm.DB
}

func NewPostgresUserRepo(db *gorm.DB) *PostgresUserRepo {
	return &PostgresUserRepo{db: db}
}

func (r *PostgresUserRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).First(&u, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresUserRepo) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("username = ?", username).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresUserRepo) GetByPhone(ctx context.Context, phone string) (*model.User, error) {
	var u model.User
	err := r.db.WithContext(ctx).Where("phone = ?", phone).First(&u).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &u, nil
}

func (r *PostgresUserRepo) Create(ctx context.Context, u *model.User) error {
	return r.db.WithContext(ctx).Create(u).Error
}

func (r *PostgresUserRepo) UpdateProfile(ctx context.Context, id int64, nickname *string, gender *int16, birthday *time.Time, avatarURL *string) error {
	updates := map[string]interface{}{}
	if nickname != nil {
		updates["nickname"] = *nickname
	}
	if gender != nil {
		updates["gender"] = *gender
	}
	if birthday != nil {
		updates["birthday"] = *birthday
	}
	if avatarURL != nil {
		updates["avatar_url"] = *avatarURL
	}
	if len(updates) == 0 {
		return nil // 无字段需更新
	}
	res := r.db.WithContext(ctx).
		Table("emotion_echo_user.users").
		Where("id = ?", id).
		Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PostgresUserRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}