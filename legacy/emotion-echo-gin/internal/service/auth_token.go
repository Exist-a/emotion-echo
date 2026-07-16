package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/jwt"
	"emotion-echo-gin/internal/repository"
	"gorm.io/gorm"
)

// TokenService Token服务
type TokenService struct {
	db         *gorm.DB
	tokenRepo  *repository.TokenRepository
	redisRepo  *repository.RedisRepository
	jwtInstance *jwt.JWT
	accessExp  time.Duration
	refreshExp time.Duration
}

// NewTokenService 创建Token服务
func NewTokenService(
	db *gorm.DB,
	tokenRepo *repository.TokenRepository,
	redisRepo *repository.RedisRepository,
	jwtInstance *jwt.JWT,
	accessExp, refreshExp time.Duration,
) *TokenService {
	return &TokenService{
		db:          db,
		tokenRepo:   tokenRepo,
		redisRepo:   redisRepo,
		jwtInstance: jwtInstance,
		accessExp:   accessExp,
		refreshExp:  refreshExp,
	}
}

// GenerateTokens 生成Token对
func (s *TokenService) GenerateTokens(ctx context.Context, userID int64, rememberMe ...bool) (*AuthResponse, string, error) {
	return s.GenerateTokensWithTx(ctx, s.db, userID, rememberMe...)
}

// GenerateTokensWithTx 生成Token（支持事务）
func (s *TokenService) GenerateTokensWithTx(ctx context.Context, tx *gorm.DB, userID int64, rememberMe ...bool) (*AuthResponse, string, error) {
	accessToken, err := s.jwtInstance.GenerateAccessToken(userID)
	if err != nil {
		return nil, "", err
	}

	refreshToken := generateRandomToken()
	tokenHash := sha256Hash(refreshToken)

	rm := false
	refreshDuration := 24 * time.Hour
	if len(rememberMe) > 0 && rememberMe[0] {
		rm = true
		refreshDuration = 30 * 24 * time.Hour
	}
	expiresAt := time.Now().Add(refreshDuration)

	if err := tx.WithContext(ctx).Create(&models.RefreshToken{
		UserID:     userID,
		TokenHash:  tokenHash,
		RememberMe: rm,
		ExpiresAt:  expiresAt,
	}).Error; err != nil {
		return nil, "", err
	}

	var user models.User
	if err := tx.WithContext(ctx).First(&user, userID).Error; err != nil {
		return nil, "", err
	}

	return &AuthResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(s.accessExp.Seconds()),
		RememberMe:  rm,
		UserID:      user.ID,
	}, refreshToken, nil
}

// RefreshTokens 刷新Token（支持Token Rotation）
func (s *TokenService) RefreshTokens(ctx context.Context, refreshToken string) (*AuthResponse, string, error) {
	isBlacklisted, err := s.redisRepo.IsBlacklisted(ctx, refreshToken)
	if err != nil {
		return nil, "", err
	}
	if isBlacklisted {
		return nil, "", err
	}

	tokenHash := sha256Hash(refreshToken)
	token, err := s.tokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, "", err
	}
	if token == nil || token.ExpiresAt.Before(time.Now()) {
		return nil, "", err
	}

	ttl := time.Until(token.ExpiresAt)
	if ttl > 0 {
		if err := s.redisRepo.AddToBlacklist(ctx, refreshToken, ttl); err != nil {
			return nil, "", err
		}
	}

	if err := s.tokenRepo.DeleteByTokenHash(ctx, tokenHash); err != nil {
		return nil, "", err
	}

	return s.GenerateTokens(ctx, token.UserID, token.RememberMe)
}

// RevokeToken 撤销Token
func (s *TokenService) RevokeToken(ctx context.Context, refreshToken string) error {
	tokenHash := sha256Hash(refreshToken)
	token, err := s.tokenRepo.GetByTokenHash(ctx, tokenHash)
	if err != nil {
		return err
	}

	if token != nil {
		ttl := time.Until(token.ExpiresAt)
		if ttl > 0 {
			if err := s.redisRepo.AddToBlacklist(ctx, refreshToken, ttl); err != nil {
				return err
			}
		}
		return s.tokenRepo.DeleteByTokenHash(ctx, tokenHash)
	}

	return nil
}

// generateRandomToken 生成随机Token
func generateRandomToken() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
