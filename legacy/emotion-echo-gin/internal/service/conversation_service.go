package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/nanoid"
	"emotion-echo-gin/internal/repository"
)

// ConversationService 会话服务
type ConversationService struct {
	convRepo *repository.ConversationRepository
}

// NewConversationService 创建会话服务
func NewConversationService(convRepo *repository.ConversationRepository) *ConversationService {
	return &ConversationService{convRepo: convRepo}
}

// CreateRequest 创建会话请求
type CreateRequest struct {
	Title string `json:"title,omitempty"`
}

// Create 创建会话
func (s *ConversationService) Create(ctx context.Context, userID int64, req *CreateRequest) (*models.Conversation, error) {
	title := req.Title
	if title == "" {
		title = "新会话"
	}

	conv := &models.Conversation{
		ID:        nanoid.GenerateWithPrefix("conv"),
		UserID:    userID,
		Title:     title,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.convRepo.Create(ctx, conv); err != nil {
		return nil, err
	}

	return conv, nil
}

// List 获取会话列表
func (s *ConversationService) List(ctx context.Context, userID int64, limit int, cursor string) ([]*models.Conversation, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	convs, err := s.convRepo.ListByUserID(ctx, userID, limit+1, cursor)
	if err != nil {
		return nil, false, err
	}

	hasMore := len(convs) > limit
	if hasMore {
		convs = convs[:limit]
	}

	return convs, hasMore, nil
}

// UpdateRequest 更新会话请求
type UpdateRequest struct {
	Title string `json:"title" binding:"required"`
}

// Update 更新会话
func (s *ConversationService) Update(ctx context.Context, userID int64, convID string, req *UpdateRequest) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	if conv == nil {
		return errors.New(errors.ErrConversationNotFound)
	}
	if conv.UserID != userID {
		return errors.New(errors.ErrNotConversationOwner)
	}

	conv.Title = req.Title
	conv.UpdatedAt = time.Now()
	return s.convRepo.Update(ctx, conv)
}

// Delete 删除会话
func (s *ConversationService) Delete(ctx context.Context, userID int64, convID string) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	if conv == nil {
		return errors.New(errors.ErrConversationNotFound)
	}
	if conv.UserID != userID {
		return errors.New(errors.ErrNotConversationOwner)
	}

	return s.convRepo.Delete(ctx, convID)
}

// PinRequest 置顶请求
type PinRequest struct {
	IsTop bool `json:"isTop"`
}

// Pin 置顶/取消置顶
func (s *ConversationService) Pin(ctx context.Context, userID int64, convID string, req *PinRequest) error {
	conv, err := s.convRepo.GetByID(ctx, convID)
	if err != nil {
		return err
	}
	if conv == nil {
		return errors.New(errors.ErrConversationNotFound)
	}
	if conv.UserID != userID {
		return errors.New(errors.ErrNotConversationOwner)
	}

	conv.IsTop = req.IsTop
	conv.UpdatedAt = time.Now()
	return s.convRepo.Update(ctx, conv)
}

// GetByID 根据 ID 获取会话
func (s *ConversationService) GetByID(ctx context.Context, userID int64, convID string) (*models.Conversation, error) {
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
	return conv, nil
}

// UpdateTitle 更新会话标题
func (s *ConversationService) UpdateTitle(ctx context.Context, convID string, title string) error {
	return s.convRepo.UpdateTitle(ctx, convID, title)
}
