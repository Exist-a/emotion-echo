package repository

import (
	"context"

	"emotion-echo-gin/internal/models"
	"gorm.io/gorm"
)

// MessageRepository 消息数据访问
type MessageRepository struct {
	db *gorm.DB
}

// NewMessageRepository 创建消息仓库
func NewMessageRepository(db *gorm.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

// Create 创建消息
func (r *MessageRepository) Create(ctx context.Context, msg *models.Message) error {
	return r.db.WithContext(ctx).Create(msg).Error
}

// GetByID 根据 ID 获取消息
func (r *MessageRepository) GetByID(ctx context.Context, id string) (*models.Message, error) {
	var msg models.Message
	err := r.db.WithContext(ctx).First(&msg, "id = ?", id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &msg, err
}

// ListByConversationID 获取会话消息列表
func (r *MessageRepository) ListByConversationID(ctx context.Context, conversationID string, limit int, cursor int64) ([]*models.Message, error) {
	var msgs []*models.Message
	query := r.db.WithContext(ctx).Where("conversation_id = ?", conversationID)
	
	if cursor > 0 {
		query = query.Where("send_time < ?", cursor)
	}
	
	err := query.Order("send_time DESC").Limit(limit).Find(&msgs).Error
	return msgs, err
}

// DeleteByConversationID 删除会话的所有消息
func (r *MessageRepository) DeleteByConversationID(ctx context.Context, conversationID string) error {
	return r.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Delete(&models.Message{}).Error
}

// UpdateEmotionTag 更新消息的情绪标签
func (r *MessageRepository) UpdateEmotionTag(ctx context.Context, id string, emotionTag *string) error {
	return r.db.WithContext(ctx).Model(&models.Message{}).Where("id = ?", id).Update("emotion_tag", emotionTag).Error
}

// UpdateIntentType 更新消息的意图类型
func (r *MessageRepository) UpdateIntentType(ctx context.Context, id string, intentType string) error {
	return r.db.WithContext(ctx).Model(&models.Message{}).Where("id = ?", id).Update("intent_type", intentType).Error
}

// CountByUserIDAndDate 统计用户在日期范围内的消息数和字数
func (r *MessageRepository) CountByUserIDAndDate(ctx context.Context, userID int64, startTime, endTime int64) (int, int, error) {
	var result struct {
		Count     int
		WordCount int
	}

	// 先获取用户当天的所有会话ID
	var conversationIDs []string
	err := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("user_id = ?", userID).
		Pluck("id", &conversationIDs).Error
	if err != nil {
		return 0, 0, err
	}

	// 统计这些会话中的消息
	err = r.db.WithContext(ctx).Model(&models.Message{}).
		Select("COUNT(*) as count, COALESCE(SUM(LENGTH(content)), 0) as word_count").
		Where("conversation_id IN ? AND send_time >= ? AND send_time < ? AND sender = ?", conversationIDs, startTime, endTime, "user").
		Scan(&result).Error

	return result.Count, result.WordCount, err
}

// CountIntentTypeByUserIDAndDate 统计用户在日期范围内不同意图类型的消息数
func (r *MessageRepository) CountIntentTypeByUserIDAndDate(ctx context.Context, userID int64, startTime, endTime int64) (map[string]int, error) {
	var conversationIDs []string
	err := r.db.WithContext(ctx).Model(&models.Conversation{}).
		Where("user_id = ?", userID).
		Pluck("id", &conversationIDs).Error
	if err != nil {
		return nil, err
	}

	var results []struct {
		IntentType string
		Count      int
	}

	err = r.db.WithContext(ctx).Model(&models.Message{}).
		Select("intent_type, COUNT(*) as count").
		Where("conversation_id IN ? AND send_time >= ? AND send_time < ? AND sender = ?", conversationIDs, startTime, endTime, "user").
		Where("intent_type IN ?", []string{"emotional_support", "study_help", "tech_help", "career_help", "lifestyle", "other"}).
		Group("intent_type").
		Scan(&results).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.IntentType] = r.Count
	}

	return counts, nil
}
