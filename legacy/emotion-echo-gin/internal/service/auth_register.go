package service

import (
	"context"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/repository"
	"gorm.io/gorm"
)

// RegisterService 注册服务
type RegisterService struct {
	db        *gorm.DB
	userRepo  *repository.UserRepository
	redisRepo *repository.RedisRepository
	tokenSvc  *TokenService
	idGen     IDGenerator
}

// NewRegisterService 创建注册服务
func NewRegisterService(
	db *gorm.DB,
	userRepo *repository.UserRepository,
	redisRepo *repository.RedisRepository,
	tokenSvc *TokenService,
	idGen IDGenerator,
) *RegisterService {
	return &RegisterService{
		db:        db,
		userRepo:  userRepo,
		redisRepo: redisRepo,
		tokenSvc:  tokenSvc,
		idGen:     idGen,
	}
}

// Register 用户注册
func (s *RegisterService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, string, error) {
	code, err := s.redisRepo.GetVerifyCode(ctx, "register", req.Username)
	if err != nil || code != req.VerificationCode {
		return nil, "", errors.New(errors.ErrInvalidVerifyCode)
	}

	exists, err := s.userRepo.ExistsByUsername(ctx, req.Username)
	if err != nil {
		return nil, "", err
	}
	if exists {
		return nil, "", errors.New(errors.ErrUserExists)
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		return nil, "", err
	}

	var resp *AuthResponse
	var refreshToken string
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user := &models.User{
			ID:           s.idGen.Generate(),
			Username:     req.Username,
			PasswordHash: hash,
		}
		if err := tx.Create(user).Error; err != nil {
			return err
		}

		resp, refreshToken, err = s.tokenSvc.GenerateTokensWithTx(ctx, tx, user.ID)
		return err
	})

	if err != nil {
		return nil, "", err
	}

	s.redisRepo.DeleteVerifyCode(ctx, "register", req.Username)

	return resp, refreshToken, nil
}
