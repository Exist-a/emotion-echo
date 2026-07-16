package repository

import (
	"context"
	"time"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// EmotionAnalysisRepository 情绪分析数据访问
type EmotionAnalysisRepository struct {
	db *gorm.DB
}

// NewEmotionAnalysisRepository 创建情绪分析仓库
func NewEmotionAnalysisRepository(db *gorm.DB) *EmotionAnalysisRepository {
	return &EmotionAnalysisRepository{db: db}
}

// Create 创建情绪分析
func (r *EmotionAnalysisRepository) Create(ctx context.Context, analysis *models.EmotionAnalysis) error {
	return r.db.WithContext(ctx).Create(analysis).Error
}

// ListByConversationID 获取会话的情绪分析
func (r *EmotionAnalysisRepository) ListByConversationID(ctx context.Context, conversationID string) ([]*models.EmotionAnalysis, error) {
	var analyses []*models.EmotionAnalysis
	err := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Find(&analyses).Error
	return analyses, err
}

// ListByUserIDAndDateRange 获取用户在日期范围内的情绪分析
func (r *EmotionAnalysisRepository) ListByUserIDAndDateRange(ctx context.Context, userID int64, startDate, endDate time.Time) ([]*models.EmotionAnalysis, error) {
	var analyses []*models.EmotionAnalysis
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND analyzed_at >= ? AND analyzed_at < ?", userID, startDate, endDate).
		Find(&analyses).Error
	return analyses, err
}

// GetLatestByConversationID 获取会话最新的情绪分析
func (r *EmotionAnalysisRepository) GetLatestByConversationID(ctx context.Context, conversationID string) (*models.EmotionAnalysis, error) {
	var analysis models.EmotionAnalysis
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("analyzed_at DESC").
		First(&analysis).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &analysis, err
}
