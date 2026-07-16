package service

import (
	"context"
	"fmt"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/nanoid"
	"emotion-echo-gin/internal/repository"
	"gorm.io/gorm"
)

// MessageService 消息服务
type MessageService struct {
	db       *gorm.DB
	msgRepo  *repository.MessageRepository
	convRepo *repository.ConversationRepository
}

// NewMessageService 创建消息服务
func NewMessageService(db *gorm.DB, msgRepo *repository.MessageRepository, convRepo *repository.ConversationRepository) *MessageService {
	return &MessageService{
		db:       db,
		msgRepo:  msgRepo,
		convRepo: convRepo,
	}
}

// SendRequest 发送消息请求
type SendRequest struct {
	Content     string  `json:"content" binding:"required"`
	ContentType string  `json:"contentType,omitempty"`
	EmotionTag  *string `json:"emotionTag,omitempty"`
}

// Send 发送消息
func (s *MessageService) Send(ctx context.Context, userID int64, convID string, req *SendRequest) (*models.Message, error) {
	// 1. 验证会话
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, err
	}
	if conv == nil {
		return nil, errors.New(errors.ErrConversationNotFound)
	}
	if conv.UserID != userID {
		return nil, errors.New(errors.ErrNotConversationOwner)
	}

	// 2. 设置默认值
	contentType := req.ContentType
	if contentType == "" {
		contentType = "text"
	}

	// 3. 创建消息并在事务中保存（原子操作：消息 + 会话最后消息）
	now := time.Now()
	msg := &models.Message{
		ID:             nanoid.GenerateWithPrefix("msg"),
		ConversationID: convID,
		Sender:         "user",
		Content:        req.Content,
		ContentType:    contentType,
		EmotionTag:     req.EmotionTag,
		SendTime:       now.UnixMilli(),
		CreatedAt:      now.Unix(),
	}

	preview := req.Content
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}

	// 使用事务保证原子性
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		return tx.Model(&models.Conversation{}).Where("id = ?", convID).Updates(map[string]interface{}{
			"last_message_content": preview,
			"last_message_time":    now.UnixMilli(),
			"updated_at":           now,
		}).Error
	})
	if err != nil {
		fmt.Printf("[ERROR] MessageService.Send transaction failed for conv %s: %v\n", convID, err)
		return nil, err
	}

	return msg, nil
}

// List 获取消息列表
func (s *MessageService) List(ctx context.Context, userID int64, convID string, limit int, cursor int64) ([]*models.Message, bool, error) {
	// 1. 验证会话
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return nil, false, err
	}
	if conv == nil {
		return nil, false, errors.New(errors.ErrConversationNotFound)
	}
	if conv.UserID != userID {
		return nil, false, errors.New(errors.ErrNotConversationOwner)
	}

	// 2. 设置默认值
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// 3. 查询消息
	msgs, err := s.msgRepo.ListByConversationID(ctx, convID, limit+1, cursor)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(msgs) > limit
	if hasMore {
		msgs = msgs[:limit]
	}

	return msgs, hasMore, nil
}

// UpdateEmotionTag 更新消息情绪标签（工作流分析后使用）
func (s *MessageService) UpdateEmotionTag(ctx context.Context, id string, emotionTag *string) error {
	return s.msgRepo.UpdateEmotionTag(ctx, id, emotionTag)
}

// UpdateIntentType 更新消息意图类型（工作流分析后使用）
func (s *MessageService) UpdateIntentType(ctx context.Context, id string, intentType string) error {
	return s.msgRepo.UpdateIntentType(ctx, id, intentType)
}

// SaveAIResponse 保存 AI 回复（内部使用）
func (s *MessageService) SaveAIResponse(ctx context.Context, convID string, content string) (*models.Message, error) {
	now := time.Now()
	msg := &models.Message{
		ID:             nanoid.GenerateWithPrefix("msg"),
		ConversationID: convID,
		Sender:         "ai",
		Content:        content,
		ContentType:    "text",
		SendTime:       now.UnixMilli(),
		CreatedAt:      now.Unix(),
	}

	// 使用事务保证原子性
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(msg).Error; err != nil {
			return err
		}
		return tx.Model(&models.Conversation{}).Where("id = ?", convID).Updates(map[string]interface{}{
			"last_message_content": content,
			"last_message_time":    now.UnixMilli(),
			"updated_at":           now,
		}).Error
	})
	if err != nil {
		fmt.Printf("[ERROR] MessageService.SaveAIResponse transaction failed for conv %s: %v\n", convID, err)
		return nil, err
	}

	return msg, nil
}
