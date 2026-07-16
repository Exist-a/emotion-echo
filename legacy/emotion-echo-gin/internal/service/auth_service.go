package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/jwt"
	"emotion-echo-gin/internal/repository"
	"gorm.io/gorm"
)

// IDGenerator ID生成器接口
type IDGenerator interface {
	Generate() int64
}

// AuthService 认证服务（主入口）
type AuthService struct {
	db         *gorm.DB
	userRepo   *repository.UserRepository
	redisRepo  *repository.RedisRepository
	tokenSvc   *TokenService
	regSvc     *RegisterService
	idGen      IDGenerator
	accessExp  time.Duration
	refreshExp time.Duration
}

// NewAuthService 创建认证服务
func NewAuthService(
	db *gorm.DB,
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	redisRepo *repository.RedisRepository,
	jwtInstance *jwt.JWT,
	idGen IDGenerator,
	accessExp time.Duration,
	refreshExp time.Duration,
) *AuthService {
	tokenSvc := NewTokenService(db, tokenRepo, redisRepo, jwtInstance, accessExp, refreshExp)
	regSvc := NewRegisterService(db, userRepo, redisRepo, tokenSvc, idGen)

	return &AuthService{
		db:         db,
		userRepo:   userRepo,
		redisRepo:  redisRepo,
		tokenSvc:   tokenSvc,
		regSvc:     regSvc,
		idGen:      idGen,
		accessExp:  accessExp,
		refreshExp: refreshExp,
	}
}

// Register 用户注册
func (s *AuthService) Register(ctx context.Context, req *RegisterRequest) (*AuthResponse, string, error) {
	return s.regSvc.Register(ctx, req)
}

// Login 用户登录
func (s *AuthService) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, string, error) {
	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return nil, "", err
	}
	if user == nil {
		return nil, "", errors.New(errors.ErrUserNotFound)
	}

	if !VerifyPassword(req.Password, user.PasswordHash) {
		return nil, "", errors.New(errors.ErrPasswordIncorrect)
	}

	return s.tokenSvc.GenerateTokens(ctx, user.ID, req.RememberMe)
}

// Refresh 刷新Token
func (s *AuthService) Refresh(ctx context.Context, refreshToken string) (*AuthResponse, string, error) {
	return s.tokenSvc.RefreshTokens(ctx, refreshToken)
}

// ResetPassword 重置密码
func (s *AuthService) ResetPassword(ctx context.Context, req *ResetPasswordRequest) error {
	code, err := s.redisRepo.GetVerifyCode(ctx, "reset", req.Username)
	if err != nil || code != req.VerificationCode {
		return errors.New(errors.ErrInvalidVerifyCode)
	}

	user, err := s.userRepo.GetByUsername(ctx, req.Username)
	if err != nil {
		return err
	}
	if user == nil {
		return errors.New(errors.ErrUserNotFound)
	}

	hash, err := HashPassword(req.NewPassword)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("id = ?", user.ID).Update("password_hash", hash).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ?", user.ID).Delete(&models.RefreshToken{}).Error
	})
}

// Logout 用户登出
func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {
	return s.tokenSvc.RevokeToken(ctx, refreshToken)
}
