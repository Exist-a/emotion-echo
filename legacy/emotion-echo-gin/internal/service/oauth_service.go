package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/jwt"
	"emotion-echo-gin/internal/pkg/password"
	"emotion-echo-gin/internal/pkg/snowflake"
	"emotion-echo-gin/internal/repository"
)

// WechatOAuthService 微信 OAuth 服务（主入口）
type WechatOAuthService struct {
	cfg        *config.Config
	userRepo   *repository.UserRepository
	tokenRepo  *repository.TokenRepository
	jwtInstance *jwt.JWT
	wechatAPI  *WechatAPI
	idGen     *snowflake.Generator
	accessExp time.Duration
	refreshExp time.Duration
}

// NewWechatOAuthService 创建微信 OAuth 服务
func NewWechatOAuthService(
	cfg *config.Config,
	userRepo *repository.UserRepository,
	tokenRepo *repository.TokenRepository,
	jwtInstance *jwt.JWT,
	idGen *snowflake.Generator,
	accessExp, refreshExp time.Duration,
) *WechatOAuthService {
	return &WechatOAuthService{
		cfg:        cfg,
		userRepo:   userRepo,
		tokenRepo:  tokenRepo,
		jwtInstance: jwtInstance,
		wechatAPI:  NewWechatAPI(cfg),
		idGen:     idGen,
		accessExp: accessExp,
		refreshExp: refreshExp,
	}
}

// GetAuthURL 获取微信授权 URL
func (s *WechatOAuthService) GetAuthURL(req *GetWechatAuthURLRequest) *GetWechatAuthURLResponse {
	return s.wechatAPI.GetAuthURL(req)
}

// LoginByCode 通过微信授权码登录
func (s *WechatOAuthService) LoginByCode(ctx context.Context, code string) (*WechatLoginResponse, string, error) {
	wechatToken, err := s.wechatAPI.ExchangeCodeForToken(code)
	if err != nil {
		return nil, "", err
	}

	wechatUser, err := s.wechatAPI.GetUserInfo(wechatToken.AccessToken, wechatToken.OpenID)
	if err != nil {
		return nil, "", err
	}

	user, err := s.getOrCreateUser(ctx, wechatUser)
	if err != nil {
		return nil, "", err
	}

	accessToken, err := s.jwtInstance.GenerateAccessToken(user.ID)
	if err != nil {
		return nil, "", errors.New(errors.ErrInternalServer, "生成访问令牌失败")
	}

	refreshToken := generateRandomToken()
	tokenHash := sha256Hash(refreshToken)
	expiresAt := time.Now().Add(s.refreshExp)

	if err := s.tokenRepo.Create(ctx, &models.RefreshToken{
		UserID:     user.ID,
		TokenHash:  tokenHash,
		RememberMe: true,
		ExpiresAt:  expiresAt,
	}); err != nil {
		return nil, "", errors.New(errors.ErrInternalServer, "生成刷新令牌失败")
	}

	return &WechatLoginResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(s.accessExp.Seconds()),
		RememberMe:  true,
		UserID:      user.ID,
	}, refreshToken, nil
}

// getOrCreateUser 获取或创建微信用户
func (s *WechatOAuthService) getOrCreateUser(ctx context.Context, wechatUser *WechatUserInfo) (*models.User, error) {
	user, err := s.userRepo.GetByWechatOpenID(ctx, wechatUser.OpenID)
	if err != nil {
		return nil, err
	}

	if user == nil {
		return s.createWechatUser(ctx, wechatUser)
	}

	updated := false
	if wechatUser.Nickname != "" && wechatUser.Nickname != user.Nickname {
		user.Nickname = wechatUser.Nickname
		updated = true
	}
	if wechatUser.AvatarURL != "" && wechatUser.AvatarURL != user.Avatar {
		user.Avatar = wechatUser.AvatarURL
		updated = true
	}
	if wechatUser.UnionID != "" && wechatUser.UnionID != user.WechatUnionID {
		user.WechatUnionID = wechatUser.UnionID
		updated = true
	}

	if updated {
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// createWechatUser 创建微信用户
func (s *WechatOAuthService) createWechatUser(ctx context.Context, wechatUser *WechatUserInfo) (*models.User, error) {
	randomPass := generateRandomState() + generateRandomState()
	hash, err := password.Hash(randomPass)
	if err != nil {
		return nil, errors.New(errors.ErrInternalServer, "创建用户失败")
	}

	nickname := wechatUser.Nickname
	if nickname == "" {
		nickname = "微信用户"
	}

	avatar := wechatUser.AvatarURL
	if avatar == "" {
		avatar = "/imgs/default-avatar.webp"
	}

	user := &models.User{
		Username:      "wx_" + wechatUser.OpenID,
		PasswordHash: hash,
		Nickname:     nickname,
		Avatar:       avatar,
		WechatOpenID: wechatUser.OpenID,
		WechatUnionID: wechatUser.UnionID,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, errors.New(errors.ErrInternalServer, "创建用户失败")
	}

	return user, nil
}
