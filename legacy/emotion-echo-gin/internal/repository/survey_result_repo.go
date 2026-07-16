package repository

import (
	"context"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// SurveyResultRepository 测验结果数据访问
type SurveyResultRepository struct {
	db *gorm.DB
}

// NewSurveyResultRepository 创建测验结果仓库
func NewSurveyResultRepository(db *gorm.DB) *SurveyResultRepository {
	return &SurveyResultRepository{db: db}
}

// Create 创建测验结果
func (r *SurveyResultRepository) Create(ctx context.Context, result *models.SurveyResult) error {
	return r.db.WithContext(ctx).Create(result).Error
}

// ListByUserID 获取用户的所有测验结果
func (r *SurveyResultRepository) ListByUserID(ctx context.Context, userID int64) ([]*models.SurveyResult, error) {
	var results []*models.SurveyResult
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&results).Error
	return results, err
}

// GetByID 根据 ID 获取测验结果
func (r *SurveyResultRepository) GetByID(ctx context.Context, id string) (*models.SurveyResult, error) {
	var result models.SurveyResult
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&result).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &result, err
}
