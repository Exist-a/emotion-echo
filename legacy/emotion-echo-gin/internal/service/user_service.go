package service

import (
	"context"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/repository"
)

// UserService 用户服务
type UserService struct {
	userRepo *repository.UserRepository
}

// NewUserService 创建用户服务
func NewUserService(userRepo *repository.UserRepository) *UserService {
	return &UserService{userRepo: userRepo}
}

// GetProfile 获取用户信息
func (s *UserService) GetProfile(ctx context.Context, userID int64) (*models.User, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, errors.New(errors.ErrUserNotFound)
	}
	return user, nil
}

// UpdateProfileRequest 更新用户信息请求
type UpdateProfileRequest struct {
	Nickname string         `json:"nickname,omitempty"`
	Age      *int           `json:"age,omitempty"`
	Config   *models.UserConfig `json:"config,omitempty"`
}

// UpdateProfile 更新用户信息
func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req *UpdateProfileRequest) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New(errors.ErrUserNotFound)
	}

	if req.Nickname != "" {
		user.Nickname = req.Nickname
	}
	if req.Age != nil {
		user.Age = req.Age
	}
	if req.Config != nil {
		user.Config = *req.Config
	}

	return s.userRepo.Update(ctx, user)
}

// UpdateAvatar 更新头像
func (s *UserService) UpdateAvatar(ctx context.Context, userID int64, avatarURL string) error {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New(errors.ErrUserNotFound)
	}

	user.Avatar = avatarURL
	return s.userRepo.Update(ctx, user)
}
