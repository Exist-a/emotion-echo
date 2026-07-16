package repository

import (
	"context"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// TokenRepository 令牌数据访问
type TokenRepository struct {
	db *gorm.DB
}

// NewTokenRepository 创建令牌仓库
func NewTokenRepository(db *gorm.DB) *TokenRepository {
	return &TokenRepository{db: db}
}

// Create 创建刷新令牌
func (r *TokenRepository) Create(ctx context.Context, token *models.RefreshToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

// GetByTokenHash 根据 TokenHash 获取
func (r *TokenRepository) GetByTokenHash(ctx context.Context, hash string) (*models.RefreshToken, error) {
	var token models.RefreshToken
	err := r.db.WithContext(ctx).Where("token_hash = ?", hash).First(&token).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &token, err
}

// DeleteByTokenHash 删除令牌
func (r *TokenRepository) DeleteByTokenHash(ctx context.Context, hash string) error {
	return r.db.WithContext(ctx).Where("token_hash = ?", hash).Delete(&models.RefreshToken{}).Error
}

// DeleteByUserID 删除用户的所有令牌（登出所有设备）
func (r *TokenRepository) DeleteByUserID(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.RefreshToken{}).Error
}
