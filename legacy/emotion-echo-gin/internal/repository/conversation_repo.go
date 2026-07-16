package repository

import (
	"context"
	"time"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// ConversationRepository 会话数据访问
type ConversationRepository struct {
	db *gorm.DB
}

// NewConversationRepository 创建会话仓库
func NewConversationRepository(db *gorm.DB) *ConversationRepository {
	return &ConversationRepository{db: db}
}

// Create 创建会话
func (r *ConversationRepository) Create(ctx context.Context, conv *models.Conversation) error {
	return r.db.WithContext(ctx).Create(conv).Error
}

// GetByID 根据 ID 获取会话
func (r *ConversationRepository) GetByID(ctx context.Context, id string) (*models.Conversation, error) {
	var conv models.Conversation
	err := r.db.WithContext(ctx).First(&conv, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &conv, err
}

// ListByUserID 获取用户会话列表
func (r *ConversationRepository) ListByUserID(ctx context.Context, userID int64, limit int, cursor string) ([]*models.Conversation, error) {
	var convs []*models.Conversation
	query := r.db.WithContext(ctx).Where("user_id = ?", userID)
	
	if cursor != "" {
		query = query.Where("updated_at < ?", cursor)
	}
	
	err := query.Order("is_top DESC, updated_at DESC").Limit(limit).Find(&convs).Error
	return convs, err
}

// Update 更新会话
func (r *ConversationRepository) Update(ctx context.Context, conv *models.Conversation) error {
	return r.db.WithContext(ctx).Save(conv).Error
}

// Delete 删除会话
func (r *ConversationRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&models.Conversation{}, "id = ?", id).Error
}

// UpdateLastMessage 更新最后消息
func (r *ConversationRepository) UpdateLastMessage(ctx context.Context, id string, content string, sendTime int64) error {
	return r.db.WithContext(ctx).Model(&models.Conversation{}).Where("id = ?", id).Updates(map[string]interface{}{
		"last_message_content": content,
		"last_message_time":    sendTime,
	}).Error
}

// UpdateTitle 更新会话标题
func (r *ConversationRepository) UpdateTitle(ctx context.Context, id string, title string) error {
	return r.db.WithContext(ctx).Model(&models.Conversation{}).Where("id = ?", id).Updates(map[string]interface{}{
		"title":      title,
		"updated_at": time.Now(),
	}).Error
}

// CountByUserIDAndDate 统计用户在日期范围内的会话数
func (r *ConversationRepository) CountByUserIDAndDate(ctx context.Context, userID int64, startDate, endDate time.Time) (int, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, startDate, endDate).
		Count(&count).Error
	return int(count), err
}

// ListRecentConversations 获取最近需要分析的会话
func (r *ConversationRepository) ListRecentConversations(ctx context.Context, startTime, endTime time.Time, limit int) ([]*models.Conversation, error) {
	var convs []*models.Conversation
	err := r.db.WithContext(ctx).
		Where("updated_at >= ? AND updated_at < ?", startTime, endTime).
		Order("updated_at DESC").
		Limit(limit).
		Find(&convs).Error
	return convs, err
}
