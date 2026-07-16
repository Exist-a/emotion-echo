package repository

import (
	"context"
	"time"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// MentalHealthRepository 心理健康评估数据访问
type MentalHealthRepository struct {
	db *gorm.DB
}

// NewMentalHealthRepository 创建仓库
func NewMentalHealthRepository(db *gorm.DB) *MentalHealthRepository {
	return &MentalHealthRepository{db: db}
}

// Create 创建评估记录
func (r *MentalHealthRepository) Create(ctx context.Context, assessment *models.MentalHealthAssessment) error {
	return r.db.WithContext(ctx).Create(assessment).Error
}

// GetByID 根据 ID 获取
func (r *MentalHealthRepository) GetByID(ctx context.Context, id int64) (*models.MentalHealthAssessment, error) {
	var assessment models.MentalHealthAssessment
	err := r.db.WithContext(ctx).First(&assessment, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &assessment, err
}

// GetLatestByUserID 获取用户最新评估
func (r *MentalHealthRepository) GetLatestByUserID(ctx context.Context, userID int64, assessmentType string) (*models.MentalHealthAssessment, error) {
	var assessment models.MentalHealthAssessment
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	if assessmentType != "" {
		query = query.Where("assessment_type = ?", assessmentType)
	}
	err := query.Order("created_at DESC").First(&assessment).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &assessment, err
}

// ListByUserID 获取用户评估历史
func (r *MentalHealthRepository) ListByUserID(ctx context.Context, userID int64, assessmentType string, limit int, cursor int64) ([]*models.MentalHealthAssessment, bool, error) {
	var assessments []*models.MentalHealthAssessment
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	
	if assessmentType != "" {
		query = query.Where("assessment_type = ?", assessmentType)
	}
	
	if cursor > 0 {
		query = query.Where("id < ?", cursor)
	}
	
	query = query.Order("created_at DESC").Limit(limit + 1)
	err := query.Find(&assessments).Error
	if err != nil {
		return nil, false, err
	}
	
	hasMore := len(assessments) > limit
	if hasMore {
		assessments = assessments[:limit]
	}
	
	return assessments, hasMore, nil
}

// ListByDateRange 获取日期范围内的评估
func (r *MentalHealthRepository) ListByDateRange(ctx context.Context, userID int64, startDate, endDate time.Time) ([]*models.MentalHealthAssessment, error) {
	var assessments []*models.MentalHealthAssessment
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startDate, endDate).
		Order("created_at ASC").
		Find(&assessments).Error
	return assessments, err
}

// GetUsersNeedingAssessment 获取需要评估的用户列表（指定时间范围内有对话的用户）
func (r *MentalHealthRepository) GetUsersNeedingAssessment(ctx context.Context, startTime, endTime time.Time, limit int) ([]int64, error) {
	var userIDs []int64
	err := r.db.WithContext(ctx).
		Model(&models.Conversation{}).
		Select("DISTINCT user_id").
		Where("updated_at >= ? AND updated_at < ?", startTime, endTime).
		Limit(limit).
		Pluck("user_id", &userIDs).Error
	return userIDs, err
}

// MarkAsNotified 标记为已通知
func (r *MentalHealthRepository) MarkAsNotified(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).
		Model(&models.MentalHealthAssessment{}).
		Where("id = ?", id).
		Update("is_notified", true).Error
}

// HasAssessmentToday 检查用户今天是否已有评估（使用北京时间）
func (r *MentalHealthRepository) HasAssessmentToday(ctx context.Context, userID int64, assessmentType string) (bool, error) {
	// 使用北京时间（UTC+8）
	cst := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cst)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cst)
	endOfDay := startOfDay.Add(24 * time.Hour)
	
	var count int64
	query := r.db.WithContext(ctx).Model(&models.MentalHealthAssessment{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startOfDay, endOfDay)
	
	if assessmentType != "" {
		query = query.Where("assessment_type = ?", assessmentType)
	}
	
	err := query.Count(&count).Error
	return count > 0, err
}
