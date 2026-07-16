package service

import (
	"context"

	"emotion-echo-gin/internal/repository"
)

// UserBehaviorService 用户行为分析服务
type UserBehaviorService struct {
	msgRepo  *repository.MessageRepository
	convRepo *repository.ConversationRepository
}

// NewUserBehaviorService 创建用户行为分析服务
func NewUserBehaviorService(msgRepo *repository.MessageRepository, convRepo *repository.ConversationRepository) *UserBehaviorService {
	return &UserBehaviorService{
		msgRepo:  msgRepo,
		convRepo: convRepo,
	}
}

// GetDayNightPattern 获取昼夜使用模式（最近30天）
func (s *UserBehaviorService) GetDayNightPattern(ctx context.Context, userID int64) (*DayNightPattern, error) {
	return GetDayNightPattern(ctx, userID, s.msgRepo, s.convRepo)
}

// GetInteractionDepth 获取互动深度（全部历史）
func (s *UserBehaviorService) GetInteractionDepth(ctx context.Context, userID int64) (*InteractionDepth, error) {
	return GetInteractionDepth(ctx, userID, s.msgRepo, s.convRepo)
}

// GetFrequencyTrend 获取对话频次趋势（最近30天）
func (s *UserBehaviorService) GetFrequencyTrend(ctx context.Context, userID int64) (*FrequencyTrend, error) {
	return GetFrequencyTrend(ctx, userID, s.msgRepo, s.convRepo)
}
