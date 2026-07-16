package repository

import (
	"context"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// SurveyRepository 测验数据访问
type SurveyRepository struct {
	db *gorm.DB
}

// NewSurveyRepository 创建测验仓库
func NewSurveyRepository(db *gorm.DB) *SurveyRepository {
	return &SurveyRepository{db: db}
}

// List 获取所有量表
func (r *SurveyRepository) List(ctx context.Context) ([]*models.Survey, error) {
	var surveys []*models.Survey
	err := r.db.WithContext(ctx).Find(&surveys).Error
	return surveys, err
}

// GetByID 根据 ID 获取量表
func (r *SurveyRepository) GetByID(ctx context.Context, id int) (*models.Survey, error) {
	var survey models.Survey
	err := r.db.WithContext(ctx).First(&survey, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &survey, err
}
